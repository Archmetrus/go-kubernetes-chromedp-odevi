package main

import (
	"context"
	"example.com/odev/internal/webapp"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	adres := os.Getenv("ADDR")
	if adres == "" {
		adres = "127.0.0.1:8080"
	}
	mesaj := os.Getenv("MESAJ")
	if mesaj == "" {
		mesaj = "Merhaba"
	}
	srv := &http.Server{Addr: adres, Handler: webapp.Handler(mesaj), ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(c); err != nil {
			log.Print(err)
		}
	}()
	log.Printf("Sunucu: http://%s", adres)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
