# Jarvis: local voice assistant in Spanish

Say **"Aleksei"**, wait for the beep, and give a command. The assistant answers out loud and switches the lights and switches of your Home Assistant. Everything runs on your own computers: no cloud and no API keys.

```text
microphone ─▶ voice-client (Go) ──▶ openWakeWord      "Aleksei"          ┐
speaker   ◀─        │          ──▶ Whisper / Parakeet  speech to text     ├─ Docker
                    │          ──▶ Piper               text to speech     │
                    │          ──▶ llama.cpp           command fallback   ┘
                    └────────────▶ Home Assistant      lights, switches
```

Status: phases 0 to 9 of [`spec.md`](spec.md) are done. It runs every day on a Windows mini PC (Intel N100) in the living room.

## What you can say

| Say | Reply |
|---|---|
| "¿Qué hora es?" | "Son las 10 y cuarto." |
| "Enciende la luz del salón" / "Apaga …" | "Hecho." |
| "¿Qué puedes hacer?" | the commands and your devices |
| "Hay demasiada luz aquí" | "Hecho." and the light of the room turns off (with `VOICE_ROOM=Salon`) |
| "Repite lo que he dicho" | "Has dicho: …" |
| "Cancela" | "Vale, cancelado." |

Devices are the lights and switches that you expose to voice assistants in Home Assistant. A new device works within 30 s, with no restart. Misheard names are corrected by sound ("Enciende el Udelsalom" switches "la luz del salón"). See [docs/commands.md](docs/commands.md).

An optional local language model (`llama.cpp` with Qwen2.5-0.5B-Instruct, in `server/models/llm`) interprets the sentences that the fixed parser does not understand. For a sentence with a power verb and one clear device it may only choose `turn_on`/`turn_off` for that device. For a state such as "hay demasiada luz aquí" it gets every device, infers an action, and runs it at once: the light turns off without a confirmation question. `VOICE_ROOM` tells it which room "aquí" means; if the device names contain the room ("la luz del salón"), that is the device it picks. The model is limited to the exposed lights and switches, but it can still choose the wrong one, so check `msg=reasoned` if a state does something unexpected. Repeated states are learned: an inferred action counts as a success, and "Alexa, mal" within 60 s penalizes the last action so that association is not repeated. Learned sentences skip the model and run from a local JSON file; see [Learning and feedback](docs/commands.md#learning-and-feedback). Fixed commands stay with the parser, and several clauses joined by "y" or "e" run in order. `VOICE_LLM_MODE=fallback` (the default) keeps the fast parser first; `always` sends every single sentence with a clear candidate device to the model; `off` disables it. See [Language model](docs/commands.md#language-model).

## Install from scratch

### 1. Prepare Home Assistant

1. **Token.** Profile > Security > Long-lived access tokens > Create token. Copy it; Home Assistant shows it only once.
2. **Devices.** Settings > Voice assistants > Expose: expose the lights and switches you want to control. Only `light` and `switch` entities are used.
3. **Names.** The spoken name is the entity name. Give it its article, for example "La luz del salón", so the replies sound natural.

### 2. Windows mini PC

The voice services run in Docker; the client runs as `voice-client.exe`, because a container on Windows cannot use the microphone. Details: [docs/windows.md](docs/windows.md).

1. Install **Docker Desktop** (WSL 2 engine, "Start Docker Desktop when you sign in" on) and **Git** (`winget install --id Git.Git -e`).
2. Clone, in PowerShell:
   ```powershell
   cd $HOME
   git clone https://github.com/MarcosRZ/jarvis.git
   cd jarvis
   Set-ExecutionPolicy -Scope CurrentUser RemoteSigned
   ```
3. Create `client\.env.prod` (`Copy-Item client\.env.prod.example client\.env.prod`, then `notepad client\.env.prod`):
   ```
   VOICE_CONFIG=client/config.yaml
   VOICE_HA_URL=http://<home-assistant-ip>:8123
   VOICE_HA_TOKEN=<token from step 1>
   VOICE_LLM_URL=http://127.0.0.1:8080
   VOICE_LLM_MODE=fallback
   VOICE_ROOM=Salon
   VOICE_LOG_TRANSCRIPTS=true
   VOICE_LOG_HTTP_ADDR=0.0.0.0:8081
   ```
4. Create `server\.env` for a fast speech engine on a small CPU (see [docs/speech-recognition.md](docs/speech-recognition.md)):
   ```
   WHISPER_STT_LIBRARY=sherpa
   WHISPER_MODEL=auto
   ```
5. Settings > Privacy & security > Microphone: turn on "Let desktop apps access your microphone". Set the network profile to **Private**.
6. Download the client and check the audio devices:
   ```powershell
   .\scripts\run.ps1 -Update -NoServices devices
   ```
   If the right microphone or speaker is not the default, add `VOICE_INPUT_DEVICE=` / `VOICE_OUTPUT_DEVICE=` with the exact name to `client\.env.prod`.
7. Start it once in a window: `.\scripts\run.ps1`. The first start downloads the Docker images and the models, which takes a while; `docker compose logs -f whisper` in `server` shows `Ready` when the speech model is loaded. Allow `voice-client.exe` in the firewall for private networks when Windows asks.
8. Start at log-on: `.\scripts\install-autostart.ps1`, and turn on the automatic log-on of Windows.

### 3. Mac (development)

1. Install Go 1.27 or later, the Xcode Command Line Tools (`xcode-select --install`) and Docker Desktop.
2. Start the voice services: `make services-up`.
3. `cp client/.env.prod.example client/.env.prod`, fill in `VOICE_HA_URL` and `VOICE_HA_TOKEN`, `chmod 600 client/.env.prod`.
4. Run: `make run ENV=prod`. Give the terminal (or the IDE) access to the microphone, and to the local network, in System Settings > Privacy & Security.

Without a real Home Assistant, `make ha-up` starts a development one with demo lights, and `make run` uses it. See [docs/home-assistant.md](docs/home-assistant.md).

## Use

| Task | Windows (PowerShell, in `jarvis`) | Mac |
|---|---|---|
| Start the assistant | `.\scripts\run.ps1`, or `Start-ScheduledTask -TaskName 'Jarvis voice assistant'` | `make run ENV=prod` |
| Stop it | Ctrl+C, or `Stop-ScheduledTask -TaskName 'Jarvis voice assistant'` | Ctrl+C |
| List audio devices | `.\scripts\run.ps1 devices` | `client/bin/voice-client devices` |
| Microphone level / record and play back | `.\scripts\run.ps1 mic-test` / `loopback` | `client/bin/voice-client mic-test` / `loopback` |
| Test the wake word / speech recognition | `.\scripts\run.ps1 wake-test` / `stt-test` | `client/bin/voice-client wake-test` / `stt-test` |
| Show the effective settings | `.\scripts\run.ps1 -NoServices config-check` | `client/bin/voice-client config-check` |
| Read the log from another computer | `curl http://<mini-pc>:8081/?n=50` | |

All commands: [docs/cli.md](docs/cli.md). Each request logs its steps and times, and with `VOICE_LOG_TRANSCRIPTS=true` what it understood (`msg=heard`). The log page keeps the last 2000 lines in memory only: [docs/assistant.md](docs/assistant.md#log-web-page).

**Where to change things**

| File | What | Reference |
|---|---|---|
| `client/.env.prod` | Home Assistant, audio devices, timings, logging | [docs/configuration.md](docs/configuration.md) |
| `client/config.yaml` | the beep, and how misheard words are corrected | [docs/configuration.md](docs/configuration.md#configuration-file) |
| `server/.env` | speech engine, wake-word sensitivity, Piper voice | [docs/voice-services.md](docs/voice-services.md), [docs/wake-word.md](docs/wake-word.md) |

Changes in `client/` apply when the assistant starts again. Changes in `server/.env` apply with `.\scripts\run.ps1` (Windows) or `make services-up` (Mac).

## Update

**Windows mini PC**

```powershell
Stop-ScheduledTask -TaskName 'Jarvis voice assistant'   # or Ctrl+C in its window
cd $HOME\jarvis
git pull
.\scripts\run.ps1 -Update          # downloads the latest voice-client.exe, checks its SHA-256, starts
```

`-Update` takes the latest GitHub release. Configuration-only changes (`compose.yaml`, scripts, docs) need only `git pull`.

**Publish a new client** (after a change in `client/`): tag it on the Mac, and GitHub Actions builds the release:

```sh
git tag v0.1.2 && git push origin v0.1.2
```

**Mac:** `git pull`, `make services-up`, `make run ENV=prod`.

## When something goes wrong

| Symptom | Fix |
|---|---|
| `Home Assistant rejected the access token` | Wrong or expired `VOICE_HA_TOKEN`; create a new one. |
| `Home Assistant not available` / `no route to host` | Check the URL. On macOS, give the terminal or IDE "Local Network" access and restart it. |
| `no input device found` / `miniaudio: unknown error` | No microphone, or no permission: see [docs/audio.md](docs/audio.md). |
| "No te he oído." after each wake word | Speak right after the beep, or lower `VOICE_SPEECH_LEVEL` (for example `-45`). |
| Recordings always last 8 s, slow replies | The room is louder than `VOICE_SPEECH_LEVEL`; raise it (for example `-35`). |
| It wakes up by itself (TV) | Raise `WAKEWORD_THRESHOLD` in `server/.env`, for example `0.6`. |
| "Ahora no puedo entenderte…" | The speech service is down or still downloading its model: `docker compose logs whisper` in `server`. |

More: [docs/limitations.md](docs/limitations.md).

## Documentation

| | |
|---|---|
| [cli.md](docs/cli.md) | client commands |
| [configuration.md](docs/configuration.md) | every setting, env files, `config.yaml` |
| [assistant.md](docs/assistant.md) | the pipeline, states, beep, errors, privacy, log web page |
| [commands.md](docs/commands.md) | voice commands, device allowlist, misheard-word correction |
| [home-assistant.md](docs/home-assistant.md) | which devices, security, development Home Assistant |
| [wake-word.md](docs/wake-word.md) | "Aleksei", sensitivity, test procedure |
| [speech-recognition.md](docs/speech-recognition.md) | end of speech, Whisper and Parakeet, measurements |
| [speech-output.md](docs/speech-output.md) | Piper, voices, playback |
| [voice-services.md](docs/voice-services.md) | Docker services, models, images, licenses, health checks |
| [audio.md](docs/audio.md) | audio design, permissions, smoke test |
| [windows.md](docs/windows.md) | Windows details, autostart, updates |
| [protocol.md](docs/protocol.md) | Wyoming protocol |
| [development.md](docs/development.md) | build, tests, platforms, CI and releases |
| [limitations.md](docs/limitations.md) | known limitations |

Licenses: the wake-word models are CC BY-NC-SA 4.0 (non-commercial use only), and the Piper engine is GPL-3.0 in its own container. See [docs/voice-services.md](docs/voice-services.md#dependency-and-compatibility-table).
