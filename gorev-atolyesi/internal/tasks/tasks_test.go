package tasks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func sample() Task {
	return Task{Title: "API testlerini yaz", Category: "Go", Priority: "high", Status: "todo"}
}
func TestPersistenceAndConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.Create(context.Background(), sample())
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	list, err := s.List(context.Background())
	if err != nil || len(list) != 1 || list[0].ID != task.ID {
		t.Fatalf("persist: %v %v", list, err)
	}
	updated := task
	updated.Status = "done"
	updated, err = s.Update(context.Background(), task.ID, updated)
	if err != nil || updated.Version != 2 {
		t.Fatalf("update: %v %v", updated, err)
	}
	if _, err = s.Update(context.Background(), task.ID, task); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale update: %v", err)
	}
	if err = s.Delete(context.Background(), task.ID, 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale delete: %v", err)
	}
	if err = s.Delete(context.Background(), task.ID, 2); err != nil {
		t.Fatal(err)
	}
}
func TestHTTPValidationAndCRUD(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h := Handler(s)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, body := range []string{`{}`, `{"title":"ab","category":"Go","priority":"high","status":"todo"}`, `{"title":"geçerli","category":"Go","priority":"high","status":"todo","due_date":"2026-02-30"}`, `{"unknown":1}`, `{} {}`} {
		if w := call("POST", "/api/tasks", body); w.Code != 400 {
			t.Fatalf("invalid %s: %d", body, w.Code)
		}
	}
	b, _ := json.Marshal(sample())
	w := call("POST", "/api/tasks", string(b))
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var task Task
	json.Unmarshal(w.Body.Bytes(), &task)
	task.Title = "Güncellenmiş görev"
	task.Status = "done"
	b, _ = json.Marshal(task)
	w = call("PUT", fmt.Sprintf("/api/tasks/%d", task.ID), string(b))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if w = call("PUT", fmt.Sprintf("/api/tasks/%d", task.ID), string(b)); w.Code != 409 {
		t.Fatalf("conflict: %d", w.Code)
	}
	if w = call("DELETE", fmt.Sprintf("/api/tasks/%d?version=2", task.ID), ""); w.Code != 204 {
		t.Fatalf("delete: %d", w.Code)
	}
	if w = call("DELETE", fmt.Sprintf("/api/tasks/%d?version=2", task.ID), ""); w.Code != 404 {
		t.Fatalf("missing: %d", w.Code)
	}
	if w = call("GET", "/api/tasks", ""); w.Body.String() != "[]\n" {
		t.Fatal(w.Body.String())
	}
	r := httptest.NewRequest("POST", "/api/tasks", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross site: %d", w.Code)
	}
}
func TestConcurrentUpdates(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	task, err := s.Create(context.Background(), sample())
	if err != nil {
		t.Fatal(err)
	}
	outcomes := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := s.Update(context.Background(), task.ID, task); outcomes <- err }()
	}
	wg.Wait()
	close(outcomes)
	ok, conflicts := 0, 0
	for err := range outcomes {
		if err == nil {
			ok++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if ok != 1 || conflicts != 7 {
		t.Fatalf("ok=%d conflicts=%d", ok, conflicts)
	}
}

func TestExportDownload(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h := Handler(s)
	download := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/api/export/tasks", nil))
		return w
	}
	w := download()
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Disposition"), "attachment;") || !strings.Contains(w.Header().Get("Content-Disposition"), ".json") || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("download headers: %d %v", w.Code, w.Header())
	}
	var result taskExport
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Tasks == nil || len(result.Tasks) != 0 || result.Count != 0 || result.ExportedAt.IsZero() || result.Format != "gorev-atolyesi-v1" {
		t.Fatalf("empty export: %+v", result)
	}
	task, err := s.Create(context.Background(), sample())
	if err != nil {
		t.Fatal(err)
	}
	task.Status = "done"
	task, err = s.Update(context.Background(), task.ID, task)
	if err != nil {
		t.Fatal(err)
	}
	w = download()
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Tasks) != 1 || result.Count != 1 || result.Tasks[0].Status != "done" || result.Tasks[0].Version != 2 {
		t.Fatalf("stale export: %+v", result)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("export must not be cached")
	}
	s.Close()
	w = download()
	if w.Code != 500 || w.Header().Get("Content-Disposition") != "" {
		t.Fatalf("failed export should not download: %d %v", w.Code, w.Header())
	}
}
