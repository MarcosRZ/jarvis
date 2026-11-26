// Settings shown in the dashboard. Keys are the environment variable names used
// by server/compose.yaml, the collector and the voice client. A stored value is
// an override; an empty value means "use the value from the .env file".

export type SettingScope = "shared" | "server" | "client";
export type SettingType =
  | "text"
  | "password"
  | "number"
  | "select"
  | "boolean"
  | "textarea";

export type SettingOption = {
  value: string;
  label: string;
};

export type SettingField = {
  key: string;
  label: string;
  description: string;
  scope: SettingScope;
  type: SettingType;
  placeholder?: string;
  options?: SettingOption[];
  min?: number;
  max?: number;
  step?: number;
  maxLength?: number;
  wide?: boolean;
};

export type SettingSection = {
  id: string;
  title: string;
  description: string;
  fields: SettingField[];
};

const trueFalse: SettingOption[] = [
  { value: "true", label: "true" },
  { value: "false", label: "false" },
];

export const settingsSections: SettingSection[] = [
  {
    id: "general",
    title: "General",
    description:
      "Shared server and Home Assistant settings. Empty fields fall back to server/.env or client/.env. The dashboard port stays in server/.env.",
    fields: [
      {
        key: "BIND_ADDRESS",
        label: "Bind address",
        description:
          "Address used by the published service ports. Keep 127.0.0.1 unless the services are needed on the LAN.",
        scope: "server",
        type: "text",
        placeholder: "127.0.0.1",
        maxLength: 255,
      },
      {
        key: "TZ",
        label: "Timezone",
        description: "Timezone for the containers, for example Europe/Madrid.",
        scope: "server",
        type: "text",
        placeholder: "Europe/Madrid",
        maxLength: 100,
      },
      {
        key: "HA_PORT",
        label: "Development Home Assistant port",
        description:
          "Host port of the development Home Assistant started by make ha-up.",
        scope: "server",
        type: "number",
        placeholder: "8123",
        min: 1,
        max: 65535,
      },
      {
        key: "VOICE_HA_URL",
        label: "Home Assistant URL",
        description:
          "Home Assistant used by the collector, the action catalog and the voice client.",
        scope: "shared",
        type: "text",
        placeholder: "http://homeassistant.local:8123",
        maxLength: 500,
      },
      {
        key: "VOICE_HA_TOKEN",
        label: "Home Assistant token",
        description:
          "Long-lived access token. It is stored in the dashboard data volume with mode 0600.",
        scope: "shared",
        type: "password",
        placeholder: "Long-lived access token",
        maxLength: 2000,
      },
    ],
  },
  {
    id: "whisper",
    title: "Whisper",
    description: "Speech recognition service from the server stack.",
    fields: [
      {
        key: "WHISPER_PORT",
        label: "Port",
        description: "Host port for the speech recognition service.",
        scope: "server",
        type: "number",
        placeholder: "10300",
        min: 1,
        max: 65535,
      },
      {
        key: "WHISPER_MODEL",
        label: "Model",
        description:
          "Whisper model, for example tiny, base, small or medium. Use auto with the sherpa library.",
        scope: "server",
        type: "text",
        placeholder: "base",
        maxLength: 100,
      },
      {
        key: "WHISPER_LANGUAGE",
        label: "Language",
        description: "Language code for transcription, for example es or en.",
        scope: "server",
        type: "text",
        placeholder: "es",
        maxLength: 20,
      },
      {
        key: "WHISPER_INITIAL_PROMPT",
        label: "Initial prompt",
        description:
          "Words that steer the recognizer toward the command vocabulary.",
        scope: "server",
        type: "textarea",
        placeholder: "¿Qué hora es? Repite lo que he dicho. Cancela. ¿Qué puedes hacer?",
        maxLength: 2000,
        wide: true,
      },
      {
        key: "WHISPER_COMPUTE_TYPE",
        label: "Compute type",
        description:
          "int8 is faster on a slow CPU. Keep default on hardware with enough capacity.",
        scope: "server",
        type: "select",
        options: [
          { value: "default", label: "default" },
          { value: "int8", label: "int8" },
          { value: "int8_float16", label: "int8_float16" },
          { value: "float16", label: "float16" },
          { value: "float32", label: "float32" },
        ],
      },
      {
        key: "WHISPER_BEAM_SIZE",
        label: "Beam size",
        description: "Keep 0; a larger beam was not faster and was less accurate.",
        scope: "server",
        type: "number",
        placeholder: "0",
        min: 0,
        max: 100,
      },
      {
        key: "WHISPER_CPU_THREADS",
        label: "CPU threads",
        description: "Number of CPU threads used by Whisper.",
        scope: "server",
        type: "number",
        placeholder: "4",
        min: 1,
        max: 256,
      },
      {
        key: "WHISPER_STT_LIBRARY",
        label: "Speech engine",
        description:
          "sherpa with model auto uses NVIDIA Parakeet; it has no initial prompt.",
        scope: "server",
        type: "select",
        options: [
          { value: "faster-whisper", label: "faster-whisper" },
          { value: "sherpa", label: "sherpa" },
        ],
      },
    ],
  },
  {
    id: "piper",
    title: "Piper",
    description: "Speech output service from the server stack.",
    fields: [
      {
        key: "PIPER_PORT",
        label: "Port",
        description: "Host port for the speech output service.",
        scope: "server",
        type: "number",
        placeholder: "10200",
        min: 1,
        max: 65535,
      },
      {
        key: "PIPER_VOICE",
        label: "Voice",
        description:
          "Voice name, for example es_ES-sharvard-medium. Piper downloads it on first use.",
        scope: "server",
        type: "text",
        placeholder: "es_ES-sharvard-medium",
        maxLength: 200,
      },
    ],
  },
  {
    id: "llama",
    title: "llama.cpp",
    description: "Optional local language model from the server stack.",
    fields: [
      {
        key: "LLM_PORT",
        label: "Port",
        description: "Host port for the language model.",
        scope: "server",
        type: "number",
        placeholder: "8080",
        min: 1,
        max: 65535,
      },
      {
        key: "LLM_BIND_ADDRESS",
        label: "Bind address",
        description:
          "Host address for the model port. Use 127.0.0.1 for a local-only model.",
        scope: "server",
        type: "text",
        placeholder: "127.0.0.1",
        maxLength: 255,
      },
      {
        key: "LLM_MODEL",
        label: "Model file",
        description:
          "GGUF file inside server/models/llm. A new file is not downloaded automatically.",
        scope: "server",
        type: "text",
        placeholder: "qwen2.5-0.5b-instruct-q4_k_m.gguf",
        maxLength: 255,
      },
      {
        key: "LLM_CTX_SIZE",
        label: "Context size",
        description: "Context window in tokens. Keep at least 4096.",
        scope: "server",
        type: "number",
        placeholder: "4096",
        min: 512,
        max: 1048576,
      },
      {
        key: "LLM_THREADS",
        label: "CPU threads",
        description: "Number of CPU threads used by llama.cpp.",
        scope: "server",
        type: "number",
        placeholder: "4",
        min: 1,
        max: 256,
      },
      {
        key: "LLM_ROPE_SCALING",
        label: "RoPE scaling",
        description: "Use yarn when the context is larger than the trained window.",
        scope: "server",
        type: "select",
        options: [
          { value: "none", label: "none" },
          { value: "linear", label: "linear" },
          { value: "yarn", label: "yarn" },
        ],
      },
      {
        key: "LLM_ROPE_SCALE",
        label: "RoPE scale",
        description: "For yarn, 2 for Qwen2.5 with a doubled context.",
        scope: "server",
        type: "number",
        placeholder: "1",
        min: 0.1,
        max: 16,
        step: 0.1,
      },
      {
        key: "LLM_YARN_ORIG_CTX",
        label: "YaRN original context",
        description: "Original context size when yarn scaling is used.",
        scope: "server",
        type: "number",
        placeholder: "0",
        min: 0,
        max: 1048576,
      },
    ],
  },
  {
    id: "openwakeword",
    title: "openWakeWord",
    description: "Wake-word service from the server stack.",
    fields: [
      {
        key: "OPENWAKEWORD_PORT",
        label: "Port",
        description: "Host port for the wake-word service.",
        scope: "server",
        type: "number",
        placeholder: "10400",
        min: 1,
        max: 65535,
      },
      {
        key: "WAKEWORD_THRESHOLD",
        label: "Threshold",
        description: "Raise it to reduce false wakes; lower it if the wake word is missed.",
        scope: "server",
        type: "number",
        placeholder: "0.5",
        min: 0,
        max: 1,
        step: 0.01,
      },
      {
        key: "WAKEWORD_REFRACTORY_SECONDS",
        label: "Refractory seconds",
        description: "Minimum time between detections.",
        scope: "server",
        type: "number",
        placeholder: "2",
        min: 0,
        max: 3600,
        step: 0.1,
      },
    ],
  },
  {
    id: "collector",
    title: "Collector",
    description:
      "Internal service that reads Docker and Home Assistant for this dashboard.",
    fields: [
      {
        key: "DOCKER_SOCKET_GID",
        label: "Docker socket group id",
        description:
          "Group id of /var/run/docker.sock on the host. Compose adds group 0 for Docker Desktop.",
        scope: "server",
        type: "number",
        placeholder: "973",
        min: 0,
        max: 2147483647,
      },
    ],
  },
  {
    id: "client",
    title: "Client",
    description:
      "Voice assistant settings. Home Assistant URL and token come from General. Restart the assistant (make run) to apply changes.",
    fields: [
      {
        key: "VOICE_HA_AGENT",
        label: "Home Assistant agent",
        description: "Conversation agent for other devices and questions, for example conversation.ollama.",
        scope: "client",
        type: "text",
        placeholder: "conversation.ollama",
        maxLength: 200,
      },
      {
        key: "VOICE_WAKEWORD_ADDR",
        label: "Wake-word address",
        description: "Host and port of the wake-word service.",
        scope: "client",
        type: "text",
        placeholder: "127.0.0.1:10400",
        maxLength: 255,
      },
      {
        key: "VOICE_STT_ADDR",
        label: "Speech recognition address",
        description: "Host and port of the speech recognition service.",
        scope: "client",
        type: "text",
        placeholder: "127.0.0.1:10300",
        maxLength: 255,
      },
      {
        key: "VOICE_TTS_ADDR",
        label: "Speech output address",
        description: "Host and port of the speech output service.",
        scope: "client",
        type: "text",
        placeholder: "127.0.0.1:10200",
        maxLength: 255,
      },
      {
        key: "VOICE_INPUT_DEVICE",
        label: "Input device",
        description: "Microphone name as shown by voice-client devices. Empty uses the system default.",
        scope: "client",
        type: "text",
        placeholder: "System default",
        maxLength: 255,
      },
      {
        key: "VOICE_OUTPUT_DEVICE",
        label: "Output device",
        description: "Speaker name as shown by voice-client devices. Empty uses the system default.",
        scope: "client",
        type: "text",
        placeholder: "System default",
        maxLength: 255,
      },
      {
        key: "VOICE_WAKEWORD_MODEL",
        label: "Wake-word model",
        description: "Model name sent to the wake-word service, for example alexa.",
        scope: "client",
        type: "text",
        placeholder: "alexa",
        maxLength: 100,
      },
      {
        key: "VOICE_WAKEWORD_COOLDOWN",
        label: "Wake-word cooldown",
        description: "Minimum time between detections, for example 2s.",
        scope: "client",
        type: "text",
        placeholder: "2s",
        maxLength: 50,
      },
      {
        key: "VOICE_STT_LANGUAGE",
        label: "Transcription language",
        description: "Language code sent to the speech recognition service.",
        scope: "client",
        type: "text",
        placeholder: "es",
        maxLength: 20,
      },
      {
        key: "VOICE_RECORD_MAX",
        label: "Maximum recording",
        description: "Maximum command length, for example 8s.",
        scope: "client",
        type: "text",
        placeholder: "8s",
        maxLength: 50,
      },
      {
        key: "VOICE_RECORD_START_TIMEOUT",
        label: "Recording start timeout",
        description: "How long to wait for speech before giving up.",
        scope: "client",
        type: "text",
        placeholder: "3s",
        maxLength: 50,
      },
      {
        key: "VOICE_RECORD_END_SILENCE",
        label: "End silence",
        description: "Silence that ends a command, for example 1s.",
        scope: "client",
        type: "text",
        placeholder: "1s",
        maxLength: 50,
      },
      {
        key: "VOICE_SPEECH_LEVEL",
        label: "Speech level",
        description: "Recording threshold in dBFS. Raise it in a noisy room; lower it for quiet speech.",
        scope: "client",
        type: "number",
        placeholder: "-40",
        min: -90,
        max: 0,
        step: 0.5,
      },
      {
        key: "VOICE_TTS_VOICE",
        label: "Speech voice",
        description: "Piper voice used by the client.",
        scope: "client",
        type: "text",
        placeholder: "es_ES-sharvard-medium",
        maxLength: 200,
      },
      {
        key: "VOICE_TTS_SPEAKER",
        label: "Speech speaker",
        description: "Speaker id for multi-speaker voices.",
        scope: "client",
        type: "text",
        placeholder: "F",
        maxLength: 20,
      },
      {
        key: "VOICE_CONNECT_TIMEOUT",
        label: "Connect timeout",
        description: "Maximum time to connect to a voice service, for example 3s.",
        scope: "client",
        type: "text",
        placeholder: "3s",
        maxLength: 50,
      },
      {
        key: "VOICE_OPERATION_TIMEOUT",
        label: "Operation timeout",
        description: "Maximum time for one voice operation, for example 30s.",
        scope: "client",
        type: "text",
        placeholder: "30s",
        maxLength: 50,
      },
      {
        key: "VOICE_LLM_MODE",
        label: "Language model mode",
        description: "fallback uses the model only for unresolved sentences; off disables it.",
        scope: "client",
        type: "select",
        options: [
          { value: "fallback", label: "fallback" },
          { value: "off", label: "off" },
        ],
      },
      {
        key: "VOICE_LLM_SAFETY",
        label: "Inferred-action safety",
        description: "guard only allows plausible actions; states only allows physical states; literal trusts the model.",
        scope: "client",
        type: "select",
        options: [
          { value: "guard", label: "guard" },
          { value: "states", label: "states" },
          { value: "literal", label: "literal" },
        ],
      },
      {
        key: "VOICE_LLM_ORDER",
        label: "Fallback order",
        description: "Home Assistant before the model, or the model before Home Assistant.",
        scope: "client",
        type: "select",
        options: [
          { value: "ha-first", label: "ha-first" },
          { value: "model-first", label: "model-first" },
        ],
      },
      {
        key: "VOICE_ROOM",
        label: "Room",
        description: "Room where the assistant is, so that here means that room.",
        scope: "client",
        type: "text",
        placeholder: "Salon",
        maxLength: 100,
      },
      {
        key: "VOICE_LEARNING_FILE",
        label: "Learning file",
        description: "Learned associations file on the client host. Use off to disable learning.",
        scope: "client",
        type: "text",
        placeholder: "client/data/learning.json",
        maxLength: 500,
      },
      {
        key: "VOICE_LEARNING_URL",
        label: "Learning dashboard URL",
        description: "Snapshot and queued-edit endpoint used by the Learning page.",
        scope: "client",
        type: "text",
        placeholder: "http://127.0.0.1:3005/api/learning",
        maxLength: 500,
      },
      {
        key: "VOICE_ACTIONS_FILE",
        label: "Action catalog file",
        description: "File where voice-client actions writes the generated catalog.",
        scope: "client",
        type: "text",
        placeholder: "client/data/actions.json",
        maxLength: 500,
      },
      {
        key: "VOICE_PROMPT_URL",
        label: "Prompt URL",
        description: "Dashboard prompt endpoint read by the client at startup.",
        scope: "client",
        type: "text",
        placeholder: "http://127.0.0.1:3005/api/settings/prompt",
        maxLength: 500,
      },
      {
        key: "VOICE_LOG_LEVEL",
        label: "Log level",
        description: "Client log level.",
        scope: "client",
        type: "select",
        options: [
          { value: "debug", label: "debug" },
          { value: "info", label: "info" },
          { value: "warn", label: "warn" },
          { value: "error", label: "error" },
        ],
      },
      {
        key: "VOICE_LOG_TRANSCRIPTS",
        label: "Log transcripts",
        description: "Log recognized speech. It can contain private data.",
        scope: "client",
        type: "select",
        options: trueFalse,
      },
      {
        key: "VOICE_LOG_HTTP_ADDR",
        label: "Log page address",
        description: "Read-only log page address, for example 0.0.0.0:8081. Empty disables it.",
        scope: "client",
        type: "text",
        placeholder: "0.0.0.0:8081",
        maxLength: 255,
      },
      {
        key: "VOICE_TEXT_ADDR",
        label: "Text command address",
        description: "Address where the assistant accepts text commands. Empty disables them.",
        scope: "client",
        type: "text",
        placeholder: "127.0.0.1:8082",
        maxLength: 255,
      },
      {
        key: "VOICE_NEWS_URL",
        label: "News service URL",
        description: "URL of the news service read by the assistant, for example http://127.0.0.1:8091.",
        scope: "client",
        type: "text",
        placeholder: "http://127.0.0.1:8091",
        maxLength: 500,
      },
    ],
  },
  {
    id: "model",
    title: "Model endpoint",
    description:
      "Set the OpenAI-compatible base URL. The model name is optional and detected from /v1/models. Connect model probes the endpoint before saving, and the voice client applies the change without a restart.",
    fields: [
      {
        key: "VOICE_LLM_URL",
        label: "Endpoint URL",
        description: "Base URL of the chat-completions endpoint, for example http://127.0.0.1:8080.",
        scope: "client",
        type: "text",
        placeholder: "http://127.0.0.1:8080",
        maxLength: 500,
      },
      {
        key: "VOICE_LLM_MODEL",
        label: "Model name",
        description: "Model sent with every request. Empty uses the first model reported by the endpoint; local is the llama.cpp default.",
        scope: "client",
        type: "text",
        placeholder: "Detected from /v1/models",
        maxLength: 200,
      },
      {
        key: "VOICE_LLM_TIMEOUT",
        label: "Timeout",
        description: "Maximum time for one model call, for example 30s.",
        scope: "client",
        type: "text",
        placeholder: "30s",
        maxLength: 50,
      },
      {
        key: "VOICE_LLM_API_KEY",
        label: "API key",
        description: "Optional bearer token for a remote endpoint. It is stored write-only and never shown again.",
        scope: "client",
        type: "password",
        placeholder: "Optional",
        maxLength: 500,
      },
    ],
  },
];

const fieldsByKey = new Map<string, SettingField>();
for (const section of settingsSections) {
  for (const field of section.fields) {
    fieldsByKey.set(field.key, field);
  }
}

export function settingField(key: string): SettingField | undefined {
  return fieldsByKey.get(key);
}

export function isKnownSetting(key: string): boolean {
  return fieldsByKey.has(key);
}

export function validateSettingValue(
  field: SettingField,
  value: string,
): string | null {
  if (value === "") {
    return null;
  }
  if (value.length > (field.maxLength ?? 2000)) {
    return `${field.label} is too long.`;
  }
  if (/[\u0000\r\n]/.test(value)) {
    return `${field.label} must fit on one line.`;
  }
  switch (field.type) {
    case "number": {
      const parsed = Number(value);
      if (!Number.isFinite(parsed)) {
        return `${field.label} must be a number.`;
      }
      if (field.min !== undefined && parsed < field.min) {
        return `${field.label} must be at least ${field.min}.`;
      }
      if (field.max !== undefined && parsed > field.max) {
        return `${field.label} must be at most ${field.max}.`;
      }
      return null;
    }
    case "select": {
      if (!field.options?.some((option) => option.value === value)) {
        return `${field.label} has an unsupported value.`;
      }
      return null;
    }
    case "boolean": {
      if (value !== "true" && value !== "false") {
        return `${field.label} must be true or false.`;
      }
      return null;
    }
    default:
      return null;
  }
}

export function serverSettingKeys(): string[] {
  return settingsSections.flatMap((section) =>
    section.fields
      .filter((field) => field.scope === "server" || field.scope === "shared")
      .map((field) => field.key),
  );
}

export function clientSettingKeys(): string[] {
  return settingsSections.flatMap((section) =>
    section.fields
      .filter((field) => field.scope === "client" || field.scope === "shared")
      .map((field) => field.key),
  );
}
