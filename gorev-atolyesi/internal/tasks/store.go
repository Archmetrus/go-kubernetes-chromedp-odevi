// Package tasks contains the persistent task board and its HTTP interface.
package tasks

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

//go:embed schema.sql
var schema string
var ErrValidation = errors.New("geçersiz görev")
var ErrConflict = errors.New("görev başka bir işlemde değişti; listeyi yenileyin")

type Task struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Priority    string `json:"priority"`
	Status      string `json:"status"`
	DueDate     string `json:"due_date"`
	Version     int    `json:"version"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	Archived    bool   `json:"archived"`
	DeletedAt   string `json:"deleted_at"`
}
type Store struct {
	db       *sql.DB
	postgres bool
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// Single connection makes connection-specific PRAGMAs predictable.
	db.SetMaxOpenConns(1)
	if _, err = db.Exec("PRAGMA busy_timeout=5000; PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;"); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db}
	if err = s.Migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func OpenPostgres(ctx context.Context, url string) (*Store, error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, postgres: true}, nil
}

// q translates placeholders in our static SQL, never user-supplied SQL.
func (s *Store) q(query string) string {
	if !s.postgres {
		return query
	}
	n := 0
	var b strings.Builder
	for _, r := range query {
		if r == '?' {
			n++
			fmt.Fprintf(&b, "$%d", n)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

type ownerKey struct{}

func WithOwner(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ownerKey{}, id)
}
func owner(ctx context.Context) string {
	if id, ok := ctx.Value(ownerKey{}).(string); ok {
		return id
	}
	return "local"
}
func (s *Store) Close() error                   { return s.db.Close() }
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }
func normalize(t *Task) error {
	t.Title = strings.TrimSpace(t.Title)
	t.Description = strings.TrimSpace(t.Description)
	if utf8.RuneCountInString(t.Title) < 3 || utf8.RuneCountInString(t.Title) > 120 {
		return fmt.Errorf("%w: başlık 3–120 karakter olmalı", ErrValidation)
	}
	if utf8.RuneCountInString(t.Description) > 2000 {
		return fmt.Errorf("%w: açıklama en fazla 2000 karakter olabilir", ErrValidation)
	}
	if t.Category != "Go" && t.Category != "Kubernetes" && t.Category != "chromedp" && t.Category != "Genel" {
		return fmt.Errorf("%w: kategori tanınmıyor", ErrValidation)
	}
	if t.Priority != "low" && t.Priority != "medium" && t.Priority != "high" {
		return fmt.Errorf("%w: öncelik tanınmıyor", ErrValidation)
	}
	if t.Status != "todo" && t.Status != "doing" && t.Status != "done" {
		return fmt.Errorf("%w: durum tanınmıyor", ErrValidation)
	}
	if t.DueDate != "" {
		if _, err := time.Parse("2006-01-02", t.DueDate); err != nil {
			return fmt.Errorf("%w: tarih YYYY-MM-DD biçiminde olmalı", ErrValidation)
		}
	}
	return nil
}

const columns = "id,title,description,category,priority,status,due_date,version,created_at,updated_at,archived,deleted_at"

func scan(row interface{ Scan(...any) error }) (Task, error) {
	var t Task
	err := row.Scan(&t.ID, &t.Title, &t.Description, &t.Category, &t.Priority, &t.Status, &t.DueDate, &t.Version, &t.CreatedAt, &t.UpdatedAt, &t.Archived, &t.DeletedAt)
	return t, err
}
func (s *Store) List(ctx context.Context) ([]Task, error) {
	return s.ListScope(ctx, "active")
}
func (s *Store) ListScope(ctx context.Context, scope string) ([]Task, error) {
	filter := " AND deleted_at=''"
	if scope == "trash" {
		filter = " AND deleted_at<>''"
	} else if scope == "archive" {
		filter += " AND archived=true"
	} else if scope != "all" {
		filter += " AND archived=false"
	}
	rows, err := s.db.QueryContext(ctx, s.q("SELECT "+columns+" FROM tasks WHERE owner_id=?"+filter+" ORDER BY id DESC"), owner(ctx))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Task{}
	for rows.Next() {
		t, err := scan(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}
func (s *Store) Create(ctx context.Context, t Task) (Task, error) {
	if err := normalize(&t); err != nil {
		return t, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return scan(s.db.QueryRowContext(ctx, s.q("INSERT INTO tasks(title,description,category,priority,status,due_date,created_at,updated_at,owner_id,archived) VALUES(?,?,?,?,?,?,?,?,?,?) RETURNING "+columns), t.Title, t.Description, t.Category, t.Priority, t.Status, t.DueDate, now, now, owner(ctx), t.Archived))
}
func (s *Store) Update(ctx context.Context, id int64, t Task) (Task, error) {
	if err := normalize(&t); err != nil {
		return t, err
	}
	if t.Version < 1 {
		return t, fmt.Errorf("%w: version gerekli", ErrValidation)
	}
	result, err := scan(s.db.QueryRowContext(ctx, s.q("UPDATE tasks SET title=?,description=?,category=?,priority=?,status=?,due_date=?,version=version+1,updated_at=?,archived=? WHERE id=? AND version=? AND owner_id=? AND deleted_at='' RETURNING "+columns), t.Title, t.Description, t.Category, t.Priority, t.Status, t.DueDate, time.Now().UTC().Format(time.RFC3339Nano), t.Archived, id, t.Version, owner(ctx)))
	if errors.Is(err, sql.ErrNoRows) {
		return t, s.missingOrConflict(ctx, id)
	}
	return result, err
}
func (s *Store) missingOrConflict(ctx context.Context, id int64) error {
	var exists int
	err := s.db.QueryRowContext(ctx, s.q("SELECT 1 FROM tasks WHERE id=? AND owner_id=? AND deleted_at=''"), id, owner(ctx)).Scan(&exists)
	if err != nil {
		return err
	}
	return ErrConflict
}
func (s *Store) Delete(ctx context.Context, id int64, version int) error {
	if version < 1 {
		return fmt.Errorf("%w: version gerekli", ErrValidation)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	r, err := s.db.ExecContext(ctx, s.q("UPDATE tasks SET deleted_at=?,updated_at=?,version=version+1 WHERE id=? AND version=? AND owner_id=? AND deleted_at=''"), now, now, id, version, owner(ctx))
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return s.missingOrConflict(ctx, id)
	}
	return nil
}
