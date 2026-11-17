package main

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	snapshotInterval = 2 * time.Second
	statsCacheTTL    = time.Second
)

type collector struct {
	cfg      config
	docker   dockerClient
	home     *homeAssistantClient
	alerts   *alertTracker
	activity *activityRing

	cacheMu  sync.Mutex
	cached   *Snapshot
	lastGood *Snapshot

	actionsMu       sync.Mutex
	actions         *ActionsSnapshot
	actionsLastGood *ActionsSnapshot

	statsMu    sync.Mutex
	statsCache map[string]statsCacheEntry

	haCacheMu sync.Mutex
	haCache   *homeAssistantResult
	haCacheAt time.Time
}

type statsCacheEntry struct {
	stats dockerStats
	at    time.Time
}

func newCollector(cfg config, docker dockerClient, home *homeAssistantClient) *collector {
	return &collector{
		cfg:        cfg,
		docker:     docker,
		home:       home,
		alerts:     newAlertTracker(),
		activity:   newActivityRing(200),
		statsCache: map[string]statsCacheEntry{},
	}
}

func (c *collector) Snapshot(ctx context.Context) Snapshot {
	c.cacheMu.Lock()
	if c.cached != nil {
		snapshot := cloneSnapshot(*c.cached)
		c.cacheMu.Unlock()
		return snapshot
	}
	c.cacheMu.Unlock()

	snapshot := c.buildSnapshot(ctx)
	c.storeSnapshot(snapshot)
	return cloneSnapshot(snapshot)
}

func (c *collector) storeSnapshot(snapshot Snapshot) {
	c.cacheMu.Lock()
	c.cached = &snapshot
	c.cacheMu.Unlock()
}

func (c *collector) runSampler(ctx context.Context) {
	ticker := time.NewTicker(snapshotInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.storeSnapshot(c.buildSnapshot(ctx))
		}
	}
}

func (c *collector) buildSnapshot(ctx context.Context) Snapshot {
	now := time.Now().UTC()
	containers, listErr := c.docker.ListContainers(ctx)
	if listErr != nil {
		return c.unavailableSnapshot(now)
	}

	snapshot := Snapshot{
		GeneratedAt: now.Format(time.RFC3339Nano),
		Services:    []Service{},
		Metrics: SnapshotMetrics{
			CPU:  CPUMetrics{Scope: "containers"},
			Host: HostMetrics{Source: "unavailable"},
		},
		Activity:      c.activity.List(),
		Notifications: []Notification{},
	}

	success := now.Format(time.RFC3339Nano)
	dockerStatus := SourceStatus{State: SourceOK, LastSuccessAt: &success}

	byService := map[string]dockerContainer{}
	for _, container := range containers {
		if c.cfg.ComposeProject != "" && container.Labels["com.docker.compose.project"] != c.cfg.ComposeProject {
			continue
		}
		service := container.Labels["com.docker.compose.service"]
		if service == "" {
			continue
		}
		if _, known := registryByID(service); !known {
			continue
		}
		if existing, ok := byService[service]; ok {
			if existing.State == "running" {
				continue
			}
		}
		byService[service] = container
	}

	haResult := c.homeAssistant(ctx, now)

	services := make([]Service, len(serviceRegistry))
	semaphore := make(chan struct{}, 4)
	var wait sync.WaitGroup
	for index, definition := range serviceRegistry {
		if definition.ExternalHA {
			// Home Assistant runs outside the monitored stack. The compose
			// project also contains the development Home Assistant, but that
			// container is not a service: only the configured instance is
			// reported, and it is omitted entirely when VOICE_HA_URL is unset.
			if haResult != nil && haResult.HomeAssistant != nil {
				services[index] = *haResult.HomeAssistant
			}
			continue
		}
		container, ok := byService[definition.ID]
		if ok {
			wait.Add(1)
			go func(index int, definition serviceDefinition, container dockerContainer) {
				defer wait.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()
				services[index] = c.serviceFromContainer(ctx, definition, container, now)
			}(index, definition, container)
			continue
		}
		services[index] = Service{
			ID:          definition.ID,
			Name:        definition.Name,
			Description: definition.Description,
			Status:      StatusOffline,
			Icon:        definition.Icon,
		}
	}
	wait.Wait()

	monitored := make([]Service, 0, len(services))
	for _, service := range services {
		if service.ID != "" {
			monitored = append(monitored, service)
		}
	}
	snapshot.Services = monitored

	snapshot.Sources = SnapshotSources{
		Docker:        dockerStatus,
		HomeAssistant: SourceStatus{State: SourceDisabled},
	}
	if haResult != nil {
		snapshot.Sources.HomeAssistant = haResult.Status
		if haResult.HostUptime != nil {
			snapshot.Metrics.Host.UptimeSeconds = haResult.HostUptime
			snapshot.Metrics.Host.Source = "home_assistant"
		}
	}

	host := snapshot.Metrics.Host
	snapshot.Metrics = deriveMetrics(snapshot.Services)
	snapshot.Metrics.Host = host
	snapshot.Notifications = c.alerts.Update(snapshot.Services, snapshot.Sources, now)
	if snapshot.Activity == nil {
		snapshot.Activity = []ActivityEvent{}
	}
	if snapshot.Notifications == nil {
		snapshot.Notifications = []Notification{}
	}

	c.cacheMu.Lock()
	lastGood := cloneSnapshot(snapshot)
	c.lastGood = &lastGood
	c.cacheMu.Unlock()

	return snapshot
}

func (c *collector) unavailableSnapshot(now time.Time) Snapshot {
	c.cacheMu.Lock()
	lastGood := c.lastGood
	if lastGood != nil {
		previous := cloneSnapshot(*lastGood)
		lastGood = &previous
	}
	c.cacheMu.Unlock()

	var snapshot Snapshot
	if lastGood != nil {
		snapshot = *lastGood
	} else {
		snapshot = Snapshot{
			GeneratedAt: now.Format(time.RFC3339Nano),
			Services:    []Service{},
			Metrics: SnapshotMetrics{
				CPU:  CPUMetrics{Scope: "containers"},
				Host: HostMetrics{Source: "unavailable"},
			},
			Sources: SnapshotSources{
				HomeAssistant: SourceStatus{State: SourceDisabled},
			},
		}
	}

	snapshot.Sources.Docker = SourceStatus{
		State:         SourceUnavailable,
		LastSuccessAt: snapshot.Sources.Docker.LastSuccessAt,
		Message:       ptr("Docker API is not available."),
	}
	snapshot.Activity = c.activity.List()
	snapshot.Notifications = c.alerts.Update(snapshot.Services, snapshot.Sources, now)
	if snapshot.Activity == nil {
		snapshot.Activity = []ActivityEvent{}
	}
	if snapshot.Notifications == nil {
		snapshot.Notifications = []Notification{}
	}

	return snapshot
}

func (c *collector) serviceFromContainer(ctx context.Context, definition serviceDefinition, container dockerContainer, now time.Time) Service {
	service := Service{
		ID:          definition.ID,
		Name:        definition.Name,
		Description: definition.Description,
		Icon:        definition.Icon,
		Status:      statusFromContainer(container.State, ""),
	}

	statsCtx, cancel := context.WithTimeout(ctx, c.cfg.RequestTimeout)
	inspect, inspectErr := c.docker.InspectContainer(statsCtx, container.ID)
	cancel()
	if inspectErr != nil {
		if container.Image != "" {
			service.Image = container.Image
		}
		return service
	}

	service.Image = inspect.Config.Image
	if service.Image == "" {
		service.Image = container.Image
	}
	health := ""
	if inspect.State.Health != nil {
		health = inspect.State.Health.Status
	}
	service.Status = statusFromContainer(inspect.State.Status, health)
	if container.State != "" && inspect.State.Status == "" {
		service.Status = statusFromContainer(container.State, "")
	}
	if inspect.State.Running {
		if started, err := time.Parse(time.RFC3339Nano, inspect.State.StartedAt); err == nil && started.Year() > 2000 {
			startedUTC := started.UTC().Format(time.RFC3339Nano)
			seconds := int64(now.Sub(started).Seconds())
			if seconds >= 0 {
				service.StartedAt = &startedUTC
				service.UptimeSeconds = &seconds
			}
		}
		statsCtx, cancel := context.WithTimeout(ctx, c.cfg.RequestTimeout)
		stats, err := c.containerStats(statsCtx, container.ID)
		cancel()
		if err == nil {
			if usage := bytesToMiB(stats.MemoryStats.Usage); usage >= 0 {
				service.MemoryMb = &usage
			}
			if inspect.HostConfig.Memory > 0 {
				limit := bytesToMiB(uint64(inspect.HostConfig.Memory))
				service.MemoryLimitMb = &limit
			}
			service.CPU = cpuPercent(stats)
		}
	}
	service.Port = selectPort(inspect, definition.PrimaryPort)
	return service
}

func (c *collector) containerStats(ctx context.Context, id string) (dockerStats, error) {
	c.statsMu.Lock()
	previous, hadPrevious := c.statsCache[id]
	if hadPrevious && time.Since(previous.at) < statsCacheTTL {
		c.statsMu.Unlock()
		return previous.stats, nil
	}
	c.statsMu.Unlock()

	stats, err := c.docker.ContainerStats(ctx, id)
	if err != nil {
		return dockerStats{}, err
	}
	if hadPrevious {
		stats.PreCPUStats.CPUUsage.TotalUsage = previous.stats.CPUStats.CPUUsage.TotalUsage
		stats.PreCPUStats.SystemCPUUsage = previous.stats.CPUStats.SystemCPUUsage
	}
	c.statsMu.Lock()
	c.statsCache[id] = statsCacheEntry{stats: stats, at: time.Now()}
	c.statsMu.Unlock()
	return stats, nil
}

func (c *collector) homeAssistant(ctx context.Context, now time.Time) *homeAssistantResult {
	if c.home == nil {
		return nil
	}
	c.haCacheMu.Lock()
	if c.haCache != nil && time.Since(c.haCacheAt) < 10*time.Second {
		cached := *c.haCache
		c.haCacheMu.Unlock()
		return &cached
	}
	c.haCacheMu.Unlock()

	haCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	states, err := c.home.States(haCtx)
	cancel()

	result := &homeAssistantResult{}
	success := now.Format(time.RFC3339Nano)
	if err != nil {
		result.Status = SourceStatus{State: SourceUnavailable, Message: ptr("Home Assistant is not reachable.")}
		result.HomeAssistant = &Service{
			ID:          "homeassistant",
			Name:        "Home Assistant",
			Description: "External Home Assistant",
			Status:      StatusOffline,
			Icon:        "home",
		}
	} else {
		result.Status = SourceStatus{State: SourceOK, LastSuccessAt: &success}
		result.Reachable = true
		result.HomeAssistant = &Service{
			ID:          "homeassistant",
			Name:        "Home Assistant",
			Description: "External Home Assistant",
			Status:      StatusOnline,
			Icon:        "home",
			StartedAt:   nil,
		}
		result.HostUptime = hostUptimeFromStates(states, now)
	}

	c.haCacheMu.Lock()
	c.haCache = result
	c.haCacheAt = time.Now()
	c.haCacheMu.Unlock()
	return result
}

func deriveMetrics(services []Service) SnapshotMetrics {
	metrics := SnapshotMetrics{
		CPU:  CPUMetrics{Scope: "containers"},
		Host: HostMetrics{Source: "unavailable"},
	}
	var memoryUsed int64
	var memoryLimit int64
	var cpu float64
	haveMemory := false
	haveLimit := false
	haveCPU := false

	for _, service := range services {
		if definition, ok := registryByID(service.ID); ok && definition.ExternalHA {
			continue
		}
		metrics.Services.Total++
		switch service.Status {
		case StatusOnline, StatusDegraded, StatusRestarting:
			metrics.Services.Active++
		case StatusOffline:
			metrics.Services.Stopped++
		}
		if service.MemoryMb != nil {
			memoryUsed += *service.MemoryMb
			haveMemory = true
		}
		if service.MemoryLimitMb != nil && *service.MemoryLimitMb > 0 {
			memoryLimit += *service.MemoryLimitMb
			haveLimit = true
		}
		if service.CPU != nil {
			cpu += *service.CPU
			haveCPU = true
		}
	}
	if haveMemory {
		metrics.Memory.UsedMb = &memoryUsed
	}
	if haveLimit {
		metrics.Memory.LimitMb = &memoryLimit
	}
	if haveCPU {
		metrics.CPU.Percent = &cpu
	}
	return metrics
}

func statusFromContainer(state, health string) ServiceStatus {
	switch state {
	case "running":
		switch health {
		case "starting":
			return StatusRestarting
		case "unhealthy":
			return StatusDegraded
		default:
			return StatusOnline
		}
	case "restarting":
		return StatusRestarting
	case "exited", "dead", "stopped", "created", "paused", "removing":
		return StatusOffline
	default:
		if state == "" {
			return StatusOffline
		}
		return StatusOffline
	}
}

func cpuPercent(stats dockerStats) *float64 {
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage) - float64(stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemCPUUsage) - float64(stats.PreCPUStats.SystemCPUUsage)
	if cpuDelta < 0 || systemDelta <= 0 {
		return nil
	}
	cpus := float64(stats.CPUStats.OnlineCPUs)
	if cpus == 0 {
		cpus = float64(len(stats.CPUStats.CPUUsage.PercpuUsage))
	}
	if cpus == 0 {
		cpus = 1
	}
	percent := (cpuDelta / systemDelta) * cpus * 100
	if percent < 0 {
		return nil
	}
	return &percent
}

func bytesToMiB(bytes uint64) int64 {
	return int64(bytes / 1024 / 1024)
}

func selectPort(inspect dockerInspect, primary int) *int {
	type binding struct {
		container int
		host      int
	}
	var bindings []binding
	for key, ports := range inspect.NetworkSettings.Ports {
		containerPort, err := strconv.Atoi(strings.Split(key, "/")[0])
		if err != nil {
			continue
		}
		for _, port := range ports {
			hostPort, err := strconv.Atoi(port.HostPort)
			if err != nil || hostPort <= 0 {
				continue
			}
			bindings = append(bindings, binding{container: containerPort, host: hostPort})
		}
	}
	if len(bindings) == 0 {
		return nil
	}
	if len(bindings) == 1 {
		host := bindings[0].host
		return &host
	}
	for _, candidate := range bindings {
		if candidate.container == primary {
			host := candidate.host
			return &host
		}
	}
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].host < bindings[j].host })
	host := bindings[0].host
	return &host
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	clone := snapshot
	clone.Services = append([]Service{}, snapshot.Services...)
	clone.Activity = append([]ActivityEvent{}, snapshot.Activity...)
	clone.Notifications = append([]Notification{}, snapshot.Notifications...)
	return clone
}

func ptr[T any](value T) *T {
	return &value
}
