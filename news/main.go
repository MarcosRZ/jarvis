// Package main runs the news service: it reads a fixed list of RSS and Atom
// feeds, keeps the items in memory only, and serves them to the dashboard and
// the voice client.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := env("NEWS_LISTEN_ADDR", ":8091")
	configFile := env("NEWS_CONFIG_FILE", "/data/news.json")
	service, err := NewService(ServiceOptions{
		ConfigFile: configFile,
		Logger:     logger,
	})
	if err != nil {
		logger.Error("configuration", "error", err)
		os.Exit(1)
	}
	if err := service.Start(ctx); err != nil {
		logger.Error("start", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/news", service.handleNews)
	mux.HandleFunc("/items", service.handleItems)
	mux.HandleFunc("/status", service.handleStatus)
	mux.HandleFunc("/config", service.handleConfig)
	mux.HandleFunc("/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	logger.Info("news service started", "addr", addr, "config", configFile, "feeds", len(service.Config().Feeds))
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("serve", "error", err)
		os.Exit(1)
	}
}

// healthcheck is the compose healthcheck entry point.
func healthcheck() int {
	url := env("NEWS_HEALTHCHECK_URL", "http://127.0.0.1:8091/v1/health")
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck:", response.Status)
		return 1
	}
	return 0
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
