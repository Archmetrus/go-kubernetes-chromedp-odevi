package tasks

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func verifyFixture(t *testing.T, s *Store, email string) {
	t.Helper()
	ctx := context.Background()
	var id string
	if err := s.db.QueryRow(s.q("SELECT id FROM users WHERE email=?"), email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := s.RequestVerification(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := s.ConfirmVerification(ctx, id, fixtureCode(t, s, email)); err != nil {
		t.Fatal(err)
	}
}
func fixtureCode(t *testing.T, s *Store, email string) string {
	t.Helper()
	messages, err := s.DevMailbox(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range messages {
		if m.Email == email {
			return m.Code
		}
	}
	t.Fatal("verification message missing", email)
	return ""
}

func TestVerificationLifecycle(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "verify.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.EnableApproval(ctx); err != nil {
		t.Fatal(err)
	}
	a, err := s.CreateUser(ctx, "a@example.test", "long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateUser(ctx, "b@example.test", "long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	code := fixtureCode(t, s, a.Email)
	if err = s.BootstrapAdmin(ctx, a.Email); err == nil {
		t.Fatal("unverified bootstrap")
	}
	var hash string
	s.db.QueryRow(s.q("SELECT token_hash FROM email_verifications WHERE user_id=?"), a.ID).Scan(&hash)
	if hash == code || hash != digest(code) {
		t.Fatal("raw verification token in verifier table")
	}
	if err = s.ConfirmVerification(ctx, b.ID, code); !errors.Is(err, errVerification) {
		t.Fatal("cross-user verification", err)
	}
	// Duplicate registration must neither replace the password nor rotate mail.
	if _, err = s.createUser(ctx, a.Email, "different-test-password", true, true); err != nil {
		t.Fatal(err)
	}
	if fixtureCode(t, s, a.Email) != code {
		t.Fatal("duplicate rotated code")
	}
	if err = s.RequestVerification(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	newer := fixtureCode(t, s, a.Email)
	if newer == code {
		t.Fatal("resend did not rotate")
	}
	if err = s.ConfirmVerification(ctx, a.ID, code); !errors.Is(err, errVerification) {
		t.Fatal("old code accepted")
	}
	if _, err = s.db.Exec(s.q("UPDATE email_verifications SET expires_at=? WHERE user_id=?"), time.Now().Unix()-1, a.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.ConfirmVerification(ctx, a.ID, newer); !errors.Is(err, errVerification) {
		t.Fatal("expired accepted")
	}
	if err = s.RequestVerification(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	code = fixtureCode(t, s, a.Email)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); results <- s.ConfirmVerification(ctx, a.ID, code) }()
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatal("one-time code winners", success)
	}
	var status string
	var verified bool
	s.db.QueryRow(s.q("SELECT status,email_verified FROM users WHERE id=?"), a.ID).Scan(&status, &verified)
	if status != "pending" || !verified {
		t.Fatal("verification improperly approved account")
	}
	messages, _ := s.DevMailbox(ctx)
	for _, m := range messages {
		if m.Email == a.Email {
			t.Fatal("consumed mail retained")
		}
	}
	enrollFixture(t, s, a.Email)
	if err = s.BootstrapAdmin(ctx, a.Email); err != nil {
		t.Fatal(err)
	}
	if err = s.changeAccount(WithOwner(ctx, a.ID), b.ID, "active", "long-test-password", 1); err != ErrConflict {
		t.Fatal("unverified approval", err)
	}
	if err = s.changeAccount(WithOwner(ctx, a.ID), b.ID, "suspended", "long-test-password", 1); err != nil {
		t.Fatal(err)
	}
	if err = s.RequestVerification(ctx, b.ID); err == nil {
		t.Fatal("suspended resend")
	}
	if err = s.changeAccount(WithOwner(ctx, a.ID), b.ID, "active", "long-test-password", 2); err != nil {
		t.Fatal(err)
	}
	s.db.QueryRow(s.q("SELECT status FROM users WHERE id=?"), b.ID).Scan(&status)
	if status != "pending" {
		t.Fatal("unverified reinstatement bypass")
	}
	if err = s.RequestVerification(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"email_verifications", "dev_mailbox"} {
		if _, err = s.db.Exec(s.q("UPDATE "+table+" SET expires_at=? WHERE user_id=?"), time.Now().Unix()-1, b.ID); err != nil {
			t.Fatal(err)
		}
	}
	if messages, err = s.DevMailbox(ctx); err != nil || len(messages) != 0 {
		t.Fatal("expired mailbox leak", err)
	}
	if err = s.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"email_verifications", "dev_mailbox"} {
		var n int
		if err = s.db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatal("expired material retained", table, n, err)
		}
	}
}

func TestVerificationHTTP(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "http.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	s.EnableApproval(ctx)
	u, err := s.CreateUser(ctx, "v@example.test", "long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	h := HandlerWithOptions(s, Options{Auth: true, Signup: true, Approval: true, Version: "test"})
	call := func(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := call("POST", "/api/email-verification/request", "{}", nil); w.Code != 401 {
		t.Fatal("anonymous", w.Code)
	}
	login := call("POST", "/api/login", `{"email":"v@example.test","password":"long-test-password"}`, nil)
	if login.Code != 200 {
		t.Fatal(login.Body.String())
	}
	cookie := login.Result().Cookies()[0]
	if w := call("GET", "/api/email-verification/confirm", "", cookie); w.Code == 200 {
		t.Fatal("GET consumed code")
	}
	if w := call("POST", "/api/email-verification/request", "{}", cookie); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call("POST", "/api/email-verification/request", "{}", cookie); w.Code != 429 {
		t.Fatal("resend limit", w.Code)
	}
	code := fixtureCode(t, s, u.Email)
	if w := call("POST", "/api/email-verification/confirm", `{"code":"`+code+`","user_id":"other"}`, cookie); w.Code != 400 {
		t.Fatal("mass assignment")
	}
	if w := call("POST", "/api/email-verification/confirm", `{"code":"`+code+`"}`, cookie); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call("POST", "/api/email-verification/confirm", `{"code":"`+code+`"}`, cookie); w.Code != 400 {
		t.Fatal("replay", w.Code)
	}
	if w := call("GET", "/api/tasks", "", cookie); w.Code != 403 {
		t.Fatal("verified pending access")
	}
	if w := call("GET", "/api/admin/mailbox", "", cookie); w.Code == 200 || strings.Contains(w.Body.String(), code) {
		t.Fatal("mailbox leak")
	}
}
