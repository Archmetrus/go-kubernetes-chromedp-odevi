package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"example.com/gorev-atolyesi/internal/browser"
	"example.com/gorev-atolyesi/internal/tasks"
	"fmt"
	"github.com/chromedp/chromedp"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func otp(secret string) string {
	key, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(time.Now().Unix()/30))
	h := hmac.New(sha1.New, key)
	h.Write(b[:])
	v := h.Sum(nil)
	n := v[len(v)-1] & 15
	return fmt.Sprintf("%06d", (binary.BigEndian.Uint32(v[n:n+4])&0x7fffffff)%1000000)
}
func run() error {
	dir, e := os.MkdirTemp("", "automation-e2e-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(dir)
	s, e := tasks.Open(filepath.Join(dir, "test.db"))
	if e != nil {
		return e
	}
	defer s.Close()
	key, e := s.LoadMFAKey(context.Background(), filepath.Join(dir, "mfa.key"))
	if e != nil {
		return e
	}
	accounts, e := s.SeedTeamDemo(context.Background(), key)
	if e != nil {
		return e
	}
	srv := httptest.NewServer(tasks.HandlerWithOptions(s, tasks.Options{Auth: true, Approval: true, MFAKey: key}))
	defer srv.Close()
	file := filepath.Join(dir, "tasks.json")
	if e = os.WriteFile(file, []byte(`{"tasks":[{"team_id":"demo-team-1","assignee_email":"go-ece@atolye.example.test","title":"Dosyadan API kontrolü","category":"Go"}]}`), 0600); e != nil {
		return e
	}
	ctx, cancel := browser.New()
	defer cancel()
	a := accounts[0]
	if e = chromedp.Run(ctx, chromedp.EmulateViewport(1280, 960), chromedp.Navigate(srv.URL), chromedp.WaitVisible("#auth-dialog"), chromedp.SetValue("#auth-email", a.Email), chromedp.SetValue("#auth-password", a.Password), chromedp.Click("#login"), chromedp.WaitVisible("#mfa-dialog"), chromedp.SetValue("#mfa-code", otp(a.TOTPSecret)), chromedp.Click("#mfa-submit"), chromedp.Poll(`!document.querySelector('#mfa-dialog').open`, nil), chromedp.Click("#teams-open"), chromedp.WaitVisible(".team-card"), chromedp.Click("#bulk-open"), chromedp.WaitVisible("#bulk-dialog"), chromedp.SetUploadFiles("#bulk-file", []string{file}), chromedp.Click("#bulk-preview"), chromedp.Poll(`!document.querySelector('#bulk-agree').disabled && document.querySelectorAll('#bulk-review td').length===4`, nil)); e != nil {
		return e
	}
	var png []byte
	if e = chromedp.Run(ctx, chromedp.CaptureScreenshot(&png)); e != nil {
		return e
	}
	if e = os.MkdirAll("outputs", 0755); e != nil {
		return e
	}
	if e = os.WriteFile("outputs/automation-preview.png", png, 0644); e != nil {
		return e
	}
	if e = chromedp.Run(ctx, chromedp.Click("#bulk-agree"), chromedp.Click("#bulk-commit"), chromedp.Poll(`!document.querySelector('#bulk-results').hidden && document.querySelector('#bulk-job-list').textContent.includes('Dosyadan API kontrolü')`, nil), chromedp.Click(`[data-bulk-tab="keys"]`), chromedp.SetValue("#bulk-key-team", "demo-team-1"), chromedp.SetValue("#bulk-key-name", "E2E robot"), chromedp.SetValue("#bulk-key-password", a.Password), chromedp.Click("#bulk-key-form button"), chromedp.WaitVisible("#bulk-key-once")); e != nil {
		return e
	}
	var raw string
	if e = chromedp.Run(ctx, chromedp.Value("#bulk-key-secret", &raw)); e != nil {
		return e
	}
	if len(raw) != 71 {
		return fmt.Errorf("key missing")
	}
	tokenFile := filepath.Join(dir, "automation.token")
	if e = os.WriteFile(tokenFile, []byte(raw), 0600); e != nil {
		return e
	}
	call := func(command string, extra ...string) ([]byte, error) {
		args := []string{"scripts/atolye_assign.py", command, "--url", srv.URL, "--token-file", tokenFile, "--preview-file", filepath.Join(dir, "preview.json")}
		args = append(args, extra...)
		cmd := exec.Command("python3", args...)
		return cmd.CombinedOutput()
	}
	if _, e = call("preview", "--file", file); e != nil {
		return fmt.Errorf("CLI preview failed: %w", e)
	}
	first, e := call("commit")
	if e != nil {
		return fmt.Errorf("CLI commit failed: %w", e)
	}
	second, e := call("commit")
	if e != nil {
		return e
	}
	var x, y tasks.AssignmentResult
	if json.Unmarshal(first, &x) != nil || json.Unmarshal(second, &y) != nil || x.ID == "" || x.ID != y.ID {
		return fmt.Errorf("CLI retry not idempotent")
	}
	if _, e = call("results"); e != nil {
		return e
	}
	if e = chromedp.Run(ctx, chromedp.Click("#bulk-key-list button"), chromedp.Poll(`document.querySelector('#bulk-key-list').textContent.includes('iptal edildi')`, nil)); e != nil {
		return e
	}
	if _, e = call("results"); e == nil {
		return fmt.Errorf("revoked key worked")
	}
	if e = chromedp.Run(ctx, chromedp.Click(`[data-bulk-tab="upload"]`), chromedp.EmulateViewport(390, 844), chromedp.CaptureScreenshot(&png)); e != nil {
		return e
	}
	if e = os.WriteFile("outputs/automation-mobile.png", png, 0644); e != nil {
		return e
	}
	fmt.Println("Automation E2E başarılı: web dosya önizleme/onay, anahtar oluşturma/iptal, gerçek CLI preview/commit/results ve tekrar denemede tek atama. Sırlar yazdırılmadı.")
	return nil
}
