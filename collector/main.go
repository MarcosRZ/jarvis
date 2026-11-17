package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}

	cfg := loadConfig()
	docker := newDockerClient(cfg.DockerSocket)
	home := newHomeAssistantClient(cfg.HomeAssistant, 3*time.Second)
	app := newCollector(cfg, docker, home)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go app.watchEvents(ctx)

	warmCtx, cancelWarm := context.WithTimeout(ctx, 10*time.Second)
	app.storeSnapshot(app.buildSnapshot(warmCtx))
	cancelWarm()
	go app.runSampler(ctx)

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           routes(app),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Printf("collector listening on %s", cfg.ListenAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdown)
}

func routes(app *collector) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/health", func(w http.ResponseWriter, r *http.Request) {
		if !allowGet(w, r) {
			return
		}
		healthCtx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		docker := SourceStatus{State: SourceOK}
		status := "ok"
		if err := app.docker.Ping(healthCtx); err != nil {
			docker = SourceStatus{State: SourceUnavailable, Message: ptr("Docker API is not available.")}
			status = "degraded"
		}
		response := map[string]any{
			"status":        status,
			"startedAt":     processStartedAt.Format(time.RFC3339Nano),
			"uptimeSeconds": int64(time.Since(processStartedAt).Seconds()),
			"sources": SnapshotSources{
				Docker:        docker,
				HomeAssistant: SourceStatus{State: SourceDisabled},
			},
		}
		code := http.StatusOK
		if status != "ok" {
			code = http.StatusServiceUnavailable
		}
		writeJSON(w, code, response)
	})

	mux.HandleFunc("/v1/snapshot", func(w http.ResponseWriter, r *http.Request) {
		if !allowGet(w, r) {
			return
		}
		writeJSON(w, http.StatusOK, app.Snapshot(r.Context()))
	})

	mux.HandleFunc("/v1/logs", func(w http.ResponseWriter, r *http.Request) {
		if !allowGet(w, r) {
			return
		}
		options := logOptions{
			Cursor:  r.URL.Query().Get("cursor"),
			Service: r.URL.Query().Get("service"),
			Level:   r.URL.Query().Get("level"),
		}
		if raw := r.URL.Query().Get("limit"); raw != "" {
			limit, err := strconv.Atoi(raw)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid limit")
				return
			}
			options.Limit = limit
		}
		response, err := app.Logs(r.Context(), options)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, response)
	})

	return mux
}

var processStartedAt = time.Now().UTC()

func healthcheck() int {
	url := env("COLLECTOR_HEALTHCHECK_URL", "http://127.0.0.1:8090/v1/health")
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(url)
	if err != nil {
		log.Print(err)
		return 1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		log.Printf("healthcheck: %s", response.Status)
		return 1
	}
	return 0
}

func allowGet(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}
	w.Header().Set("Allow", "GET, HEAD")
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	return false
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if status == http.StatusNoContent {
		return
	}
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, ErrorEnvelope{Error: message})
}
