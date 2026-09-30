package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"k8s-chromedp-synthetics/pkg/controller"
	"k8s-chromedp-synthetics/pkg/runner"
	"k8s-chromedp-synthetics/pkg/security"
	"k8s-chromedp-synthetics/pkg/store"
	"k8s-chromedp-synthetics/pkg/types"
)

func main() {
	log.Println("==========================================================")
	log.Println(" Starting k8s-chromedp-synthetics CRD Controller Operator")
	log.Println("==========================================================")

	secConfig := types.SecurityConfig{
		EnableSSRFGuard: true,
	}

	ssrfGuard := security.NewSSRFGuard(secConfig)
	storageMgr, err := store.NewStorageManager("./data")
	if err != nil {
		log.Fatalf("Error initializing storage manager: %v", err)
	}

	chromeRunner := runner.NewChromeRunner(ssrfGuard, storageMgr)

	k8sCtrl, err := controller.NewSyntheticController(chromeRunner, storageMgr)
	if err != nil {
		log.Fatalf("Error initializing K8s controller: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go k8sCtrl.StartInformer(ctx)

	log.Println("Operator initialized. Press Ctrl+C to terminate.")
	<-stop
	log.Println("Operator shutting down...")
}
