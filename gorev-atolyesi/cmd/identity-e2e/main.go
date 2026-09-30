// Browser acceptance against a disposable identity database; never uses user data.
package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"example.com/gorev-atolyesi/internal/browser"
	"example.com/gorev-atolyesi/internal/tasks"
	"github.com/chromedp/chromedp"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	dir, err := os.MkdirTemp("", "identity-e2e-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	s, err := tasks.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		return err
	}
	defer s.Close()
	if err = s.EnableApproval(context.Background()); err != nil {
		return err
	}
	member, err := s.CreateUser(context.Background(), "deniz@example.test", "browser-long-password")
	if err != nil {
		return err
	}
	messages, err := s.DevMailbox(context.Background())
	if err != nil {
		return err
	}
	for _, m := range messages {
		if m.Email == member.Email {
			if err = s.ConfirmVerification(context.Background(), member.ID, m.Code); err != nil {
				return err
			}
		}
	}
	key, err := s.LoadMFAKey(context.Background(), filepath.Join(dir, "mfa.key"))
	if err != nil {
		return err
	}
	srv := httptest.NewServer(tasks.HandlerWithOptions(s, tasks.Options{Auth: true, Signup: true, Approval: true, Version: "identity-e2e", MFAKey: key}))
	defer srv.Close()
	ctx, cancel := browser.New()
	defer cancel()
	var png []byte
	if err = os.MkdirAll("outputs", 0755); err != nil {
		return err
	}
	if err = chromedp.Run(ctx, chromedp.EmulateViewport(390, 844), chromedp.Navigate(srv.URL), chromedp.WaitVisible("#auth-dialog"),
		chromedp.SendKeys("#auth-email", "ada@example.test"), chromedp.SendKeys("#auth-password", "browser-long-password"), chromedp.Click("#register"),
		chromedp.Poll(`document.querySelector('#auth-error').textContent.includes('kayıt isteğin')`, nil), chromedp.Click("#login"), chromedp.WaitVisible("#pending-dialog"),
		chromedp.Poll(`document.fonts.status==='loaded'`, nil), chromedp.CaptureScreenshot(&png)); err != nil {
		return err
	}
	if err = os.WriteFile("outputs/identity-verification-mobile.png", png, 0644); err != nil {
		return err
	}
	messages, err = s.DevMailbox(context.Background())
	if err != nil {
		return err
	}
	code := ""
	for _, m := range messages {
		if m.Email == "ada@example.test" {
			code = m.Code
		}
	}
	if code == "" {
		return fmt.Errorf("geliştirme doğrulama iletisi bulunamadı")
	}
	if err = chromedp.Run(ctx, chromedp.SetValue("#verification-code", code), chromedp.Click("#verification-confirm"), chromedp.Poll(`document.querySelector('#verification-form').hidden && document.querySelector('#pending-title').textContent==='Onay bekleniyor'`, nil), chromedp.CaptureScreenshot(&png)); err != nil {
		return err
	}
	if err = os.WriteFile("outputs/identity-pending-mobile.png", png, 0644); err != nil {
		return err
	}
	var secret string
	var recovery []string
	if err = chromedp.Run(ctx, chromedp.Click("#pending-security"), chromedp.WaitVisible("#mfa-dialog"), chromedp.SetValue("#mfa-password", "browser-long-password"), chromedp.Click("#mfa-submit"), chromedp.WaitVisible("#mfa-setup-key"), chromedp.Value("#mfa-secret", &secret), chromedp.CaptureScreenshot(&png)); err != nil {
		return err
	}
	if err = os.WriteFile("outputs/identity-mfa-setup-mobile.png", png, 0644); err != nil {
		return err
	}
	if err = chromedp.Run(ctx, chromedp.SetValue("#mfa-code", browserTOTP(secret)), chromedp.Click("#mfa-submit"), chromedp.WaitVisible("#mfa-recovery-box"), chromedp.Evaluate(`Array.from(document.querySelectorAll('#mfa-recovery-list li'),x=>x.textContent)`, &recovery), chromedp.Click("#mfa-saved"), chromedp.Click("#mfa-done"), chromedp.WaitVisible("#pending-dialog")); err != nil {
		return err
	}
	if len(recovery) != 10 {
		return fmt.Errorf("10 kurtarma kodu bekleniyordu")
	}
	if err = s.BootstrapAdmin(context.Background(), "ada@example.test"); err != nil {
		return err
	}
	if err = chromedp.Run(ctx, chromedp.Click("#pending-refresh"), chromedp.WaitVisible("#auth-dialog"),
		chromedp.SetValue("#auth-email", "ada@example.test"), chromedp.SetValue("#auth-password", "browser-long-password"), chromedp.Click("#login"),
		chromedp.WaitVisible("#mfa-dialog"), chromedp.SetValue("#mfa-code", recovery[0]), chromedp.Click("#mfa-submit"),
		chromedp.Poll(`!document.querySelector('#mfa-dialog').open`, nil),
		chromedp.WaitVisible("#admin-open"), chromedp.Click("#admin-open"), chromedp.WaitVisible(".account-row"),
		chromedp.CaptureScreenshot(&png)); err != nil {
		return err
	}
	if err = os.WriteFile("outputs/identity-admin-mobile.png", png, 0644); err != nil {
		return err
	}
	if err = chromedp.Run(ctx, chromedp.Click(".account-row button"), chromedp.WaitVisible("#account-confirm-dialog"),
		chromedp.SendKeys("#admin-password", "browser-long-password"), chromedp.Click("#account-confirm-save"),
		chromedp.Poll(`!document.querySelector('#account-confirm-dialog').open && document.querySelectorAll('.account-row').length===0`, nil),
		chromedp.Evaluate(`(()=>{const s=document.querySelector('#accounts-filter');s.value='all';s.dispatchEvent(new Event('change'));})()`, nil),
		chromedp.Poll(`document.querySelectorAll('.account-row').length===2`, nil)); err != nil {
		return err
	}
	var overflow bool
	if err = chromedp.Run(ctx, chromedp.Evaluate(`document.documentElement.scrollWidth>innerWidth`, &overflow)); err != nil {
		return err
	}
	if overflow {
		return fmt.Errorf("mobil hesap ekranında yatay taşma")
	}
	if err = chromedp.Run(ctx, chromedp.Click("#accounts-close"), chromedp.EmulateViewport(1440, 1000), chromedp.Click("#theme-toggle"),
		chromedp.Click("#admin-open"), chromedp.WaitVisible(".account-row"), chromedp.CaptureScreenshot(&png)); err != nil {
		return err
	}
	if err = os.WriteFile("outputs/identity-admin-dark.png", png, 0644); err != nil {
		return err
	}
	fmt.Println("Identity E2E: kayıt, e-posta doğrulama, MFA kurulumu, kurtarma kodları, bootstrap, ikinci faktörle giriş ve hesap onayı başarılı; mobil/koyu tema görüntüleri outputs/ altında.")
	return nil
}

// Independent authenticator for the disposable browser fixture only.
func browserTOTP(secret string) string {
	key, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(time.Now().Unix()/30))
	m := hmac.New(sha1.New, key)
	m.Write(b[:])
	sum := m.Sum(nil)
	offset := sum[len(sum)-1] & 15
	return fmt.Sprintf("%06d", (binary.BigEndian.Uint32(sum[offset:offset+4])&0x7fffffff)%1000000)
}
