package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"apiservices/uptime-monitoring/internal/uptime/api"
	"apiservices/uptime-monitoring/internal/uptime/auth"
	"apiservices/uptime-monitoring/internal/uptime/monitor"
)

func main() {
	logger := log.New(os.Stdout, "[uptime] ", log.LstdFlags)

	port := envString("PORT", "30017")
	apiKey := envString("UPTIME_API_KEY", "dev-uptime-key")
	if apiKey == "dev-uptime-key" {
		logger.Println("UPTIME_API_KEY not set, using default development key")
	}

	pollSeconds := envInt("UPTIME_POLL_SECONDS", 5)
	pollInterval := time.Duration(pollSeconds) * time.Second

	store := monitor.NewStore()
	checker := monitor.NewChecker()
	notifier := monitor.NewWebhookNotifier(5 * time.Second)
	service := monitor.NewService(store, checker, notifier, pollInterval, logger)
	handler := api.NewHandler(store, service)

	mux := http.NewServeMux()
	mux.Handle("/v1/uptime/", auth.Middleware(apiKey)(handler))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go service.Start(ctx)

	go func() {
		logger.Printf("service listening on :%s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("server failed: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Printf("shutdown error: %v", err)
	}
}

func envString(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
