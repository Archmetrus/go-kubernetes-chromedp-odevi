package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTeamHierarchyAndAuthorization(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "team.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	accounts, err := s.SeedTeamDemo(ctx, testMFAKey)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.VerifyTeamDemo(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SeedTeamDemo(ctx, testMFAKey); err == nil {
		t.Fatal("seed overwrote populated database")
	}
	admin := WithOwner(ctx, accounts[0].ID)
	leader := WithOwner(ctx, accounts[1].ID)
	assigner := WithOwner(ctx, accounts[2].ID)
	member := WithOwner(ctx, accounts[3].ID)
	other := WithOwner(ctx, accounts[4].ID)
	task := TeamTask{TeamID: "demo-team-1", AssigneeID: accounts[3].ID, Title: "Yetkili atama testi", Description: "Takip", Category: "Go", Priority: "medium", Status: "todo"}
	if _, err = s.SaveTeamTask(member, "", task); !errors.Is(err, ErrForbidden) {
		t.Fatal("member assigned", err)
	}
	if _, err = s.SaveTeamTask(other, "", task); !errors.Is(err, ErrForbidden) {
		t.Fatal("cross-team leader assigned", err)
	}
	bad := task
	bad.AssigneeID = accounts[5].ID
	if _, err = s.SaveTeamTask(admin, "", bad); !errors.Is(err, ErrValidation) {
		t.Fatal("cross-team assignee", err)
	}
	created, err := s.SaveTeamTask(assigner, "", task)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.TeamStatus(member, created.ID, "doing", created.Version); err != nil {
		t.Fatal(err)
	}
	if err = s.TeamStatus(other, created.ID, "done", 2); !errors.Is(err, ErrForbidden) {
		t.Fatal("cross-group status", err)
	}
	changed := created
	changed.Version = 2
	changed.Title = "Üye içeriği değiştiremez"
	if _, err = s.SaveTeamTask(member, created.ID, changed); !errors.Is(err, ErrForbidden) {
		t.Fatal("assignee content edit", err)
	}
	if _, err = s.TeamHistory(other, created.ID); !errors.Is(err, ErrForbidden) {
		t.Fatal("history leak", err)
	}
	if history, e := s.TeamHistory(member, created.ID); e != nil || len(history) != 2 {
		t.Fatal(history, e)
	}
	// Two writers using the same version: exactly one succeeds, one conflicts.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- s.TeamStatus(leader, created.ID, "done", 2) }()
	}
	wg.Wait()
	close(results)
	wins, conflicts := 0, 0
	for e := range results {
		if e == nil {
			wins++
		} else if errors.Is(e, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatal(wins, conflicts)
	}
	// Removing assignment permission immediately hides someone else's created task.
	if err = s.ChangeTeam(leader, "demo-team-1", TeamChange{Action: "member", UserID: accounts[2].ID, Version: 1, CanAssign: false}); err != nil {
		t.Fatal(err)
	}
	out, err := s.TeamSnapshot(assigner)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range out.Tasks {
		if v.ID == created.ID {
			t.Fatal("revoked grant retained visibility")
		}
	}
	if err = s.ChangeTeam(leader, "demo-team-1", TeamChange{Action: "leader", UserID: accounts[3].ID, Version: 2}); !errors.Is(err, ErrValidation) {
		t.Fatal("leader without MFA", err)
	}
	if err = s.ChangeTeam(leader, "demo-team-1", TeamChange{Action: "remove", UserID: accounts[1].ID, Version: 2}); !errors.Is(err, ErrValidation) {
		t.Fatal("removed leader", err)
	}
	if err = s.ChangeTeam(leader, "demo-team-1", TeamChange{Action: "member", UserID: accounts[4].ID, Version: 2}); err != nil {
		t.Fatal(err)
	}
	if err = s.ChangeTeam(leader, "demo-team-1", TeamChange{Action: "leader", UserID: accounts[4].ID, Version: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveTeamTask(leader, "", task); !errors.Is(err, ErrForbidden) {
		t.Fatal("old leader kept powers", err)
	}
	// New leader removes member: outstanding tasks transfer, completed history stays.
	open, err := s.SaveTeamTask(admin, "", task)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ChangeTeam(other, "demo-team-1", TeamChange{Action: "remove", UserID: accounts[3].ID, Version: 4}); err != nil {
		t.Fatal(err)
	}
	var assignee string
	if err = s.db.QueryRow("SELECT assignee_id FROM team_tasks WHERE id=?", open.ID).Scan(&assignee); err != nil || assignee != accounts[4].ID {
		t.Fatal(assignee, err)
	}
	if _, err = s.TeamHistory(member, created.ID); !errors.Is(err, ErrForbidden) {
		t.Fatal("removed member kept history", err)
	}
	if err = s.changeAccount(admin, accounts[7].ID, "suspended", accounts[0].Password, 1); !errors.Is(err, ErrValidation) {
		t.Fatal("suspended a serving leader", err)
	}
	if err = s.changeAccount(admin, accounts[8].ID, "suspended", accounts[0].Password, 1); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow("SELECT assignee_id FROM team_tasks WHERE id='demo-task-3-2'").Scan(&assignee); err != nil || assignee != accounts[7].ID {
		t.Fatal("suspension transfer", assignee, err)
	}
	if err = s.ChangeTeam(member, "", TeamChange{Name: "Yetkisiz grup", UserID: accounts[7].ID}); !errors.Is(err, ErrForbidden) {
		t.Fatal("member created group", err)
	}
	if err = s.ChangeTeam(admin, "", TeamChange{Name: "Yeni doğrulanmış grup", Email: accounts[7].Email}); err != nil {
		t.Fatal("admin create group", err)
	}
	// Real HTTP auth boundary, scoped export, expired MFA and forged ownership.
	h := HandlerWithOptions(s, Options{Auth: true, Approval: true, MFAKey: testMFAKey})
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
	if w := call("GET", "/api/teams", "", nil); w.Code != 401 {
		t.Fatal(w.Code)
	}
	cookieFor := func(id string, verified int64) *http.Cookie {
		raw := token()
		if _, e := s.db.Exec("INSERT INTO sessions(token_hash,user_id,expires_at,mfa_verified_at) VALUES(?,?,?,?)", digest(raw), id, time.Now().Add(time.Hour).Unix(), verified); e != nil {
			t.Fatal(e)
		}
		return &http.Cookie{Name: cookieName(Options{}), Value: raw}
	}
	stale := cookieFor(accounts[4].ID, time.Now().Add(-6*time.Minute).Unix())
	body, _ := json.Marshal(task)
	if w := call("POST", "/api/team-tasks", string(body), stale); w.Code != 403 || !strings.Contains(w.Body.String(), "mfa_required") {
		t.Fatal(w.Code, w.Body.String())
	}
	fresh := cookieFor(accounts[4].ID, time.Now().Unix())
	if w := call("GET", "/api/teams/export", "", fresh); w.Code != 200 || strings.Contains(w.Body.String(), "demo-team-3") {
		t.Fatal("export scope", w.Code)
	}
	if w := call("POST", "/api/teams/demo-team-3/members", `{"action":"member","user_id":"demo-go-deniz","version":1}`, fresh); w.Code != 403 {
		t.Fatal("cross group membership", w.Code)
	}
	if _, err = s.db.Exec("UPDATE users SET status='suspended' WHERE id=?", accounts[4].ID); err != nil {
		t.Fatal(err)
	}
	if w := call("GET", "/api/teams", "", fresh); w.Code != 403 {
		t.Fatal("suspended access", w.Code)
	}
}

func TestTeamSeedPreservesPersonalData(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "personal.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	original, err := s.Create(ctx, sample())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SeedTeamDemo(ctx, testMFAKey); err == nil {
		t.Fatal("nonempty seed")
	}
	list, err := s.List(ctx)
	if err != nil || len(list) != 1 || list[0].ID != original.ID {
		t.Fatal(list, err)
	}
}
