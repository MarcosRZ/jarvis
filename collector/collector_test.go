package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

type fakeDocker struct {
	containers []dockerContainer
	inspects   map[string]dockerInspect
	stats      map[string]dockerStats
	logs       map[string][]rawLogLine
	logErr     error
	listErr    error
	pingErr    error
}

func (f *fakeDocker) Ping(context.Context) error {
	return f.pingErr
}

func (f *fakeDocker) ListContainers(context.Context) ([]dockerContainer, error) {
	return f.containers, f.listErr
}

func (f *fakeDocker) InspectContainer(_ context.Context, id string) (dockerInspect, error) {
	return f.inspects[id], nil
}

func (f *fakeDocker) ContainerStats(_ context.Context, id string) (dockerStats, error) {
	return f.stats[id], nil
}

func (f *fakeDocker) ContainerLogs(_ context.Context, id string, _ time.Time, _ int) ([]rawLogLine, error) {
	if f.logErr != nil {
		return nil, f.logErr
	}
	return f.logs[id], nil
}

func (f *fakeDocker) Events(context.Context, time.Time, func(dockerEvent)) error {
	return nil
}

func TestStatusFromContainer(t *testing.T) {
	cases := []struct {
		state  string
		health string
		want   ServiceStatus
	}{
		{"running", "healthy", StatusOnline},
		{"running", "starting", StatusRestarting},
		{"running", "unhealthy", StatusDegraded},
		{"running", "", StatusOnline},
		{"restarting", "", StatusRestarting},
		{"exited", "", StatusOffline},
		{"dead", "", StatusOffline},
		{"", "", StatusOffline},
	}
	for _, test := range cases {
		if got := statusFromContainer(test.state, test.health); got != test.want {
			t.Errorf("statusFromContainer(%q, %q) = %q, want %q", test.state, test.health, got, test.want)
		}
	}
}

func TestCPUPercent(t *testing.T) {
	stats := dockerStats{}
	stats.PreCPUStats.CPUUsage.TotalUsage = 100
	stats.CPUStats.CPUUsage.TotalUsage = 300
	stats.PreCPUStats.SystemCPUUsage = 1000
	stats.CPUStats.SystemCPUUsage = 2000
	stats.CPUStats.OnlineCPUs = 2
	percent := cpuPercent(stats)
	if percent == nil {
		t.Fatal("cpuPercent returned nil")
	}
	if got, want := *percent, 40.0; got != want {
		t.Errorf("cpuPercent = %v, want %v", got, want)
	}

	empty := cpuPercent(dockerStats{})
	if empty != nil {
		t.Errorf("cpuPercent without deltas = %v, want nil", *empty)
	}
}

func TestSelectPort(t *testing.T) {
	var inspect dockerInspect
	inspect.NetworkSettings.Ports = map[string][]struct {
		HostIP   string `json:"HostIp"`
		HostPort string `json:"HostPort"`
	}{}
	inspect.NetworkSettings.Ports["3000/tcp"] = []struct {
		HostIP   string `json:"HostIp"`
		HostPort string `json:"HostPort"`
	}{{HostIP: "127.0.0.1", HostPort: "3005"}}
	inspect.NetworkSettings.Ports["9000/tcp"] = []struct {
		HostIP   string `json:"HostIp"`
		HostPort string `json:"HostPort"`
	}{{HostIP: "127.0.0.1", HostPort: "9001"}}

	port := selectPort(inspect, 3000)
	if port == nil || *port != 3005 {
		t.Fatalf("selectPort = %v, want 3005", port)
	}

	none := selectPort(dockerInspect{}, 3000)
	if none != nil {
		t.Errorf("selectPort without bindings = %v, want nil", *none)
	}
}

func TestParseLogStreamMultiplexed(t *testing.T) {
	payload := append(frame(1, []byte("2026-11-11T20:00:00Z hello\n")), frame(2, []byte("2026-11-11T20:00:01Z warning: careful\n"))...)
	lines, err := parseLogStream(payload, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if lines[0].Message != "hello" || lines[1].Message != "warning: careful" {
		t.Fatalf("unexpected messages: %+v", lines)
	}
	if lines[0].Time.UTC().Format(time.RFC3339) != "2026-11-11T20:00:00Z" {
		t.Fatalf("unexpected timestamp: %v", lines[0].Time)
	}
}

func TestParseLogStreamTTY(t *testing.T) {
	lines, err := parseLogStream([]byte("2026-11-11T20:00:00Z plain output\n"), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0].Message != "plain output" {
		t.Fatalf("unexpected lines: %+v", lines)
	}
}

func frame(stream byte, payload []byte) []byte {
	header := []byte{stream, 0, 0, 0, byte(len(payload) >> 24), byte(len(payload) >> 16), byte(len(payload) >> 8), byte(len(payload))}
	return append(header, payload...)
}

func TestInferLevelAndRedact(t *testing.T) {
	cases := map[string]string{
		"all good":            "info",
		"WARNING low memory":  "warn",
		"ERROR failed to run": "error",
		"debug trace":         "debug",
	}
	for message, want := range cases {
		if got := inferLevel(message); got != want {
			t.Errorf("inferLevel(%q) = %q, want %q", message, got, want)
		}
	}
	redacted := redact("Authorization: Bearer abc123 token=secret")
	if redacted == "Authorization: Bearer abc123 token=secret" {
		t.Fatal("redact did not change the message")
	}
}

func TestSnapshotBuild(t *testing.T) {
	now := time.Now().UTC()
	started := now.Add(-2 * time.Hour).Format(time.RFC3339Nano)
	whisper := inspectFor(started, "healthy", 3000, 3005)
	piper := inspectFor(started, "", 0, 0)
	piper.State.Status = "exited"
	piper.State.Running = false
	fake := &fakeDocker{
		containers: []dockerContainer{
			{ID: "w1", Image: "whisper:1", State: "running", Labels: labels("whisper")},
			{ID: "p1", Image: "piper:1", State: "exited", Labels: labels("piper")},
		},
		inspects: map[string]dockerInspect{
			"w1": whisper,
			"p1": piper,
		},
		stats: map[string]dockerStats{
			"w1": statsFor(100, 300, 1000, 2000, 2, 512<<20, 1024<<20),
		},
	}
	cfg := config{ComposeProject: "voice-assistant", RequestTimeout: time.Second, LogTail: 10}
	app := newCollector(cfg, fake, nil)
	snapshot := app.buildSnapshot(context.Background())

	if len(snapshot.Services) != len(serviceRegistry) {
		t.Fatalf("got %d services, want %d", len(snapshot.Services), len(serviceRegistry))
	}
	statuses := map[string]ServiceStatus{}
	for _, service := range snapshot.Services {
		statuses[service.ID] = service.Status
	}
	if statuses["whisper"] != StatusOnline {
		t.Errorf("whisper status = %q, want online", statuses["whisper"])
	}
	if statuses["piper"] != StatusOffline {
		t.Errorf("piper status = %q, want offline", statuses["piper"])
	}
	if snapshot.Metrics.Services.Total != len(serviceRegistry)-1 ||
		snapshot.Metrics.Services.Active != 1 ||
		snapshot.Metrics.Services.Stopped != len(serviceRegistry)-2 {
		t.Errorf("unexpected service metrics: %+v", snapshot.Metrics.Services)
	}
	if snapshot.Metrics.Memory.UsedMb == nil || *snapshot.Metrics.Memory.UsedMb != 512 {
		t.Errorf("unexpected memory metrics: %+v", snapshot.Metrics.Memory)
	}
	if len(snapshot.Notifications) == 0 {
		t.Error("expected derived notifications for offline services")
	}
	if snapshot.Sources.Docker.State != SourceOK {
		t.Errorf("docker source = %q, want ok", snapshot.Sources.Docker.State)
	}
}

func TestSnapshotKeepsLastGoodDataWhenDockerFails(t *testing.T) {
	now := time.Now().UTC()
	started := now.Add(-time.Hour).Format(time.RFC3339Nano)
	fake := &fakeDocker{
		containers: []dockerContainer{
			{ID: "w1", Image: "whisper:1", State: "running", Labels: labels("whisper")},
		},
		inspects: map[string]dockerInspect{"w1": inspectFor(started, "healthy", 0, 0)},
		stats:    map[string]dockerStats{"w1": statsFor(0, 100, 0, 1000, 1, 1<<20, 0)},
	}
	app := newCollector(config{ComposeProject: "voice-assistant", RequestTimeout: time.Second, LogTail: 10}, fake, nil)
	good := app.buildSnapshot(context.Background())
	fake.listErr = context.DeadlineExceeded

	stale := app.buildSnapshot(context.Background())
	if stale.Sources.Docker.State != SourceUnavailable {
		t.Fatalf("docker source = %q, want unavailable", stale.Sources.Docker.State)
	}
	if stale.GeneratedAt != good.GeneratedAt {
		t.Errorf("generatedAt = %q, want previous %q", stale.GeneratedAt, good.GeneratedAt)
	}
	if len(stale.Services) != len(good.Services) {
		t.Fatalf("got %d services, want %d", len(stale.Services), len(good.Services))
	}
	for _, service := range stale.Services {
		if service.ID == "whisper" && service.Status != StatusOnline {
			t.Errorf("whisper status = %q, want online from cache", service.Status)
		}
	}
	if len(stale.Notifications) != len(good.Notifications)+1 {
		t.Errorf("notifications = %d, want %d (one source alert)", len(stale.Notifications), len(good.Notifications)+1)
	}
}

func TestSnapshotDockerUnavailable(t *testing.T) {
	app := newCollector(config{RequestTimeout: time.Second, LogTail: 10}, &fakeDocker{listErr: context.DeadlineExceeded}, nil)
	snapshot := app.buildSnapshot(context.Background())
	if snapshot.Sources.Docker.State != SourceUnavailable {
		t.Fatalf("docker source = %q, want unavailable", snapshot.Sources.Docker.State)
	}
	if snapshot.Metrics.Memory.UsedMb != nil {
		t.Errorf("memory = %v, want nil", *snapshot.Metrics.Memory.UsedMb)
	}
}

func TestLogsFilterAndCursor(t *testing.T) {
	first := time.Date(2026, 11, 11, 20, 0, 0, 0, time.UTC)
	fake := &fakeDocker{
		containers: []dockerContainer{
			{ID: "w1", State: "running", Labels: labels("whisper")},
			{ID: "p1", State: "running", Labels: labels("piper")},
		},
		logs: map[string][]rawLogLine{
			"w1": {
				{Time: first, Message: "hello"},
				{Time: first.Add(time.Second), Message: "WARNING low"},
			},
			"p1": {
				{Time: first.Add(2 * time.Second), Message: "spoke"},
			},
		},
	}
	app := newCollector(config{ComposeProject: "voice-assistant", RequestTimeout: time.Second, LogTail: 10}, fake, nil)
	response, err := app.Logs(context.Background(), logOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(response.Entries))
	}
	if response.NextCursor == nil {
		t.Fatal("expected a next cursor")
	}

	next, err := app.Logs(context.Background(), logOptions{Limit: 10, Cursor: *response.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Entries) != 0 {
		t.Fatalf("cursor returned %d entries, want 0", len(next.Entries))
	}

	filtered, err := app.Logs(context.Background(), logOptions{Limit: 10, Level: "warn"})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Entries) != 1 || filtered.Entries[0].Service != "whisper" {
		t.Fatalf("unexpected filtered entries: %+v", filtered.Entries)
	}

	if _, err := app.Logs(context.Background(), logOptions{Cursor: "not-base64"}); err == nil {
		t.Fatal("expected an invalid cursor error")
	}
}

func TestAlertTrackerTransitions(t *testing.T) {
	tracker := newAlertTracker()
	now := time.Now().UTC()
	services := []Service{{ID: "piper", Name: "Piper", Status: StatusOffline}}
	first := tracker.Update(services, SnapshotSources{Docker: SourceStatus{State: SourceOK}, HomeAssistant: SourceStatus{State: SourceDisabled}}, now)
	if len(first) != 1 || first[0].Type != "error" {
		t.Fatalf("unexpected first alerts: %+v", first)
	}
	second := tracker.Update(services, SnapshotSources{Docker: SourceStatus{State: SourceOK}, HomeAssistant: SourceStatus{State: SourceDisabled}}, now.Add(time.Minute))
	if len(second) != 1 {
		t.Fatalf("repeat observation created %d alerts, want 1", len(second))
	}
	services[0].Status = StatusOnline
	third := tracker.Update(services, SnapshotSources{Docker: SourceStatus{State: SourceOK}, HomeAssistant: SourceStatus{State: SourceDisabled}}, now.Add(2*time.Minute))
	if len(third) != 2 || third[0].Type != "success" {
		t.Fatalf("unexpected recovery alerts: %+v", third)
	}
}

func TestAlertTrackerSkipsExternalHomeAssistant(t *testing.T) {
	tracker := newAlertTracker()
	services := []Service{{ID: "homeassistant", Name: "Home Assistant", Status: StatusOffline}}
	got := tracker.Update(services, SnapshotSources{Docker: SourceStatus{State: SourceOK}, HomeAssistant: SourceStatus{State: SourceDisabled}}, time.Now().UTC())
	if len(got) != 0 {
		t.Fatalf("external Home Assistant raised %d alerts, want 0", len(got))
	}
}

func labels(service string) map[string]string {
	return map[string]string{
		"com.docker.compose.project": "voice-assistant",
		"com.docker.compose.service": service,
	}
}

func inspectFor(started, health string, containerPort, hostPort int) dockerInspect {
	var inspect dockerInspect
	inspect.State.Status = "running"
	inspect.State.Running = true
	inspect.State.StartedAt = started
	if health != "" {
		inspect.State.Health = &struct {
			Status string `json:"Status"`
		}{Status: health}
	}
	if containerPort > 0 {
		inspect.NetworkSettings.Ports = map[string][]struct {
			HostIP   string `json:"HostIp"`
			HostPort string `json:"HostPort"`
		}{}
		key := itoa(containerPort) + "/tcp"
		inspect.NetworkSettings.Ports[key] = []struct {
			HostIP   string `json:"HostIp"`
			HostPort string `json:"HostPort"`
		}{{HostIP: "127.0.0.1", HostPort: itoa(hostPort)}}
	}
	return inspect
}

func statsFor(cpuDelta, cpuTotal, systemPre, systemNow uint64, cpus uint32, usage, limit uint64) dockerStats {
	var stats dockerStats
	stats.PreCPUStats.CPUUsage.TotalUsage = cpuDelta
	stats.CPUStats.CPUUsage.TotalUsage = cpuTotal
	stats.PreCPUStats.SystemCPUUsage = systemPre
	stats.CPUStats.SystemCPUUsage = systemNow
	stats.CPUStats.OnlineCPUs = cpus
	stats.MemoryStats.Usage = usage
	stats.MemoryStats.Limit = limit
	return stats
}

func TestContainerStatsUsesPreviousSampleForCPU(t *testing.T) {
	fake := &fakeDocker{
		stats: map[string]dockerStats{
			"c1": statsFor(0, 100, 0, 1000, 1, 1<<20, 0),
		},
	}
	app := newCollector(config{RequestTimeout: time.Second}, fake, nil)
	first, err := app.containerStats(context.Background(), "c1")
	if err != nil {
		t.Fatal(err)
	}

	app.statsMu.Lock()
	app.statsCache["c1"] = statsCacheEntry{stats: first, at: time.Now().Add(-time.Minute)}
	app.statsMu.Unlock()
	fake.stats["c1"] = statsFor(0, 300, 0, 2000, 1, 2<<20, 0)

	second, err := app.containerStats(context.Background(), "c1")
	if err != nil {
		t.Fatal(err)
	}
	if got := second.PreCPUStats.CPUUsage.TotalUsage; got != 100 {
		t.Fatalf("previous CPU total = %d, want 100", got)
	}
	if got := second.PreCPUStats.SystemCPUUsage; got != 1000 {
		t.Fatalf("previous system CPU = %d, want 1000", got)
	}
	if percent := cpuPercent(second); percent == nil || *percent != 20 {
		t.Fatalf("cpuPercent = %v, want 20", percent)
	}
}

func TestErrorEnvelopeJSON(t *testing.T) {
	body, err := json.Marshal(ErrorEnvelope{Error: "invalid cursor"})
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"error":"invalid cursor"}` {
		t.Fatalf("unexpected body: %s", body)
	}
}
