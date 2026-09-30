package tasks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Approval mode is persisted: restarting without an environment variable cannot
// silently reopen this database in anonymous mode.
func (s *Store) ApprovalEnabled(ctx context.Context) (bool, error) {
	var enabled bool
	err := s.db.QueryRowContext(ctx, "SELECT approval_required FROM identity_settings WHERE id=1").Scan(&enabled)
	return enabled, err
}

func (s *Store) EnableApproval(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "UPDATE identity_settings SET approval_required=true WHERE id=1")
	return err
}

func (s *Store) Users(ctx context.Context, after string) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, s.q("SELECT id,email,role,status,version,email_verified,mfa_enabled FROM users WHERE id>? ORDER BY id LIMIT 100"), after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.Role, &u.Status, &u.Version, &u.EmailVerified, &u.MFAEnabled); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *Store) BootstrapAdmin(ctx context.Context, email string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// A real UPDATE serializes this critical section on both SQLite and PG.
	if _, err = tx.ExecContext(ctx, "UPDATE identity_settings SET revision=revision+1 WHERE id=1"); err != nil {
		return err
	}
	var approval, bootstrapped bool
	if err = tx.QueryRowContext(ctx, "SELECT approval_required,bootstrapped FROM identity_settings WHERE id=1").Scan(&approval, &bootstrapped); err != nil {
		return err
	}
	if !approval {
		return fmt.Errorf("önce hesap onaylı modu etkinleştirin")
	}
	if bootstrapped {
		return fmt.Errorf("ilk yönetici daha önce belirlendi; bootstrap tekrar kullanılamaz")
	}
	var id string
	if err = tx.QueryRowContext(ctx, s.q("SELECT id FROM users WHERE email=? AND status<>'suspended' AND email_verified=true AND mfa_enabled=true"), strings.ToLower(strings.TrimSpace(email))).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("e-postası doğrulanmış, MFA kurulmuş ve askıya alınmamış hesap gerekli")
		}
		return err
	}
	if _, err = tx.ExecContext(ctx, s.q("UPDATE users SET role='superadmin',status='active',version=version+1 WHERE id=?"), id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE identity_settings SET bootstrapped=true WHERE id=1"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.q("DELETE FROM sessions WHERE user_id=?"), id); err != nil {
		return err
	}
	if err = s.identityAudit(ctx, tx, "local-operator", id, "admin.bootstrap"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) identityAudit(ctx context.Context, tx *sql.Tx, actor, target, action string) error {
	_, err := tx.ExecContext(ctx, s.q("INSERT INTO identity_audit(id,actor_id,target_id,action,created_at) VALUES(?,?,?,?,?)"), token(), actor, target, action, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) isAdmin(ctx context.Context) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, s.q("SELECT count(*) FROM users WHERE id=? AND role='superadmin' AND status='active' AND email_verified=true AND mfa_enabled=true"), owner(ctx)).Scan(&n)
	return n == 1, err
}

// Account state changes and audit/session revocation are one atomic operation.
// Role assignment deliberately has no HTTP endpoint in this initial release.
func (s *Store) changeAccount(ctx context.Context, target, status, password string, version int) error {
	if (status != "active" && status != "suspended") || version < 1 || len(password) > 72 {
		return fmt.Errorf("%w: hesap durumu, sürüm ve mevcut parola gerekli", ErrValidation)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "UPDATE identity_settings SET revision=revision+1 WHERE id=1"); err != nil {
		return err
	}
	var hash string
	err = tx.QueryRowContext(ctx, s.q("UPDATE users SET version=version WHERE id=? AND role='superadmin' AND status='active' AND email_verified=true AND mfa_enabled=true RETURNING password_hash"), owner(ctx)).Scan(&hash)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return errReauthenticate
	}
	if session, ok := ctx.Value(mfaSessionContextKey{}).(string); ok {
		var n int
		if err = tx.QueryRowContext(ctx, s.q("SELECT count(*) FROM sessions WHERE token_hash=? AND user_id=? AND expires_at>? AND mfa_verified_at>?"), session, owner(ctx), time.Now().Unix(), time.Now().Add(-5*time.Minute).Unix()).Scan(&n); err != nil {
			return err
		}
		if n != 1 {
			return errReauthenticate
		}
	}
	if target == owner(ctx) {
		return fmt.Errorf("%w: kendi yönetici hesabının durumu bu işlemle değiştirilemez", ErrValidation)
	}
	if status == "suspended" {
		var n int
		if err = tx.QueryRowContext(ctx, s.q("SELECT count(*) FROM teams WHERE leader_id=?"), target).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return fmt.Errorf("%w: hesabı askıya almadan önce grup liderliğini devret", ErrValidation)
		}
		rows, e := tx.QueryContext(ctx, s.q("SELECT g.id,g.leader_id FROM teams g JOIN team_members m ON m.team_id=g.id WHERE m.user_id=?"), target)
		if e != nil {
			return e
		}
		type transfer struct{ id, leader string }
		groups := []transfer{}
		for rows.Next() {
			var g transfer
			if e = rows.Scan(&g.id, &g.leader); e != nil {
				rows.Close()
				return e
			}
			groups = append(groups, g)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		for _, g := range groups {
			if err = s.transferTeamTasks(ctx, tx, g.id, target, g.leader); err != nil {
				return err
			}
		}
	}
	result, err := tx.ExecContext(ctx, s.q("UPDATE users SET status=CASE WHEN ?='active' AND email_verified=false THEN 'pending' ELSE ? END,version=version+1 WHERE id=? AND role='member' AND version=? AND (email_verified=true OR ?='suspended' OR status='suspended')"), status, status, target, version, status)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrConflict
	}
	if _, err = tx.ExecContext(ctx, s.q("DELETE FROM sessions WHERE user_id=?"), target); err != nil {
		return err
	}
	if status == "suspended" {
		for _, table := range []string{"email_verifications", "dev_mailbox"} {
			if _, err = tx.ExecContext(ctx, s.q("DELETE FROM "+table+" WHERE user_id=?"), target); err != nil {
				return err
			}
		}
	}
	var actualStatus string
	if err = tx.QueryRowContext(ctx, s.q("SELECT status FROM users WHERE id=?"), target).Scan(&actualStatus); err != nil {
		return err
	}
	if err = s.identityAudit(ctx, tx, owner(ctx), target, "account."+actualStatus); err != nil {
		return err
	}
	return tx.Commit()
}

var errReauthenticate = errors.New("işlemi onaylamak için mevcut yönetici parolanı doğru gir")

func (s *Store) identityRoutes(mux *http.ServeMux, o Options) {
	guard := func(w http.ResponseWriter, r *http.Request) bool {
		if !o.Auth || !o.Approval {
			http.NotFound(w, r)
			return false
		}
		ok, err := s.isAdmin(r.Context())
		if err != nil {
			fail(w, err)
			return false
		}
		if !ok {
			respond(w, 403, map[string]string{"error": "Bu işlem ana yönetici yetkisi gerektirir."})
			return false
		}
		return true
	}
	mux.HandleFunc("GET /api/admin/users", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		after := r.URL.Query().Get("after")
		if len(after) > 64 {
			respond(w, 400, map[string]string{"error": "Geçersiz sayfa anahtarı"})
			return
		}
		users, err := s.Users(r.Context(), after)
		if err != nil {
			fail(w, err)
			return
		}
		next := ""
		if len(users) == 100 {
			next = users[99].ID
		}
		respond(w, 200, map[string]any{"users": users, "next": next})
	})
	mux.HandleFunc("POST /api/admin/users/{id}/status", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		if ok, err := s.allow(r.Context(), "admin-action:"+owner(r.Context()), 5, time.Minute); err != nil {
			fail(w, err)
			return
		} else if !ok {
			limited(w)
			return
		}
		var in struct {
			Status   string `json:"status"`
			Password string `json:"password"`
			Version  int    `json:"version"`
		}
		if !decode(w, r, &in) {
			return
		}
		err := s.changeAccount(r.Context(), r.PathValue("id"), in.Status, in.Password, in.Version)
		if errors.Is(err, ErrConflict) {
			respond(w, 409, map[string]string{"error": "Hesap değişmiş veya bu işlem için uygun değil. Listeyi yenileyin."})
			return
		}
		if errors.Is(err, errReauthenticate) {
			respond(w, 403, map[string]string{"error": err.Error()})
			return
		}
		if err != nil {
			fail(w, err)
			return
		}
		w.WriteHeader(204)
	})
}
