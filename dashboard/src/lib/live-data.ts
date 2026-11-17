export type ServiceStatus = "online" | "degraded" | "offline" | "restarting";

export type ServiceIcon =
  | "model"
  | "voice"
  | "mic"
  | "brain"
  | "home"
  | "dashboard"
  | "unknown";

export type Service = {
  id: string;
  name: string;
  image: string;
  description: string;
  status: ServiceStatus;
  startedAt: string | null;
  uptimeSeconds: number | null;
  memoryMb: number | null;
  memoryLimitMb: number | null;
  cpu: number | null;
  port: number | null;
  icon: ServiceIcon;
};

export type LogLevel = "info" | "warn" | "error" | "debug";

export type LogEntry = {
  id: string;
  time: string;
  level: LogLevel;
  service: string;
  message: string;
};

export type NotificationType = "info" | "success" | "warning" | "error";

export type Notification = {
  id: string;
  title: string;
  description: string;
  time: string;
  type: NotificationType;
  read: boolean;
};

export type ActivityEvent = {
  id: string;
  title: string;
  description: string;
  time: string;
};

export type SourceState = "ok" | "degraded" | "unavailable" | "disabled";

export type SourceStatus = {
  state: SourceState;
  lastSuccessAt: string | null;
  message: string | null;
};

export type SnapshotMetrics = {
  services: {
    total: number;
    active: number;
    stopped: number;
  };
  memory: {
    usedMb: number | null;
    limitMb: number | null;
  };
  cpu: {
    percent: number | null;
    scope: "containers";
  };
  host: {
    uptimeSeconds: number | null;
    source: "home_assistant" | "unavailable";
  };
};

export type Snapshot = {
  generatedAt: string;
  sources: {
    docker: SourceStatus;
    homeAssistant: SourceStatus;
  };
  services: Service[];
  metrics: SnapshotMetrics;
  activity: ActivityEvent[];
  notifications: Notification[];
};

export type LogsResponse = {
  generatedAt: string;
  entries: LogEntry[];
  nextCursor: string | null;
  hasMore: boolean;
  sources: {
    docker: SourceStatus;
    assistantLog: SourceStatus;
  };
};

export type Stat = {
  label: string;
  value: string;
  detail: string;
  trend: "up" | "down" | "flat";
  trendLabel: string;
  icon: "services" | "memory" | "cpu" | "uptime";
};

export const unknownValue = "—";

export function formatDuration(seconds: number | null): string {
  if (seconds === null || !Number.isFinite(seconds) || seconds < 0) {
    return unknownValue;
  }
  const total = Math.floor(seconds);
  const days = Math.floor(total / 86400);
  const hours = Math.floor((total % 86400) / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  if (days > 0) {
    return `${days}d ${hours}h`;
  }
  if (hours > 0) {
    return `${hours}h ${minutes}m`;
  }
  if (minutes > 0) {
    return `${minutes}m`;
  }
  return `${total}s`;
}

export function formatRelativeTime(value: string | null): string {
  if (!value) {
    return unknownValue;
  }
  const timestamp = Date.parse(value);
  if (Number.isNaN(timestamp)) {
    return unknownValue;
  }
  const difference = Date.now() - timestamp;
  if (difference < 45_000) {
    return "just now";
  }
  const minutes = Math.floor(difference / 60_000);
  if (minutes < 60) {
    return `${minutes} min ago`;
  }
  const hours = Math.floor(minutes / 60);
  if (hours < 24) {
    return `${hours} h ago`;
  }
  const days = Math.floor(hours / 24);
  return `${days} d ago`;
}

export function formatClock(value: string): string {
  const timestamp = Date.parse(value);
  if (Number.isNaN(timestamp)) {
    return unknownValue;
  }
  return new Date(timestamp).toLocaleTimeString([], {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

export function formatMemory(mebibytes: number | null): string {
  if (mebibytes === null || !Number.isFinite(mebibytes)) {
    return unknownValue;
  }
  return `${Math.round(mebibytes / 102.4) / 10} GB`;
}

export function formatPercent(value: number | null): string {
  if (value === null || !Number.isFinite(value)) {
    return unknownValue;
  }
  return `${Math.round(value)}%`;
}

export function deriveStats(snapshot: Snapshot | null): Stat[] {
  const metrics = snapshot?.metrics;
  if (!metrics) {
    return [
      { label: "Servicios activos", value: unknownValue, detail: "No data yet", trend: "flat", trendLabel: "no history", icon: "services" },
      { label: "Memoria en uso", value: unknownValue, detail: "No data yet", trend: "flat", trendLabel: "no history", icon: "memory" },
      { label: "CPU", value: unknownValue, detail: "No data yet", trend: "flat", trendLabel: "no history", icon: "cpu" },
      { label: "Uptime del host", value: unknownValue, detail: "No data yet", trend: "flat", trendLabel: "no history", icon: "uptime" },
    ];
  }
  const used = metrics.memory.usedMb;
  const limit = metrics.memory.limitMb;
  return [
    {
      label: "Servicios activos",
      value: `${metrics.services.active} / ${metrics.services.total}`,
      detail: `${metrics.services.stopped} stopped`,
      trend: "flat",
      trendLabel: "no history",
      icon: "services",
    },
    {
      label: "Memoria en uso",
      value: used === null ? unknownValue : `${Math.round(used / 102.4) / 10} GB`,
      detail: limit === null ? "no container limits set" : `of ${Math.round(limit / 102.4) / 10} GB reserved`,
      trend: "flat",
      trendLabel: "no history",
      icon: "memory",
    },
    {
      label: "CPU",
      value: formatPercent(metrics.cpu.percent),
      detail: "containers, latest sample",
      trend: "flat",
      trendLabel: "no history",
      icon: "cpu",
    },
    {
      label: "Uptime del host",
      value: formatDuration(metrics.host.uptimeSeconds),
      detail: metrics.host.source === "unavailable" ? "not available" : "from Home Assistant",
      trend: "flat",
      trendLabel: "no history",
      icon: "uptime",
    },
  ];
}
