export type ServiceStatus = "online" | "degraded" | "offline" | "restarting";

export type Service = {
  id: string;
  name: string;
  image: string;
  description: string;
  status: ServiceStatus;
  uptime: string;
  memoryMb: number;
  memoryLimitMb: number;
  cpu: number;
  port: number;
  icon: "model" | "voice" | "mic" | "brain" | "home" | "dashboard";
};

export type LogLevel = "info" | "warn" | "error" | "debug";

export type LogEntry = {
  id: number;
  time: string;
  level: LogLevel;
  service: string;
  message: string;
};

export type Notification = {
  id: number;
  title: string;
  description: string;
  time: string;
  type: "info" | "success" | "warning" | "error";
  read: boolean;
};

export type Stat = {
  label: string;
  value: string;
  detail: string;
  trend: "up" | "down" | "flat";
  trendLabel: string;
  icon: "services" | "memory" | "cpu" | "uptime";
};

export const services: Service[] = [
  {
    id: "whisper",
    name: "Whisper",
    image: "rhasspy/wyoming-whisper:3.8.1",
    description: "Reconocimiento de voz (STT)",
    status: "online",
    uptime: "12d 4h",
    memoryMb: 2840,
    memoryLimitMb: 4096,
    cpu: 12,
    port: 10300,
    icon: "voice",
  },
  {
    id: "llama-server",
    name: "llama-server",
    image: "ghcr.io/ggml-org/llama.cpp:server",
    description: "Modelo de lenguaje local",
    status: "online",
    uptime: "12d 4h",
    memoryMb: 3720,
    memoryLimitMb: 6144,
    cpu: 41,
    port: 8080,
    icon: "brain",
  },
  {
    id: "piper",
    name: "Piper",
    image: "rhasspy/wyoming-piper:2.5.2",
    description: "Sintesis de voz (TTS)",
    status: "online",
    uptime: "12d 4h",
    memoryMb: 210,
    memoryLimitMb: 512,
    cpu: 3,
    port: 10200,
    icon: "model",
  },
  {
    id: "openwakeword",
    name: "openWakeWord",
    image: "rhasspy/wyoming-openwakeword:2.1.0",
    description: "Deteccion de palabra de activacion",
    status: "online",
    uptime: "12d 4h",
    memoryMb: 96,
    memoryLimitMb: 256,
    cpu: 1,
    port: 10400,
    icon: "mic",
  },
  {
    id: "dashboard",
    name: "M4nhattan",
    image: "jarvis/dashboard:dev",
    description: "Este panel de control",
    status: "online",
    uptime: "0d 2h",
    memoryMb: 128,
    memoryLimitMb: 512,
    cpu: 2,
    port: 3005,
    icon: "dashboard",
  },
  {
    id: "homeassistant",
    name: "Home Assistant",
    image: "ghcr.io/home-assistant/home-assistant:2026.10.2",
    description: "Entorno de pruebas local",
    status: "offline",
    uptime: "detenido",
    memoryMb: 0,
    memoryLimitMb: 1024,
    cpu: 0,
    port: 8123,
    icon: "home",
  },
];

export const stats: Stat[] = [
  {
    label: "Servicios activos",
    value: "5 / 6",
    detail: "1 detenido",
    trend: "flat",
    trendLabel: "sin cambios",
    icon: "services",
  },
  {
    label: "Memoria en uso",
    value: "6.8 GB",
    detail: "de 12.2 GB reservados",
    trend: "up",
    trendLabel: "+2.4% en 24h",
    icon: "memory",
  },
  {
    label: "CPU",
    value: "34%",
    detail: "promedio ultimos 5 min",
    trend: "down",
    trendLabel: "-6.1% en 1h",
    icon: "cpu",
  },
  {
    label: "Uptime del host",
    value: "12d 4h",
    detail: "ultimo reinicio: 29 oct",
    trend: "up",
    trendLabel: "estable",
    icon: "uptime",
  },
];

export const logs: LogEntry[] = [
  {
    id: 1,
    time: "22:41:08",
    level: "info",
    service: "whisper",
    message: "Transcription completed in 318 ms (language=es, model=base)",
  },
  {
    id: 2,
    time: "22:40:52",
    level: "info",
    service: "llama-server",
    message: "POST /v1/chat/completions 200 in 1842 ms",
  },
  {
    id: 3,
    time: "22:39:31",
    level: "warn",
    service: "llama-server",
    message: "Context window at 78% (3198 / 4096 tokens)",
  },
  {
    id: 4,
    time: "22:38:14",
    level: "info",
    service: "piper",
    message: "Synthesized 42 tokens with es_ES-sharvard-medium",
  },
  {
    id: 5,
    time: "22:36:09",
    level: "error",
    service: "homeassistant",
    message: "container exited with code 137 (stopped by user)",
  },
  {
    id: 6,
    time: "22:35:47",
    level: "debug",
    service: "openwakeword",
    message: "Wake word detected: alexa (score=0.71)",
  },
  {
    id: 7,
    time: "22:34:22",
    level: "info",
    service: "dashboard",
    message: "GET / 200 in 41 ms",
  },
  {
    id: 8,
    time: "22:32:58",
    level: "info",
    service: "whisper",
    message: "Model base loaded from /data (int8, 4 threads)",
  },
  {
    id: 9,
    time: "22:30:12",
    level: "warn",
    service: "openwakeword",
    message: "Audio buffer underrun detected, recovering",
  },
  {
    id: 10,
    time: "22:28:03",
    level: "info",
    service: "piper",
    message: "Voice es_ES-sharvard-medium ready",
  },
];

export const notifications: Notification[] = [
  {
    id: 1,
    title: "Home Assistant detenido",
    description: "El contenedor de pruebas salio con codigo 137. Revisa los logs.",
    time: "hace 5 min",
    type: "error",
    read: false,
  },
  {
    id: 2,
    title: "Contexto de llama-server alto",
    description: "El contexto del modelo esta al 78% de su capacidad.",
    time: "hace 11 min",
    type: "warning",
    read: false,
  },
  {
    id: 3,
    title: "Whisper actualizado",
    description: "Nueva imagen wyoming-whisper:3.8.1 lista para desplegar.",
    time: "hace 2 h",
    type: "info",
    read: false,
  },
  {
    id: 4,
    title: "Backup completado",
    description: "Copia de seguridad diaria de configuracion finalizada.",
    time: "hace 6 h",
    type: "success",
    read: true,
  },
];

export const activity = [
  {
    id: 1,
    title: "piper reiniciado",
    description: "por marcos",
    time: "hace 3 h",
  },
  {
    id: 2,
    title: "Memoria ajustada en whisper",
    description: "2 GB -> 4 GB",
    time: "hace 9 h",
  },
  {
    id: 3,
    title: "Imagen de llama-server actualizada",
    description: "build 8052c13",
    time: "ayer",
  },
  {
    id: 4,
    title: "Dashboard desplegado",
    description: "v0.1.0",
    time: "hace 2 dias",
  },
];
