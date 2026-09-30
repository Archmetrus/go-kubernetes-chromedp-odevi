package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type automationContextKey struct{}
type AutomationKey struct {
	ID      string `json:"id"`
	TeamID  string `json:"team_id"`
	Name    string `json:"name"`
	Expires int64  `json:"expires_at"`
	Revoked bool   `json:"revoked"`
}
type automationIdentity struct {
	ID, UserID, TeamID, Hash string
	Version                  int
}

func principal(ctx context.Context) string {
	if a, ok := ctx.Value(automationContextKey{}).(automationIdentity); ok {
		return "key:" + a.ID
	}
	return "user:" + owner(ctx)
}
func (s *Store) automationAuth(ctx context.Context, raw string) (automationIdentity, error) {
	var a automationIdentity
	a.Hash = digest(raw)
	if !strings.HasPrefix(raw, "atolye_") || len(raw) != 71 {
		return a, ErrForbidden
	}
	err := s.db.QueryRowContext(ctx, s.q("SELECT k.id,k.user_id,k.team_id,k.user_version FROM automation_keys k JOIN users u ON u.id=k.user_id WHERE k.token_hash=? AND k.revoked=false AND k.expires_at>? AND u.status='active' AND u.email_verified=true AND u.mfa_enabled=true AND u.version=k.user_version"), a.Hash, time.Now().Unix()).Scan(&a.ID, &a.UserID, &a.TeamID, &a.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrForbidden
	}
	return a, err
}
func (s *Store) checkAutomation(ctx context.Context, tx *sql.Tx, u User) error {
	a, ok := ctx.Value(automationContextKey{}).(automationIdentity)
	if !ok {
		return nil
	}
	var n int
	err := tx.QueryRowContext(ctx, s.q("SELECT count(*) FROM automation_keys k JOIN users u ON u.id=k.user_id WHERE k.id=? AND k.token_hash=? AND k.user_id=? AND k.team_id=? AND k.revoked=false AND k.expires_at>? AND u.version=k.user_version AND u.mfa_enabled=true"), a.ID, a.Hash, u.ID, a.TeamID, time.Now().Unix()).Scan(&n)
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrForbidden
	}
	return nil
}
func (s *Store) requireKeyReauth(ctx context.Context, tx *sql.Tx, u User, password string) error {
	session, ok := ctx.Value(mfaSessionContextKey{}).(string)
	if !ok || !u.MFAEnabled {
		return errTeamMFA
	}
	var n int
	if err := tx.QueryRowContext(ctx, s.q("SELECT count(*) FROM sessions WHERE token_hash=? AND user_id=? AND expires_at>? AND mfa_verified_at>?"), session, u.ID, time.Now().Unix(), time.Now().Add(-5*time.Minute).Unix()).Scan(&n); err != nil {
		return err
	}
	if n != 1 {
		return errTeamMFA
	}
	var hash string
	if err := tx.QueryRowContext(ctx, s.q("SELECT password_hash FROM users WHERE id=?"), u.ID).Scan(&hash); err != nil {
		return err
	}
	if len(password) > 72 || bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return ErrForbidden
	}
	return nil
}
func (s *Store) CreateAutomationKey(ctx context.Context, team, name, password string, days int) (AutomationKey, string, error) {
	k := AutomationKey{}
	if len(name) < 3 || len(name) > 80 || days < 1 || days > 30 {
		return k, "", ErrValidation
	}
	tx, u, err := s.teamTx(ctx, true)
	if err != nil {
		return k, "", err
	}
	defer tx.Rollback()
	if _, ok := ctx.Value(automationContextKey{}).(automationIdentity); ok {
		return k, "", ErrForbidden
	}
	if err = s.requireKeyReauth(ctx, tx, u, password); err != nil {
		return k, "", err
	}
	g, err := s.teamRights(ctx, tx, u, team)
	if err != nil {
		return k, "", err
	}
	if !g.CanAssign {
		return k, "", ErrForbidden
	}
	var version int
	var active int
	if err = tx.QueryRowContext(ctx, s.q("SELECT count(*) FROM automation_keys WHERE user_id=? AND revoked=false AND expires_at>?"), u.ID, time.Now().Unix()).Scan(&active); err != nil {
		return k, "", err
	}
	if active >= 20 {
		return k, "", fmt.Errorf("%w: en fazla 20 etkin anahtar; eski anahtarları iptal et", ErrValidation)
	}
	if err = tx.QueryRowContext(ctx, s.q("SELECT version FROM users WHERE id=?"), u.ID).Scan(&version); err != nil {
		return k, "", err
	}
	raw := "atolye_" + token()
	k = AutomationKey{ID: token(), TeamID: team, Name: name, Expires: time.Now().Add(time.Duration(days) * 24 * time.Hour).Unix()}
	_, err = tx.ExecContext(ctx, s.q("INSERT INTO automation_keys(id,token_hash,user_id,user_version,team_id,name,expires_at) VALUES(?,?,?,?,?,?,?)"), k.ID, digest(raw), u.ID, version, team, name, k.Expires)
	if err != nil {
		return k, "", err
	}
	if err = s.identityAudit(ctx, tx, u.ID, k.ID, "automation.key.created:"+team); err != nil {
		return k, "", err
	}
	return k, raw, tx.Commit()
}
func (s *Store) RevokeAutomationKey(ctx context.Context, id string) error {
	tx, u, err := s.teamTx(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	r, err := tx.ExecContext(ctx, s.q("UPDATE automation_keys SET revoked=true WHERE id=? AND user_id=?"), id, u.ID)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return ErrForbidden
	}
	if err = s.identityAudit(ctx, tx, u.ID, id, "automation.key.revoked"); err != nil {
		return err
	}
	return tx.Commit()
}

type AssignmentPreviewRow struct {
	Row int `json:"row"`
	AssignmentRow
	Error string `json:"error,omitempty"`
}
type AssignmentPreview struct {
	ID      string                 `json:"id"`
	Ready   bool                   `json:"ready"`
	Expires int64                  `json:"expires_at"`
	Rows    []AssignmentPreviewRow `json:"rows"`
}
type AssignmentResult struct {
	ID        string          `json:"id"`
	CreatedAt string          `json:"created_at"`
	TaskIDs   []string        `json:"task_ids"`
	Rows      []AssignmentRow `json:"rows"`
}

func (s *Store) validateAssignments(ctx context.Context, tx *sql.Tx, u User, rows []AssignmentRow) (AssignmentPreview, []TeamTask, error) {
	out := AssignmentPreview{Ready: true, Rows: []AssignmentPreviewRow{}}
	tasks := []TeamTask{}
	if len(rows) < 1 || len(rows) > maxAssignmentRows {
		return out, nil, ErrValidation
	}
	for i, a := range rows {
		a.TeamID = strings.TrimSpace(a.TeamID)
		a.AssigneeEmail = strings.ToLower(strings.TrimSpace(a.AssigneeEmail))
		if a.Status == "" {
			a.Status = "todo"
		}
		if a.Category == "" {
			a.Category = "Genel"
		}
		if a.Priority == "" {
			a.Priority = "medium"
		}
		t := Task{Title: a.Title, Description: a.Description, Category: a.Category, Priority: a.Priority, Status: a.Status, DueDate: a.DueDate}
		rowErr := ""
		if err := normalize(&t); err != nil {
			rowErr = err.Error()
		}
		a.Title = t.Title
		a.Description = t.Description
		if identity, ok := ctx.Value(automationContextKey{}).(automationIdentity); ok && identity.TeamID != a.TeamID {
			rowErr = "Anahtar bu grubu kapsamıyor."
		}
		g, err := s.teamRights(ctx, tx, u, a.TeamID)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) && !errors.Is(err, ErrForbidden) {
				return out, nil, err
			}
			rowErr = "Bu gruba atama yetkin yok."
		} else if !g.CanAssign {
			rowErr = "Bu gruba atama yetkin yok."
		}
		assignee := ""
		if rowErr == "" {
			err = tx.QueryRowContext(ctx, s.q("SELECT u.id FROM users u JOIN team_members m ON m.user_id=u.id WHERE m.team_id=? AND u.email=? AND u.status='active' AND u.email_verified=true"), a.TeamID, a.AssigneeEmail).Scan(&assignee)
			if errors.Is(err, sql.ErrNoRows) {
				rowErr = "Aktif ve doğrulanmış sorumlu bu grupta bulunamadı."
			} else if err != nil {
				return out, nil, err
			}
		}
		if rowErr != "" {
			out.Ready = false
		}
		out.Rows = append(out.Rows, AssignmentPreviewRow{Row: i + 1, AssignmentRow: a, Error: rowErr})
		tasks = append(tasks, TeamTask{TeamID: a.TeamID, AssigneeID: assignee, Title: a.Title, Description: a.Description, Category: a.Category, Priority: a.Priority, Status: a.Status, DueDate: a.DueDate})
	}
	return out, tasks, nil
}
func (s *Store) PreviewAssignments(ctx context.Context, rows []AssignmentRow) (AssignmentPreview, error) {
	tx, u, err := s.teamTx(ctx, true)
	if err != nil {
		return AssignmentPreview{}, err
	}
	defer tx.Rollback()
	out, _, err := s.validateAssignments(ctx, tx, u, rows)
	if err != nil {
		return out, err
	}
	if !out.Ready {
		return out, nil
	}
	normalized := []AssignmentRow{}
	for _, r := range out.Rows {
		normalized = append(normalized, r.AssignmentRow)
	}
	b, err := json.Marshal(normalized)
	if err != nil {
		return out, err
	}
	out.ID = token()
	out.Expires = time.Now().Add(15 * time.Minute).Unix()
	_, err = tx.ExecContext(ctx, s.q("INSERT INTO assignment_drafts(id,principal,user_id,payload,expires_at) VALUES(?,?,?,?,?)"), out.ID, principal(ctx), u.ID, string(b), out.Expires)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}

var requestKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,100}$`)

func (s *Store) CommitAssignments(ctx context.Context, id, key string) (AssignmentResult, error) {
	out := AssignmentResult{}
	if !requestKeyPattern.MatchString(key) {
		return out, fmt.Errorf("%w: Idempotency-Key 16–100 harf/rakam/_/- içermeli", ErrValidation)
	}
	tx, u, err := s.teamTx(ctx, true)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	var payload, job string
	var expires int64
	err = tx.QueryRowContext(ctx, s.q("SELECT payload,expires_at,job_id FROM assignment_drafts WHERE id=? AND principal=? AND user_id=?"), id, principal(ctx), u.ID).Scan(&payload, &expires, &job)
	if err != nil {
		return out, err
	}
	var rows []AssignmentRow
	if err = json.Unmarshal([]byte(payload), &rows); err != nil {
		return out, err
	}
	preview, list, err := s.validateAssignments(ctx, tx, u, rows)
	if err != nil {
		return out, err
	}
	if !preview.Ready {
		return out, fmt.Errorf("%w: yetki veya sorumlu değişmiş; yeni önizleme oluştur", ErrForbidden)
	}
	var priorDraft, result string
	err = tx.QueryRowContext(ctx, s.q("SELECT draft_id,result FROM assignment_jobs WHERE principal=? AND request_key=?"), principal(ctx), key).Scan(&priorDraft, &result)
	if err == nil {
		if priorDraft != id {
			return out, ErrConflict
		}
		err = json.Unmarshal([]byte(result), &out)
		return out, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	if job != "" {
		return out, fmt.Errorf("%w: bu önizleme zaten işlendi", ErrConflict)
	}
	if expires <= time.Now().Unix() {
		return out, fmt.Errorf("%w: önizleme süresi doldu", ErrValidation)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	out = AssignmentResult{ID: token(), CreatedAt: now, TaskIDs: []string{}, Rows: rows}
	for _, t := range list {
		taskID := token()
		_, err = tx.ExecContext(ctx, s.q("INSERT INTO team_tasks(id,team_id,assignee_id,creator_id,title,description,category,priority,status,due_date,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)"), taskID, t.TeamID, t.AssigneeID, u.ID, t.Title, t.Description, t.Category, t.Priority, t.Status, t.DueDate, now, now)
		if err != nil {
			return out, err
		}
		if err = s.teamEvent(ctx, tx, taskID, "Toplu atama: "+out.ID+" / "+principal(ctx)); err != nil {
			return out, err
		}
		out.TaskIDs = append(out.TaskIDs, taskID)
	}
	b, _ := json.Marshal(out)
	_, err = tx.ExecContext(ctx, s.q("INSERT INTO assignment_jobs(id,principal,user_id,draft_id,request_key,result,created_at) VALUES(?,?,?,?,?,?,?)"), out.ID, principal(ctx), u.ID, id, key, string(b), now)
	if err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, s.q("UPDATE assignment_drafts SET job_id=? WHERE id=?"), out.ID, id); err != nil {
		return out, err
	}
	if err = s.identityAudit(ctx, tx, u.ID, out.ID, "assignment.committed:"+principal(ctx)); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s *Store) AssignmentResults(ctx context.Context) ([]AssignmentResult, error) {
	tx, u, err := s.teamTx(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, s.q("SELECT result FROM assignment_jobs WHERE principal=? AND user_id=? ORDER BY created_at DESC LIMIT 50"), principal(ctx), u.ID)
	if err != nil {
		return nil, err
	}
	raw := []string{}
	for rows.Next() {
		var v string
		if err = rows.Scan(&v); err != nil {
			rows.Close()
			return nil, err
		}
		raw = append(raw, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	out := []AssignmentResult{}
	for _, v := range raw {
		var r AssignmentResult
		if err = json.Unmarshal([]byte(v), &r); err != nil {
			return nil, err
		}
		p, _, e := s.validateAssignments(ctx, tx, u, r.Rows)
		if e != nil {
			return nil, e
		}
		if p.Ready {
			out = append(out, r)
		}
	}
	return out, nil
}
func (s *Store) automationRoutes(mux *http.ServeMux, o Options) {
	guard := func(w http.ResponseWriter, r *http.Request) bool {
		if !o.Auth || !o.Approval {
			http.NotFound(w, r)
			return false
		}
		return true
	}
	mux.HandleFunc("POST /api/automation-keys", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		ok, e := s.allow(r.Context(), "key-create:"+owner(r.Context()), 5, time.Minute)
		if e != nil {
			teamFail(w, e)
			return
		}
		if !ok {
			limited(w)
			return
		}
		var in struct {
			TeamID   string `json:"team_id"`
			Name     string `json:"name"`
			Password string `json:"password"`
			Days     int    `json:"days"`
		}
		if !decode(w, r, &in) {
			return
		}
		k, raw, err := s.CreateAutomationKey(r.Context(), in.TeamID, in.Name, in.Password, in.Days)
		if err != nil {
			teamFail(w, err)
			return
		}
		respond(w, 201, map[string]any{"key": k, "token": raw})
	})
	mux.HandleFunc("GET /api/automation-keys", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		tx, u, err := s.teamTx(r.Context(), false)
		if err != nil {
			teamFail(w, err)
			return
		}
		defer tx.Rollback()
		rows, err := tx.QueryContext(r.Context(), s.q("SELECT id,team_id,name,expires_at,revoked FROM automation_keys WHERE user_id=? ORDER BY expires_at DESC LIMIT 100"), u.ID)
		if err != nil {
			teamFail(w, err)
			return
		}
		defer rows.Close()
		out := []AutomationKey{}
		for rows.Next() {
			var k AutomationKey
			if err = rows.Scan(&k.ID, &k.TeamID, &k.Name, &k.Expires, &k.Revoked); err != nil {
				teamFail(w, err)
				return
			}
			out = append(out, k)
		}
		if err = rows.Err(); err != nil {
			teamFail(w, err)
			return
		}
		respond(w, 200, out)
	})
	mux.HandleFunc("DELETE /api/automation-keys/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		if err := s.RevokeAutomationKey(r.Context(), r.PathValue("id")); err != nil {
			teamFail(w, err)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /api/automation/imports", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		ok, err := s.allow(r.Context(), "import:"+principal(r.Context()), 20, time.Minute)
		if err != nil {
			teamFail(w, err)
			return
		}
		if !ok {
			limited(w)
			return
		}
		media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		r.Body = http.MaxBytesReader(w, r.Body, maxAssignmentFile)
		b, err := io.ReadAll(r.Body)
		if err != nil {
			respond(w, 413, map[string]string{"error": "Dosya en fazla 2 MiB olabilir."})
			return
		}
		rows, err := parseAssignments(b, media)
		if err != nil {
			teamFail(w, err)
			return
		}
		out, err := s.PreviewAssignments(r.Context(), rows)
		if err != nil {
			teamFail(w, err)
			return
		}
		respond(w, 200, out)
	})
	mux.HandleFunc("POST /api/automation/imports/{id}/commit", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		var in struct{}
		if !decode(w, r, &in) {
			return
		}
		out, err := s.CommitAssignments(r.Context(), r.PathValue("id"), r.Header.Get("Idempotency-Key"))
		if err != nil {
			teamFail(w, err)
			return
		}
		respond(w, 200, out)
	})
	mux.HandleFunc("GET /api/automation/results", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		out, err := s.AssignmentResults(r.Context())
		if err != nil {
			teamFail(w, err)
			return
		}
		respond(w, 200, out)
	})
}
