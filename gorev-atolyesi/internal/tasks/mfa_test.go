package tasks

import (
	"context"
	"encoding/base32"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var testMFAKey = []byte("0123456789abcdef0123456789abcdef")

func enrollFixture(t *testing.T, s *Store, email string) []string {
	t.Helper()
	ctx := context.Background()
	var id string
	if err := s.db.QueryRow(s.q("SELECT id FROM users WHERE email=?"), email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	session := digest(token())
	if _, err := s.db.Exec(s.q("INSERT INTO sessions(token_hash,user_id,expires_at) VALUES(?,?,?)"), session, id, time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	secret, err := s.BeginMFA(ctx, id, session, "long-test-password", testMFAKey)
	if err != nil {
		t.Fatal(err)
	}
	codes, _, err := s.EnableMFA(ctx, id, session, totp(secret, time.Now().Unix()/30), testMFAKey)
	if err != nil {
		t.Fatal(err)
	}
	return codes
}

func TestTOTPVectorsAndEncryption(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	for _, v := range []struct {
		at   int64
		code string
	}{{59, "287082"}, {1111111109, "081804"}, {1111111111, "050471"}, {1234567890, "005924"}, {2000000000, "279037"}, {20000000000, "353130"}} {
		if got := totp(secret, v.at/30); got != v.code {
			t.Fatal(v, got)
		}
	}
	encrypted, err := sealMFA(testMFAKey, "a", secret)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(encrypted, secret) {
		t.Fatal("plaintext MFA secret")
	}
	if _, err = openMFA(testMFAKey, "b", encrypted); err == nil {
		t.Fatal("cross-user encrypted secret accepted")
	}
	if _, err = openMFA([]byte("fedcba9876543210fedcba9876543210"), "a", encrypted); err == nil {
		t.Fatal("wrong key accepted")
	}
	if _, ok := matchTOTP(secret, "287082", 1, 59); ok {
		t.Fatal("reused counter accepted")
	}
}

func TestMFAKeyPersistence(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	path := filepath.Join(dir, "key")
	ctx := context.Background()
	key, err := s.LoadMFAKey(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.LoadMFAKey(ctx, path)
	if err != nil || string(key) != string(again) {
		t.Fatal(err)
	}
	os.Chmod(path, 0644)
	if _, err = s.LoadMFAKey(ctx, path); err == nil {
		t.Fatal("world readable key")
	}
	os.Chmod(path, 0600)
	os.Remove(path)
	if _, err = s.LoadMFAKey(ctx, path); err == nil {
		t.Fatal("missing key replaced")
	}
}

func TestMFARecoveryAndSessionLifecycle(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "mfa.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.EnableApproval(ctx)
	u, err := s.CreateUser(ctx, "mfa@example.test", "long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	verifyFixture(t, s, u.Email)
	if err = s.BootstrapAdmin(ctx, u.Email); err == nil {
		t.Fatal("bootstrap without MFA")
	}
	codes := enrollFixture(t, s, u.Email)
	if len(codes) != 10 {
		t.Fatal("recovery count")
	}
	if err = s.BootstrapAdmin(ctx, u.Email); err != nil {
		t.Fatal(err)
	}
	h := HandlerWithOptions(s, Options{Auth: true, Approval: true, MFAKey: testMFAKey})
	login := func() *http.Cookie {
		r := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"email":"mfa@example.test","password":"long-test-password"}`))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		return w.Result().Cookies()[0]
	}
	call := func(cookie *http.Cookie, path string) int {
		r := httptest.NewRequest("GET", path, nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	cookie := login()
	if call(cookie, "/api/tasks") != 403 {
		t.Fatal("password-only access")
	}
	session := digest(cookie.Value)
	var wg sync.WaitGroup
	results := make(chan string, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			raw, e := s.VerifyMFA(ctx, u.ID, session, codes[0], testMFAKey)
			if e == nil {
				results <- raw
			}
		}()
	}
	wg.Wait()
	close(results)
	var raw string
	n := 0
	for value := range results {
		raw = value
		n++
	}
	if n != 1 {
		t.Fatal("recovery race", n)
	}
	if call(cookie, "/api/tasks") != 401 {
		t.Fatal("old partial session retained")
	}
	cookie.Value = raw
	if call(cookie, "/api/admin/users") != 200 {
		t.Fatal("full MFA access")
	}
	if _, err = s.VerifyMFA(ctx, u.ID, digest(raw), codes[0], testMFAKey); err == nil {
		t.Fatal("recovery reuse")
	}
	if _, err = s.db.Exec(s.q("UPDATE sessions SET mfa_verified_at=? WHERE token_hash=?"), time.Now().Add(-6*time.Minute).Unix(), digest(raw)); err != nil {
		t.Fatal(err)
	}
	if call(cookie, "/api/admin/users") != 403 {
		t.Fatal("stale MFA admin access")
	}
	if call(cookie, "/api/tasks") != 200 {
		t.Fatal("ordinary session should remain valid")
	}
	newCodes, newRaw, err := s.RegenerateRecovery(ctx, u.ID, digest(raw), "long-test-password", codes[1], testMFAKey)
	if err != nil || len(newCodes) != 10 {
		t.Fatal(err)
	}
	if _, err = s.VerifyMFA(ctx, u.ID, digest(newRaw), codes[2], testMFAKey); err == nil {
		t.Fatal("old recovery set retained")
	}
	if err = s.ResetMFA(ctx, u.Email); err != nil {
		t.Fatal(err)
	}
	cookie.Value = newRaw
	if call(cookie, "/api/tasks") != 401 {
		t.Fatal("reset retained sessions")
	}
	cookie = login()
	if call(cookie, "/api/tasks") != 403 {
		t.Fatal("admin reset bypassed required enrollment")
	}
}

func TestMFAEnrollmentBindingAndRateLimit(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "enrollment.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.EnableApproval(ctx)
	u, err := s.CreateUser(ctx, "enroll@example.test", "long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	verifyFixture(t, s, u.Email)
	rawA, rawB := token(), token()
	for _, raw := range []string{rawA, rawB} {
		if _, err = s.db.Exec(s.q("INSERT INTO sessions(token_hash,user_id,expires_at) VALUES(?,?,?)"), digest(raw), u.ID, time.Now().Add(time.Hour).Unix()); err != nil {
			t.Fatal(err)
		}
	}
	secret, err := s.BeginMFA(ctx, u.ID, digest(rawA), "long-test-password", testMFAKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.EnableMFA(ctx, u.ID, digest(rawB), totp(secret, time.Now().Unix()/30), testMFAKey); err == nil {
		t.Fatal("enrollment from other session")
	}
	if _, err = s.db.Exec(s.q("UPDATE user_mfa SET pending_until=? WHERE user_id=?"), time.Now().Unix()-1, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.EnableMFA(ctx, u.ID, digest(rawA), totp(secret, time.Now().Unix()/30), testMFAKey); err == nil {
		t.Fatal("expired enrollment")
	}
	secret, err = s.BeginMFA(ctx, u.ID, digest(rawA), "long-test-password", testMFAKey)
	if err != nil {
		t.Fatal(err)
	}
	codes, raw, err := s.EnableMFA(ctx, u.ID, digest(rawA), totp(secret, time.Now().Unix()/30), testMFAKey)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	s.db.QueryRow(s.q("SELECT count(*) FROM sessions WHERE user_id=?"), u.ID).Scan(&count)
	if count != 1 {
		t.Fatal("enrollment retained other sessions")
	}
	var codeHash string
	s.db.QueryRow(s.q("SELECT code_hash FROM mfa_recovery WHERE user_id=? LIMIT 1"), u.ID).Scan(&codeHash)
	for _, code := range codes {
		if strings.Contains(codeHash, strings.ReplaceAll(code, "-", "")) {
			t.Fatal("raw recovery stored")
		}
	}
	h := HandlerWithOptions(s, Options{Auth: true, Approval: true, MFAKey: testMFAKey})
	for i := 0; i < 11; i++ {
		r := httptest.NewRequest("POST", "/api/mfa/verify", strings.NewReader(`{"code":"invalid"}`))
		r.Header.Set("Content-Type", "application/json")
		r.AddCookie(&http.Cookie{Name: "atolye", Value: raw})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		expected := 400
		if i == 10 {
			expected = 429
		}
		if w.Code != expected {
			t.Fatal("MFA rate limit", i, w.Code, w.Body.String())
		}
	}
}
