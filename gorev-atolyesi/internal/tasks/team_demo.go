package tasks

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"time"
)

type DemoAccount struct {
	ID         string `json:"id"`
	Email      string `json:"email"`
	Password   string `json:"password"`
	Role       string `json:"role"`
	TOTPSecret string `json:"totp_secret,omitempty"`
}

// SeedTeamDemo is an operator-only fixture import into an EMPTY database.
// No web endpoint, shared password, production delivery or login bypass exists.
func (s *Store) SeedTeamDemo(ctx context.Context, key []byte) ([]DemoAccount, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "UPDATE identity_settings SET revision=revision+1 WHERE id=1"); err != nil {
		return nil, err
	}
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT (SELECT count(*) FROM users)+(SELECT count(*) FROM tasks)+(SELECT count(*) FROM teams)").Scan(&count); err != nil {
		return nil, err
	}
	if count != 0 {
		return nil, fmt.Errorf("demo yalnızca boş veritabanında oluşturulur; mevcut kayıtlar değiştirilmedi")
	}
	accounts := []DemoAccount{}
	names := []string{"yonetici", "go-lider", "go-deniz", "go-ece", "platform-lider", "platform-ali", "platform-selin", "kalite-lider", "kalite-can", "kalite-derya"}
	for i, name := range names {
		a := DemoAccount{ID: "demo-" + name, Email: name + "@atolye.example.test", Password: token(), Role: "member"}
		if i == 0 {
			a.Role = "superadmin"
		}
		leader := i == 0 || i == 1 || i == 4 || i == 7
		hash, e := bcrypt.GenerateFromPassword([]byte(a.Password), bcrypt.DefaultCost)
		if e != nil {
			return nil, e
		}
		_, err = tx.ExecContext(ctx, s.q("INSERT INTO users(id,email,password_hash,role,status,email_verified,mfa_enabled) VALUES(?,?,?,?,'active',true,?)"), a.ID, a.Email, string(hash), a.Role, leader)
		if err != nil {
			return nil, err
		}
		if leader {
			raw := make([]byte, 20)
			if _, err = rand.Read(raw); err != nil {
				return nil, err
			}
			a.TOTPSecret = base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
			encrypted, e := sealMFA(key, a.ID+":active", a.TOTPSecret)
			if e != nil {
				return nil, e
			}
			if _, err = tx.ExecContext(ctx, s.q("INSERT INTO user_mfa(user_id,secret) VALUES(?,?)"), a.ID, encrypted); err != nil {
				return nil, err
			}
		}
		if err = s.identityAudit(ctx, tx, "local-demo-seed", a.ID, "demo.account.verified"); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	titles := [][]string{{"Go API hata sözleşmesini gözden geçir", "Görev doğrulama kurallarını tamamla", "Eşzamanlı güncelleme testini yaz", "API kullanım örneklerini belgele"}, {"Dağıtım öncesi kontrol listesini hazırla", "Readiness ve liveness kontrollerini ekle", "Kaynak limitlerini ölç", "Geri dönüş senaryosunu doğrula"}, {"Kabul senaryolarını ekiplerle paylaş", "Görev atama akışını tarayıcıda test et", "Mobil hiyerarşi ekranını kontrol et", "Yetkisiz erişim senaryolarını tamamla"}}
	for g, name := range []string{"Go Ekibi", "Platform Ekibi", "Kalite Ekibi"} {
		id := fmt.Sprintf("demo-team-%d", g+1)
		start := 1 + g*3
		leader := accounts[start].ID
		if _, err = tx.ExecContext(ctx, s.q("INSERT INTO teams(id,name,leader_id) VALUES(?,?,?)"), id, name, leader); err != nil {
			return nil, err
		}
		for j := 0; j < 3; j++ {
			if _, err = tx.ExecContext(ctx, s.q("INSERT INTO team_members(team_id,user_id,can_assign) VALUES(?,?,?)"), id, accounts[start+j].ID, j == 1); err != nil {
				return nil, err
			}
		}
		// Main manager is also a member, and can receive work like everyone else.
		if g == 0 {
			if _, err = tx.ExecContext(ctx, s.q("INSERT INTO team_members(team_id,user_id) VALUES(?,?)"), id, accounts[0].ID); err != nil {
				return nil, err
			}
		}
		for j, title := range titles[g] {
			taskID := fmt.Sprintf("demo-task-%d-%d", g+1, j+1)
			assignee := accounts[start+j%3].ID
			if g == 0 && j == 3 {
				assignee = accounts[0].ID
			}
			now := time.Now().UTC().Format(time.RFC3339Nano)
			status := []string{"doing", "todo", "done", "todo"}[j]
			_, err = tx.ExecContext(ctx, s.q("INSERT INTO team_tasks(id,team_id,assignee_id,creator_id,title,description,category,priority,status,due_date,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)"), taskID, id, assignee, leader, title, "Demo ekip çalışması. Kabul ölçütünü doğrula ve görev durumunu güncelle.", []string{"Go", "Kubernetes", "chromedp"}[g], []string{"high", "medium", "low", "medium"}[j], status, time.Now().AddDate(0, 0, j-1).Format("2006-01-02"), now, now)
			if err != nil {
				return nil, err
			}
			if err = s.teamEvent(WithOwner(ctx, leader), tx, taskID, "Demo görev atandı: "+assignee); err != nil {
				return nil, err
			}
		}
	}
	if _, err = tx.ExecContext(ctx, "UPDATE identity_settings SET approval_required=true,bootstrapped=true WHERE id=1"); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO demo_fixture(id,version) VALUES(1,1)"); err != nil {
		return nil, err
	}
	return accounts, tx.Commit()
}

// VerifyTeamDemo checks persisted relationships, verified identities, leader MFA,
// fixture volume and the member/leader/admin visibility policy after seeding.
func (s *Store) VerifyTeamDemo(ctx context.Context) error {
	checks := []struct {
		q string
		n int
	}{
		{"SELECT count(*) FROM demo_fixture WHERE version=1", 1},
		{"SELECT count(*) FROM users WHERE id LIKE 'demo-%' AND email_verified=true AND status='active'", 10},
		{"SELECT count(*) FROM teams g JOIN team_members m ON m.team_id=g.id AND m.user_id=g.leader_id JOIN users u ON u.id=g.leader_id WHERE u.mfa_enabled=true", 3},
		{"SELECT count(*) FROM team_tasks t JOIN team_members m ON m.team_id=t.team_id AND m.user_id=t.assignee_id", 12},
		{"SELECT count(*) FROM team_members WHERE can_assign=true", 3},
		{"SELECT count(*) FROM team_events", 12},
	}
	for _, c := range checks {
		var n int
		if err := s.db.QueryRowContext(ctx, c.q).Scan(&n); err != nil {
			return err
		}
		if n != c.n {
			return fmt.Errorf("demo doğrulaması başarısız: beklenen %d, bulunan %d", c.n, n)
		}
	}
	for _, c := range []struct {
		id            string
		groups, tasks int
	}{{"demo-yonetici", 3, 12}, {"demo-go-lider", 1, 4}, {"demo-go-ece", 1, 1}} {
		out, err := s.TeamSnapshot(WithOwner(ctx, c.id))
		if err != nil {
			return err
		}
		if len(out.Teams) != c.groups || len(out.Tasks) != c.tasks {
			return fmt.Errorf("demo görünürlük doğrulaması başarısız")
		}
	}
	return nil
}
