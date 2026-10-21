# Voice services

Prerequisite: Docker with Compose v2.20 or later (for `start_interval` and `!reset`). No API key or secret is necessary.

## Start, stop, inspect

Run these from the repository root:

```sh
make services-up        # docker compose up -d --wait: returns when all three are healthy
make services-status    # state, reason of a failed health check, then scripts/verify.sh
make services-logs      # last 100 log lines of each service, then follows
make services-down      # stops and removes the containers, also the development Home Assistant; the models stay on disk
```

Configuration is in environment variables. To change a default, copy `server/.env.example` to `server/.env` and edit it. `server/.env` is not committed.

Network: each service publishes one port, on `127.0.0.1` only (`BIND_ADDRESS`). Other computers cannot connect. Set `BIND_ADDRESS=0.0.0.0` only when the client runs on another computer of a trusted network. The services have no authentication.

Shutdown: `make services-down` stops each service in about 1 s. openWakeWord and Whisper use `init: true`: without it, their PID 1 ignores SIGTERM, and Docker kills them after 10 s.

## Models

| Service | Folder | First start | Next starts |
|---|---|---|---|
| Whisper | `server/models/whisper` (Hugging Face cache) | Downloads `Systran/faster-whisper-base` (about 145 MB) | Uses the cache, no network necessary |
| Piper | `server/models/piper` | Downloads `PIPER_VOICE` (`.onnx` and `.onnx.json`, about 77 MB for `es_ES-sharvard-medium`) | Uses the files, no network necessary |
| openWakeWord | inside the image | Nothing to download | Nothing to download |

The folders are bind mounts, so the models stay after `make services-down` and after a new image. Verified: after `down` and `up`, the services were healthy in 6 s and the logs had no download. `server/models/*` is in `.gitignore`.

Piper downloads other voices on the first `synthesize` that names them, into the same folder.

## When a model is missing

| Case | What you see |
|---|---|
| Unknown `WHISPER_MODEL` | Whisper exits with `Invalid model size '...', expected one of: ...`. `docker compose ps` shows `Restarting`. |
| Whisper model not on disk and no network | Whisper exits with `LocalEntryNotFoundError ... check your internet connection`. Shows `Restarting`. |
| Unknown `PIPER_VOICE` | Piper exits with `VoiceNotFoundError: <voice>`. Shows `Restarting`. |
| Piper voice not on disk and no network | Piper logs `URLError` and starts. The health check fails with `unhealthy: voice file /data/<voice>.onnx is missing; see the piper logs`. |

`make services-status` prints the last log lines of a service that is not running, and the health check output of a service that is `unhealthy`.

If a first start stops during the download, Whisper can fail with `Unable to open file 'model.bin'`. Remove `server/models/whisper/models--Systran--faster-whisper-base` and start again.

## Dependency and compatibility table

| Service | Image (pinned) | Upstream source | Port | Model or voice | License |
|---|---|---|---|---|---|
| Wake word | `rhasspy/wyoming-openwakeword:2.1.0@sha256:52cb1168…d42b` | github.com/rhasspy/wyoming-openwakeword, github.com/dscripka/openWakeWord | 10400 | `alexa` v0.1 (built in), used for "Aleksei" | Code: Apache-2.0. Pre-trained models: CC BY-NC-SA 4.0 (non-commercial) |
| STT | `rhasspy/wyoming-whisper:3.8.1@sha256:ba6fcb60…0d80` | github.com/rhasspy/wyoming-faster-whisper, github.com/SYSTRAN/faster-whisper | 10300 | `Systran/faster-whisper-base` (multilingual, `es` set) | Wrapper: MIT. faster-whisper: MIT. Model: MIT |
| TTS | `rhasspy/wyoming-piper:2.5.2@sha256:7d39aafa…af7a` | github.com/rhasspy/wyoming-piper, github.com/OHF-Voice/piper1-gpl | 10200 | `es_ES-sharvard-medium`, speaker `F` (22050 Hz) | Wrapper: MIT. Piper engine: GPL-3.0. Voice dataset: CC BY 3.0 (attribution required) |

With `WHISPER_STT_LIBRARY=sherpa` and `WHISPER_MODEL=auto`, the STT service runs NVIDIA Parakeet TDT 0.6B v3 (CC BY 4.0) instead of Whisper; see [speech-recognition.md](speech-recognition.md). The full digests are in `server/compose.yaml`. No service needs an API key or an external API at run time. Internet access is only necessary for the first model download (Hugging Face).

## Health checks

Each health check sends `describe` and checks the `info` reply, so it fails when the service accepts connections but does not answer.

- Whisper: the image check (`wyoming_faster_whisper.health_check`).
- Piper: `server/compose.yaml` first checks that the voice file is on disk, then runs the image check (`wyoming_piper.health_check`). The image check alone stays healthy when the voice download failed.
- openWakeWord: the image has no check. `server/compose.yaml` adds the same `describe` check, with the `wyoming` library of the image. It also checks that the `info` reply has a wake program.
- During the start period, the checks run every 5 s, so `make services-up` returns as soon as the services are ready.
