package tasks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrForbidden = errors.New("Bu işlem için yetkin yok.")
var errTeamMFA = errors.New("Ekip yönetimi için güncel iki adımlı doğrulama gerekli.")

type TeamMember struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Status    string `json:"status"`
	MFA       bool   `json:"mfa_enabled"`
	CanAssign bool   `json:"can_assign"`
}
type Team struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	LeaderID  string       `json:"leader_id"`
	Version   int          `json:"version"`
	Members   []TeamMember `json:"members"`
	CanManage bool         `json:"can_manage"`
	CanAssign bool         `json:"can_assign"`
}
type TeamTask struct {
	ID          string `json:"id"`
	TeamID      string `json:"team_id"`
	AssigneeID  string `json:"assignee_id"`
	CreatorID   string `json:"creator_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Priority    string `json:"priority"`
	Status      string `json:"status"`
	DueDate     string `json:"due_date"`
	Version     int    `json:"version"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	CanEdit     bool   `json:"can_edit"`
	CanStatus   bool   `json:"can_status"`
}
type TeamEvent struct {
	Actor     string `json:"actor"`
	Action    string `json:"action"`
	CreatedAt string `json:"created_at"`
}
type TeamSnapshot struct {
	UserID   string       `json:"user_id"`
	Admin    bool         `json:"admin"`
	Demo     bool         `json:"demo"`
	Managers []TeamMember `json:"managers"`
	Teams    []Team       `json:"teams"`
	Tasks    []TeamTask   `json:"tasks"`
}

// Every team mutation shares the identity-policy lock with account suspension.
// Authorization, membership changes, assignment and audit commit together.
func (s *Store) teamTx(ctx context.Context, write bool) (*sql.Tx, User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, User{}, err
	}
	fail := func(e error) (*sql.Tx, User, error) { tx.Rollback(); return nil, User{}, e }
	if write {
		if _, err = tx.ExecContext(ctx, "UPDATE identity_settings SET revision=revision+1 WHERE id=1"); err != nil {
			return fail(err)
		}
	}
	var u User
	err = tx.QueryRowContext(ctx, s.q("SELECT id,email,role,status,email_verified,mfa_enabled FROM users WHERE id=? AND status='active' AND email_verified=true"), owner(ctx)).Scan(&u.ID, &u.Email, &u.Role, &u.Status, &u.EmailVerified, &u.MFAEnabled)
	if errors.Is(err, sql.ErrNoRows) {
		return fail(ErrForbidden)
	}
	if err != nil {
		return fail(err)
	}
	var leaders int
	if err = s.checkAutomation(ctx, tx, u); err != nil {
		return fail(err)
	}
	if err = tx.QueryRowContext(ctx, s.q("SELECT count(*) FROM teams WHERE leader_id=?"), u.ID).Scan(&leaders); err != nil {
		return fail(err)
	}
	elevated := u.Role == "superadmin" || leaders > 0
	if elevated && !u.MFAEnabled {
		return fail(errTeamMFA)
	}
	if session, ok := ctx.Value(mfaSessionContextKey{}).(string); ok {
		var verified int64
		err = tx.QueryRowContext(ctx, s.q("SELECT mfa_verified_at FROM sessions WHERE token_hash=? AND user_id=? AND expires_at>?"), session, u.ID, time.Now().Unix()).Scan(&verified)
		if errors.Is(err, sql.ErrNoRows) {
			return fail(ErrForbidden)
		}
		if err != nil {
			return fail(err)
		}
		if u.MFAEnabled && verified == 0 {
			return fail(errTeamMFA)
		}
		if elevated && write && verified < time.Now().Add(-5*time.Minute).Unix() {
			return fail(errTeamMFA)
		}
	}
	return tx, u, nil
}
func (s *Store) teamRights(ctx context.Context, tx *sql.Tx, u User, id string) (Team, error) {
	var g Team
	err := tx.QueryRowContext(ctx, s.q("SELECT id,name,leader_id,version FROM teams WHERE id=?"), id).Scan(&g.ID, &g.Name, &g.LeaderID, &g.Version)
	if err != nil {
		return g, err
	}
	g.CanManage = u.Role == "superadmin" || g.LeaderID == u.ID
	var assign bool
	err = tx.QueryRowContext(ctx, s.q("SELECT can_assign FROM team_members WHERE team_id=? AND user_id=?"), id, u.ID).Scan(&assign)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return g, err
	}
	if !g.CanManage && errors.Is(err, sql.ErrNoRows) {
		return g, ErrForbidden
	}
	g.CanAssign = g.CanManage || assign
	return g, nil
}
func (s *Store) teamEligible(ctx context.Context, tx *sql.Tx, id string, mfa bool) error {
	var n int
	err := tx.QueryRowContext(ctx, s.q("SELECT count(*) FROM users WHERE id=? AND status='active' AND email_verified=true AND (?=false OR mfa_enabled=true)"), id, mfa).Scan(&n)
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("%w: aktif, doğrulanmış hesap gerekli; liderde MFA da kurulmalı", ErrValidation)
	}
	return nil
}
func (s *Store) TeamSnapshot(ctx context.Context) (TeamSnapshot, error) {
	out := TeamSnapshot{Teams: []Team{}, Tasks: []TeamTask{}, Managers: []TeamMember{}}
	tx, u, err := s.teamTx(ctx, false)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	out.UserID = u.ID
	out.Admin = u.Role == "superadmin"
	var demo int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM demo_fixture").Scan(&demo); err != nil {
		return out, err
	}
	out.Demo = demo > 0
	rows, err := tx.QueryContext(ctx, "SELECT id,email,status,mfa_enabled FROM users WHERE role='superadmin' AND status='active' ORDER BY email")
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var m TeamMember
		if err = rows.Scan(&m.ID, &m.Email, &m.Status, &m.MFA); err != nil {
			rows.Close()
			return out, err
		}
		out.Managers = append(out.Managers, m)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	rows, err = tx.QueryContext(ctx, s.q("SELECT g.id,g.name,g.leader_id,g.version FROM teams g WHERE ?=true OR EXISTS(SELECT 1 FROM team_members m WHERE m.team_id=g.id AND m.user_id=?) ORDER BY g.name"), out.Admin, u.ID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var g Team
		g.Members = []TeamMember{}
		if err = rows.Scan(&g.ID, &g.Name, &g.LeaderID, &g.Version); err != nil {
			rows.Close()
			return out, err
		}
		out.Teams = append(out.Teams, g)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	for i := range out.Teams {
		g := &out.Teams[i]
		g.CanManage = out.Admin || g.LeaderID == u.ID
		g.CanAssign = g.CanManage
		rows, err = tx.QueryContext(ctx, s.q("SELECT u.id,u.email,u.status,u.mfa_enabled,m.can_assign FROM team_members m JOIN users u ON u.id=m.user_id WHERE m.team_id=? ORDER BY u.email"), g.ID)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			var m TeamMember
			if err = rows.Scan(&m.ID, &m.Email, &m.Status, &m.MFA, &m.CanAssign); err != nil {
				rows.Close()
				return out, err
			}
			g.Members = append(g.Members, m)
			if m.ID == u.ID && m.CanAssign {
				g.CanAssign = true
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
		rows, err = tx.QueryContext(ctx, s.q("SELECT "+teamColumns+" FROM team_tasks WHERE team_id=? AND (?=true OR assignee_id=? OR (creator_id=? AND ?=true)) ORDER BY updated_at DESC,id"), g.ID, g.CanManage, u.ID, u.ID, g.CanAssign)
		if err != nil {
			return out, err
		}
		for rows.Next() {
			t, e := scanTeamTask(rows)
			if e != nil {
				rows.Close()
				return out, e
			}
			t.CanEdit = g.CanManage || (g.CanAssign && t.CreatorID == u.ID)
			t.CanStatus = t.CanEdit || t.AssigneeID == u.ID
			out.Tasks = append(out.Tasks, t)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
	}
	return out, tx.Commit()
}

const teamColumns = "id,team_id,assignee_id,creator_id,title,description,category,priority,status,due_date,version,created_at,updated_at"

func scanTeamTask(row interface{ Scan(...any) error }) (TeamTask, error) {
	var t TeamTask
	err := row.Scan(&t.ID, &t.TeamID, &t.AssigneeID, &t.CreatorID, &t.Title, &t.Description, &t.Category, &t.Priority, &t.Status, &t.DueDate, &t.Version, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}
func (s *Store) teamEvent(ctx context.Context, tx *sql.Tx, id, action string) error {
	_, err := tx.ExecContext(ctx, s.q("INSERT INTO team_events(id,task_id,actor_id,action,created_at) VALUES(?,?,?,?,?)"), token(), id, owner(ctx), action, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
func (s *Store) SaveTeamTask(ctx context.Context, id string, t TeamTask) (TeamTask, error) {
	v := Task{Title: t.Title, Description: t.Description, Category: t.Category, Priority: t.Priority, Status: t.Status, DueDate: t.DueDate}
	if err := normalize(&v); err != nil {
		return t, err
	}
	t.Title = v.Title
	t.Description = v.Description
	tx, u, err := s.teamTx(ctx, true)
	if err != nil {
		return t, err
	}
	defer tx.Rollback()
	g, err := s.teamRights(ctx, tx, u, t.TeamID)
	if err != nil {
		return t, err
	}
	if !g.CanAssign {
		return t, ErrForbidden
	}
	if err = s.teamEligible(ctx, tx, t.AssigneeID, false); err != nil {
		return t, err
	}
	var n int
	if err = tx.QueryRowContext(ctx, s.q("SELECT count(*) FROM team_members WHERE team_id=? AND user_id=?"), g.ID, t.AssigneeID).Scan(&n); err != nil {
		return t, err
	}
	if n != 1 {
		return t, fmt.Errorf("%w: sorumlu bu grubun üyesi olmalı", ErrValidation)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	action := "Görev atandı: " + t.AssigneeID
	if id == "" {
		t, err = scanTeamTask(tx.QueryRowContext(ctx, s.q("INSERT INTO team_tasks(id,team_id,assignee_id,creator_id,title,description,category,priority,status,due_date,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?) RETURNING "+teamColumns), token(), g.ID, t.AssigneeID, u.ID, t.Title, t.Description, t.Category, t.Priority, t.Status, t.DueDate, now, now))
	} else {
		old, e := scanTeamTask(tx.QueryRowContext(ctx, s.q("SELECT "+teamColumns+" FROM team_tasks WHERE id=? AND team_id=?"), id, g.ID))
		if e != nil {
			return t, e
		}
		if !g.CanManage && old.CreatorID != u.ID {
			return t, ErrForbidden
		}
		if old.Version != t.Version {
			return t, ErrConflict
		}
		action = "Görev düzenlendi; sorumlu: " + t.AssigneeID + "; durum: " + t.Status
		t, err = scanTeamTask(tx.QueryRowContext(ctx, s.q("UPDATE team_tasks SET assignee_id=?,title=?,description=?,category=?,priority=?,status=?,due_date=?,version=version+1,updated_at=? WHERE id=? AND version=? RETURNING "+teamColumns), t.AssigneeID, t.Title, t.Description, t.Category, t.Priority, t.Status, t.DueDate, now, id, t.Version))
	}
	if err != nil {
		return t, err
	}
	if err = s.teamEvent(ctx, tx, t.ID, action); err != nil {
		return t, err
	}
	return t, tx.Commit()
}
func (s *Store) teamTaskAccess(ctx context.Context, tx *sql.Tx, u User, id string) (TeamTask, error) {
	t, err := scanTeamTask(tx.QueryRowContext(ctx, s.q("SELECT "+teamColumns+" FROM team_tasks WHERE id=?"), id))
	if err != nil {
		return t, err
	}
	g, err := s.teamRights(ctx, tx, u, t.TeamID)
	if err != nil {
		return t, err
	}
	t.CanEdit = g.CanManage || (g.CanAssign && t.CreatorID == u.ID)
	t.CanStatus = t.CanEdit || t.AssigneeID == u.ID
	if !t.CanStatus {
		return t, ErrForbidden
	}
	return t, nil
}
func (s *Store) TeamStatus(ctx context.Context, id, status string, version int) error {
	if status != "todo" && status != "doing" && status != "done" {
		return ErrValidation
	}
	tx, u, err := s.teamTx(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	t, err := s.teamTaskAccess(ctx, tx, u, id)
	if err != nil {
		return err
	}
	if t.Version != version {
		return ErrConflict
	}
	_, err = tx.ExecContext(ctx, s.q("UPDATE team_tasks SET status=?,version=version+1,updated_at=? WHERE id=?"), status, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return err
	}
	if err = s.teamEvent(ctx, tx, id, "Durum: "+t.Status+" → "+status); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) TeamHistory(ctx context.Context, id string) ([]TeamEvent, error) {
	out := []TeamEvent{}
	tx, u, err := s.teamTx(ctx, false)
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if _, err = s.teamTaskAccess(ctx, tx, u, id); err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, s.q("SELECT u.email,e.action,e.created_at FROM team_events e JOIN users u ON u.id=e.actor_id WHERE task_id=? ORDER BY e.created_at DESC LIMIT 100"), id)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var e TeamEvent
		if err = rows.Scan(&e.Actor, &e.Action, &e.CreatedAt); err != nil {
			return out, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Membership edits also handle leader transfer; a group always has one leader.
type TeamChange struct {
	Name      string `json:"name"`
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	Action    string `json:"action"`
	CanAssign bool   `json:"can_assign"`
	Version   int    `json:"version"`
}

func (s *Store) ChangeTeam(ctx context.Context, id string, in TeamChange) error {
	tx, u, err := s.teamTx(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Check group management before looking up any external account.
	if id == "" {
		if u.Role != "superadmin" {
			return ErrForbidden
		}
	} else {
		g, e := s.teamRights(ctx, tx, u, id)
		if e != nil {
			return e
		}
		if !g.CanManage {
			return ErrForbidden
		}
	}
	if in.Email != "" {
		if err = tx.QueryRowContext(ctx, s.q("SELECT id FROM users WHERE email=? AND status='active' AND email_verified=true"), strings.ToLower(strings.TrimSpace(in.Email))).Scan(&in.UserID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: uygun hesap bulunamadı", ErrValidation)
			}
			return err
		}
	}
	if id == "" {
		if u.Role != "superadmin" {
			return ErrForbidden
		}
		in.Name = strings.TrimSpace(in.Name)
		if utf8.RuneCountInString(in.Name) < 3 || utf8.RuneCountInString(in.Name) > 70 {
			return ErrValidation
		}
		if err = s.teamEligible(ctx, tx, in.UserID, true); err != nil {
			return err
		}
		id = token()
		_, err = tx.ExecContext(ctx, s.q("INSERT INTO teams(id,name,leader_id) VALUES(?,?,?)"), id, in.Name, in.UserID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, s.q("INSERT INTO team_members(team_id,user_id) VALUES(?,?)"), id, in.UserID)
	} else {
		g, e := s.teamRights(ctx, tx, u, id)
		if e != nil {
			return e
		}
		if !g.CanManage {
			return ErrForbidden
		}
		if g.Version != in.Version {
			return ErrConflict
		}
		switch in.Action {
		case "member":
			if err = s.teamEligible(ctx, tx, in.UserID, false); err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, s.q("INSERT INTO team_members(team_id,user_id,can_assign) VALUES(?,?,?) ON CONFLICT(team_id,user_id) DO UPDATE SET can_assign=excluded.can_assign"), id, in.UserID, in.CanAssign)
		case "leader":
			if err = s.teamEligible(ctx, tx, in.UserID, true); err != nil {
				return err
			}
			var n int
			if err = tx.QueryRowContext(ctx, s.q("SELECT count(*) FROM team_members WHERE team_id=? AND user_id=?"), id, in.UserID).Scan(&n); err != nil {
				return err
			}
			if n != 1 {
				return ErrValidation
			}
			_, err = tx.ExecContext(ctx, s.q("UPDATE teams SET leader_id=? WHERE id=?"), in.UserID, id)
		case "remove":
			if in.UserID == g.LeaderID {
				return fmt.Errorf("%w: önce liderliği devret", ErrValidation)
			}
			if err = s.transferTeamTasks(ctx, tx, id, in.UserID, g.LeaderID); err != nil {
				return err
			}
			_, err = tx.ExecContext(ctx, s.q("DELETE FROM team_members WHERE team_id=? AND user_id=?"), id, in.UserID)
		default:
			return ErrValidation
		}
		if err == nil {
			if in.Action == "remove" || (in.Action == "member" && !in.CanAssign) || in.Action == "leader" {
				target := in.UserID
				if in.Action == "leader" {
					target = g.LeaderID
				}
				if _, err = tx.ExecContext(ctx, s.q("UPDATE automation_keys SET revoked=true WHERE team_id=? AND user_id=?"), id, target); err != nil {
					return err
				}
			}
			_, err = tx.ExecContext(ctx, s.q("UPDATE teams SET version=version+1 WHERE id=?"), id)
		}
	}
	if err != nil {
		return err
	}
	if err = s.identityAudit(ctx, tx, u.ID, id, "team."+in.Action+":"+in.UserID); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) transferTeamTasks(ctx context.Context, tx *sql.Tx, group, user, leader string) error {
	rows, err := tx.QueryContext(ctx, s.q("SELECT id FROM team_tasks WHERE team_id=? AND assignee_id=? AND status<>'done'"), group, user)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err = tx.ExecContext(ctx, s.q("UPDATE team_tasks SET assignee_id=?,version=version+1,updated_at=? WHERE id=?"), leader, time.Now().UTC().Format(time.RFC3339Nano), id); err != nil {
			return err
		}
		if err = s.teamEvent(ctx, tx, id, "Üyelik/hesap kapandı; açık görev lidere aktarıldı: "+leader); err != nil {
			return err
		}
	}
	return nil
}
func teamFail(w http.ResponseWriter, err error) {
	if errors.Is(err, errTeamMFA) {
		respond(w, 403, map[string]string{"error": err.Error(), "code": "mfa_required"})
		return
	}
	if errors.Is(err, ErrForbidden) {
		respond(w, 403, map[string]string{"error": err.Error()})
		return
	}
	fail(w, err)
}
func (s *Store) teamRoutes(mux *http.ServeMux, o Options) {
	guard := func(w http.ResponseWriter, r *http.Request) bool {
		if !o.Auth || !o.Approval {
			http.NotFound(w, r)
			return false
		}
		return true
	}
	mux.HandleFunc("GET /api/teams", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		out, err := s.TeamSnapshot(r.Context())
		if err != nil {
			teamFail(w, err)
			return
		}
		respond(w, 200, out)
	})
	mux.HandleFunc("GET /api/teams/export", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		out, err := s.TeamSnapshot(r.Context())
		if err != nil {
			teamFail(w, err)
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="ekip-gorevleri.json"`)
		respond(w, 200, map[string]any{"format": "atolye-teams-v1", "tasks": out.Tasks})
	})
	mux.HandleFunc("POST /api/teams", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		var in TeamChange
		if !decode(w, r, &in) {
			return
		}
		if err := s.ChangeTeam(r.Context(), "", in); err != nil {
			teamFail(w, err)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /api/teams/{id}/members", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		var in TeamChange
		if !decode(w, r, &in) {
			return
		}
		if err := s.ChangeTeam(r.Context(), r.PathValue("id"), in); err != nil {
			teamFail(w, err)
			return
		}
		w.WriteHeader(204)
	})
	for _, pattern := range []string{"POST /api/team-tasks", "PUT /api/team-tasks/{id}"} {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if !guard(w, r) {
				return
			}
			var in TeamTask
			if !decode(w, r, &in) {
				return
			}
			out, err := s.SaveTeamTask(r.Context(), r.PathValue("id"), in)
			if err != nil {
				teamFail(w, err)
				return
			}
			respond(w, 200, out)
		})
	}
	mux.HandleFunc("POST /api/team-tasks/{id}/status", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		var in struct {
			Status  string `json:"status"`
			Version int    `json:"version"`
		}
		if !decode(w, r, &in) {
			return
		}
		if err := s.TeamStatus(r.Context(), r.PathValue("id"), in.Status, in.Version); err != nil {
			teamFail(w, err)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("GET /api/team-tasks/{id}/history", func(w http.ResponseWriter, r *http.Request) {
		if !guard(w, r) {
			return
		}
		out, err := s.TeamHistory(r.Context(), r.PathValue("id"))
		if err != nil {
			teamFail(w, err)
			return
		}
		respond(w, 200, out)
	})
}
