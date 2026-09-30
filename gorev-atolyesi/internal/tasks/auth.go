package tasks

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

type Options struct {
	Auth         bool
	Signup       bool
	Approval     bool
	Production   bool
	Origin       string
	Version      string
	MetricsToken string
	MFAKey       []byte
}

func (o Options) Validate(postgres bool) error {
	if o.Approval && (!o.Auth || o.Production) {
		return fmt.Errorf("hesap onayı şu an yalnızca AUTH_ENABLED=true ile geliştirme ortamında kullanılabilir; gerçek posta teslimatı ve parola kurtarma yayın kapıları henüz tamamlanmadı")
	}
	if o.Production && (!o.Auth || !postgres || o.MetricsToken == "" || o.Version == "" || o.Version == "dev") {
		return fmt.Errorf("production: PostgreSQL, AUTH_ENABLED=true, METRICS_TOKEN ve APP_VERSION gerekli")
	}
	if o.Origin != "" {
		u, e := url.Parse(o.Origin)
		if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || (u.Scheme != "https" && u.Scheme != "http") {
			return fmt.Errorf("APP_ORIGIN yalnızca http(s) origin olmalı")
		}
		if o.Production && u.Scheme != "https" {
			return fmt.Errorf("production HTTPS origin gerektirir")
		}
	}
	if o.Production && o.Origin == "" {
		return fmt.Errorf("production APP_ORIGIN gerekli")
	}
	return nil
}

type User struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	Role          string `json:"role"`
	Status        string `json:"status"`
	Version       int    `json:"version"`
	EmailVerified bool   `json:"email_verified"`
	MFAEnabled    bool   `json:"mfa_enabled"`
	MFAVerifiedAt int64  `json:"mfa_verified_at"`
}

func token() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func digest(v string) string { b := sha256.Sum256([]byte(v)); return hex.EncodeToString(b[:]) }
func (s *Store) CreateUser(ctx context.Context, email, password string) (User, error) {
	approval, err := s.ApprovalEnabled(ctx)
	if err != nil {
		return User{}, err
	}
	return s.createUser(ctx, email, password, approval, false)
}
func (s *Store) createUser(ctx context.Context, email, password string, pending, ignoreDuplicate bool) (User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	a, e := mail.ParseAddress(email)
	if e != nil || a.Address != email || len(email) > 254 || utf8.RuneCountInString(password) < 12 || len(password) > 72 {
		return User{}, fmt.Errorf("%w: geçerli e-posta ve 12–72 bayt parola gerekli", ErrValidation)
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(password), 12)
	if e != nil {
		return User{}, e
	}
	u := User{ID: token(), Email: email, Role: "member", Status: "active", Version: 1}
	if pending {
		u.Status = "pending"
	}
	query := "INSERT INTO users(id,email,password_hash,status) VALUES(?,?,?,?)"
	if ignoreDuplicate {
		query += " ON CONFLICT(email) DO NOTHING"
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return User{}, e
	}
	defer tx.Rollback()
	result, e := tx.ExecContext(ctx, s.q(query), u.ID, email, string(hash), u.Status)
	if e != nil {
		return User{}, e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return User{}, e
	}
	if pending && n == 1 {
		if e = s.issueVerificationTx(ctx, tx, u.ID); e != nil {
			return User{}, e
		}
	}
	return u, tx.Commit()
}
func (s *Store) ClaimLocal(ctx context.Context, email string) (int64, error) {
	var id string
	if err := s.db.QueryRowContext(ctx, s.q("SELECT id FROM users WHERE email=?"), strings.ToLower(strings.TrimSpace(email))).Scan(&id); err != nil {
		return 0, err
	}
	r, err := s.db.ExecContext(ctx, s.q("UPDATE tasks SET owner_id=?,version=version+1 WHERE owner_id='local'"), id)
	if err != nil {
		return 0, err
	}
	return r.RowsAffected()
}

var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("not-a-real-user-password"), 12)

func (s *Store) authRoutes(mux *http.ServeMux, o Options) {
	mux.HandleFunc("GET /api/session", func(w http.ResponseWriter, r *http.Request) {
		u, _, err := s.session(r, o)
		if err != nil && !errors.Is(err, sql.ErrNoRows) && !errors.Is(err, http.ErrNoCookie) {
			fail(w, err)
			return
		}
		respond(w, 200, map[string]any{"auth_enabled": o.Auth, "signup_enabled": o.Auth && o.Signup, "approval_required": o.Approval, "user": u, "version": o.Version})
	})
	mux.HandleFunc("POST /api/register", func(w http.ResponseWriter, r *http.Request) {
		if !o.Auth || !o.Signup {
			respond(w, 403, map[string]string{"error": "Kayıt kapalı; yöneticiden hesap isteyin."})
			return
		}
		var in struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if !decode(w, r, &in) {
			return
		}
		_, err := s.createUser(r.Context(), in.Email, in.Password, o.Approval, o.Approval)
		if err != nil {
			if errors.Is(err, ErrValidation) {
				fail(w, err)
			} else {
				respond(w, 409, map[string]string{"error": "Hesap oluşturulamadı. Bilgileri kontrol edin."})
			}
			return
		}
		if o.Approval {
			respond(w, 202, map[string]string{"status": "accepted", "message": "Bilgiler uygunsa kayıt isteğin alındı. Giriş yaparak hesap durumunu görebilirsin."})
		} else {
			respond(w, 201, map[string]string{"status": "created"})
		}
	})
	mux.HandleFunc("POST /api/login", func(w http.ResponseWriter, r *http.Request) {
		if !o.Auth {
			respond(w, 400, map[string]string{"error": "Yerel modda giriş gerekmiyor."})
			return
		}
		var in struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if !decode(w, r, &in) {
			return
		}
		email := strings.ToLower(strings.TrimSpace(in.Email))
		if ok, err := s.allow(r.Context(), "login-email:"+digest(email), 10, time.Minute*15); err != nil {
			fail(w, err)
			return
		} else if !ok {
			limited(w)
			return
		}
		var u User
		var hash string
		err := s.db.QueryRowContext(r.Context(), s.q("SELECT id,email,password_hash,role,status,version,email_verified,mfa_enabled FROM users WHERE email=?"), email).Scan(&u.ID, &u.Email, &hash, &u.Role, &u.Status, &u.Version, &u.EmailVerified, &u.MFAEnabled)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			fail(w, err)
			return
		}
		if hash == "" {
			hash = string(dummyHash)
		}
		valid := bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password)) == nil
		if err != nil || !valid || u.Status == "suspended" {
			respond(w, 401, map[string]string{"error": "E-posta veya parola hatalı."})
			return
		}
		raw := token()
		tx, err := s.db.BeginTx(r.Context(), nil)
		if err != nil {
			fail(w, err)
			return
		}
		defer tx.Rollback()
		if err = tx.QueryRowContext(r.Context(), s.q("UPDATE users SET version=version WHERE id=? AND password_hash=? AND status<>'suspended' RETURNING mfa_enabled"), u.ID, hash).Scan(&u.MFAEnabled); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				respond(w, 401, map[string]string{"error": "E-posta veya parola hatalı."})
			} else {
				fail(w, err)
			}
			return
		}
		if old, e := r.Cookie(cookieName(o)); e == nil {
			if _, err = tx.ExecContext(r.Context(), s.q("DELETE FROM sessions WHERE token_hash=?"), digest(old.Value)); err != nil {
				fail(w, err)
				return
			}
		}
		if _, err = tx.ExecContext(r.Context(), s.q("DELETE FROM sessions WHERE expires_at<?"), time.Now().Unix()); err != nil {
			fail(w, err)
			return
		}
		duration := 12 * time.Hour
		if u.MFAEnabled {
			duration = 5 * time.Minute
		}
		if _, err = tx.ExecContext(r.Context(), s.q("INSERT INTO sessions(token_hash,user_id,expires_at) VALUES(?,?,?)"), digest(raw), u.ID, time.Now().Add(duration).Unix()); err != nil {
			fail(w, err)
			return
		}
		if err = tx.Commit(); err != nil {
			fail(w, err)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: cookieName(o), Value: raw, Path: "/", HttpOnly: true, Secure: o.Production, SameSite: http.SameSiteStrictMode, MaxAge: 43200})
		respond(w, 200, u)
	})
	mux.HandleFunc("POST /api/logout", func(w http.ResponseWriter, r *http.Request) {
		if c, e := r.Cookie(cookieName(o)); e == nil {
			if _, e = s.db.ExecContext(r.Context(), s.q("DELETE FROM sessions WHERE token_hash=?"), digest(c.Value)); e != nil {
				fail(w, e)
				return
			}
		}
		http.SetCookie(w, &http.Cookie{Name: cookieName(o), Value: "", Path: "/", HttpOnly: true, Secure: o.Production, SameSite: http.SameSiteStrictMode, MaxAge: -1})
		w.WriteHeader(204)
	})
}
func cookieName(o Options) string {
	if o.Production {
		return "__Host-atolye"
	}
	return "atolye"
}
func (s *Store) session(r *http.Request, o Options) (*User, string, error) {
	if !o.Auth {
		return nil, "local", nil
	}
	c, err := r.Cookie(cookieName(o))
	if err != nil {
		return nil, "", err
	}
	var u User
	err = s.db.QueryRowContext(r.Context(), s.q("SELECT u.id,u.email,u.role,u.status,u.version,u.email_verified,u.mfa_enabled,s.mfa_verified_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND s.expires_at>?"), digest(c.Value), time.Now().Unix()).Scan(&u.ID, &u.Email, &u.Role, &u.Status, &u.Version, &u.EmailVerified, &u.MFAEnabled, &u.MFAVerifiedAt)
	if err != nil {
		return nil, "", err
	}
	return &u, u.ID, nil
}
func limited(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "60")
	respond(w, 429, map[string]string{"error": "Çok fazla istek. Biraz bekleyip tekrar deneyin."})
}
func (s *Store) allow(ctx context.Context, key string, max int, window time.Duration) (bool, error) {
	now := time.Now().Unix()
	var hits int
	err := s.db.QueryRowContext(ctx, s.q(`INSERT INTO rate_limits(key,hits,reset_at) VALUES(?,1,?) ON CONFLICT(key) DO UPDATE SET hits=CASE WHEN rate_limits.reset_at<=? THEN 1 ELSE rate_limits.hits+1 END,reset_at=CASE WHEN rate_limits.reset_at<=? THEN ? ELSE rate_limits.reset_at END RETURNING hits`), key, now+int64(window.Seconds()), now, now, now+int64(window.Seconds())).Scan(&hits)
	return hits <= max, err
}
func (s *Store) Cleanup(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, s.q("DELETE FROM assignment_drafts WHERE expires_at<? AND job_id=''"), time.Now().Unix()); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, s.q("DELETE FROM rate_limits WHERE reset_at<?"), time.Now().Unix())
	if err != nil {
		return err
	}
	for _, table := range []string{"email_verifications", "dev_mailbox"} {
		if _, err = s.db.ExecContext(ctx, s.q("DELETE FROM "+table+" WHERE expires_at<=?"), time.Now().Unix()); err != nil {
			return err
		}
	}
	_, err = s.db.ExecContext(ctx, s.q("UPDATE user_mfa SET pending_secret='',pending_session='',pending_until=0 WHERE pending_until>0 AND pending_until<=?"), time.Now().Unix())
	return err
}
func (s *Store) protect(next http.Handler, o Options) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		required, err := s.ApprovalEnabled(r.Context())
		if err != nil {
			fail(w, err)
			return
		}
		if required && (!o.Auth || !o.Approval) {
			respond(w, 503, map[string]string{"error": "Hesap politikası değişti. Sunucuyu hesap onaylı modda yeniden başlatın."})
			return
		}
		// Exact Origin check is needed behind a TLS-terminating proxy. No forwarded
		// headers are trusted for authorization or rate-limit identity.
		if r.Method != "GET" && r.Method != "HEAD" {
			origin := r.Header.Get("Origin")
			if origin != "" {
				expected := o.Origin
				if expected == "" {
					scheme := "http"
					if r.TLS != nil {
						scheme = "https"
					}
					expected = scheme + "://" + r.Host
				}
				if origin != expected {
					respond(w, 403, map[string]string{"error": "İstek kaynağı kabul edilmedi."})
					return
				}
			}
		}
		if authorization := r.Header.Get("Authorization"); authorization != "" {
			if !o.Auth || !o.Approval || !strings.HasPrefix(r.URL.Path, "/api/automation/") || !strings.HasPrefix(authorization, "Bearer ") {
				respond(w, 403, map[string]string{"error": "Anahtar bu API işlemini kapsamıyor."})
				return
			}
			host, _, _ := net.SplitHostPort(r.RemoteAddr)
			ok, e := s.allow(r.Context(), "automation-ip:"+host, 120, time.Minute)
			if e != nil {
				fail(w, e)
				return
			}
			if !ok {
				limited(w)
				return
			}
			a, e := s.automationAuth(r.Context(), strings.TrimPrefix(authorization, "Bearer "))
			if e != nil {
				if errors.Is(e, ErrForbidden) {
					respond(w, 401, map[string]string{"error": "Erişim anahtarı geçersiz veya süresi dolmuş."})
				} else {
					fail(w, e)
				}
				return
			}
			ctx := context.WithValue(WithOwner(r.Context(), a.UserID), automationContextKey{}, a)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		public := r.URL.Path == "/api/session" || r.URL.Path == "/api/login" || r.URL.Path == "/api/register" || r.URL.Path == "/api/logout"
		id := "local"
		if !public {
			u, uid, err := s.session(r, o)
			if err != nil {
				if errors.Is(err, http.ErrNoCookie) || errors.Is(err, sql.ErrNoRows) {
					respond(w, 401, map[string]string{"error": "Devam etmek için giriş yapın."})
				} else {
					fail(w, err)
				}
				return
			}
			id = uid
			verification := r.Method == "POST" && (r.URL.Path == "/api/email-verification/request" || r.URL.Path == "/api/email-verification/confirm")
			security := r.Method == "POST" && (r.URL.Path == "/api/mfa/setup" || r.URL.Path == "/api/mfa/confirm" || r.URL.Path == "/api/mfa/verify" || r.URL.Path == "/api/mfa/recovery")
			if u != nil && (u.Status == "suspended" || (!verification && ((u.Status != "active" && !security) || (o.Approval && !u.EmailVerified)))) {
				respond(w, 403, map[string]string{"error": "Hesabın henüz aktif değil. Hesap durumunu kontrol et.", "code": "account_inactive"})
				return
			}
			if u != nil && ((u.MFAEnabled && u.MFAVerifiedAt == 0) || (o.Approval && u.Role == "superadmin" && !u.MFAEnabled)) && !security {
				respond(w, 403, map[string]string{"error": "İki adımlı doğrulamayı tamamla.", "code": "mfa_required"})
				return
			}
			if u != nil && u.Role == "superadmin" && strings.HasPrefix(r.URL.Path, "/api/admin/") && (!u.MFAEnabled || u.MFAVerifiedAt < time.Now().Add(-5*time.Minute).Unix()) {
				respond(w, 403, map[string]string{"error": "Yönetim için güncel MFA doğrulaması gerekli.", "code": "mfa_required"})
				return
			}
		}
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		key := "api:" + id
		max := 300
		if public {
			key = "public:" + host
			max = 120
			if r.Method == "POST" {
				key = "auth:" + host
				max = 20
			}
		}
		if o.Auth {
			if ok, err := s.allow(r.Context(), key, max, time.Minute); err != nil {
				fail(w, err)
				return
			} else if !ok {
				limited(w)
				return
			}
		}
		ctx := WithOwner(r.Context(), id)
		if c, e := r.Cookie(cookieName(o)); e == nil {
			ctx = context.WithValue(ctx, mfaSessionContextKey{}, digest(c.Value))
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
func equalSecret(a, b string) bool {
	return len(a) > 0 && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
