package main

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type alertTracker struct {
	mu      sync.Mutex
	active  map[string]Notification
	seen    map[string]ServiceStatus
	sources map[string]SourceState
	ring    []Notification
}

func newAlertTracker() *alertTracker {
	return &alertTracker{
		active:  map[string]Notification{},
		seen:    map[string]ServiceStatus{},
		sources: map[string]SourceState{},
	}
}

func (t *alertTracker) Update(services []Service, sources SnapshotSources, now time.Time) []Notification {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, service := range services {
		if definition, ok := registryByID(service.ID); ok && definition.ExternalHA {
			continue
		}
		previous, known := t.seen[service.ID]
		t.seen[service.ID] = service.Status
		switch service.Status {
		case StatusOffline:
			key := "stopped:" + service.ID
			if !known || previous != StatusOffline {
				t.raise(key, "error", service.Name+" stopped", "The container is not running.", now)
			}
		case StatusDegraded:
			key := "unhealthy:" + service.ID
			if !known || previous != StatusDegraded {
				t.raise(key, "warning", service.Name+" is unhealthy", "The container health check is failing.", now)
			}
		case StatusOnline:
			t.recover("stopped:"+service.ID, service.Name+" is running again", "The container is running.", now)
			t.recover("unhealthy:"+service.ID, service.Name+" is healthy again", "The container health check passed.", now)
		}
	}

	t.updateSource("docker", sources.Docker.State, "Docker API unavailable", "The collector cannot reach the Docker API.", now)
	t.updateSource("homeassistant", sources.HomeAssistant.State, "Home Assistant unavailable", "The Home Assistant API did not respond.", now)

	return append([]Notification(nil), t.ring...)
}

func (t *alertTracker) updateSource(key string, state SourceState, title, description string, now time.Time) {
	previous, known := t.sources[key]
	t.sources[key] = state
	if state == SourceUnavailable {
		if !known || previous != SourceUnavailable {
			t.raise("source:"+key, "error", title, description, now)
		}
		return
	}
	if state == SourceOK {
		t.recover("source:"+key, "Source available again", "The data source responded.", now)
	}
}

func (t *alertTracker) raise(key, alertType, title, description string, now time.Time) {
	if _, ok := t.active[key]; ok {
		return
	}
	timestamp := now.UTC().Format(time.RFC3339Nano)
	notification := Notification{
		ID:          fmt.Sprintf("alert:%s:%s", alertType, strings.ReplaceAll(timestamp, ":", "-")),
		Title:       title,
		Description: description,
		Time:        timestamp,
		Type:        alertType,
		Read:        false,
	}
	t.active[key] = notification
	t.ring = append([]Notification{notification}, t.ring...)
	t.trim()
}

func (t *alertTracker) recover(key, title, description string, now time.Time) {
	if _, ok := t.active[key]; !ok {
		return
	}
	delete(t.active, key)
	timestamp := now.UTC().Format(time.RFC3339Nano)
	t.ring = append([]Notification{{
		ID:          fmt.Sprintf("alert:success:%s", strings.ReplaceAll(timestamp, ":", "-")),
		Title:       title,
		Description: description,
		Time:        timestamp,
		Type:        "success",
		Read:        false,
	}}, t.ring...)
	t.trim()
}

func (t *alertTracker) trim() {
	if len(t.ring) <= 100 {
		return
	}
	t.ring = t.ring[:100]
}
