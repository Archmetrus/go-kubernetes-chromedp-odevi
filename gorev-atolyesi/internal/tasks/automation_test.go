package tasks

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func xlsxFixture(t *testing.T, sheet, shared string, extras map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	parts := map[string]string{"xl/worksheets/sheet1.xml": sheet}
	if shared != "" {
		parts["xl/sharedStrings.xml"] = shared
	}
	for k, v := range extras {
		parts[k] = v
	}
	for k, v := range parts {
		w, e := z.Create(k)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write([]byte(v)); e != nil {
			t.Fatal(e)
		}
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestAssignmentFiles(t *testing.T) {
	const head = `<row><c r="A1" t="inlineStr"><is><t>team_id</t></is></c><c r="B1" t="inlineStr"><is><t>assignee_email</t></is></c><c r="C1" t="inlineStr"><is><t>title</t></is></c></row>`
	const row = `<row><c r="A2" t="inlineStr"><is><t>demo-team-1</t></is></c><c r="B2" t="inlineStr"><is><t>go-ece@atolye.example.test</t></is></c><c r="C2" t="s"><v>0</v></c></row>`
	const shared = `<sst><si><r><t>API </t></r><r><t>kontrolü</t></r></si></sst>`
	sheet := "<worksheet><sheetData>" + head + row + "</sheetData></worksheet>"
	data := xlsxFixture(t, sheet, shared, nil)
	if _, err := parseAssignmentXLSX(xlsxFixture(t, sheet, shared, map[string]string{"xl/worksheets/_rels/sheet1.xml.rels": `<Relationships><Relationship TargetMode="External" Target="https://example.com"/></Relationships>`})); err == nil {
		t.Fatal("external relationship accepted")
	}
	rows, e := parseAssignments(data, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	if e != nil || len(rows) != 1 || rows[0].Title != "API kontrolü" {
		t.Fatal(rows, e)
	}
	for _, bad := range [][]byte{xlsxFixture(t, strings.Replace(sheet, "<v>0</v>", "<f>1+1</f><v>0</v>", 1), shared, nil), xlsxFixture(t, sheet, shared, map[string]string{"xl/vbaProject.bin": "bad"}), xlsxFixture(t, sheet, shared, map[string]string{"xl/worksheets/sheet2.xml": "<worksheet/>"}), xlsxFixture(t, sheet, shared, map[string]string{"../outside": "bad"}), xlsxFixture(t, strings.Replace(sheet, "<v>0</v>", "<v>9</v>", 1), shared, nil)} {
		if _, e = parseAssignments(bad, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"); e == nil {
			t.Fatal("unsafe XLSX accepted")
		}
	}
	for _, bad := range []string{`{"tasks":[]}`, `{"tasks":[{"title":"abc","role":"superadmin"}]}`, `{"tasks":[]} {}`, `[]`} {
		if _, e = parseAssignments([]byte(bad), "application/json"); e == nil {
			t.Fatal("bad JSON accepted", bad)
		}
	}
	if _, e = parseAssignments(make([]byte, maxAssignmentFile+1), "application/json"); e == nil {
		t.Fatal("oversized input")
	}
}

func TestAutomationScopesAndAtomicImport(t *testing.T) {
	s, e := Open(filepath.Join(t.TempDir(), "bulk.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	ctx := context.Background()
	accounts, e := s.SeedTeamDemo(ctx, testMFAKey)
	if e != nil {
		t.Fatal(e)
	}
	user := accounts[1]
	session := token()
	if _, e = s.db.Exec("INSERT INTO sessions(token_hash,user_id,expires_at,mfa_verified_at) VALUES(?,?,?,?)", digest(session), user.ID, time.Now().Add(time.Hour).Unix(), time.Now().Unix()); e != nil {
		t.Fatal(e)
	}
	cookieCtx := context.WithValue(WithOwner(ctx, user.ID), mfaSessionContextKey{}, digest(session))
	key, raw, e := s.CreateAutomationKey(cookieCtx, "demo-team-1", "Robomotion", user.Password, 7)
	if e != nil {
		t.Fatal(e)
	}
	var stored string
	if e = s.db.QueryRow("SELECT token_hash FROM automation_keys WHERE id=?", key.ID).Scan(&stored); e != nil || stored == raw || stored != digest(raw) {
		t.Fatal("key storage", e)
	}
	a, e := s.automationAuth(ctx, raw)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.db.Exec("UPDATE automation_keys SET expires_at=0 WHERE id=?", key.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.automationAuth(ctx, raw); !errors.Is(e, ErrForbidden) {
		t.Fatal("expired key accepted", e)
	}
	if _, e = s.db.Exec("UPDATE automation_keys SET expires_at=? WHERE id=?", key.Expires, key.ID); e != nil {
		t.Fatal(e)
	}
	apiCtx := context.WithValue(WithOwner(ctx, user.ID), automationContextKey{}, a)
	input := []AssignmentRow{{TeamID: "demo-team-1", AssigneeEmail: accounts[3].Email, Title: "Toplu API kontrolü"}}
	count := func() int {
		var n int
		if e := s.db.QueryRow("SELECT count(*) FROM team_tasks").Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	preview, e := s.PreviewAssignments(apiCtx, input)
	if e != nil || !preview.Ready || count() != 12 {
		t.Fatal(preview, e)
	}
	if _, e = s.CommitAssignments(cookieCtx, preview.ID, "request-0000000001"); e == nil {
		t.Fatal("principal crossover")
	}
	bad := append([]AssignmentRow{}, input...)
	bad = append(bad, AssignmentRow{TeamID: "demo-team-2", AssigneeEmail: accounts[5].Email, Title: "Başka grup"})
	invalid, e := s.PreviewAssignments(apiCtx, bad)
	if e != nil || invalid.Ready || invalid.ID != "" || invalid.Rows[1].Error == "" || count() != 12 {
		t.Fatal(invalid, e)
	}
	var wg sync.WaitGroup
	results := make(chan AssignmentResult, 2)
	failures := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := s.CommitAssignments(apiCtx, preview.ID, "request-0000000001")
			results <- r
			failures <- e
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for e := range failures {
		if e != nil {
			t.Fatal(e)
		}
	}
	jobID := ""
	for r := range results {
		if jobID != "" && jobID != r.ID {
			t.Fatal("duplicate job")
		}
		jobID = r.ID
	}
	if count() != 13 {
		t.Fatal("duplicate tasks", count())
	}
	if _, e = s.CommitAssignments(apiCtx, preview.ID, "request-0000000002"); !errors.Is(e, ErrConflict) {
		t.Fatal("same draft different key", e)
	}
	next, e := s.PreviewAssignments(apiCtx, input)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.CommitAssignments(apiCtx, next.ID, "request-0000000001"); !errors.Is(e, ErrConflict) {
		t.Fatal("same key different draft", e)
	}
	if _, e = s.db.Exec("CREATE TRIGGER fail_bulk_audit BEFORE INSERT ON team_events BEGIN SELECT RAISE(ABORT,'test failure'); END"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.CommitAssignments(apiCtx, next.ID, "request-0000000003"); e == nil || count() != 13 {
		t.Fatal("partial batch committed", e)
	}
	if _, e = s.db.Exec("DROP TRIGGER fail_bulk_audit"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.db.Exec("UPDATE assignment_drafts SET expires_at=0 WHERE id=?", next.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.CommitAssignments(apiCtx, next.ID, "request-0000000004"); !errors.Is(e, ErrValidation) {
		t.Fatal("expired draft", e)
	}
	if r, e := s.AssignmentResults(apiCtx); e != nil || len(r) != 1 {
		t.Fatal(r, e)
	}
	h := HandlerWithOptions(s, Options{Auth: true, Approval: true, MFAKey: testMFAKey})
	call := func(method, path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(`{}`))
		r.Header.Set("Authorization", "Bearer "+raw)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, path := range []string{"/api/admin/users", "/api/teams", "/api/automation-keys", "/api/tasks"} {
		if w := call("GET", path); w.Code != 403 {
			t.Fatal("scope escape", path, w.Code)
		}
	}
	if w := call("GET", "/api/automation/results"); w.Code != 200 || !strings.Contains(w.Body.String(), jobID) {
		t.Fatal(w.Code, w.Body.String())
	}
	changed, e := s.PreviewAssignments(apiCtx, input)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.ChangeTeam(cookieCtx, "demo-team-1", TeamChange{Action: "remove", UserID: accounts[3].ID, Version: 1}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.CommitAssignments(apiCtx, changed.ID, "changed-membership-0001"); !errors.Is(e, ErrForbidden) || count() != 13 {
		t.Fatal("membership not rechecked", e)
	}
	if e = s.RevokeAutomationKey(cookieCtx, key.ID); e != nil {
		t.Fatal(e)
	}
	if w := call("GET", "/api/automation/results"); w.Code != 401 {
		t.Fatal("revoked token", w.Code)
	}
	if _, e = s.CommitAssignments(apiCtx, preview.ID, "request-0000000001"); !errors.Is(e, ErrForbidden) {
		t.Fatal("revocation race", e)
	}
	// A key cannot be minted with just a password or from a stale MFA session.
	if _, _, e = s.CreateAutomationKey(WithOwner(ctx, user.ID), "demo-team-1", "No MFA", user.Password, 1); !errors.Is(e, errTeamMFA) {
		t.Fatal(e)
	}
	if _, e = s.db.Exec("UPDATE sessions SET mfa_verified_at=1 WHERE token_hash=?", digest(session)); e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.CreateAutomationKey(cookieCtx, "demo-team-1", "Old MFA", user.Password, 1); !errors.Is(e, errTeamMFA) {
		t.Fatal(e)
	}
	b, _ := json.Marshal(input)
	if strings.Contains(string(b), raw) {
		t.Fatal("token in payload")
	}
}
