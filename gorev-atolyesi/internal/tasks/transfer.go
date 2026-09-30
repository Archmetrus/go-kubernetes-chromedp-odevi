package tasks

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// TransferSQLite opens the source read-only, including its committed WAL data.
// Only legacy/local-owned, non-deleted tasks can be transferred to one user.
func (s *Store) TransferSQLite(ctx context.Context, path, email string) (int, error) {
	var id string
	if err := s.db.QueryRowContext(ctx, s.q("SELECT id FROM users WHERE email=?"), strings.ToLower(strings.TrimSpace(email))).Scan(&id); err != nil {
		return 0, err
	}
	var existing int
	if err := s.db.QueryRowContext(ctx, s.q("SELECT count(*) FROM tasks WHERE owner_id=?"), id).Scan(&existing); err != nil {
		return 0, err
	}
	if existing != 0 {
		return 0, fmt.Errorf("hedef hesabın görevleri var; tekrar taşıma reddedildi")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return 0, err
	}
	u := url.URL{Scheme: "file", Path: absolute, RawQuery: "mode=ro"}
	source, err := sql.Open("sqlite", u.String())
	if err != nil {
		return 0, err
	}
	defer source.Close()
	var modern int
	if err = source.QueryRowContext(ctx, "SELECT count(*) FROM pragma_table_info('tasks') WHERE name='owner_id'").Scan(&modern); err != nil {
		return 0, err
	}
	query := "SELECT id,title,description,category,priority,status,due_date,version,created_at,updated_at,false,'' FROM tasks"
	if modern > 0 {
		query = "SELECT " + columns + " FROM tasks WHERE owner_id='local' AND deleted_at=''"
	}
	rows, err := source.QueryContext(ctx, query)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	list := []Task{}
	for rows.Next() {
		t, e := scan(rows)
		if e != nil {
			return 0, e
		}
		list = append(list, t)
	}
	if err = rows.Err(); err != nil {
		return 0, err
	}
	return s.Import(WithOwner(ctx, id), list)
}
