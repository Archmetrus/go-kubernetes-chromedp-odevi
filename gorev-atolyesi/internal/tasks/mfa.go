package tasks

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"database/sql"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type mfaSessionContextKey struct{}

// The DB records a fingerprint, never its encryption key. A missing or changed
// key must not silently replace the key protecting previously enrolled devices.
func (s *Store) LoadMFAKey(ctx context.Context, path string) ([]byte, error) {
	var fingerprint string
	if err := s.db.QueryRowContext(ctx, "SELECT mfa_key_hash FROM identity_settings WHERE id=1").Scan(&fingerprint); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) && fingerprint == "" {
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
		f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e == nil {
			_, e = f.WriteString(token())
			if closeErr := f.Close(); e == nil {
				e = closeErr
			}
		}
		if e != nil && !errors.Is(e, os.ErrExist) {
			return nil, e
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return nil, fmt.Errorf("MFA anahtar dosyası okunamadı; kayıtlı anahtarı geri yükleyin: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("MFA anahtarı symlink olmayan, yalnızca sahibinin eriştiği bir dosya olmalı (chmod 600)")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("MFA anahtarı 32 bayt hex olmalı")
	}
	hash := digest(string(key))
	if _, err = s.db.ExecContext(ctx, s.q("UPDATE identity_settings SET mfa_key_hash=? WHERE id=1 AND mfa_key_hash=''"), hash); err != nil {
		return nil, err
	}
	if err = s.db.QueryRowContext(ctx, "SELECT mfa_key_hash FROM identity_settings WHERE id=1").Scan(&fingerprint); err != nil {
		return nil, err
	}
	if !equalSecret(hash, fingerprint) {
		return nil, fmt.Errorf("MFA anahtarı bu veritabanıyla eşleşmiyor; asıl anahtar dosyasını kullanın")
	}
	return key, nil
}

func sealMFA(key []byte, id, secret string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	return hex.EncodeToString(aead.Seal(nonce, nonce, []byte(secret), []byte(id))), nil
}
func openMFA(key []byte, id, sealed string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	data, err := hex.DecodeString(sealed)
	if err != nil || len(data) < aead.NonceSize() {
		return "", fmt.Errorf("MFA sırrı okunamadı")
	}
	plain, err := aead.Open(nil, data[:aead.NonceSize()], data[aead.NonceSize():], []byte(id))
	return string(plain), err
}

// RFC 6238: SHA-1, 30 seconds, six digits. SHA-1 is the interoperable HMAC
// construction here, not a password hash. Compare codes in constant time.
func totp(secret string, step int64) string {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return ""
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(counter[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 15
	n := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", n%1000000)
}
func matchTOTP(secret, code string, last, now int64) (int64, bool) {
	if len(code) != 6 {
		return 0, false
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	for delta := int64(-1); delta <= 1; delta++ {
		step := now/30 + delta
		if step > last && step >= 0 && subtle.ConstantTimeCompare([]byte(totp(secret, step)), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}

var errMFA = errors.New("Kod geçersiz veya daha önce kullanılmış. Uygulamadaki yeni kodu ya da kullanılmamış kurtarma kodunu gir.")

func (s *Store) mfaUser(ctx context.Context, tx *sql.Tx, id string) (User, string, error) {
	var u User
	var hash string
	err := tx.QueryRowContext(ctx, s.q("UPDATE users SET version=version WHERE id=? AND status<>'suspended' AND email_verified=true RETURNING id,email,role,status,password_hash,mfa_enabled"), id).Scan(&u.ID, &u.Email, &u.Role, &u.Status, &hash, &u.MFAEnabled)
	return u, hash, err
}
func (s *Store) mfaSession(ctx context.Context, tx *sql.Tx, id, session string) error {
	var n int
	if err := tx.QueryRowContext(ctx, s.q("SELECT count(*) FROM sessions WHERE token_hash=? AND user_id=? AND expires_at>?"), session, id, time.Now().Unix()).Scan(&n); err != nil {
		return err
	}
	if n != 1 {
		return errMFA
	}
	return nil
}

func (s *Store) BeginMFA(ctx context.Context, id, session, password string, key []byte) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	u, hash, err := s.mfaUser(ctx, tx, id)
	if err != nil {
		return "", err
	}
	if err = s.mfaSession(ctx, tx, id, session); err != nil {
		return "", err
	}
	if u.MFAEnabled {
		return "", fmt.Errorf("MFA zaten açık; mevcut sır tekrar gösterilmez")
	}
	if len(password) > 72 || bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return "", errReauthenticate
	}
	raw := make([]byte, 20)
	if _, err = rand.Read(raw); err != nil {
		return "", err
	}
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
	encrypted, err := sealMFA(key, id+":pending", secret)
	if err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, s.q(`INSERT INTO user_mfa(user_id,pending_secret,pending_session,pending_until) VALUES(?,?,?,?) ON CONFLICT(user_id) DO UPDATE SET pending_secret=excluded.pending_secret,pending_session=excluded.pending_session,pending_until=excluded.pending_until`), id, encrypted, session, time.Now().Add(10*time.Minute).Unix())
	if err != nil {
		return "", err
	}
	return secret, tx.Commit()
}

func (s *Store) recoveryCodes(ctx context.Context, tx *sql.Tx, id string) ([]string, error) {
	if _, err := tx.ExecContext(ctx, s.q("DELETE FROM mfa_recovery WHERE user_id=?"), id); err != nil {
		return nil, err
	}
	codes := []string{}
	for range 10 {
		raw := token()[:32]
		if _, err := tx.ExecContext(ctx, s.q("INSERT INTO mfa_recovery(user_id,code_hash) VALUES(?,?)"), id, digest(raw)); err != nil {
			return nil, err
		}
		codes = append(codes, raw[:8]+"-"+raw[8:16]+"-"+raw[16:24]+"-"+raw[24:])
	}
	return codes, nil
}
func (s *Store) rotateMFASession(ctx context.Context, tx *sql.Tx, id, old string, all bool) (string, error) {
	if all {
		if _, err := tx.ExecContext(ctx, s.q("DELETE FROM sessions WHERE user_id=?"), id); err != nil {
			return "", err
		}
	} else {
		if _, err := tx.ExecContext(ctx, s.q("DELETE FROM sessions WHERE token_hash=? AND user_id=?"), old, id); err != nil {
			return "", err
		}
	}
	raw := token()
	_, err := tx.ExecContext(ctx, s.q("INSERT INTO sessions(token_hash,user_id,expires_at,mfa_verified_at) VALUES(?,?,?,?)"), digest(raw), id, time.Now().Add(12*time.Hour).Unix(), time.Now().Unix())
	return raw, err
}

func (s *Store) EnableMFA(ctx context.Context, id, session, code string, key []byte) ([]string, string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback()
	u, _, err := s.mfaUser(ctx, tx, id)
	if err != nil {
		return nil, "", err
	}
	if u.MFAEnabled {
		return nil, "", errMFA
	}
	if err = s.mfaSession(ctx, tx, id, session); err != nil {
		return nil, "", err
	}
	var encrypted string
	if err = tx.QueryRowContext(ctx, s.q("SELECT pending_secret FROM user_mfa WHERE user_id=? AND pending_session=? AND pending_until>?"), id, session, time.Now().Unix()).Scan(&encrypted); err != nil {
		return nil, "", errMFA
	}
	secret, err := openMFA(key, id+":pending", encrypted)
	if err != nil {
		return nil, "", err
	}
	step, ok := matchTOTP(secret, code, -1, time.Now().Unix())
	if !ok {
		return nil, "", errMFA
	}
	encrypted, err = sealMFA(key, id+":active", secret)
	if err != nil {
		return nil, "", err
	}
	if _, err = tx.ExecContext(ctx, s.q("UPDATE user_mfa SET secret=?,pending_secret='',pending_session='',pending_until=0,last_step=? WHERE user_id=?"), encrypted, step, id); err != nil {
		return nil, "", err
	}
	if _, err = tx.ExecContext(ctx, s.q("UPDATE users SET mfa_enabled=true,version=version+1 WHERE id=?"), id); err != nil {
		return nil, "", err
	}
	codes, err := s.recoveryCodes(ctx, tx, id)
	if err != nil {
		return nil, "", err
	}
	raw, err := s.rotateMFASession(ctx, tx, id, session, true)
	if err != nil {
		return nil, "", err
	}
	if err = s.identityAudit(ctx, tx, id, id, "mfa.enabled"); err != nil {
		return nil, "", err
	}
	return codes, raw, tx.Commit()
}

func (s *Store) useMFA(ctx context.Context, tx *sql.Tx, id, code string, key []byte) error {
	code = strings.TrimSpace(code)
	if len(code) == 6 {
		var encrypted string
		var last int64
		if err := tx.QueryRowContext(ctx, s.q("SELECT secret,last_step FROM user_mfa WHERE user_id=?"), id).Scan(&encrypted, &last); err != nil {
			return err
		}
		secret, err := openMFA(key, id+":active", encrypted)
		if err != nil {
			return err
		}
		step, ok := matchTOTP(secret, code, last, time.Now().Unix())
		if !ok {
			return errMFA
		}
		_, err = tx.ExecContext(ctx, s.q("UPDATE user_mfa SET last_step=? WHERE user_id=?"), step, id)
		return err
	}
	raw := strings.ToLower(strings.ReplaceAll(code, "-", ""))
	if len(raw) != 32 {
		return errMFA
	}
	result, err := tx.ExecContext(ctx, s.q("DELETE FROM mfa_recovery WHERE user_id=? AND code_hash=?"), id, digest(raw))
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errMFA
	}
	return nil
}

func (s *Store) VerifyMFA(ctx context.Context, id, session, code string, key []byte) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	u, _, err := s.mfaUser(ctx, tx, id)
	if err != nil {
		return "", err
	}
	if !u.MFAEnabled {
		return "", errMFA
	}
	if err = s.mfaSession(ctx, tx, id, session); err != nil {
		return "", err
	}
	if err = s.useMFA(ctx, tx, id, code, key); err != nil {
		return "", err
	}
	raw, err := s.rotateMFASession(ctx, tx, id, session, false)
	if err != nil {
		return "", err
	}
	if err = s.identityAudit(ctx, tx, id, id, "mfa.verified"); err != nil {
		return "", err
	}
	return raw, tx.Commit()
}

func (s *Store) RegenerateRecovery(ctx context.Context, id, session, password, code string, key []byte) ([]string, string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, "", err
	}
	defer tx.Rollback()
	u, hash, err := s.mfaUser(ctx, tx, id)
	if err != nil {
		return nil, "", err
	}
	if !u.MFAEnabled {
		return nil, "", errMFA
	}
	if err = s.mfaSession(ctx, tx, id, session); err != nil {
		return nil, "", err
	}
	if len(password) > 72 || bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return nil, "", errReauthenticate
	}
	if err = s.useMFA(ctx, tx, id, code, key); err != nil {
		return nil, "", err
	}
	codes, err := s.recoveryCodes(ctx, tx, id)
	if err != nil {
		return nil, "", err
	}
	raw, err := s.rotateMFASession(ctx, tx, id, session, true)
	if err != nil {
		return nil, "", err
	}
	if err = s.identityAudit(ctx, tx, id, id, "mfa.recovery.regenerated"); err != nil {
		return nil, "", err
	}
	return codes, raw, tx.Commit()
}

func (s *Store) ResetMFA(ctx context.Context, email string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id string
	if err = tx.QueryRowContext(ctx, s.q("UPDATE users SET mfa_enabled=false,version=version+1 WHERE email=? AND mfa_enabled=true RETURNING id"), strings.ToLower(strings.TrimSpace(email))).Scan(&id); err != nil {
		return fmt.Errorf("MFA etkin hesap bulunamadı: %w", err)
	}
	for _, table := range []string{"user_mfa", "mfa_recovery", "sessions"} {
		if _, err = tx.ExecContext(ctx, s.q("DELETE FROM "+table+" WHERE user_id=?"), id); err != nil {
			return err
		}
	}
	if err = s.identityAudit(ctx, tx, "local-operator", id, "mfa.reset"); err != nil {
		return err
	}
	return tx.Commit()
}

func setMFACookie(w http.ResponseWriter, o Options, raw string) {
	http.SetCookie(w, &http.Cookie{Name: cookieName(o), Value: raw, Path: "/", HttpOnly: true, Secure: o.Production, SameSite: http.SameSiteStrictMode, MaxAge: 43200})
}
func (s *Store) mfaRoutes(mux *http.ServeMux, o Options) {
	for _, name := range []string{"setup", "confirm", "verify", "recovery"} {
		mux.HandleFunc("POST /api/mfa/"+name, func(w http.ResponseWriter, r *http.Request) {
			if !o.Auth || !o.Approval || len(o.MFAKey) != 32 {
				respond(w, 503, map[string]string{"error": "MFA anahtarı yapılandırılmamış."})
				return
			}
			ok, err := s.allow(r.Context(), "mfa:"+owner(r.Context()), 10, 5*time.Minute)
			if err != nil {
				fail(w, err)
				return
			}
			if !ok {
				limited(w)
				return
			}
			var in struct {
				Password string `json:"password"`
				Code     string `json:"code"`
			}
			if !decode(w, r, &in) {
				return
			}
			c, err := r.Cookie(cookieName(o))
			if err != nil {
				respond(w, 401, map[string]string{"error": "Yeniden giriş yapın."})
				return
			}
			id, session := owner(r.Context()), digest(c.Value)
			var raw string
			var codes []string
			if name == "setup" {
				var secret string
				secret, err = s.BeginMFA(r.Context(), id, session, in.Password, o.MFAKey)
				if err == nil {
					respond(w, 200, map[string]string{"secret": secret, "uri": "otpauth://totp/" + url.PathEscape("Görev Atölyesi:"+id) + "?secret=" + secret + "&issuer=" + url.QueryEscape("Görev Atölyesi") + "&algorithm=SHA1&digits=6&period=30"})
					return
				}
			} else if name == "confirm" {
				codes, raw, err = s.EnableMFA(r.Context(), id, session, in.Code, o.MFAKey)
			} else if name == "verify" {
				raw, err = s.VerifyMFA(r.Context(), id, session, in.Code, o.MFAKey)
			} else {
				codes, raw, err = s.RegenerateRecovery(r.Context(), id, session, in.Password, in.Code, o.MFAKey)
			}
			if err != nil {
				if errors.Is(err, errMFA) || errors.Is(err, errReauthenticate) || errors.Is(err, sql.ErrNoRows) {
					respond(w, 400, map[string]string{"error": "Doğrulama başarısız. Parolanı ve kullanılmamış kodunu kontrol et."})
				} else {
					fail(w, err)
				}
				return
			}
			setMFACookie(w, o, raw)
			respond(w, 200, map[string]any{"status": "ok", "recovery_codes": codes})
		})
	}
}
