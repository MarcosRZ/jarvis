# Jarvis: local voice assistant

Jarvis listens for a wake word, records one command, answers out loud and controls the devices exposed in Home Assistant. The voice pipeline runs on your own machines: no cloud and no API keys.

There is no fixed list of phrases. A fast local parser handles a few built-in intents, then a safe action catalog, the Home Assistant conversation agent and an optional local model resolve everything else. Ask a question or name an exposed device; if one layer cannot handle it, the next one tries.

```text
microphone -> voice-client -> openWakeWord -> Whisper / Parakeet -> command resolution -> Piper -> speaker
                                                                  |-> Home Assistant (devices, conversation)
                                                                  |-> llama.cpp (optional local model)
```

## Requirements

- Docker with Compose v2.20 or later.
- A Home Assistant URL and a long-lived access token.
- Go 1.27 or later to build the client.

## Quick start

1. Home Assistant: create a long-lived token (Profile > Security) and expose the entities you want to control (Settings > Voice assistants > Expose).
2. Configure the client:
   ```sh
   cp client/.env.example client/.env
   # set VOICE_HA_URL, VOICE_HA_TOKEN, VOICE_STT_LANGUAGE and VOICE_TTS_VOICE
   chmod 600 client/.env
   ```
3. Start the voice services:
   ```sh
   make services-up
   ```
4. Start the assistant:
   ```sh
   make run
   ```

On Windows, use `scripts/run.ps1` instead of `make`. See [Windows](#windows).

To test without a real Home Assistant:

```sh
make ha-up
make run ENV=dev
```

## Use

Say the wake word (default: "alexa"), wait for the beep, then speak. The assistant replies out loud.

Text goes through the same pipeline:

```sh
client/bin/voice-client text --text "..."
```

```sh
curl -sS -X POST http://127.0.0.1:8082/text \
  -H 'Content-Type: application/json' \
  -d '{"text":"..."}'
```

The examples above are not a command list. Ask the time, control an exposed device, ask a question, or use any sentence your Home Assistant conversation agent understands. See [docs/commands.md](docs/commands.md).

## Dashboard

The M4nhattan dashboard (`dashboard/`) shows service status, memory, logs and notifications. It starts with the voice services and listens on http://127.0.0.1:3005. See [dashboard/README.md](dashboard/README.md).

## Client commands

| Command | Purpose |
|---|---|
| `voice-client run` | Run the assistant |
| `voice-client text --text "..."` | Send text to a running assistant |
| `voice-client say --text "..."` | Speak text with Piper |
| `voice-client devices` | List audio devices |
| `voice-client mic-test` | Show the microphone level |
| `voice-client loopback` | Record and play back |
| `voice-client wake-test` | Test wake-word detection |
| `voice-client stt-test` | Test speech recognition |
| `voice-client config-check` | Print and validate the effective settings |
| `voice-client version` | Print the version |

## Configuration

| File | Purpose |
|---|---|
| `client/.env` | Home Assistant, audio devices, language, voice, model and logging |
| `client/config.yaml` | Beep and command-matching settings |
| `server/.env` | Voice services, wake-word threshold, STT engine, TTS voice |

Client changes apply when the assistant restarts. Server changes apply with `make services-up`.

All settings: [docs/configuration.md](docs/configuration.md). Services and tuning: [docs/services.md](docs/services.md).

## Windows

The client runs on Windows; the voice services run in Docker Desktop (WSL 2).

1. Install Docker Desktop, Git and the repository.
2. Create `client\.env` from `client\.env.example` and fill in the Home Assistant URL and token.
3. Allow local scripts once: `Set-ExecutionPolicy -Scope CurrentUser RemoteSigned`.
4. Check the audio devices and start:
   ```powershell
   .\scripts\run.ps1 -Update -NoServices devices
   .\scripts\run.ps1
   ```
5. Optional: `.\scripts\install-autostart.ps1` starts the assistant at log-on.

## Update

```powershell
Stop-ScheduledTask -TaskName 'Jarvis voice assistant'
cd $HOME\jarvis
git pull
.\scripts\run.ps1 -Update
```

On macOS or Linux: `git pull`, `make services-up`, `make run`.

Publish a new client after a change in `client/`:

```sh
git tag v0.1.2 && git push origin v0.1.2
```

## Troubleshooting

| Symptom | Fix |
|---|---|
| Home Assistant rejected the token | Create a new `VOICE_HA_TOKEN` |
| Home Assistant not available | Check `VOICE_HA_URL` and network access |
| No input device | Check `voice-client devices` and microphone permissions |
| Recordings always run to the maximum | Raise `VOICE_SPEECH_LEVEL` |
| Quiet speech is cut | Lower `VOICE_SPEECH_LEVEL` |
| False wake words | Raise `WAKEWORD_THRESHOLD` in `server/.env` |
| No transcription | Check `make services-status` and `make services-logs` |

More: [docs/limitations.md](docs/limitations.md).

## Documentation

| Document | Content |
|---|---|
| [configuration.md](docs/configuration.md) | Client and server settings |
| [commands.md](docs/commands.md) | How commands are resolved, learned and guarded |
| [home-assistant.md](docs/home-assistant.md) | Exposure, token and development Home Assistant |
| [services.md](docs/services.md) | Docker services, models, tuning and troubleshooting |
| [development.md](docs/development.md) | Build, tests, CI and releases |
| [limitations.md](docs/limitations.md) | Known limitations |

## Licenses

The wake-word models are CC BY-NC-SA 4.0 (non-commercial use only). The Piper engine is GPL-3.0 and runs in its own container. See `server/compose.yaml` for the pinned images.
