package main

import (
	"os"
	"strconv"
	"time"
)

type config struct {
	ListenAddr     string
	DockerSocket   string
	ComposeProject string
	HomeAssistant  homeAssistantConfig
	RequestTimeout time.Duration
	LogTail        int
}

type homeAssistantConfig struct {
	URL   string
	Token string
}

func loadConfig() config {
	return config{
		ListenAddr:     env("COLLECTOR_LISTEN_ADDR", ":8090"),
		DockerSocket:   env("COLLECTOR_DOCKER_SOCKET", "/var/run/docker.sock"),
		ComposeProject: env("COLLECTOR_COMPOSE_PROJECT", "voice-assistant"),
		HomeAssistant: homeAssistantConfig{
			URL:   os.Getenv("VOICE_HA_URL"),
			Token: os.Getenv("VOICE_HA_TOKEN"),
		},
		RequestTimeout: time.Duration(envInt("COLLECTOR_REQUEST_TIMEOUT_MS", 4000)) * time.Millisecond,
		LogTail:        envInt("COLLECTOR_LOG_TAIL", 200),
	}
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envInt(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
