package main

import (
	"context"
	"encoding/base64"
	"errors"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	tokenPattern  = regexp.MustCompile(`(?i)(token|authorization|bearer)([=: ]+)([^\s&]+)`)
	secretPattern = regexp.MustCompile(`(?i)(VOICE_HA_TOKEN\s*[=:]\s*)(\S+)`)
)

type logOptions struct {
	Cursor  string
	Limit   int
	Service string
	Level   string
}

func (c *collector) Logs(ctx context.Context, options logOptions) (LogsResponse, error) {
	now := time.Now().UTC()
	response := LogsResponse{
		GeneratedAt: now.Format(time.RFC3339Nano),
		Entries:     []LogEntry{},
		Sources: LogSources{
			Docker:       SourceStatus{State: SourceOK},
			AssistantLog: SourceStatus{State: SourceDisabled},
		},
	}

	limit := options.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}

	var cursor time.Time
	if options.Cursor != "" {
		parsed, err := decodeCursor(options.Cursor)
		if err != nil {
			return response, err
		}
		cursor = parsed
	}

	if options.Service != "" {
		if _, ok := registryByID(options.Service); !ok {
			return response, errors.New("invalid service")
		}
	}
	switch options.Level {
	case "", "info", "warn", "error", "debug":
	default:
		return response, errors.New("invalid level")
	}

	containers, err := c.docker.ListContainers(ctx)
	if err != nil {
		response.Sources.Docker = SourceStatus{State: SourceUnavailable, Message: ptr("Docker API is not available.")}
		if options.Cursor != "" {
			response.NextCursor = &options.Cursor
		}
		return response, nil
	}
	response.Sources.Docker.LastSuccessAt = ptr(now.Format(time.RFC3339Nano))

	type target struct {
		id      string
		name    string
		service string
	}
	var targets []target
	for _, container := range containers {
		if c.cfg.ComposeProject != "" && container.Labels["com.docker.compose.project"] != c.cfg.ComposeProject {
			continue
		}
		service := container.Labels["com.docker.compose.service"]
		if _, known := registryByID(service); !known {
			continue
		}
		if options.Service != "" && options.Service != service {
			continue
		}
		targets = append(targets, target{id: container.ID, name: containerName(container), service: service})
	}

	type result struct {
		service string
		lines   []rawLogLine
		err     error
	}
	results := make(chan result, len(targets))
	semaphore := make(chan struct{}, 8)
	var wait sync.WaitGroup
	for _, item := range targets {
		wait.Add(1)
		go func(item target) {
			defer wait.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			logCtx, cancel := context.WithTimeout(ctx, c.cfg.RequestTimeout)
			defer cancel()
			tail := c.cfg.LogTail
			if cursor.IsZero() && limit > tail {
				tail = limit
			}
			lines, err := c.docker.ContainerLogs(logCtx, item.id, cursor, tail)
			results <- result{service: item.service, lines: lines, err: err}
		}(item)
	}
	wait.Wait()
	close(results)

	for item := range results {
		if item.err != nil {
			response.Sources.Docker.State = SourceDegraded
			continue
		}
		for index, line := range item.lines {
			entry := LogEntry{
				ID:      item.service + ":" + line.Time.UTC().Format(time.RFC3339Nano) + ":" + itoa(index),
				Time:    line.Time.UTC().Format(time.RFC3339Nano),
				Level:   inferLevel(line.Message),
				Service: item.service,
				Message: redact(capMessage(line.Message)),
			}
			if !cursor.IsZero() && !entryTimeAfter(entry.Time, cursor) {
				continue
			}
			if options.Service != "" && entry.Service != options.Service {
				continue
			}
			if options.Level != "" && entry.Level != options.Level {
				continue
			}
			response.Entries = append(response.Entries, entry)
		}
	}

	sort.Slice(response.Entries, func(i, j int) bool {
		if response.Entries[i].Time == response.Entries[j].Time {
			return response.Entries[i].ID < response.Entries[j].ID
		}
		return response.Entries[i].Time < response.Entries[j].Time
	})

	if cursor.IsZero() && len(response.Entries) > limit {
		response.Entries = response.Entries[len(response.Entries)-limit:]
		response.HasMore = true
	}
	if len(response.Entries) > limit {
		response.Entries = response.Entries[:limit]
		response.HasMore = true
	}

	if len(response.Entries) > 0 {
		cursorValue := encodeCursor(response.Entries[len(response.Entries)-1].Time)
		response.NextCursor = &cursorValue
	} else if options.Cursor != "" {
		value := options.Cursor
		response.NextCursor = &value
	}

	return response, nil
}

func containerName(container dockerContainer) string {
	for _, name := range container.Names {
		name = strings.TrimPrefix(name, "/")
		if name != "" {
			return name
		}
	}
	return container.ID
}

func encodeCursor(timestamp string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(timestamp))
}

func decodeCursor(value string) (time.Time, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return time.Time{}, errors.New("invalid cursor")
	}
	parsed, err := time.Parse(time.RFC3339Nano, string(raw))
	if err != nil {
		return time.Time{}, errors.New("invalid cursor")
	}
	return parsed.UTC(), nil
}

func entryTimeAfter(value string, cursor time.Time) bool {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return false
	}
	return parsed.After(cursor)
}

func inferLevel(message string) string {
	lower := strings.ToLower(message)
	switch {
	case containsAny(lower, "panic", "fatal", "error", "failed", "failure"):
		return "error"
	case containsAny(lower, "warn", "warning"):
		return "warn"
	case containsAny(lower, "debug", "trace"):
		return "debug"
	default:
		return "info"
	}
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func redact(message string) string {
	message = tokenPattern.ReplaceAllString(message, "$1$2[redacted]")
	message = secretPattern.ReplaceAllString(message, "$1[redacted]")
	return message
}

func capMessage(message string) string {
	const max = 4096
	if len(message) <= max {
		return message
	}
	return message[:max-3] + "..."
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	position := len(digits)
	for value > 0 {
		position--
		digits[position] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[position:])
}
