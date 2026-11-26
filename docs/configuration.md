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
| `VOICE_KEEP_OUTPUT_AWAKE` | Hold the output open while the assistant runs, so an idle Bluetooth sink does not swallow the beep; default `true` |
| `VOICE_TAIL_DIR` | Directory for diagnostic microphone recordings after each reply; empty turns them off |
| `VOICE_TAIL_SECONDS` | Length of each diagnostic recording, default `3s` |
| `VOICE_TAIL_KEEP` | Number of diagnostic recordings kept, default `10` |
| `VOICE_STT_LANGUAGE` | Recognition language code |
| `VOICE_TTS_VOICE` | Piper voice name for your language |
| `VOICE_TTS_SPEAKER` | Speaker for multi-speaker voices |
| `VOICE_WAKEWORD_MODEL` | Wake-word model, default `alexa` |
| `VOICE_WAKEWORD_COOLDOWN` | Ignore repeat detections for this time |
| `VOICE_WAKEWORD_HIGHPASS` | High-pass cutoff in Hz for the wake-word audio only; `0` off, `120` recommended against rumble |
| `VOICE_RECORD_MAX` | Maximum recording length |
| `VOICE_RECORD_START_TIMEOUT` | Time to wait for speech after the beep |
| `VOICE_RECORD_END_SILENCE` | Silence that ends the recording |
| `VOICE_SPEECH_LEVEL` | Level that counts as speech, in dBFS |
| `VOICE_CONNECT_TIMEOUT` | Timeout for connecting to a service |
| `VOICE_OPERATION_TIMEOUT` | Timeout for one service operation |

See [audio.md](audio.md) for the wake-word high-pass and the diagnostic tails.

### Language model

| Setting | Use |
|---|---|
| `VOICE_LLM_URL` | Base URL of an OpenAI-compatible endpoint; default is the local llama.cpp at `http://127.0.0.1:8080` |
| `VOICE_LLM_MODEL` | Model name; empty uses the first model reported by the endpoint. `local` is the llama.cpp default |
| `VOICE_LLM_API_KEY` | Optional bearer token for a remote endpoint |
| `VOICE_LLM_MODE` | `fallback` (default) or `off` |
| `VOICE_LLM_ORDER` | `ha-first` (default) or `model-first` |
| `VOICE_LLM_SAFETY` | `guard` (default), `literal` or `states` |
| `VOICE_LLM_TIMEOUT` | Timeout for one model answer |
| `VOICE_PROMPT_URL` | Dashboard prompt endpoint; empty uses the built-in prompt |
| `VOICE_SETTINGS_URL` | Dashboard settings overrides read at startup; empty or `off` disables the lookup |
| `VOICE_ROOM` | Room used to resolve "here"; match it to a device name |
| `VOICE_LEARNING_FILE` | Learned associations; `off` disables learning |
| `VOICE_LEARNING_URL` | Dashboard snapshot and operation queue endpoint; `off` disables publishing and remote edits |
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
| `LLM_CTX_SIZE` | Context window, default `8192` for two ~2k-token prompts in the shared KV cache |
| `LLM_THREADS` | CPU threads for the model |
| `HA_PORT` | Development Home Assistant port |
| `DASHBOARD_PORT` | Dashboard port, default `3005` |
| `VOICE_HA_URL` | Home Assistant URL used by the collector for status, uptime and the action catalog |
| `VOICE_HA_TOKEN` | Long-lived token used by the collector; keep it aligned with `client/.env` |

The remaining model and port settings are documented in `server/.env.example`.

## Dashboard Settings overrides

The Settings page at http://127.0.0.1:3005/settings edits overrides stored in
`/data/settings.json` inside the `dashboard-data` volume (mode `0600`). The
`.env` files remain the fallback: an empty field keeps the value from
`server/.env` or `client/.env`.

For server and shared values, `GET /api/settings/compose` returns exported
variables. `make services-up` fetches them and exports them before running
`docker compose`, so environment values win over `server/.env`. If the
dashboard is not running yet, the fetch is skipped and `server/.env` is used;
run `make services-up` again once the dashboard is up. The values are not
written back to `server/.env`.

For client values, `GET /api/settings/client` returns the overrides. The
assistant reads them before the rest of the configuration when
`VOICE_SETTINGS_URL` is set (default in `client/.env.example`:
`http://127.0.0.1:3005/api/settings/client`). Restart `make run` after a change.
If the dashboard is unreachable, the assistant uses `client/.env`.

The HA URL and token are shared by the collector and the client. The settings
endpoints can return the token, so keep the dashboard on localhost or a trusted
network. Port changes require `make services-up`; the client sections require a
client restart. The prompt keeps its own endpoint and file (`/data/prompt.json`).

## Prompt

The prompt for questions and general conversation is edited in the dashboard at
http://127.0.0.1:3005/settings. It is stored in the `dashboard-data` volume. The
assistant reads it at startup, so restart `make run` after a change.

The prompt may use `{{context}}` where the read-only Home Assistant context
should go. Without it, the context is appended. The JSON reply format is always
appended. The editor shows the built-in prompt when no custom prompt is saved;
`Reset to built-in` removes the custom prompt.

## Learning store and dashboard

The dashboard `Learning` page is fed by the client, not by the collector. The
client owns the learning store; the dashboard is the editing surface and keeps a
copy of the last published state.

The client posts a JSON snapshot to `VOICE_LEARNING_URL` (default
`http://127.0.0.1:3005/api/learning`) at startup, after a change, after applying
a queued edit and every 60 seconds as a heartbeat. The dashboard stores the
snapshot in `/data/learning.json` in the `dashboard-data` volume with mode
`0600`.

Create, edit and delete operations from `/learning` and `/learning/edit` go to
`POST /api/learning/operations`. The dashboard appends them to
`/data/learning-operations.json` (also mode `0600`) and shows their result
immediately. The client polls `GET /api/learning/operations?after=<id>` every 2
seconds, applies each operation to its local store and publishes a snapshot with
`appliedThrough` set to the last handled id. The dashboard drops acknowledged
operations from the queue.

Manual entries follow the same key rules as automatic learning: lowercase, no
accents or punctuation, no filler words such as "por favor". The store is keyed
by that normalized sentence, so editing the sentence replaces the old key. The
dashboard rejects a second association with the same normalized sentence unless
it is editing the existing one. The client does not check whether a device or
action still exists in Home Assistant; the command handler applies the same
existence checks as for learned entries.

Set `VOICE_LEARNING_URL` to `off`, or pass an empty `--learning-url`, to disable
publishing and remote edits. Changes queued while the client is stopped stay in
the dashboard until it runs again, so the page can show a state that the local
store does not have yet.

The phrases are recognized speech and can be private. The dashboard has no
authentication; keep it on localhost or a trusted network. If the client runs on
another machine, point `VOICE_LEARNING_URL` at the dashboard host and prefer
HTTPS or a trusted private network.
