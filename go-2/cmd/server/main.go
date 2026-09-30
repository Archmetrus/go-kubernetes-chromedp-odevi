package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s-chromedp-synthetics/pkg/api"
	"k8s-chromedp-synthetics/pkg/automation"
	"k8s-chromedp-synthetics/pkg/runner"
	"k8s-chromedp-synthetics/pkg/security"
	"k8s-chromedp-synthetics/pkg/store"
	"k8s-chromedp-synthetics/pkg/types"
)

func main() {
	log.Println("==========================================================")
	log.Println(" Starting k8s-chromedp-synthetics & Robomotion Gateway")
	log.Println("==========================================================")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		apiKey = "dev-secret-key"
		log.Printf("Notice: No API_KEY specified. Using default: %s", apiKey)
	}

	secConfig := types.SecurityConfig{
		APIKey:            apiKey,
		EnableSSRFGuard:   true,
		RateLimitRequests: 20.0,
		RateLimitBurst:    40,
	}

	secMiddleware := security.NewSecurityMiddleware(secConfig)
	ssrfGuard := security.NewSSRFGuard(secConfig)

	storageMgr, err := store.NewStorageManager("./data")
	if err != nil {
		log.Fatalf("Fatal error initializing storage manager: %v", err)
	}

	chromeRunner := runner.NewChromeRunner(ssrfGuard, storageMgr)
	autoGateway := automation.NewAutomationGateway(chromeRunner, storageMgr)

	server := api.NewServer(autoGateway, chromeRunner, storageMgr, secMiddleware)
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%s", port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
	}

	go func() {
		log.Printf("🚀 Server running on http://0.0.0.0:%s", port)
		log.Printf("🔗 Robomotion Gateway Endpoint: http://0.0.0.0:%s/api/v1/automation/execute", port)
		log.Printf("📊 Dashboard Web UI: http://0.0.0.0:%s/", port)
		log.Printf("📈 Prometheus Metrics: http://0.0.0.0:%s/metrics", port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Graceful Shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down server gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("Error shutting down server: %v", err)
	}
	log.Println("Server stopped cleanly.")
}
