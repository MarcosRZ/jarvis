// Package textinput adds a text channel to the assistant: other programs send
// a sentence over HTTP and get the reply that the voice channel would speak.
package textinput

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	// maxTextBytes bounds one request, so a bad client cannot exhaust memory.
	maxTextBytes = 4 << 10
	// maxReplyBytes bounds what Send reads from a reply.
	maxReplyBytes = 64 << 10
)

type request struct {
	Text string `json:"text"`
}

type response struct {
	Reply string `json:"reply"`
}

// Submit handles one text command and returns the reply. It may block while
// the assistant handles a voice command.
type Submit func(ctx context.Context, text string) (string, error)

// Server serves POST /text until its context ends.
type Server struct {
	addr   string
	submit Submit
	logger *slog.Logger
}

func NewServer(addr string, submit Submit, logger *slog.Logger) *Server {
	return &Server{addr: addr, submit: submit, logger: logger}
}

// Start listens on addr and serves in the background. It returns a busy-port
// error at once, like logging.Serve.
func (s *Server) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("text input: %w", err)
	}
	srv := &http.Server{
		Handler:           s,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) && s.logger != nil {
			s.logger.Warn("text input stopped", "error", err)
		}
	}()
	return nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/text" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "use POST", http.StatusMethodNotAllowed)
		return
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxTextBytes))
	var req request
	if err := dec.Decode(&req); err != nil {
		http.Error(w, `invalid JSON body; use {"text": "..."}`, http.StatusBadRequest)
		return
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		http.Error(w, "text is empty", http.StatusBadRequest)
		return
	}
	reply, err := s.submit(r.Context(), text)
	if err != nil {
		code := http.StatusServiceUnavailable
		if errors.Is(err, context.DeadlineExceeded) {
			code = http.StatusGatewayTimeout
		}
		http.Error(w, err.Error(), code)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(response{Reply: reply})
}

// endpoint returns the URL for addr. An unspecified host such as 0.0.0.0 or ::
// means the local computer, which is where the text command is usually run.
func endpoint(addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", err
	}
	switch host {
	case "", "0.0.0.0":
		host = "127.0.0.1"
	case "::":
		host = "::1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/text", nil
}

// Send posts text to a running assistant and returns its reply.
func Send(ctx context.Context, addr, text string) (string, error) {
	url, err := endpoint(addr)
	if err != nil {
		return "", fmt.Errorf("text input address %q: %w", addr, err)
	}
	body, err := json.Marshal(request{Text: text})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("text input at %s: %w", addr, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxReplyBytes))
	if err != nil {
		return "", fmt.Errorf("text input at %s: %w", addr, err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("text input at %s: %s", addr, strings.TrimSpace(string(data)))
	}
	var out response
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("text input at %s: invalid reply: %w", addr, err)
	}
	return out.Reply, nil
}
