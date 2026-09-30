package tasks

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// DevMessage is an OS-operator-only development mail sink. Never expose it in an
// HTTP route. It is deliberately not a production email delivery mechanism.
type DevMessage struct {
	Email     string `json:"email"`
	Code      string `json:"code"`
	ExpiresAt int64  `json:"expires_at"`
}

func (s *Store) issueVerificationTx(ctx context.Context, tx *sql.Tx, id string) error {
	raw := token()[:32] // 128 random bits; the authenticated account is also required.
	expires := time.Now().Add(24 * time.Hour).Unix()
	if _, err := tx.ExecContext(ctx, s.q(`INSERT INTO email_verifications(user_id,token_hash,expires_at) VALUES(?,?,?) ON CONFLICT(user_id) DO UPDATE SET token_hash=excluded.token_hash,expires_at=excluded.expires_at`), id, digest(raw), expires); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, s.q(`INSERT INTO dev_mailbox(user_id,code,expires_at) VALUES(?,?,?) ON CONFLICT(user_id) DO UPDATE SET code=excluded.code,expires_at=excluded.expires_at`), id, raw, expires)
	return err
}

func (s *Store) RequestVerification(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Row lock serializes resend/confirm with account suspension.
	var verified bool
	err = tx.QueryRowContext(ctx, s.q("UPDATE users SET version=version WHERE id=? AND status<>'suspended' RETURNING email_verified"), id).Scan(&verified)
	if err != nil {
		return err
	}
	if !verified {
		if err = s.issueVerificationTx(ctx, tx, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

var errVerification = errors.New("Kod geçersiz veya süresi dolmuş. En son iletiyi kontrol et ya da yeni kod iste.")

func (s *Store) ConfirmVerification(ctx context.Context, id, raw string) error {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if len(raw) != 32 {
		return errVerification
	}
	if _, err := hex.DecodeString(raw); err != nil {
		return errVerification
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var verified bool
	err = tx.QueryRowContext(ctx, s.q("UPDATE users SET version=version WHERE id=? AND status<>'suspended' RETURNING email_verified"), id).Scan(&verified)
	if errors.Is(err, sql.ErrNoRows) || verified {
		return errVerification
	}
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, s.q("DELETE FROM email_verifications WHERE user_id=? AND token_hash=? AND expires_at>?"), id, digest(raw), time.Now().Unix())
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errVerification
	}
	if _, err = tx.ExecContext(ctx, s.q("UPDATE users SET email_verified=true WHERE id=?"), id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.q("DELETE FROM dev_mailbox WHERE user_id=?"), id); err != nil {
		return err
	}
	if err = s.identityAudit(ctx, tx, id, id, "email.verified"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DevMailbox(ctx context.Context) ([]DevMessage, error) {
	rows, err := s.db.QueryContext(ctx, s.q("SELECT u.email,m.code,m.expires_at FROM dev_mailbox m JOIN users u ON u.id=m.user_id WHERE m.expires_at>? AND u.email_verified=false AND u.status<>'suspended' ORDER BY m.expires_at DESC LIMIT 100"), time.Now().Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := []DevMessage{}
	for rows.Next() {
		var m DevMessage
		if err = rows.Scan(&m.Email, &m.Code, &m.ExpiresAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

func (s *Store) verificationRoutes(mux *http.ServeMux, o Options) {
	for _, name := range []string{"request", "confirm"} {
		mux.HandleFunc("POST /api/email-verification/"+name, func(w http.ResponseWriter, r *http.Request) {
			if !o.Auth || !o.Approval || o.Production {
				http.NotFound(w, r)
				return
			}
			limit, window := 10, time.Minute
			if name == "request" {
				limit, window = 1, time.Minute
			}
			ok, err := s.allow(r.Context(), "verification:"+name+":"+owner(r.Context()), limit, window)
			if err != nil {
				fail(w, err)
				return
			}
			if !ok {
				limited(w)
				return
			}
			if name == "request" {
				var in struct{}
				if !decode(w, r, &in) {
					return
				}
				err = s.RequestVerification(r.Context(), owner(r.Context()))
			} else {
				var in struct {
					Code string `json:"code"`
				}
				if !decode(w, r, &in) {
					return
				}
				err = s.ConfirmVerification(r.Context(), owner(r.Context()), in.Code)
			}
			if errors.Is(err, errVerification) || errors.Is(err, sql.ErrNoRows) {
				respond(w, 400, map[string]string{"error": errVerification.Error()})
				return
			}
			if err != nil {
				fail(w, err)
				return
			}
			respond(w, 200, map[string]string{"status": fmt.Sprintf("verification_%s_ok", name)})
		})
	}
}
