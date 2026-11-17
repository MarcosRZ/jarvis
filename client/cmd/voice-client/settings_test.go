package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchRemoteSettings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/settings/client" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"values":{"VOICE_ROOM":"Remote","VOICE_HA_TOKEN":"secret","PATH":"/tmp"}}`)
	}))
	defer server.Close()
	getenv := func(key string) string {
		if key == "VOICE_SETTINGS_URL" {
			return server.URL + "/api/settings/client"
		}
		return ""
	}
	got := fetchRemoteSettings(context.Background(), getenv, io.Discard)
	if got["VOICE_ROOM"] != "Remote" || got["VOICE_HA_TOKEN"] != "secret" {
		t.Fatalf("settings = %#v", got)
	}
	if _, ok := got["PATH"]; ok {
		t.Fatalf("a non-VOICE key was accepted: %#v", got)
	}
}

func TestFetchRemoteSettingsDisabled(t *testing.T) {
	getenv := func(key string) string {
		if key == "VOICE_SETTINGS_URL" {
			return "off"
		}
		return ""
	}
	if got := fetchRemoteSettings(context.Background(), getenv, io.Discard); got != nil {
		t.Fatalf("settings = %#v, want nil", got)
	}
}

func TestFetchRemoteSettingsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	getenv := func(key string) string {
		if key == "VOICE_SETTINGS_URL" {
			return server.URL
		}
		return ""
	}
	if got := fetchRemoteSettings(context.Background(), getenv, io.Discard); got != nil {
		t.Fatalf("settings = %#v, want nil", got)
	}
}
