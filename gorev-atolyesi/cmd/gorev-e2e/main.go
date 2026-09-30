// Browser acceptance check against an isolated real SQLite database.
package main

import (
	"context"
	"encoding/json"
	"example.com/gorev-atolyesi/internal/browser"
	"example.com/gorev-atolyesi/internal/tasks"
	"fmt"
	cdpbrowser "github.com/chromedp/cdproto/browser"
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
func run() error {
	dir, err := os.MkdirTemp("", "gorev-e2e-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	s, err := tasks.Open(filepath.Join(dir, "tasks.db"))
	if err != nil {
		return err
	}
	defer s.Close()
	srv := httptest.NewServer(tasks.Handler(s))
	defer srv.Close()
	ctx, cancel := browser.New()
	defer cancel()
	err = chromedp.Run(ctx,
		chromedp.EmulateViewport(1440, 1000), chromedp.Navigate(srv.URL), chromedp.WaitVisible("#board", chromedp.ByQuery),
		chromedp.Click("#new-task", chromedp.ByQuery), chromedp.SendKeys("#title", "Tarayıcı kabul testi", chromedp.ByQuery), chromedp.SendKeys("#description", "Kalıcı görev kaydı ve durum geçişini doğrula.", chromedp.ByQuery), chromedp.Click("#save-task", chromedp.ByQuery),
		chromedp.Poll(`document.querySelectorAll('.card').length === 1 && !document.querySelector('#editor').open`, nil),
		chromedp.Reload(), chromedp.WaitVisible(".card", chromedp.ByQuery),
		chromedp.Click(".card-title", chromedp.ByQuery), chromedp.SetValue("#title", "Güncellenmiş kabul testi", chromedp.ByQuery), chromedp.Click("#save-task", chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.card-title').textContent === 'Güncellenmiş kabul testi' && !document.querySelector('#editor').open`, nil),
		chromedp.Evaluate(`(()=>{const s=document.querySelector('.status-select');s.value='done';s.dispatchEvent(new Event('change',{bubbles:true}));})()`, nil),
		chromedp.Poll(`document.querySelectorAll('#done-list .card').length === 1`, nil),
		chromedp.SendKeys("#search", "eşleşmeyen sorgu", chromedp.ByQuery), chromedp.Poll(`document.querySelectorAll('.card').length === 0`, nil), chromedp.Click("#clear-filters", chromedp.ByQuery),
		chromedp.WaitVisible(".card", chromedp.ByQuery), chromedp.Click(".card-actions button", chromedp.ByQuery), chromedp.Click("#confirm-delete", chromedp.ByQuery), chromedp.Poll(`document.querySelectorAll('.card').length === 0 && !document.querySelector('#delete-dialog').open`, nil))
	if err != nil {
		return err
	}
	// Screenshots use fixture tasks only in the disposable test database.
	fixtures := []tasks.Task{
		{Title: "Go API hata yönetimini tamamla", Description: "İstek doğrulama ve tutarlı hata cevaplarını gözden geçir.", Category: "Go", Priority: "high", Status: "todo", DueDate: time.Now().Format("2006-01-02")},
		{Title: "Tarayıcı senaryolarını genişlet", Description: "Görev düzenleme ve filtreleme akışlarını kapsa.", Category: "chromedp", Priority: "medium", Status: "todo"},
		{Title: "Kalıcı disk bağlantısını doğrula", Description: "Pod yeniden başladığında görevlerin korunduğunu kontrol et.", Category: "Kubernetes", Priority: "high", Status: "doing"},
		{Title: "Görev panosunu tasarla", Description: "Üç aşamalı çalışma düzeni ve mobil görünüm.", Category: "Genel", Priority: "medium", Status: "done"},
		{Title: "SQLite veri katmanını kur", Description: "Kalıcı kayıtlar ve sürüm kontrollü güncellemeler.", Category: "Go", Priority: "low", Status: "done"},
	}
	for _, t := range fixtures {
		if _, err = s.Create(context.Background(), t); err != nil {
			return err
		}
	}
	err = chromedp.Run(ctx,
		chromedp.Navigate(srv.URL), chromedp.WaitVisible(".card", chromedp.ByQuery),
		chromedp.Click(`[data-scope="focus"]`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelectorAll('.card').length === 2`, nil),
		chromedp.Click(`[data-scope="upcoming"]`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelectorAll('.card').length === 1`, nil),
		chromedp.Click(`[data-scope="all"]`, chromedp.ByQuery),
		chromedp.Click(".day.today", chromedp.ByQuery),
		chromedp.Poll(`document.querySelectorAll('.card').length === 1`, nil),
		chromedp.Click("#clear-day", chromedp.ByQuery),
		chromedp.Click("#list-view", chromedp.ByQuery),
		chromedp.Poll(`document.querySelectorAll('#task-table article').length === 5 && document.querySelector('#board').hidden`, nil),
		chromedp.Evaluate(`(()=>{const s=document.querySelector('#sort');s.value='priority';s.dispatchEvent(new Event('input',{bubbles:true}));})()`, nil),
		chromedp.Poll(`document.querySelector('#task-table article .priority').classList.contains('high')`, nil),
		chromedp.Click("#theme-toggle", chromedp.ByQuery),
		chromedp.Reload(), chromedp.WaitVisible("#task-table article", chromedp.ByQuery),
		chromedp.Poll(`document.body.classList.contains('dark') && document.querySelector('#list-view').getAttribute('aria-pressed') === 'true'`, nil),
		chromedp.Click("#theme-toggle", chromedp.ByQuery),
		chromedp.Click("#board-view", chromedp.ByQuery),
		chromedp.Click(`[data-template="Kubernetes"]`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('#editor').open && document.querySelector('#category').value === 'Kubernetes' && document.querySelector('#title').value.length > 3`, nil),
		chromedp.Click("#cancel-editor", chromedp.ByQuery),
	)
	if err != nil {
		return fmt.Errorf("yeni özellikler: %w", err)
	}
	// A new DB row, absent from the loaded page, must still be exported.
	fresh, err := s.Create(ctx, tasks.Task{Title: "Sayfa yüklendikten sonra eklendi", Category: "Genel", Priority: "low", Status: "todo"})
	if err != nil {
		return err
	}
	downloadDir := filepath.Join(dir, "downloads")
	if err = os.MkdirAll(downloadDir, 0700); err != nil {
		return err
	}
	err = chromedp.Run(ctx,
		cdpbrowser.SetDownloadBehavior(cdpbrowser.SetDownloadBehaviorBehaviorAllow).WithDownloadPath(downloadDir),
		chromedp.SendKeys("#search", "eşleşmeyen arama", chromedp.ByQuery),
		chromedp.Poll(`document.querySelectorAll('.card').length === 0`, nil),
		chromedp.Click("#export", chromedp.ByQuery),
		chromedp.ActionFunc(func(ctx context.Context) error {
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				files, err := filepath.Glob(filepath.Join(downloadDir, "*.json"))
				if err != nil {
					return err
				}
				if len(files) > 0 {
					data, err := os.ReadFile(files[0])
					if err != nil {
						return err
					}
					var export struct {
						Format string       `json:"format"`
						Count  int          `json:"count"`
						Tasks  []tasks.Task `json:"tasks"`
					}
					if err = json.Unmarshal(data, &export); err != nil {
						return err
					}
					if export.Format != "gorev-atolyesi-v1" || export.Count != 6 || len(export.Tasks) != 6 {
						return fmt.Errorf("export count/format incorrect")
					}
					for _, t := range export.Tasks {
						if t.ID == fresh.ID {
							return nil
						}
					}
					return fmt.Errorf("export used stale page data")
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-ticker.C:
				}
			}
		}),
		chromedp.Click("#clear-filters", chromedp.ByQuery),
	)
	if err != nil {
		return fmt.Errorf("gerçek JSON indirme: %w", err)
	}
	if err = s.Delete(ctx, fresh.ID, fresh.Version); err != nil {
		return err
	}
	if err = os.MkdirAll("outputs", 0755); err != nil {
		return err
	}
	for _, size := range []struct {
		name string
		w, h int64
		mode string
	}{{"gorev-desktop", 1440, 1000, ""}, {"gorev-mobile", 390, 844, ""}, {"gorev-dark", 1440, 1000, "dark"}, {"gorev-list", 1440, 1000, "list"}, {"gorev-form", 390, 844, "form"}} {
		var png []byte
		var overflow bool
		actions := []chromedp.Action{chromedp.EmulateViewport(size.w, size.h), chromedp.Navigate(srv.URL), chromedp.Click("#board-view"), chromedp.WaitVisible(".card", chromedp.ByQuery), chromedp.Poll(`document.fonts.status === "loaded"`, nil)}
		switch size.mode {
		case "dark":
			actions = append(actions, chromedp.Click("#theme-toggle"))
		case "list":
			actions = append(actions, chromedp.Click("#list-view"))
		case "form":
			actions = append(actions, chromedp.Click("#new-task"), chromedp.WaitVisible("#editor"))
		}
		actions = append(actions, chromedp.Evaluate(`document.documentElement.scrollWidth > window.innerWidth`, &overflow), chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.Poll(`document.getAnimations().every(a => a.playState !== "running")`, nil).Do(ctx)
		}), chromedp.FullScreenshot(&png, 90))
		if err = chromedp.Run(ctx, actions...); err != nil {
			return err
		}
		if size.mode == "dark" {
			if err = chromedp.Run(ctx, chromedp.Click("#theme-toggle")); err != nil {
				return err
			}
		}
		if overflow {
			return fmt.Errorf("yatay taşma: %s", size.name)
		}
		if err = os.WriteFile("outputs/"+size.name+".png", png, 0644); err != nil {
			return err
		}
	}
	if err = accountChecks(ctx, s, dir); err != nil {
		return fmt.Errorf("hesap ve canlı güncelleme: %w", err)
	}
	fmt.Println("E2E başarılı: oluştur, yenile, düzenle, durum değiştir, filtrele, sil; odak/tarih filtreleri, liste, sıralama, tema kalıcılığı, şablon ve JSON dışa aktarma; masaüstü ve mobil görünüm.")
	return nil
}

func accountChecks(ctx context.Context, s *tasks.Store, dir string) error {
	srv := httptest.NewServer(tasks.HandlerWithOptions(s, tasks.Options{Auth: true, Signup: true, Version: "e2e"}))
	defer srv.Close()
	var png []byte
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(390, 844), chromedp.Navigate(srv.URL), chromedp.WaitVisible("#auth-dialog"), chromedp.Poll(`document.fonts.status === 'loaded'`, nil), chromedp.CaptureScreenshot(&png)); err != nil {
		return err
	}
	if err := os.WriteFile("outputs/gorev-login.png", png, 0644); err != nil {
		return err
	}
	if err := chromedp.Run(ctx, chromedp.SendKeys("#auth-email", "browser@example.test"), chromedp.SendKeys("#auth-password", "browser-long-password"), chromedp.Click("#register"), chromedp.Poll(`document.querySelector('#auth-error').textContent.includes('oluşturuldu')`, nil), chromedp.Click("#login"), chromedp.WaitVisible("#board")); err != nil {
		return err
	}
	// Claim a committed external write to this owner; polling must pick it up.
	t, err := s.Create(ctx, tasks.Task{Title: "Dışarıdan eklenen canlı görev", Category: "Go", Priority: "medium", Status: "todo"})
	if err != nil {
		return err
	}
	if _, err = s.ClaimLocal(ctx, "browser@example.test"); err != nil {
		return err
	}
	// Legacy fixtures are also claimed. Observe title instead of global count.
	if err = chromedp.Run(ctx, chromedp.Poll(`Array.from(document.querySelectorAll('.card-title')).some(x=>x.textContent==='Dışarıdan eklenen canlı görev')`, nil, chromedp.WithPollingTimeout(8*time.Second)), chromedp.Click(fmt.Sprintf(`.card[data-id="%d"] .card-title`, t.ID)), chromedp.Click("#archived"), chromedp.Click("#save-task"), chromedp.Poll(`!document.querySelector('#editor').open`, nil), chromedp.Evaluate(`(()=>{let s=document.querySelector('#storage-scope');s.value='archive';s.dispatchEvent(new Event('change'));})()`, nil), chromedp.WaitVisible(fmt.Sprintf(`.card[data-id="%d"]`, t.ID)), chromedp.Click(fmt.Sprintf(`.card[data-id="%d"] .card-actions button`, t.ID)), chromedp.Click("#confirm-delete"), chromedp.Poll(`!document.querySelector('#delete-dialog').open && document.querySelectorAll('.card').length===0`, nil), chromedp.Evaluate(`(()=>{let s=document.querySelector('#storage-scope');s.value='trash';s.dispatchEvent(new Event('change'));})()`, nil), chromedp.WaitVisible(fmt.Sprintf(`.card[data-id="%d"]`, t.ID)), chromedp.Click(fmt.Sprintf(`.card[data-id="%d"] .card-actions button`, t.ID)), chromedp.Poll(fmt.Sprintf(`!document.querySelector('.card[data-id="%d"]')`, t.ID), nil)); err != nil {
		return err
	}
	raw := `{"format":"gorev-atolyesi-v1","exported_at":"2026-09-10T00:00:00Z","count":1,"tasks":[{"title":"Dosyadan gelen görev","description":"","category":"Go","priority":"low","status":"todo","due_date":""}]}`
	file := filepath.Join(dir, "import.json")
	if err = os.WriteFile(file, []byte(raw), 0600); err != nil {
		return err
	}
	if err = chromedp.Run(ctx, chromedp.Evaluate(`(()=>{let s=document.querySelector('#storage-scope');s.value='active';s.dispatchEvent(new Event('change'));})()`, nil), chromedp.Click("#import-open"), chromedp.SetUploadFiles("#import-file", []string{file}), chromedp.Poll(`!document.querySelector('#import-confirm').disabled`, nil), chromedp.Click("#import-confirm"), chromedp.Poll(`!document.querySelector('#import-dialog').open && Array.from(document.querySelectorAll('.card-title')).some(x=>x.textContent==='Dosyadan gelen görev')`, nil), chromedp.Click("#logout"), chromedp.WaitVisible("#auth-dialog"), chromedp.SendKeys("#auth-email", "second@example.test"), chromedp.SendKeys("#auth-password", "browser-long-password"), chromedp.Click("#register"), chromedp.Poll(`document.querySelector('#auth-error').textContent.includes('oluşturuldu')`, nil), chromedp.Click("#login"), chromedp.WaitVisible("#board"), chromedp.Poll(`document.querySelectorAll('.card').length===0`, nil)); err != nil {
		return err
	}
	return nil
}
