package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/zvikanaparstek/hpa-stats/internal/kube"
)

type config struct {
	kubeconfig string
	namespace  string
	interval   time.Duration
	hpaName    string
}

func main() {
	cfg := parseFlags()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	clientset, err := kube.NewClientset(cfg.kubeconfig)
	if err != nil {
		log.Fatalf("failed to build kubernetes client: %v", err)
	}

	ticker := time.NewTicker(cfg.interval)
	defer ticker.Stop()

	if err := kube.PrintHPAStats(ctx, clientset, cfg.namespace, cfg.hpaName); err != nil {
		log.Printf("error fetching HPA stats: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			fmt.Println("Received interrupt, shutting down gracefully...")
			return
		case <-ticker.C:
			if err := kube.PrintHPAStats(ctx, clientset, cfg.namespace, cfg.hpaName); err != nil {
				log.Printf("error fetching HPA stats: %v", err)
			}
		}
	}
}

func parseFlags() config {
	var cfg config

	flag.StringVar(&cfg.kubeconfig, "kubeconfig", "", "Path to the kubeconfig file. Defaults to in-cluster config if not specified")
	flag.StringVar(&cfg.namespace, "namespace", "", "Namespace to inspect HPAs in. Leave empty for all namespaces")
	flag.DurationVar(&cfg.interval, "interval", 30*time.Second, "Polling interval for fetching HPA statistics")
	flag.StringVar(&cfg.hpaName, "hpa", "", "Name of a single HPA to inspect. Requires -namespace when set")

	flag.Parse()

	if cfg.interval <= 0 {
		log.Fatal("interval must be greater than zero")
	}

	if cfg.hpaName != "" && cfg.namespace == "" {
		log.Fatal("namespace must be specified when using -hpa")
	}

	return cfg
}
