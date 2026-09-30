// Isolated browser acceptance for the actual admin, leader and member screens.
package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"example.com/gorev-atolyesi/internal/browser"
	"example.com/gorev-atolyesi/internal/tasks"
	"fmt"
	"github.com/chromedp/chromedp"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
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
	dir, err := os.MkdirTemp("", "teams-e2e-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	s, err := tasks.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		return err
	}
	defer s.Close()
	key, err := s.LoadMFAKey(context.Background(), filepath.Join(dir, "mfa.key"))
	if err != nil {
		return err
	}
	accounts, err := s.SeedTeamDemo(context.Background(), key)
	if err != nil {
		return err
	}
	srv := httptest.NewServer(tasks.HandlerWithOptions(s, tasks.Options{Auth: true, Approval: true, Signup: true, MFAKey: key, Version: "teams-e2e"}))
	defer srv.Close()
	if err = os.MkdirAll("outputs", 0755); err != nil {
		return err
	}
	for _, idx := range []int{0, 1, 3} {
		if err = role(srv.URL, accounts[idx], idx); err != nil {
			return fmt.Errorf("role %d: %w", idx, err)
		}
	}
	fmt.Println("Teams E2E başarılı: admin ağacı, görev atama, lider/üye kapsamı, geçmiş, mobil/koyu görünüm; test sunucusu kapatıldı.")
	return nil
}
func role(url string, a tasks.DemoAccount, idx int) error {
	ctx, cancel := browser.New()
	defer cancel()
	width, height := int64(1440), int64(1000)
	if idx == 3 {
		width, height = 390, 844
	}
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(width, height), chromedp.Navigate(url), chromedp.WaitVisible("#auth-dialog"), chromedp.SetValue("#auth-email", a.Email), chromedp.SetValue("#auth-password", a.Password), chromedp.Click("#login")); err != nil {
		return err
	}
	if a.TOTPSecret != "" {
		if err := chromedp.Run(ctx, chromedp.WaitVisible("#mfa-dialog"), chromedp.SetValue("#mfa-code", otp(a.TOTPSecret)), chromedp.Click("#mfa-submit"), chromedp.Poll(`!document.querySelector('#mfa-dialog').open`, nil)); err != nil {
			return err
		}
	}
	if err := chromedp.Run(ctx, chromedp.WaitVisible("#teams-open"), chromedp.Click("#teams-open"), chromedp.WaitVisible(".team-card"), chromedp.Poll(`document.fonts.status==='loaded'`, nil)); err != nil {
		return err
	}
	var png []byte
	if idx == 0 {
		if err := chromedp.Run(ctx, chromedp.Poll(`document.querySelectorAll('.team-card').length===12 && document.querySelectorAll('.team-branch').length===3`, nil), chromedp.CaptureScreenshot(&png)); err != nil {
			return err
		}
		if err := os.WriteFile("outputs/teams-admin-desktop.png", png, 0644); err != nil {
			return err
		}
		if err := chromedp.Run(ctx, chromedp.Click("#team-assign"), chromedp.WaitVisible("#team-editor"), chromedp.SetValue("#task-team", "demo-team-1"), chromedp.Evaluate(`document.querySelector('#task-team').dispatchEvent(new Event('change'))`, nil), chromedp.SetValue("#task-assignee", "demo-go-ece"), chromedp.SetValue("#team-task-title", "Tarayıcıdan atanan ekip görevi"), chromedp.Click("#team-save"), chromedp.Poll(`!document.querySelector('#team-editor').open && document.querySelectorAll('.team-card').length===13`, nil)); err != nil {
			return err
		}
	} else if idx == 1 {
		if err := chromedp.Run(ctx, chromedp.Poll(`document.querySelectorAll('.team-branch').length===1 && document.querySelectorAll('.team-card').length===5 && document.querySelector('#team-create').hidden`, nil), chromedp.Click(".tree-group"), chromedp.Click("#team-manage"), chromedp.WaitVisible("#team-member-list"), chromedp.Poll(`document.querySelectorAll('.team-member-row').length===4`, nil), chromedp.Click(`[data-team-close="team-members-dialog"]`), chromedp.CaptureScreenshot(&png)); err != nil {
			return err
		}
		if err := os.WriteFile("outputs/teams-leader-desktop.png", png, 0644); err != nil {
			return err
		}
	} else {
		if err := chromedp.Run(ctx, chromedp.Poll(`document.querySelectorAll('.team-card').length===2 && document.querySelector('#team-assign').hidden && document.querySelector('#team-manage').hidden`, nil), chromedp.Evaluate(`(()=>{let s=document.querySelector('.team-card-actions select');s.value='doing';s.dispatchEvent(new Event('change'))})()`, nil), chromedp.Poll(`document.querySelector('.team-column.doing .team-card')!==null`, nil), chromedp.Click(".team-card-actions button"), chromedp.WaitVisible("#team-history-dialog"), chromedp.Poll(`document.querySelector('#team-history').textContent.includes('Durum:')`, nil), chromedp.Click(`[data-team-close="team-history-dialog"]`), chromedp.EmulateViewport(390, 844), chromedp.Evaluate(`document.body.classList.add('dark')`, nil), chromedp.CaptureScreenshot(&png)); err != nil {
			return err
		}
		if err := os.WriteFile("outputs/teams-member-mobile.png", png, 0644); err != nil {
			return err
		}
	}
	var overflow bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('#teams-dialog').scrollWidth>document.querySelector('#teams-dialog').clientWidth+1`, &overflow)); err != nil {
		return err
	}
	if overflow {
		return fmt.Errorf("horizontal overflow")
	}
	return nil
}
