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
