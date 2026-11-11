# Configuration

## Files

| File | Scope |
|---|---|
| `client/.env` | Client: Home Assistant, audio, language, voice, model, logging |
| `client/.env.dev` | Development client; used by `make run ENV=dev` |
| `client/config.yaml` | Beep and command matching |
| `server/.env` | Voice services, models, thresholds, dashboard port |

Copy `client/.env.example` to `client/.env` and `server/.env.example` to `server/.env`. Do not commit either file. A CLI flag wins over an environment variable; an environment variable wins over the default.

Every setting is listed with a comment in `client/.env.example` and `server/.env.example`. The example files ship with one language and voice configured; set the language and voice settings to match your setup.

## Client

### Home Assistant

| Setting | Use |
|---|---|
| `VOICE_HA_URL` | Home Assistant URL, for example `http://homeassistant.local:8123` |
| `VOICE_HA_TOKEN` | Long-lived access token |
| `VOICE_HA_AGENT` | Conversation agent for unresolved sentences; empty uses the default agent |
| `VOICE_CONFIG` | Path to `config.yaml` |

### Voice pipeline

| Setting | Use |
|---|---|
| `VOICE_WAKEWORD_ADDR` | openWakeWord address, default `127.0.0.1:10400` |
| `VOICE_STT_ADDR` | Speech recognition address, default `127.0.0.1:10300` |
| `VOICE_TTS_ADDR` | Piper address, default `127.0.0.1:10200` |
| `VOICE_INPUT_DEVICE` | Microphone name; empty uses the system default |
| `VOICE_OUTPUT_DEVICE` | Speaker name; empty uses the system default |
| `VOICE_STT_LANGUAGE` | Recognition language code |
| `VOICE_TTS_VOICE` | Piper voice name for your language |
| `VOICE_TTS_SPEAKER` | Speaker for multi-speaker voices |
| `VOICE_WAKEWORD_MODEL` | Wake-word model, default `alexa` |
| `VOICE_WAKEWORD_COOLDOWN` | Ignore repeat detections for this time |
| `VOICE_RECORD_MAX` | Maximum recording length |
| `VOICE_RECORD_START_TIMEOUT` | Time to wait for speech after the beep |
| `VOICE_RECORD_END_SILENCE` | Silence that ends the recording |
| `VOICE_SPEECH_LEVEL` | Level that counts as speech, in dBFS |
| `VOICE_CONNECT_TIMEOUT` | Timeout for connecting to a service |
| `VOICE_OPERATION_TIMEOUT` | Timeout for one service operation |

### Language model

| Setting | Use |
|---|---|
| `VOICE_LLM_URL` | llama.cpp server, default `http://127.0.0.1:8080` |
| `VOICE_LLM_MODE` | `fallback` (default) or `off` |
| `VOICE_LLM_ORDER` | `ha-first` (default) or `model-first` |
| `VOICE_LLM_SAFETY` | `guard` (default), `literal` or `states` |
| `VOICE_LLM_TIMEOUT` | Timeout for one model answer |
| `VOICE_ROOM` | Room used to resolve "here"; match it to a device name |
| `VOICE_LEARNING_FILE` | Learned associations; `off` disables learning |
| `VOICE_ACTIONS_FILE` | Snapshot written by `voice-client actions` |

### Logging and text

| Setting | Use |
|---|---|
| `VOICE_LOG_LEVEL` | `debug`, `info`, `warn` or `error` |
| `VOICE_LOG_TRANSCRIPTS` | Log what was heard; may contain private text |
| `VOICE_LOG_HTTP_ADDR` | Read-only log page, for example `127.0.0.1:8081` |
| `VOICE_TEXT_ADDR` | Text command endpoint, default `127.0.0.1:8082` |

## config.yaml

```yaml
wake_word:
  beep:
    enabled: true
    notes: [880, 1318.5]
    note_duration: 70ms
    fade: 8ms
    level: -14
commands:
  matching:
    accept: 0.8
    margin: 0.1
```

`notes` are frequencies in Hz. `level` is the peak in dBFS. `accept` is the minimum sound-alike score for a built-in phrase; `margin` is how far the best guess must beat the next one.

## Server

| Setting | Use |
|---|---|
| `BIND_ADDRESS` | Address for published ports, default `127.0.0.1` |
| `WAKEWORD_THRESHOLD` | Wake-word sensitivity; raise it to reduce false wakes |
| `WAKEWORD_REFRACTORY_SECONDS` | Minimum time between detections |
| `WHISPER_MODEL` | Recognition model, for example `base`, `small`, `medium` |
| `WHISPER_LANGUAGE` | Recognition language code |
| `WHISPER_STT_LIBRARY` | `faster-whisper` (default) or `sherpa` |
| `WHISPER_COMPUTE_TYPE` | `default` or `int8` for speed on a slow CPU |
| `WHISPER_CPU_THREADS` | CPU threads for recognition |
| `WHISPER_INITIAL_PROMPT` | Words that steer recognition; add your device names |
| `PIPER_VOICE` | Voice downloaded and used by Piper |
| `LLM_MODEL` | GGUF file in `server/models/llm` |
| `LLM_BIND_ADDRESS` | Address for the model port |
| `LLM_CTX_SIZE` | Context window; keep at least `4096` |
| `LLM_THREADS` | CPU threads for the model |
| `HA_PORT` | Development Home Assistant port |
| `DASHBOARD_PORT` | Dashboard port, default `3005` |

The remaining model and port settings are documented in `server/.env.example`.
