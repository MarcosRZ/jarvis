package textinput

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

func TestSend(t *testing.T) {
	addr := freeAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := NewServer(addr, func(_ context.Context, text string) (string, error) {
		return "reply to " + text, nil
	}, testLogger())
	if err := server.Start(ctx); err != nil {
		t.Fatal(err)
	}
	reply, err := Send(ctx, addr, "¿Qué hora es?")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if reply != "reply to ¿Qué hora es?" {
		t.Errorf("reply %q", reply)
	}

	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	if reply, err := Send(ctx, "0.0.0.0:"+port, "hola"); err != nil || reply != "reply to hola" {
		t.Errorf("Send to an unspecified host: reply %q, error %v", reply, err)
	}
}

func TestServerRejectsBadRequests(t *testing.T) {
	submit := func(context.Context, string) (string, error) { return "ok", nil }
	server := httptest.NewServer(NewServer("", submit, testLogger()))
	defer server.Close()

	post := func(path, body string) *http.Response {
		t.Helper()
		resp, err := http.Post(server.URL+path, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	tests := []struct {
		name string
		resp *http.Response
		code int
	}{
		{"not JSON", post("/text", "hola"), http.StatusBadRequest},
		{"empty text", post("/text", `{"text":"  "}`), http.StatusBadRequest},
		{"wrong path", post("/other", `{"text":"hola"}`), http.StatusNotFound},
	}
	for _, tt := range tests {
		tt.resp.Body.Close()
		if tt.resp.StatusCode != tt.code {
			t.Errorf("%s: status %d, want %d", tt.name, tt.resp.StatusCode, tt.code)
		}
	}
	resp, err := http.Get(server.URL + "/text")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed || resp.Header.Get("Allow") != http.MethodPost {
		t.Errorf("GET status %d, Allow %q", resp.StatusCode, resp.Header.Get("Allow"))
	}
}

func TestServerReportsSubmitErrors(t *testing.T) {
	tests := []struct {
		err  error
		code int
	}{
		{errors.New("stopped"), http.StatusServiceUnavailable},
		{context.DeadlineExceeded, http.StatusGatewayTimeout},
	}
	for _, tt := range tests {
		server := httptest.NewServer(NewServer("", func(context.Context, string) (string, error) {
			return "", tt.err
		}, testLogger()))
		body, _ := json.Marshal(request{Text: "hola"})
		resp, err := http.Post(server.URL+"/text", "application/json", strings.NewReader(string(body)))
		if err != nil {
			server.Close()
			t.Fatal(err)
		}
		resp.Body.Close()
		server.Close()
		if resp.StatusCode != tt.code {
			t.Errorf("%v: status %d, want %d", tt.err, resp.StatusCode, tt.code)
		}
	}
}
