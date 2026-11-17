package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type activityRing struct {
	mu     sync.Mutex
	limit  int
	events []ActivityEvent
	seen   map[string]struct{}
}

func newActivityRing(limit int) *activityRing {
	return &activityRing{limit: limit, seen: map[string]struct{}{}}
}

func (r *activityRing) Add(event ActivityEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.seen[event.ID]; ok {
		return
	}
	r.seen[event.ID] = struct{}{}
	r.events = append([]ActivityEvent{event}, r.events...)
	if len(r.events) > r.limit {
		for _, removed := range r.events[r.limit:] {
			delete(r.seen, removed.ID)
		}
		r.events = r.events[:r.limit]
	}
}

func (r *activityRing) List() []ActivityEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]ActivityEvent(nil), r.events...)
}

func (c *collector) watchEvents(ctx context.Context) {
	backoff := time.Second
	since := time.Now().UTC().Add(-time.Hour)
	for {
		if ctx.Err() != nil {
			return
		}
		err := c.docker.Events(ctx, since, func(event dockerEvent) {
			since = time.Unix(0, event.TimeNano).UTC()
			c.handleEvent(event)
		})
		if ctx.Err() != nil {
			return
		}
		_ = err
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (c *collector) handleEvent(event dockerEvent) {
	if event.Type != "container" {
		return
	}
	if c.cfg.ComposeProject != "" && event.Actor.Attributes["com.docker.compose.project"] != c.cfg.ComposeProject {
		return
	}
	serviceID := event.Actor.Attributes["com.docker.compose.service"]
	definition, ok := registryByID(serviceID)
	if !ok {
		return
	}
	action := event.Action
	title := fmt.Sprintf("%s %s", definition.ID, strings.ReplaceAll(action, ":", " "))
	description := eventDescription(action)
	if description == "" {
		return
	}
	when := time.Unix(0, event.TimeNano).UTC().Format(time.RFC3339Nano)
	c.activity.Add(ActivityEvent{
		ID:          fmt.Sprintf("event:%d:%s:%s", event.TimeNano, serviceID, action),
		Title:       title,
		Description: description,
		Time:        when,
	})
}

func eventDescription(action string) string {
	switch {
	case action == "start":
		return "Container started."
	case action == "die" || action == "stop" || action == "kill":
		return "Container stopped."
	case action == "restart":
		return "Container restarted."
	case action == "create":
		return "Container created."
	case action == "destroy":
		return "Container removed."
	case strings.Contains(action, "health_status"):
		if strings.Contains(action, "unhealthy") {
			return "Health check failed."
		}
		return "Health check is healthy."
	default:
		return ""
	}
}
