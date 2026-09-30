package tasks

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

//go:embed static/*
var assets embed.FS

func Handler(s *Store) http.Handler {
	return HandlerWithOptions(s, Options{Version: "dev"})
}
func HandlerWithOptions(s *Store, o Options) http.Handler {
	mux := http.NewServeMux()
	s.authRoutes(mux, o)
	s.identityRoutes(mux, o)
	s.verificationRoutes(mux, o)
	s.mfaRoutes(mux, o)
	s.teamRoutes(mux, o)
	s.automationRoutes(mux, o)
	var requests, serverErrors, duration atomic.Int64
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || !equalSecret(o.MetricsToken, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")) {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		fmt.Fprintf(w, "atolye_http_requests_total %d\natolye_http_errors_total %d\natolye_http_duration_seconds_sum %f\n", requests.Load(), serverErrors.Load(), float64(duration.Load())/1e9)
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]string{"status": "ok", "version": o.Version})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Ping(r.Context()); err != nil {
			respond(w, 503, map[string]string{"error": "veritabanı hazır değil"})
			return
		}
		respond(w, 200, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("GET /api/tasks", func(w http.ResponseWriter, r *http.Request) {
		scope := r.URL.Query().Get("scope")
		if scope != "" && scope != "active" && scope != "archive" && scope != "trash" {
			respond(w, 400, map[string]string{"error": "Geçersiz görünüm"})
			return
		}
		list, err := s.ListScope(r.Context(), scope)
		if err != nil {
			fail(w, err)
			return
		}
		respond(w, 200, list)
	})
	mux.HandleFunc("GET /api/export/tasks", func(w http.ResponseWriter, r *http.Request) {
		list, err := s.ListScope(r.Context(), "all")
		if err != nil {
			fail(w, err)
			return
		}
		exported := taskExport{Format: "gorev-atolyesi-v1", ExportedAt: time.Now().UTC(), Count: len(list), Tasks: list}
		// Encode successfully before sending attachment headers.
		data, err := json.MarshalIndent(exported, "", "  ")
		if err != nil {
			fail(w, err)
			return
		}
		data = append(data, '\n')
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
			"filename": "gorev-atolyesi-" + exported.ExportedAt.Format("2006-01-02-150405") + ".json",
		}))
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})
	mux.HandleFunc("POST /api/tasks/{id}/restore", func(w http.ResponseWriter, r *http.Request) {
		id, ok := taskID(w, r)
		if !ok {
			return
		}
		var in struct {
			Version int `json:"version"`
		}
		if !decode(w, r, &in) {
			return
		}
		t, err := s.Restore(r.Context(), id, in.Version)
		if err != nil {
			fail(w, err)
			return
		}
		respond(w, 200, t)
	})
	mux.HandleFunc("POST /api/import/tasks", func(w http.ResponseWriter, r *http.Request) {
		media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if media != "application/json" {
			respond(w, 415, map[string]string{"error": "JSON dosyası gerekli"})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4*1024*1024)
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		var in taskExport
		if err := d.Decode(&in); err != nil || in.Format != "gorev-atolyesi-v1" || in.Count != len(in.Tasks) || len(in.Tasks) > 1000 {
			respond(w, 400, map[string]string{"error": "Geçerli Görev Atölyesi dosyası gerekli; en fazla 1000 görev / 4 MiB."})
			return
		}
		if d.Decode(new(any)) != io.EOF {
			respond(w, 400, map[string]string{"error": "Tek JSON nesnesi gerekli"})
			return
		}
		for i := range in.Tasks {
			if err := normalize(&in.Tasks[i]); err != nil {
				fail(w, err)
				return
			}
			if in.Tasks[i].DeletedAt != "" {
				respond(w, 400, map[string]string{"error": "Silinmiş görev içe aktarılamaz"})
				return
			}
			now := time.Now().UTC().Format(time.RFC3339Nano)
			in.Tasks[i].CreatedAt = now
			in.Tasks[i].UpdatedAt = now
		}
		if r.URL.Query().Get("preview") == "true" {
			respond(w, 200, map[string]any{"count": len(in.Tasks), "tasks": in.Tasks})
			return
		}
		n, err := s.Import(r.Context(), in.Tasks)
		if err != nil {
			fail(w, err)
			return
		}
		respond(w, 201, map[string]int{"imported": n})
	})
	mux.HandleFunc("POST /api/tasks", func(w http.ResponseWriter, r *http.Request) {
		var t Task
		if !decode(w, r, &t) {
			return
		}
		t, err := s.Create(r.Context(), t)
		if err != nil {
			fail(w, err)
			return
		}
		w.Header().Set("Location", fmt.Sprintf("/api/tasks/%d", t.ID))
		respond(w, 201, t)
	})
	mux.HandleFunc("PUT /api/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := taskID(w, r)
		if !ok {
			return
		}
		var t Task
		if !decode(w, r, &t) {
			return
		}
		t, err := s.Update(r.Context(), id, t)
		if err != nil {
			fail(w, err)
			return
		}
		respond(w, 200, t)
	})
	mux.HandleFunc("DELETE /api/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := taskID(w, r)
		if !ok {
			return
		}
		v, err := strconv.Atoi(r.URL.Query().Get("version"))
		if err != nil {
			respond(w, 400, map[string]string{"error": "version gerekli"})
			return
		}
		if err = s.Delete(r.Context(), id, v); err != nil {
			fail(w, err)
			return
		}
		w.WriteHeader(204)
	})
	sub, _ := fs.Sub(assets, "static")
	mux.Handle("GET /", http.FileServer(http.FS(sub)))
	cross := http.NewCrossOriginProtection()
	if o.Origin != "" {
		if err := cross.AddTrustedOrigin(o.Origin); err != nil {
			panic(err)
		}
	}
	protected := cross.Handler(s.protect(mux, o))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		r = r.WithContext(ctx)
		requestID := token()[:16]
		w.Header().Set("X-Request-ID", requestID)
		recorder := &statusWriter{ResponseWriter: w, status: 200}
		defer func() {
			if p := recover(); p != nil {
				slog.Error("panic", "request_id", requestID)
				if !recorder.wrote {
					respond(recorder, 500, map[string]string{"error": "İşlem tamamlanamadı."})
				}
			}
			requests.Add(1)
			if recorder.status >= 500 {
				serverErrors.Add(1)
			}
			duration.Add(time.Since(start).Nanoseconds())
			slog.Info("http", "request_id", requestID, "method", r.Method, "path", r.URL.Path, "status", recorder.status, "duration_ms", time.Since(start).Milliseconds(), "version", o.Version)
		}()
		if o.Production {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		w.Header().Set("Cache-Control", "no-store")
		protected.ServeHTTP(recorder, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (w *statusWriter) WriteHeader(code int) {
	if w.wrote {
		return
	}
	w.status = code
	w.wrote = true
	w.ResponseWriter.WriteHeader(code)
}
func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(b)
}
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, err error) {
	code := 500
	message := "İşlem tamamlanamadı; tekrar deneyin."
	switch {
	case errors.Is(err, ErrValidation):
		code = 400
		message = err.Error()
	case errors.Is(err, sql.ErrNoRows):
		code = 404
		message = "Görev bulunamadı."
	case errors.Is(err, ErrConflict):
		code = 409
		message = err.Error()
	default:
		slog.Error("storage", "error", err)
	}
	respond(w, code, map[string]string{"error": message})
}
func taskID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		respond(w, 400, map[string]string{"error": "Geçersiz görev numarası."})
		return 0, false
	}
	return id, true
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		respond(w, 415, map[string]string{"error": "Content-Type application/json olmalı."})
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err = d.Decode(v); err != nil {
		respond(w, 400, map[string]string{"error": "Geçerli bir JSON görev nesnesi gönderin (en fazla 16 KB)."})
		return false
	}
	if err = d.Decode(new(any)); err != io.EOF {
		respond(w, 400, map[string]string{"error": "Yalnızca bir JSON nesnesi gönderin."})
		return false
	}
	return true
}

// taskExport is the versioned, complete export contract, independent of UI filters.
type taskExport struct {
	Format     string    `json:"format"`
	ExportedAt time.Time `json:"exported_at"`
	Count      int       `json:"count"`
	Tasks      []Task    `json:"tasks"`
}
