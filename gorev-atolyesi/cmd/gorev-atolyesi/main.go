package main

import (
	"context"
	"encoding/json"
	"errors"
	"example.com/gorev-atolyesi/internal/tasks"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if err := run(); err != nil {
		slog.Error("sunucu durdu", "error", err)
		os.Exit(1)
	}
}
func run() error {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "data/gorevler.db"
	}
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = "127.0.0.1:8090"
	}
	ctxOpen, cancelOpen := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelOpen()
	production := os.Getenv("APP_ENV") == "production" || os.Getenv("APP_ENV") == "staging"
	environment := os.Getenv("APP_ENV")
	if environment != "" && environment != "development" && !production {
		return fmt.Errorf("APP_ENV development, staging veya production olmalı")
	}
	opts := tasks.Options{Auth: os.Getenv("AUTH_ENABLED") == "true", Signup: os.Getenv("ALLOW_SIGNUP") == "true", Production: production, Origin: os.Getenv("APP_ORIGIN"), Version: os.Getenv("APP_VERSION"), MetricsToken: os.Getenv("METRICS_TOKEN")}
	if opts.Version == "" {
		opts.Version = "dev"
	}
	url := os.Getenv("DATABASE_URL")
	if production {
		u, e := neturl.Parse(url)
		if e != nil || u.Query().Get("sslmode") != "verify-full" {
			return fmt.Errorf("production DATABASE_URL sslmode=verify-full gerektirir")
		}
	}
	if err := opts.Validate(url != ""); err != nil {
		return err
	}
	if len(os.Args) == 1 && !opts.Auth && os.Getenv("ALLOW_LOCAL_NETWORK") != "true" {
		host, _, e := net.SplitHostPort(addr)
		if e != nil {
			return e
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return fmt.Errorf("hesapsız mod yalnızca loopback adresinde açılır; yerel kind için ALLOW_LOCAL_NETWORK=true gerekli")
		}
	}
	var s *tasks.Store
	var err error
	if url != "" {
		s, err = tasks.OpenPostgres(ctxOpen, url)
	} else {
		s, err = tasks.Open(dbPath)
	}
	if err != nil {
		return err
	}
	defer s.Close()
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		return s.Migrate(ctxOpen)
	}
	if url != "" {
		if err = s.CheckSchema(ctxOpen); err != nil {
			return err
		}
	}
	opts.Approval, err = s.ApprovalEnabled(ctxOpen)
	if err != nil {
		return err
	}
	if os.Getenv("ACCOUNT_APPROVAL") == "true" {
		opts.Approval = true
	}
	if err = opts.Validate(url != ""); err != nil {
		return err
	}
	if opts.Approval {
		if err = s.EnableApproval(ctxOpen); err != nil {
			return err
		}
	}
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "migrate":
			return s.Migrate(ctxOpen)
		case "admin-list":
			users, e := s.Users(ctxOpen, os.Getenv("USER_AFTER"))
			if e != nil {
				return e
			}
			return json.NewEncoder(os.Stdout).Encode(users)
		case "demo-seed":
			if production || os.Getenv("ALLOW_DEMO_SEED") != "true" || url != "" {
				return fmt.Errorf("demo seed yalnızca açıkça seçilmiş yerel geliştirme DB'sinde çalışır")
			}
			path := os.Getenv("DEMO_ACCOUNTS_FILE")
			if path == "" {
				return fmt.Errorf("DEMO_ACCOUNTS_FILE gerekli")
			}
			file, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if e != nil {
				return e
			}
			defer file.Close()
			key, e := s.LoadMFAKey(ctxOpen, os.Getenv("MFA_KEY_FILE"))
			if e != nil {
				os.Remove(path)
				return e
			}
			accounts, e := s.SeedTeamDemo(ctxOpen, key)
			if e != nil {
				os.Remove(path)
				return e
			}
			if e = json.NewEncoder(file).Encode(accounts); e != nil {
				return e
			}
			if e = file.Sync(); e != nil {
				return e
			}
			if e = s.VerifyTeamDemo(ctxOpen); e != nil {
				return e
			}
			fmt.Println("Demo DB doğrulandı: 10 hesap, 3 grup, 12 görev; admin/lider/üye erişim kapsamları geçti. Giriş bilgileri korumalı yerel dosyada.")
			return nil
		case "admin-mailbox":
			if production {
				return fmt.Errorf("geliştirme posta kutusu yalnızca development içindir")
			}
			messages, e := s.DevMailbox(ctxOpen)
			if e != nil {
				return e
			}
			return json.NewEncoder(os.Stdout).Encode(messages)
		case "identity-enable":
			if !opts.Approval {
				return fmt.Errorf("ACCOUNT_APPROVAL=true gerekli")
			}
			fmt.Println("Web kaydı ve yönetici onayı etkin. İlk yöneticiyi web kaydından sonra belirleyin.")
			return nil
		case "admin-bootstrap":
			if err = s.BootstrapAdmin(ctxOpen, os.Getenv("USER_EMAIL")); err != nil {
				return err
			}
			fmt.Println("İlk ana yönetici belirlendi. Web arayüzünde yeniden giriş yapın.")
			return nil
		case "create-user":
			// Existing CLI provisioning remains separate from operator MFA recovery.
			_, err = s.CreateUser(ctxOpen, os.Getenv("USER_EMAIL"), os.Getenv("USER_PASSWORD"))
			return err
		case "claim-local":
			n, e := s.ClaimLocal(ctxOpen, os.Getenv("USER_EMAIL"))
			if e == nil {
				fmt.Printf("%d görev hesaba aktarıldı\n", n)
			}
			return e
		case "import-sqlite":
			path := os.Getenv("SOURCE_SQLITE")
			if path == "" || url == "" {
				return fmt.Errorf("SOURCE_SQLITE ve PostgreSQL DATABASE_URL gerekli")
			}
			n, e := s.TransferSQLite(ctxOpen, path, os.Getenv("USER_EMAIL"))
			if e == nil {
				fmt.Printf("%d görev PostgreSQL hesabına kopyalandı; kaynak korundu\n", n)
			}
			return e
		default:
			return fmt.Errorf("bilinmeyen komut")
		case "admin-mfa-reset":
			if err = s.ResetMFA(ctxOpen, os.Getenv("USER_EMAIL")); err != nil {
				return err
			}
			fmt.Println("MFA sıfırlandı. Tüm oturumlar kapatıldı; yönetici yeniden MFA kurmalıdır.")
			return nil
		}
	}
	if url != "" {
		var currentErr error
		currentErr = s.CheckSchema(ctxOpen)
		if currentErr != nil {
			return currentErr
		}
	}
	if opts.Approval {
		path := os.Getenv("MFA_KEY_FILE")
		if path == "" {
			path = ".control/mfa.key"
		}
		opts.MFAKey, err = s.LoadMFAKey(ctxOpen, path)
		if err != nil {
			return err
		}
	}
	srv := &http.Server{Addr: addr, Handler: tasks.HandlerWithOptions(s, opts), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c, cancel := context.WithTimeout(ctx, 10*time.Second)
				if e := s.Cleanup(c); e != nil {
					slog.Error("rate cleanup failed")
				}
				cancel()
			}
		}
	}()
	result := make(chan error, 1)
	go func() { slog.Info("Görev Atölyesi hazır", "url", "http://"+addr); result <- srv.ListenAndServe() }()
	select {
	case err = <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(c)
	}
}
