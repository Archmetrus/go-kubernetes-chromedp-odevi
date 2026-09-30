package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestApprovalLifecycle(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "identity.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	// An existing account and its data retain their identity through enablement.
	old, err := s.CreateUser(ctx, "legacy@example.test", "long-test-password")
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.Create(WithOwner(ctx, old.ID), sample())
	if err != nil {
		t.Fatal(err)
	}
	if err = s.EnableApproval(ctx); err != nil {
		t.Fatal(err)
	}
	for _, stale := range []Options{{Version: "test"}, {Auth: true, Signup: true, Version: "test"}} {
		r := httptest.NewRequest("GET", "/api/tasks", nil)
		w := httptest.NewRecorder()
		HandlerWithOptions(s, stale).ServeHTTP(w, r)
		if w.Code != 503 {
			t.Fatal("stale policy bypass", w.Code)
		}
	}
	h := HandlerWithOptions(s, Options{Auth: true, Signup: true, Approval: true, Version: "test", MFAKey: testMFAKey})
	recovery := map[string][]string{}
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
	login := func(email string) *http.Cookie {
		w := call("POST", "/api/login", fmt.Sprintf(`{"email":%q,"password":"long-test-password"}`, email), nil)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		cookie := w.Result().Cookies()[0]
		if codes := recovery[email]; len(codes) > 0 {
			var id string
			s.db.QueryRow(s.q("SELECT id FROM users WHERE email=?"), email).Scan(&id)
			raw, e := s.VerifyMFA(ctx, id, digest(cookie.Value), codes[0], testMFAKey)
			if e != nil {
				t.Fatal(e)
			}
			cookie.Value = raw
			recovery[email] = codes[1:]
		}
		return cookie
	}
	register := func(email string) {
		w := call("POST", "/api/register", fmt.Sprintf(`{"email":%q,"password":"long-test-password"}`, email), nil)
		if w.Code != 202 {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	if w := call("POST", "/api/register", `{"email":"attack@example.test","password":"long-test-password","role":"superadmin"}`, nil); w.Code != 400 {
		t.Fatal("role injection", w.Code)
	}
	register("admin@example.test")
	register("member@example.test")
	verifyFixture(t, s, "admin@example.test")
	recovery["admin@example.test"] = enrollFixture(t, s, "admin@example.test")
	verifyFixture(t, s, "member@example.test")
	verifyFixture(t, s, old.Email)
	first := call("POST", "/api/register", `{"email":"duplicate@example.test","password":"long-test-password"}`, nil)
	again := call("POST", "/api/register", `{"email":"duplicate@example.test","password":"long-test-password"}`, nil)
	if first.Code != again.Code || first.Body.String() != again.Body.String() {
		t.Fatal("registration enumeration")
	}
	pending := login("member@example.test")
	for _, path := range []string{"/api/tasks", "/api/export/tasks", "/api/admin/users"} {
		if w := call("GET", path, "", pending); w.Code != 403 {
			t.Fatal("pending access", path, w.Code)
		}
	}
	adminOld := login("admin@example.test")
	if err = s.BootstrapAdmin(ctx, "admin@example.test"); err != nil {
		t.Fatal(err)
	}
	if w := call("GET", "/api/tasks", "", adminOld); w.Code != 401 {
		t.Fatal("bootstrap did not revoke session")
	}
	if err = s.BootstrapAdmin(ctx, "member@example.test"); err == nil {
		t.Fatal("second bootstrap")
	}
	admin := login("admin@example.test")
	w := call("GET", "/api/admin/users", "", admin)
	if w.Code != 200 || strings.Contains(w.Body.String(), "password_hash") {
		t.Fatal(w.Code, w.Body.String())
	}
	var list struct {
		Users []User `json:"users"`
	}
	json.Unmarshal(w.Body.Bytes(), &list)
	var member User
	for _, u := range list.Users {
		if u.Email == "member@example.test" {
			member = u
		}
	}
	endpoint := "/api/admin/users/" + member.ID + "/status"
	if w = call("POST", endpoint, `{"status":"active","version":1,"password":"wrong"}`, admin); w.Code != 403 {
		t.Fatal("reauth", w.Code)
	}
	if w = call("POST", endpoint, `{"status":"active","version":1,"password":"long-test-password"}`, admin); w.Code != 204 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = call("GET", "/api/tasks", "", pending); w.Code != 401 {
		t.Fatal("approval did not rotate sessions")
	}
	memberCookie := login(member.Email)
	if w = call("GET", "/api/admin/users", "", memberCookie); w.Code != 403 {
		t.Fatal("member admin access")
	}
	if w = call("POST", endpoint, `{"status":"suspended","version":1,"password":"long-test-password"}`, admin); w.Code != 409 {
		t.Fatal("stale version", w.Code)
	}
	if w = call("POST", endpoint, `{"status":"suspended","version":2,"password":"long-test-password"}`, admin); w.Code != 204 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = call("GET", "/api/tasks", "", memberCookie); w.Code != 401 {
		t.Fatal("suspended session")
	}
	if w = call("POST", "/api/login", `{"email":"member@example.test","password":"long-test-password"}`, nil); w.Code != 401 {
		t.Fatal("suspended login")
	}
	legacyCookie := login(old.Email)
	if w = call("GET", "/api/tasks", "", legacyCookie); w.Code != 200 || !strings.Contains(w.Body.String(), fmt.Sprintf(`"id":%d`, task.ID)) {
		t.Fatal("legacy data lost")
	}
	enabled, err := s.ApprovalEnabled(ctx)
	if err != nil || !enabled {
		t.Fatal("approval persistence", err)
	}
	var count int
	s.db.QueryRow("SELECT count(*) FROM identity_audit WHERE action IN ('admin.bootstrap','account.active','account.suspended')").Scan(&count)
	if count != 3 {
		t.Fatal("audit", count)
	}
}

func TestBootstrapConcurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "race.db")
	a, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	ctx := context.Background()
	if err = a.EnableApproval(ctx); err != nil {
		t.Fatal(err)
	}
	for _, email := range []string{"a@example.test", "b@example.test"} {
		if _, err = a.CreateUser(ctx, email, "long-test-password"); err != nil {
			t.Fatal(err)
		}
		verifyFixture(t, a, email)
		enrollFixture(t, a, email)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i, s := range []*Store{a, b} {
		wg.Add(1)
		go func(i int, s *Store) {
			defer wg.Done()
			results <- s.BootstrapAdmin(ctx, []string{"a@example.test", "b@example.test"}[i])
		}(i, s)
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
		t.Fatal("bootstrap winners", success)
	}
}

func TestApprovalDeploymentBoundary(t *testing.T) {
	for _, o := range []Options{{Approval: true}, {Auth: true, Approval: true, Production: true, Origin: "https://example.test", Version: "1", MetricsToken: "abc"}} {
		if o.Validate(true) == nil {
			t.Fatal("unsafe approval configuration accepted")
		}
	}
}
