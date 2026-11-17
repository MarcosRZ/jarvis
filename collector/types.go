package main

import actioncatalog "jarvis/actions"

type ServiceStatus string

const (
	StatusOnline     ServiceStatus = "online"
	StatusDegraded   ServiceStatus = "degraded"
	StatusOffline    ServiceStatus = "offline"
	StatusRestarting ServiceStatus = "restarting"
)

type Service struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Image         string        `json:"image"`
	Description   string        `json:"description"`
	Status        ServiceStatus `json:"status"`
	StartedAt     *string       `json:"startedAt"`
	UptimeSeconds *int64        `json:"uptimeSeconds"`
	MemoryMb      *int64        `json:"memoryMb"`
	MemoryLimitMb *int64        `json:"memoryLimitMb"`
	CPU           *float64      `json:"cpu"`
	Port          *int          `json:"port"`
	Icon          string        `json:"icon"`
}

type SourceState string

const (
	SourceOK          SourceState = "ok"
	SourceDegraded    SourceState = "degraded"
	SourceUnavailable SourceState = "unavailable"
	SourceDisabled    SourceState = "disabled"
)

type SourceStatus struct {
	State         SourceState `json:"state"`
	LastSuccessAt *string     `json:"lastSuccessAt"`
	Message       *string     `json:"message"`
}

type ServiceMetrics struct {
	Total   int `json:"total"`
	Active  int `json:"active"`
	Stopped int `json:"stopped"`
}

type MemoryMetrics struct {
	UsedMb  *int64 `json:"usedMb"`
	LimitMb *int64 `json:"limitMb"`
}

type CPUMetrics struct {
	Percent *float64 `json:"percent"`
	Scope   string   `json:"scope"`
}

type HostMetrics struct {
	UptimeSeconds *int64 `json:"uptimeSeconds"`
	Source        string `json:"source"`
}

type SnapshotMetrics struct {
	Services ServiceMetrics `json:"services"`
	Memory   MemoryMetrics  `json:"memory"`
	CPU      CPUMetrics     `json:"cpu"`
	Host     HostMetrics    `json:"host"`
}

type ActivityEvent struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Time        string `json:"time"`
}

type Notification struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Time        string `json:"time"`
	Type        string `json:"type"`
	Read        bool   `json:"read"`
}

type Snapshot struct {
	GeneratedAt   string          `json:"generatedAt"`
	Sources       SnapshotSources `json:"sources"`
	Services      []Service       `json:"services"`
	Metrics       SnapshotMetrics `json:"metrics"`
	Activity      []ActivityEvent `json:"activity"`
	Notifications []Notification  `json:"notifications"`
}

type SnapshotSources struct {
	Docker        SourceStatus `json:"docker"`
	HomeAssistant SourceStatus `json:"homeAssistant"`
}

// ActionsSnapshot is the deterministic action catalog generated from the
// Home Assistant entities exposed to the voice assistant. It is generated and
// cached separately from the container snapshot because it changes slowly.
type ActionsSnapshot struct {
	Version     int                    `json:"version"`
	GeneratedAt string                 `json:"generatedAt"`
	Source      SourceStatus           `json:"source"`
	EntityCount int                    `json:"entityCount"`
	Actions     []actioncatalog.Action `json:"actions"`
}

type LogEntry struct {
	ID      string `json:"id"`
	Time    string `json:"time"`
	Level   string `json:"level"`
	Service string `json:"service"`
	Message string `json:"message"`
}

type LogSources struct {
	Docker       SourceStatus `json:"docker"`
	AssistantLog SourceStatus `json:"assistantLog"`
}

type LogsResponse struct {
	GeneratedAt string     `json:"generatedAt"`
	Entries     []LogEntry `json:"entries"`
	NextCursor  *string    `json:"nextCursor"`
	HasMore     bool       `json:"hasMore"`
	Sources     LogSources `json:"sources"`
}

type ErrorEnvelope struct {
	Error string `json:"error"`
}
