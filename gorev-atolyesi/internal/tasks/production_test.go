package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestUserIsolationAndLifecycle(t *testing.T) {
	backends := []string{"sqlite"}
	if os.Getenv("TEST_DATABASE_URL") != "" {
		backends = append(backends, "postgres")
	}
	for _, backend := range backends {
		t.Run(backend, func(t *testing.T) {
			ctx := context.Background()
			var s *Store
			var err error
			if backend == "postgres" {
				s, err = OpenPostgres(ctx, os.Getenv("TEST_DATABASE_URL"))
			} else {
				s, err = Open(filepath.Join(t.TempDir(), "tasks.db"))
			}
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if err = s.Migrate(ctx); err != nil {
				t.Fatal(err)
			}
			if err = s.Migrate(ctx); err != nil {
				t.Fatal("idempotent migration", err)
			}
			suffix := token()[:12]
			email := "alice-" + suffix + "@example.test"
			password := "test-password-long-enough"
			a, err := s.CreateUser(ctx, email, password)
			if err != nil {
				t.Fatal(err)
			}
			b, err := s.CreateUser(ctx, "bob-"+suffix+"@example.test", password)
			if err != nil {
				t.Fatal(err)
			}
			opts := Options{Auth: true, Signup: true, Version: "test", Origin: "https://atolye.test", Production: true, MetricsToken: "test-metrics"}
			h := HandlerWithOptions(s, opts)
			call := func(handler http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
				r := httptest.NewRequest(method, path, strings.NewReader(body))
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("Origin", opts.Origin)
				if cookie != nil {
					r.AddCookie(cookie)
				}
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				return w
			}
			login := func(email string) *http.Cookie {
				w := call(h, "POST", "/api/login", fmt.Sprintf(`{"email":%q,"password":%q}`, email, password), nil)
				if w.Code != 200 {
					t.Fatal(w.Code, w.Body.String())
				}
				c := w.Result().Cookies()[0]
				if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteStrictMode || c.Name != "__Host-atolye" {
					t.Fatal("insecure cookie", c)
				}
				return c
			}
			ca := login(email)
			cb := login(b.Email)
			for _, path := range []string{"/api/tasks", "/api/export/tasks", "/api/tasks?scope=trash"} {
				if w := call(h, "GET", path, "", nil); w.Code != 401 {
					t.Fatal("anonymous", path, w.Code)
				}
			}
			body, _ := json.Marshal(sample())
			w := call(h, "POST", "/api/tasks", string(body), ca)
			if w.Code != 201 {
				t.Fatal(w.Body.String())
			}
			var task Task
			json.Unmarshal(w.Body.Bytes(), &task)
			if w = call(h, "GET", "/api/tasks", "", cb); w.Body.String() != "[]\n" {
				t.Fatal("owner leak", w.Body.String())
			}
			if w = call(h, "GET", "/api/export/tasks", "", cb); strings.Contains(w.Body.String(), task.Title) {
				t.Fatal("export leak")
			}
			body, _ = json.Marshal(task)
			for _, method := range []string{"PUT", "DELETE"} {
				if w = call(h, method, fmt.Sprintf("/api/tasks/%d?version=1", task.ID), string(body), cb); w.Code != 404 {
					t.Fatal("foreign write", method, w.Code)
				}
			}
			// A second handler models another replica sharing sessions and rate limits.
			var second *Store
			if backend == "postgres" {
				second, err = OpenPostgres(ctx, os.Getenv("TEST_DATABASE_URL"))
			} else {
				second = s
			}
			if err != nil {
				t.Fatal(err)
			}
			if second != s {
				defer second.Close()
			}
			h2 := HandlerWithOptions(second, opts)
			if w = call(h2, "GET", "/api/tasks", "", ca); !strings.Contains(w.Body.String(), task.Title) {
				t.Fatal("session not shared")
			}
			// Eight clients updating the same revision: exactly one succeeds.
			var wg sync.WaitGroup
			out := make(chan error, 8)
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func() { defer wg.Done(); _, e := s.Update(WithOwner(ctx, a.ID), task.ID, task); out <- e }()
			}
			wg.Wait()
			close(out)
			wins := 0
			for e := range out {
				if e == nil {
					wins++
				} else if e != ErrConflict {
					t.Fatal(e)
				}
			}
			if wins != 1 {
				t.Fatal("concurrency", wins)
			}
			if w = call(h, "DELETE", fmt.Sprintf("/api/tasks/%d?version=2", task.ID), "", ca); w.Code != 204 {
				t.Fatal("delete", w.Code)
			}
			if w = call(h, "GET", "/api/tasks", "", ca); w.Body.String() != "[]\n" {
				t.Fatal("soft delete")
			}
			if w = call(h, "GET", "/api/tasks?scope=trash", "", ca); !strings.Contains(w.Body.String(), task.Title) {
				t.Fatal("trash missing")
			}
			if w = call(h, "POST", fmt.Sprintf("/api/tasks/%d/restore", task.ID), `{"version":3}`, cb); w.Code != 404 {
				t.Fatal("foreign restore")
			}
			if w = call(h, "POST", fmt.Sprintf("/api/tasks/%d/restore", task.ID), `{"version":3}`, ca); w.Code != 200 {
				t.Fatal("restore", w.Body.String())
			}
			json.Unmarshal(w.Body.Bytes(), &task)
			task.Archived = true
			body, _ = json.Marshal(task)
			if w = call(h, "PUT", fmt.Sprintf("/api/tasks/%d", task.ID), string(body), ca); w.Code != 200 {
				t.Fatal("archive", w.Body.String())
			}
			exported := call(h, "GET", "/api/export/tasks", "", ca).Body.String()
			if w = call(h, "POST", "/api/import/tasks?preview=true", exported, cb); w.Code != 200 {
				t.Fatal("preview", w.Body.String())
			}
			if w = call(h, "GET", "/api/tasks?scope=archive", "", cb); w.Body.String() != "[]\n" {
				t.Fatal("preview wrote data")
			}
			if w = call(h, "POST", "/api/import/tasks", exported, cb); w.Code != 201 {
				t.Fatal("import", w.Body.String())
			}
			if w = call(h, "GET", "/api/tasks?scope=archive", "", cb); !strings.Contains(w.Body.String(), task.Title) {
				t.Fatal("import owner")
			}
			r := httptest.NewRequest("POST", "/api/logout", nil)
			r.AddCookie(ca)
			r.Header.Set("Origin", "https://evil.test")
			w = httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 403 {
				t.Fatal("CSRF", w.Code)
			}
			if w = call(h, "POST", "/api/logout", "", ca); w.Code != 204 {
				t.Fatal("logout")
			}
			if w = call(h2, "GET", "/api/tasks", "", ca); w.Code != 401 {
				t.Fatal("logout not shared")
			}
			if _, err = s.db.ExecContext(ctx, s.q("UPDATE sessions SET expires_at=0 WHERE user_id=?"), b.ID); err != nil {
				t.Fatal(err)
			}
			if w = call(h, "GET", "/api/tasks", "", cb); w.Code != 401 {
				t.Fatal("expiry")
			}
			for i := 0; i < 3; i++ {
				ok, err := s.allow(ctx, "test:"+suffix, 2, time.Minute)
				if err != nil || ok != (i < 2) {
					t.Fatal("rate limit", i, ok, err)
				}
			}
			if w = call(h, "GET", "/metrics", "", nil); w.Code != 401 {
				t.Fatal("metrics public")
			}
			if backend == "postgres" {
				legacyPath := filepath.Join(t.TempDir(), "transfer.db")
				legacy, e := Open(legacyPath)
				if e != nil {
					t.Fatal(e)
				}
				original, e := legacy.Create(ctx, sample())
				if e != nil {
					t.Fatal(e)
				}
				defer legacy.Close()
				u, e := s.CreateUser(ctx, "transfer-"+suffix+"@example.test", password)
				if e != nil {
					t.Fatal(e)
				}
				if n, e := s.TransferSQLite(ctx, legacyPath, u.Email); e != nil || n != 1 {
					t.Fatal("transfer", n, e)
				}
				copied, e := s.List(WithOwner(ctx, u.ID))
				if e != nil || len(copied) != 1 || copied[0].Title != original.Title {
					t.Fatal("transfer data", copied, e)
				}
				unchanged, e := legacy.List(ctx)
				if e != nil || len(unchanged) != 1 || unchanged[0].Version != original.Version {
					t.Fatal("source changed")
				}
				if _, e = s.TransferSQLite(ctx, legacyPath, u.Email); e == nil {
					t.Fatal("duplicate transfer accepted")
				}
			}
		})
	}
}

func TestLegacyMigrationAndExternalCommit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(schema); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec("INSERT INTO tasks(title,category,priority,status,created_at,updated_at) VALUES('Eski görev','Go','high','todo','','')"); e != nil {
		t.Fatal(e)
	}
	db.Close()
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	ctx := context.Background()
	list, e := s.List(ctx)
	if e != nil || len(list) != 1 {
		t.Fatal("legacy", list, e)
	}
	external, e := sql.Open("sqlite", path)
	if e != nil {
		t.Fatal(e)
	}
	defer external.Close()
	if _, e = external.Exec("UPDATE tasks SET title='DBeaver gibi dış bağlantı',version=version+1"); e != nil {
		t.Fatal(e)
	}
	list, e = s.List(ctx)
	if e != nil || list[0].Title != "DBeaver gibi dış bağlantı" {
		t.Fatal("external commit invisible", e)
	}
	u, e := s.CreateUser(ctx, "owner@example.test", "long-enough-password")
	if e != nil {
		t.Fatal(e)
	}
	if n, e := s.ClaimLocal(ctx, u.Email); e != nil || n != 1 {
		t.Fatal("claim", n, e)
	}
	list, e = s.List(ctx)
	if e != nil || len(list) != 0 {
		t.Fatal("local still owns data")
	}
	list, e = s.List(WithOwner(ctx, u.ID))
	if e != nil || len(list) != 1 {
		t.Fatal("claim missing")
	}
}

func TestProductionConfigAndImportAtomicity(t *testing.T) {
	if (Options{Production: true}).Validate(false) == nil {
		t.Fatal("unsafe production allowed")
	}
	s, e := Open(filepath.Join(t.TempDir(), "tasks.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	bad := sample()
	bad.Title = "x"
	if _, e = s.Import(context.Background(), []Task{sample(), bad}); e == nil {
		t.Fatal("invalid import")
	}
	list, _ := s.List(context.Background())
	if len(list) != 0 {
		t.Fatal("partial import")
	}
}
