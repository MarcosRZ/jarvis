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
	"strings"
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
	go app.runActionsSampler(ctx)

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

	mux.HandleFunc("/v1/actions", func(w http.ResponseWriter, r *http.Request) {
		if !allowGet(w, r) {
			return
		}
		writeJSON(w, http.StatusOK, app.Actions(r.Context()))
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

	mux.HandleFunc("/v1/actions/run", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var request struct {
			ID string `json:"id"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil || strings.TrimSpace(request.ID) == "" {
			writeError(w, http.StatusBadRequest, "invalid action request")
			return
		}
		action, err := app.RunAction(r.Context(), request.ID)
		if err != nil {
			switch {
			case errors.Is(err, ErrActionNotFound):
				writeError(w, http.StatusNotFound, "action not found")
			case errors.Is(err, ErrActionsUnavailable):
				writeError(w, http.StatusConflict, "action catalog unavailable")
			default:
				log.Printf("run action %s: %v", request.ID, err)
				writeError(w, http.StatusBadGateway, "action failed")
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"id":     action.ID,
			"name":   action.Name,
			"status": "run",
		})
	})

	mux.HandleFunc("/v1/services/{id}/restart", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		id := r.PathValue("id")
		restartCtx, cancel := context.WithTimeout(r.Context(), restartTimeout)
		defer cancel()
		if err := app.RestartService(restartCtx, id); err != nil {
			switch {
			case errors.Is(err, ErrServiceNotFound):
				writeError(w, http.StatusNotFound, "service not found")
			case errors.Is(err, ErrServiceRestartNotAllowed):
				writeError(w, http.StatusForbidden, "service cannot be restarted from the dashboard")
			default:
				log.Printf("restart %s: %v", id, err)
				writeError(w, http.StatusBadGateway, "restart failed")
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"service": id, "status": "restarted"})
	})

	return mux
}

var processStartedAt = time.Now().UTC()

// restartTimeout bounds a Docker restart request. Docker waits for the stop
// timeout and the start before it answers.
const restartTimeout = 45 * time.Second

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
