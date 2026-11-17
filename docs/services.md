# Services

The voice services and the dashboard run in Docker. The client runs on the machine with the microphone and speaker.

## Commands

Run these from the repository root:

```sh
make services-up       # download the model if needed and wait until healthy
make services-status   # state, failed health checks and verification
make services-logs     # last 100 lines, then follow
make services-down     # stop and remove the containers; models stay on disk
```

Configuration comes from `server/.env`. Copy `server/.env.example` to `server/.env` to change defaults. It is not committed.

## Services

| Service | Port | Purpose |
|---|---|---|
| openWakeWord | 10400 | Wake-word detection |
| Whisper / Parakeet | 10300 | Speech recognition |
| Piper | 10200 | Speech output |
| llama.cpp | 8080 | Optional local language model |
| dashboard | 3005 | M4nhattan status dashboard |
| collector | none (internal 8090) | Read-only Docker and Home Assistant collector for the dashboard |
| Home Assistant (dev) | 8123 | Development only, with `make ha-up` |

Ports bind to `127.0.0.1` by default. Set `BIND_ADDRESS=0.0.0.0` only on a trusted network: the services have no authentication. Keep the model port on `127.0.0.1` unless another machine needs it (`LLM_BIND_ADDRESS`).

## Models

| Service | Folder | First start |
|---|---|---|
| Whisper | `server/models/whisper` | Downloads the selected model |
| Piper | `server/models/piper` | Downloads `PIPER_VOICE` |
| openWakeWord | Inside the image | Nothing |
| llama.cpp | `server/models/llm` | `make services-up` downloads the default model |

The folders are bind mounts, so models survive `make services-down` and image updates. They are git-ignored. The first start is slower while models download and load.

If a download is interrupted, Whisper can fail with `Unable to open file 'model.bin'`. Remove the incomplete cache folder under `server/models/whisper` and start again.

## Tuning

### Wake word

- `WAKEWORD_THRESHOLD` (default `0.5`): raise it to reduce false wakes, lower it if the wake word is missed.
- `WAKEWORD_REFRACTORY_SECONDS` (default `2`): minimum time between detections.
- `VOICE_WAKEWORD_MODEL`: model name sent by the client. The bundled models are English; other wake words need a custom model.

### Speech recognition

- `WHISPER_MODEL`: `base` is fast; `small` and `medium` are more accurate and slower.
- `WHISPER_LANGUAGE`: set it to your language.
- `WHISPER_STT_LIBRARY=sherpa` with `WHISPER_MODEL=auto` uses NVIDIA Parakeet, which is faster on some machines and detects the language itself.
- `WHISPER_COMPUTE_TYPE=int8` and `WHISPER_CPU_THREADS=<cores>` speed up a slow CPU.
- `WHISPER_INITIAL_PROMPT`: add the words and device names the recognizer should expect.

### Speech output

- `PIPER_VOICE`: the voice to download and use. Choose a voice for your language.
- Piper downloads another requested voice on its first use into `server/models/piper`.

### Language model

- `LLM_MODEL`: GGUF file in `server/models/llm`. The downloader knows only the default file; put another file there yourself.
- `LLM_CTX_SIZE`: keep at least `4096` for the command prompt and the state snapshot.
- `LLM_THREADS`: CPU threads for the model.
- `VOICE_LLM_MODE=off` in the client disables the model entirely.

## Dashboard settings

Open http://127.0.0.1:3005/settings to edit the language model prompt. It is stored in the `dashboard-data` volume and served at `/api/settings/prompt`. The client reads it at startup through `VOICE_PROMPT_URL`; restart the assistant after saving.

The editor shows the built-in prompt when no custom prompt is saved. `Reset to built-in` removes the custom prompt. `{{context}}` is replaced with the read-only Home Assistant context.

## Dashboard live data

The dashboard does not have the Docker socket. The `collector` service is the only consumer: it mounts `/var/run/docker.sock` read-only, runs as a non-root user with a read-only filesystem, drops all capabilities and does not publish a port. The dashboard route handlers call it on the compose network and expose `GET /api/status`, `GET /api/logs` and `GET /api/actions` to the browser.

The collector reads only an allowlist of compose services and makes read-only Docker API calls. It never proxies arbitrary Docker paths or container ids. Log lines are capped and common token patterns are redacted.

The collector keeps a background snapshot refreshed every 2 seconds and serves that cached copy to status requests. If Docker is temporarily unavailable, the API keeps serving the last good snapshot with the Docker source marked unavailable instead of returning empty services.

The action catalog is generated separately from the container snapshot. The collector reads the Home Assistant exposure list over the WebSocket API and the entity states over the REST API, then applies the same catalog rules as the client. It refreshes the catalog every 30 seconds and keeps the last good catalog when Home Assistant is unreachable.

The collector container needs read access to the socket group:

```sh
stat -c '%g' /var/run/docker.sock   # for example 973
```

`make services-up` passes the host group id automatically. For a direct `docker compose` call, set `DOCKER_SOCKET_GID` in `server/.env`. Compose also adds group 0 because Docker Desktop exposes the mounted socket as `root:root` inside the container.

Set `VOICE_HA_URL` and `VOICE_HA_TOKEN` in `server/.env` to add the external Home Assistant status card, the dashboard action catalog and, when a System Monitor `last_boot` or uptime sensor exists, the host uptime. Without them the Home Assistant card is offline, the action catalog is disabled and host uptime shows `—`. Keep these values aligned with `client/.env`, so the dashboard catalog matches the running assistant.

The standalone `dashboard/compose.yaml` does not include the collector. It is useful for UI work only; live data routes report the collector as unavailable.

## Text and log endpoints

| Endpoint | Default | Purpose |
|---|---|---|
| `POST /text` | `127.0.0.1:8082` | Send `{"text":"..."}` to a running assistant |
| `GET /` | `VOICE_LOG_HTTP_ADDR` | Read-only log page |

Both are disabled by default or bind only to localhost. They have no login and the text endpoint can control exposed devices, so use them only on a trusted network. Do not use port 8080 for the log page: the model listens there.

## Audio checks

```sh
client/bin/voice-client devices
client/bin/voice-client mic-test --duration 10s
client/bin/voice-client loopback --duration 5s
```

`mic-test` shows the peak level; speech should rise above about `-40 dBFS`. On macOS, a terminal without microphone permission captures zeros: allow it in System Settings > Privacy & Security > Microphone and restart the terminal.

## Common failures

| Symptom | Check |
|---|---|
| Service restarts | `make services-status` and `make services-logs` |
| Piper unhealthy | The voice file is missing; see the Piper logs |
| Wake word not detected | `WAKEWORD_THRESHOLD`, microphone, and the model name |
| Recording never ends | `VOICE_SPEECH_LEVEL`; steady noise can count as speech |
| Model answers are slow or missing | `LLM_CTX_SIZE`, `LLM_THREADS`, and whether the model container is healthy |

`make services-status` prints the last log lines of a service that is not running and the health-check output of an unhealthy service.
