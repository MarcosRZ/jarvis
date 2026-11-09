# Jarvis: local voice assistant in Spanish

Say **"Alexa"**, wait for the beep, and give a command. The assistant answers out loud and controls the devices exposed to your Home Assistant through a safe local action catalog, asks the Home Assistant conversation agent for phrases and questions that the catalog does not handle, and falls back to a read-only local context for questions if the agent cannot answer. The voice pipeline runs on your own computers: no cloud and no API keys. A cloud-based conversation agent in Home Assistant would send that sentence out; the default agent is local.

You can also send the same commands as text while the assistant runs: `client/bin/voice-client text --text "¿Qué hora es?"`. The text skips the wake word and speech recognition and takes the same path as a spoken command, including the reply out loud. Both channels stay available. See [Text channel](docs/assistant.md#text-channel).

```text
microphone ─▶ voice-client (Go) ──▶ openWakeWord      "Alexa"            ┐
speaker   ◀─        │          ──▶ Whisper / Parakeet  speech to text     ├─ Docker
                    │          ──▶ Piper               text to speech     │
                    │          ──▶ llama.cpp           commands / replies ┘
                    └────────────▶ Home Assistant      devices, Assist
```

Status: phases 0 to 9 of [`spec.md`](spec.md) are done. It runs every day on a Windows mini PC (Intel N100) in the living room.

## What you can say

| Say | Reply |
|---|---|
| "¿Qué hora es?" | "Son las 10 y cuarto." |
| "Enciende la luz del salón" / "Apaga …" | "Hecho." |
| "¿Qué puedes hacer?" | the commands and your devices |
| "Hay demasiada luz aquí" | "Hecho." and the light of the room turns off (with `VOICE_ROOM=Salon`) |
| "Abre la persiana" | "Hecho." and the cover opens |
| "Pon el aire a 22 grados" | "Hecho." and the climate entity changes |
| "¿Qué tiempo hace?" | the weather answer from Home Assistant, or from the local context if Home Assistant cannot answer |
| "Repite lo que he dicho" | "Has dicho: …" |
| "Cancela" | "Vale, cancelado." |

Devices are the entities that you expose to the `conversation` assistant in Home Assistant. Lights and switches run locally when the sentence clearly names the device; an unclear or unexposed device is rejected. Any other exposed domain, and questions such as the weather, go first to the Home Assistant conversation agent; if it does not answer a question, the local model can answer from a read-only snapshot of exposed states and forecasts. Expose the weather entity to `conversation`, not only to Home Assistant's Amazon Alexa integration (`cloud.alexa`). A new light or switch works within 30 s, with no restart. See [docs/commands.md](docs/commands.md) and [docs/home-assistant.md](docs/home-assistant.md).

An optional local language model (`llama.cpp` with Qwen2.5-0.5B-Instruct, in `server/models/llm`) handles any unresolved sentence that is not in learned memory. A deterministic matcher already runs a known verb plus a unique exposed device (for example "enciende la tele", "sube el volumen", "abre la persiana"); the model receives the same safe action catalog for the ambiguous phrases. It can choose one or more exposed catalog actions (up to four) or an allowlisted light/switch, which run in the requested order without a confirmation question when the safety guard accepts them. The default order asks Home Assistant first and the model after it fails. `VOICE_ROOM` tells it which room "aquí" means; if the device names contain the room ("la luz del salón"), that is the device it picks. The catalog limits actions to exposed entities, but it includes locks, covers, valves, scenes, scripts and climate when they are exposed; the guard also requires a known verb for those sensitive domains. It can still choose the wrong exposed action, so check `msg=reasoned` if an inference does something unexpected. It also gets a read-only snapshot of up to 50 exposed entities and 4 forecast entries, so it may answer questions such as "¿Va a llover?" from the exposed forecast when Home Assistant does not. With the 0.5B model this is best effort: it can echo or misread the context, and the assistant then says it does not know. It has no internet or general chat knowledge, and replies are not learned. Repeated single-action catalog and inferred commands are learned: an action counts as a success, and "Alexa, mal" within 60 s penalizes the last action so that association is not repeated. A positive learned sentence skips the model and Home Assistant and runs from a local JSON file; see [Learning and feedback](docs/commands.md#learning-and-feedback). Fixed commands stay with the parser; one fixed command per clause, joined by "y" or "e", runs in order. `VOICE_LLM_MODE=fallback` (the default) keeps the fast parser first and runs the model only for unresolved sentences; `off` disables it. `VOICE_LLM_SAFETY=guard` (the default) runs an inferred action only when the sentence names the chosen entity/action or describes a matching physical state; `literal` runs any model action, and `states` only physical states. `VOICE_LLM_ORDER=ha-first` (the default) asks Home Assistant before the model; `model-first` reverses that order and lets the model learn phrases that Home Assistant would otherwise handle. See [Language model](docs/commands.md#language-model).

## Install from scratch

### 1. Prepare Home Assistant

1. **Token.** Profile > Security > Long-lived access tokens > Create token. Copy it; Home Assistant shows it only once.
2. **Devices.** Settings > Voice assistants > Expose: expose every entity you want to control or ask about, including lights, switches, covers, climate, media players, locks and weather. The exposed entities become a safe local action catalog; ambiguous phrases and questions use the Home Assistant conversation agent, with the local model as the fallback for questions from the exposed state/forecast snapshot.
3. **Names.** For local catalog devices the spoken name and aliases are the entity name and aliases: give the name its article, for example "La luz del salón", so the replies sound natural. The conversation agent uses the names, aliases and areas that it understands.

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
   VOICE_LLM_SAFETY=guard
   VOICE_LLM_ORDER=ha-first
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
| Send a text command | `.\scripts\run.ps1 text --text "¿Qué hora es?"` | `client/bin/voice-client text --text "¿Qué hora es?"` |
| Show the effective settings | `.\scripts\run.ps1 -NoServices config-check` | `client/bin/voice-client config-check` |
| Read the log from another computer | `curl http://<mini-pc>:8081/?n=50` | |

All commands: [docs/cli.md](docs/cli.md). Each request logs its steps and times, and with `VOICE_LOG_TRANSCRIPTS=true` what it understood (`msg=heard`). The log page keeps the last 2000 lines in memory only: [docs/assistant.md](docs/assistant.md#log-web-page).

**Where to change things**

| File | What | Reference |
|---|---|---|
| `client/.env.prod` | Home Assistant, audio devices, timings, logging | [docs/configuration.md](docs/configuration.md) |
| `client/config.yaml` | the beep, and how misheard fixed commands are corrected | [docs/configuration.md](docs/configuration.md#configuration-file) |
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
| [commands.md](docs/commands.md) | voice commands, device allowlist, fixed-phrase correction |
| [home-assistant.md](docs/home-assistant.md) | which devices, security, development Home Assistant |
| [wake-word.md](docs/wake-word.md) | "Alexa", sensitivity, test procedure |
| [speech-recognition.md](docs/speech-recognition.md) | end of speech, Whisper and Parakeet, measurements |
| [speech-output.md](docs/speech-output.md) | Piper, voices, playback |
| [voice-services.md](docs/voice-services.md) | Docker services, models, images, licenses, health checks |
| [audio.md](docs/audio.md) | audio design, permissions, smoke test |
| [windows.md](docs/windows.md) | Windows details, autostart, updates |
| [protocol.md](docs/protocol.md) | Wyoming protocol |
| [development.md](docs/development.md) | build, tests, platforms, CI and releases |
| [limitations.md](docs/limitations.md) | known limitations |

Licenses: the wake-word models are CC BY-NC-SA 4.0 (non-commercial use only), and the Piper engine is GPL-3.0 in its own container. See [docs/voice-services.md](docs/voice-services.md#dependency-and-compatibility-table).
