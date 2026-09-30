package tasks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *Store) CheckSchema(ctx context.Context) error {
	var version int
	if err := s.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version),0) FROM schema_migrations").Scan(&version); err != nil {
		return fmt.Errorf("önce migrate çalıştırın: %w", err)
	}
	if version != 7 {
		return fmt.Errorf("beklenen şema 7, mevcut %d; migrate çalıştırın", version)
	}
	return nil
}

// Migrations are additive. PostgreSQL serializes concurrent deploy jobs with a
// transaction-scoped advisory lock. SQLite migration runs before serving traffic.
func (s *Store) Migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if s.postgres {
		if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(73461902)"); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY)"); err != nil {
		return err
	}
	base := strings.ReplaceAll(schema, "PRAGMA user_version=1;", "")
	if s.postgres {
		base = strings.ReplaceAll(base, "INTEGER PRIMARY KEY AUTOINCREMENT", "BIGSERIAL PRIMARY KEY")
	}
	migrations := []string{base, `
 CREATE TABLE users(id TEXT PRIMARY KEY,email TEXT NOT NULL UNIQUE,password_hash TEXT NOT NULL);
 CREATE TABLE sessions(token_hash TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,expires_at BIGINT NOT NULL);
 CREATE INDEX sessions_expiry ON sessions(expires_at);
 ALTER TABLE tasks ADD COLUMN owner_id TEXT NOT NULL DEFAULT 'local';
 ALTER TABLE tasks ADD COLUMN archived BOOLEAN NOT NULL DEFAULT false;
 ALTER TABLE tasks ADD COLUMN deleted_at TEXT NOT NULL DEFAULT '';
 CREATE INDEX tasks_owner ON tasks(owner_id,id);
 CREATE TABLE rate_limits(key TEXT PRIMARY KEY,hits INTEGER NOT NULL,reset_at BIGINT NOT NULL);
 `, `
 ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'member' CHECK(role IN ('member','superadmin'));
 ALTER TABLE users ADD COLUMN status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('pending','active','suspended'));
 ALTER TABLE users ADD COLUMN version INTEGER NOT NULL DEFAULT 1;
 CREATE TABLE identity_settings(id INTEGER PRIMARY KEY CHECK(id=1),approval_required BOOLEAN NOT NULL DEFAULT false,bootstrapped BOOLEAN NOT NULL DEFAULT false,revision INTEGER NOT NULL DEFAULT 1);
 INSERT INTO identity_settings(id) VALUES(1);
 CREATE TABLE identity_audit(id TEXT PRIMARY KEY,actor_id TEXT NOT NULL,target_id TEXT NOT NULL,action TEXT NOT NULL,created_at TEXT NOT NULL);
 CREATE INDEX users_status ON users(status,id);
 `, `
 ALTER TABLE users ADD COLUMN email_verified BOOLEAN NOT NULL DEFAULT false;
 CREATE TABLE email_verifications(user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,token_hash TEXT NOT NULL,expires_at BIGINT NOT NULL);
 CREATE TABLE dev_mailbox(user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,code TEXT NOT NULL,expires_at BIGINT NOT NULL);
 `, `
 ALTER TABLE users ADD COLUMN mfa_enabled BOOLEAN NOT NULL DEFAULT false;
 ALTER TABLE sessions ADD COLUMN mfa_verified_at BIGINT NOT NULL DEFAULT 0;
 ALTER TABLE identity_settings ADD COLUMN mfa_key_hash TEXT NOT NULL DEFAULT '';
 CREATE TABLE user_mfa(user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,secret TEXT NOT NULL DEFAULT '',pending_secret TEXT NOT NULL DEFAULT '',pending_session TEXT NOT NULL DEFAULT '',pending_until BIGINT NOT NULL DEFAULT 0,last_step BIGINT NOT NULL DEFAULT -1);
 CREATE TABLE mfa_recovery(user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,code_hash TEXT NOT NULL,PRIMARY KEY(user_id,code_hash));
 `, `
 CREATE TABLE teams(id TEXT PRIMARY KEY,name TEXT NOT NULL UNIQUE,leader_id TEXT NOT NULL REFERENCES users(id),version INTEGER NOT NULL DEFAULT 1);
 CREATE TABLE team_members(team_id TEXT NOT NULL REFERENCES teams(id),user_id TEXT NOT NULL REFERENCES users(id),can_assign BOOLEAN NOT NULL DEFAULT false,PRIMARY KEY(team_id,user_id));
 CREATE TABLE team_tasks(id TEXT PRIMARY KEY,team_id TEXT NOT NULL REFERENCES teams(id),assignee_id TEXT NOT NULL REFERENCES users(id),creator_id TEXT NOT NULL REFERENCES users(id),title TEXT NOT NULL,description TEXT NOT NULL,category TEXT NOT NULL,priority TEXT NOT NULL,status TEXT NOT NULL CHECK(status IN ('todo','doing','done')),due_date TEXT NOT NULL,version INTEGER NOT NULL DEFAULT 1,created_at TEXT NOT NULL,updated_at TEXT NOT NULL);
 CREATE INDEX team_tasks_group ON team_tasks(team_id,assignee_id);
 CREATE TABLE team_events(id TEXT PRIMARY KEY,task_id TEXT NOT NULL REFERENCES team_tasks(id),actor_id TEXT NOT NULL REFERENCES users(id),action TEXT NOT NULL,created_at TEXT NOT NULL);
 CREATE TABLE demo_fixture(id INTEGER PRIMARY KEY CHECK(id=1),version INTEGER NOT NULL);
 `, `
 CREATE TABLE automation_keys(id TEXT PRIMARY KEY,token_hash TEXT NOT NULL UNIQUE,user_id TEXT NOT NULL REFERENCES users(id),user_version INTEGER NOT NULL,team_id TEXT NOT NULL REFERENCES teams(id),name TEXT NOT NULL,expires_at BIGINT NOT NULL,revoked BOOLEAN NOT NULL DEFAULT false);
 CREATE TABLE assignment_drafts(id TEXT PRIMARY KEY,principal TEXT NOT NULL,user_id TEXT NOT NULL REFERENCES users(id),payload TEXT NOT NULL,expires_at BIGINT NOT NULL,job_id TEXT NOT NULL DEFAULT '');
 CREATE TABLE assignment_jobs(id TEXT PRIMARY KEY,principal TEXT NOT NULL,user_id TEXT NOT NULL REFERENCES users(id),draft_id TEXT NOT NULL UNIQUE REFERENCES assignment_drafts(id),request_key TEXT NOT NULL,result TEXT NOT NULL,created_at TEXT NOT NULL,UNIQUE(principal,request_key));
 `}
	for i, m := range migrations {
		var n int
		if err = tx.QueryRowContext(ctx, s.q("SELECT count(*) FROM schema_migrations WHERE version=?"), i+1).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		// pgx extended query protocol accepts one statement at a time.
		for _, statement := range strings.Split(m, ";") {
			if strings.TrimSpace(statement) == "" {
				continue
			}
			if _, err = tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("migration %d: %w", i+1, err)
			}
		}
		if _, err = tx.ExecContext(ctx, s.q("INSERT INTO schema_migrations(version) VALUES(?)"), i+1); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Restore(ctx context.Context, id int64, version int) (Task, error) {
	if version < 1 {
		return Task{}, fmt.Errorf("%w: version gerekli", ErrValidation)
	}
	t, err := scan(s.db.QueryRowContext(ctx, s.q("UPDATE tasks SET deleted_at='',updated_at=?,version=version+1 WHERE id=? AND owner_id=? AND version=? AND deleted_at<>'' RETURNING "+columns), time.Now().UTC().Format(time.RFC3339Nano), id, owner(ctx), version))
	if errors.Is(err, sql.ErrNoRows) {
		var n int
		check := s.db.QueryRowContext(ctx, s.q("SELECT 1 FROM tasks WHERE id=? AND owner_id=? AND deleted_at<>''"), id, owner(ctx)).Scan(&n)
		if check == nil {
			return t, ErrConflict
		}
		return t, check
	}
	return t, err
}

// Import copies validated tasks in one transaction. IDs and versions belong to
// the destination; importing never overwrites existing tasks.
func (s *Store) Import(ctx context.Context, list []Task) (int, error) {
	if len(list) > 1000 {
		return 0, fmt.Errorf("%w: en fazla 1000 görev", ErrValidation)
	}
	for i := range list {
		if err := normalize(&list[i]); err != nil {
			return 0, err
		}
		if list[i].DeletedAt != "" {
			return 0, fmt.Errorf("%w: çöp kutusu içe aktarılamaz", ErrValidation)
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	for _, t := range list {
		_, err = tx.ExecContext(ctx, s.q("INSERT INTO tasks(title,description,category,priority,status,due_date,created_at,updated_at,owner_id,archived) VALUES(?,?,?,?,?,?,?,?,?,?)"), t.Title, t.Description, t.Category, t.Priority, t.Status, t.DueDate, t.CreatedAt, t.UpdatedAt, owner(ctx), t.Archived)
		if err != nil {
			return 0, err
		}
	}
	return len(list), tx.Commit()
}
