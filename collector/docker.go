package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type dockerContainer struct {
	ID     string            `json:"Id"`
	Names  []string          `json:"Names"`
	Image  string            `json:"Image"`
	State  string            `json:"State"`
	Status string            `json:"Status"`
	Labels map[string]string `json:"Labels"`
}

type dockerInspect struct {
	ID     string `json:"Id"`
	Name   string `json:"Name"`
	Config struct {
		Image string `json:"Image"`
		Tty   bool   `json:"Tty"`
	} `json:"Config"`
	State struct {
		Status    string `json:"Status"`
		Running   bool   `json:"Running"`
		StartedAt string `json:"StartedAt"`
		Health    *struct {
			Status string `json:"Status"`
		} `json:"Health"`
	} `json:"State"`
	HostConfig struct {
		Memory int64 `json:"Memory"`
	} `json:"HostConfig"`
	NetworkSettings struct {
		Ports map[string][]struct {
			HostIP   string `json:"HostIp"`
			HostPort string `json:"HostPort"`
		} `json:"Ports"`
	} `json:"NetworkSettings"`
}

type dockerStats struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage  uint64   `json:"total_usage"`
			PercpuUsage []uint64 `json:"percpu_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs     uint32 `json:"online_cpus"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
	} `json:"memory_stats"`
}

type dockerEvent struct {
	Type   string `json:"Type"`
	Action string `json:"Action"`
	Actor  struct {
		ID         string            `json:"ID"`
		Attributes map[string]string `json:"Attributes"`
	} `json:"Actor"`
	TimeNano int64 `json:"timeNano"`
}

type rawLogLine struct {
	Time    time.Time
	Stream  string
	Message string
}

type dockerClient interface {
	Ping(ctx context.Context) error
	ListContainers(ctx context.Context) ([]dockerContainer, error)
	InspectContainer(ctx context.Context, id string) (dockerInspect, error)
	ContainerStats(ctx context.Context, id string) (dockerStats, error)
	ContainerLogs(ctx context.Context, id string, since time.Time, tail int) ([]rawLogLine, error)
	Events(ctx context.Context, since time.Time, handle func(dockerEvent)) error
}

type httpDockerClient struct {
	base   string
	client *http.Client

	ttyMu    sync.Mutex
	ttyCache map[string]bool
	knownTTY map[string]bool
}

func newDockerClient(socket string) *httpDockerClient {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socket)
		},
		MaxIdleConns:      4,
		IdleConnTimeout:   30 * time.Second,
		DisableKeepAlives: false,
	}
	return &httpDockerClient{
		base:     "http://docker",
		client:   &http.Client{Transport: transport},
		ttyCache: map[string]bool{},
		knownTTY: map[string]bool{},
	}
}

func (c *httpDockerClient) get(ctx context.Context, path string, out any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return fmt.Errorf("docker %s: %s: %s", path, response.Status, strings.TrimSpace(string(body)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(response.Body, 8<<20)).Decode(out)
}

func (c *httpDockerClient) Ping(ctx context.Context) error {
	return c.get(ctx, "/_ping", nil)
}

func (c *httpDockerClient) ListContainers(ctx context.Context) ([]dockerContainer, error) {
	var containers []dockerContainer
	if err := c.get(ctx, "/containers/json?all=1", &containers); err != nil {
		return nil, err
	}
	return containers, nil
}

func (c *httpDockerClient) InspectContainer(ctx context.Context, id string) (dockerInspect, error) {
	var inspect dockerInspect
	err := c.get(ctx, "/containers/"+url.PathEscape(id)+"/json", &inspect)
	return inspect, err
}

func (c *httpDockerClient) ContainerStats(ctx context.Context, id string) (dockerStats, error) {
	var stats dockerStats
	err := c.get(ctx, "/containers/"+url.PathEscape(id)+"/stats?stream=false&one-shot=true", &stats)
	return stats, err
}

func (c *httpDockerClient) ContainerLogs(ctx context.Context, id string, since time.Time, tail int) ([]rawLogLine, error) {
	query := url.Values{}
	query.Set("stdout", "1")
	query.Set("stderr", "1")
	query.Set("timestamps", "1")
	if tail > 0 {
		query.Set("tail", strconv.Itoa(tail))
	}
	if !since.IsZero() {
		query.Set("since", since.UTC().Format(time.RFC3339Nano))
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/containers/"+url.PathEscape(id)+"/logs?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("docker logs: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	tty := c.tty(ctx, id)
	return parseLogStream(data, tty)
}

func (c *httpDockerClient) tty(ctx context.Context, id string) bool {
	c.ttyMu.Lock()
	if c.knownTTY[id] {
		value := c.ttyCache[id]
		c.ttyMu.Unlock()
		return value
	}
	c.ttyMu.Unlock()

	inspect, err := c.InspectContainer(ctx, id)
	if err != nil {
		return false
	}
	c.ttyMu.Lock()
	c.ttyCache[id] = inspect.Config.Tty
	c.knownTTY[id] = true
	c.ttyMu.Unlock()
	return inspect.Config.Tty
}

func (c *httpDockerClient) Events(ctx context.Context, since time.Time, handle func(dockerEvent)) error {
	query := url.Values{}
	query.Set("filters", `{"type":["container"]}`)
	if !since.IsZero() {
		query.Set("since", since.UTC().Format(time.RFC3339Nano))
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/events?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("docker events: %s", response.Status)
	}
	decoder := json.NewDecoder(response.Body)
	for {
		var event dockerEvent
		if err := decoder.Decode(&event); err != nil {
			if errors.Is(err, io.EOF) || ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		handle(event)
	}
}

func parseLogStream(data []byte, tty bool) ([]rawLogLine, error) {
	if tty {
		return parseLogPayload(data, "")
	}
	var lines []rawLogLine
	reader := bytes.NewReader(data)
	for reader.Len() >= 8 {
		header := make([]byte, 8)
		if _, err := io.ReadFull(reader, header); err != nil {
			return nil, err
		}
		size := binary.BigEndian.Uint32(header[4:8])
		if size == 0 {
			continue
		}
		payload := make([]byte, size)
		if _, err := io.ReadFull(reader, payload); err != nil {
			return nil, err
		}
		stream := ""
		switch header[0] {
		case 1:
			stream = "stdout"
		case 2:
			stream = "stderr"
		}
		parsed, err := parseLogPayload(payload, stream)
		if err != nil {
			return nil, err
		}
		lines = append(lines, parsed...)
	}
	if reader.Len() > 0 {
		rest := make([]byte, reader.Len())
		_, _ = io.ReadFull(reader, rest)
		parsed, err := parseLogPayload(rest, "")
		if err != nil {
			return nil, err
		}
		lines = append(lines, parsed...)
	}
	return lines, nil
}

func parseLogPayload(data []byte, stream string) ([]rawLogLine, error) {
	var lines []rawLogLine
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		text := scanner.Text()
		if text == "" {
			continue
		}
		timestamp, message := splitLogTimestamp(text)
		lines = append(lines, rawLogLine{Time: timestamp, Stream: stream, Message: message})
	}
	return lines, scanner.Err()
}

func splitLogTimestamp(text string) (time.Time, string) {
	space := strings.IndexByte(text, ' ')
	if space <= 0 {
		return time.Now().UTC(), text
	}
	timestamp, err := time.Parse(time.RFC3339Nano, text[:space])
	if err != nil {
		return time.Now().UTC(), text
	}
	return timestamp.UTC(), text[space+1:]
}
