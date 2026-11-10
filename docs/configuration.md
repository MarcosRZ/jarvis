# Configuration

All client settings: flags, environment variables, env files and the YAML file. The voice services are configured in `server/.env` (see [voice-services.md](voice-services.md)).

A flag wins over an environment variable. An environment variable wins over the default.

| Flag | Environment variable | Default | Rule |
|---|---|---|---|
| `--wakeword-addr` | `VOICE_WAKEWORD_ADDR` | `127.0.0.1:10400` | `host:port`, port 1 to 65535 |
| `--stt-addr` | `VOICE_STT_ADDR` | `127.0.0.1:10300` | `host:port`, port 1 to 65535 |
| `--tts-addr` | `VOICE_TTS_ADDR` | `127.0.0.1:10200` | `host:port`, port 1 to 65535 |
| `--input-device` | `VOICE_INPUT_DEVICE` | empty (system default) | device name from `devices` |
| `--output-device` | `VOICE_OUTPUT_DEVICE` | empty (system default) | device name from `devices` |
| `--wakeword-model` | `VOICE_WAKEWORD_MODEL` | `alexa` (say "Alexa") | a model that openWakeWord lists |
| `--wakeword-cooldown` | `VOICE_WAKEWORD_COOLDOWN` | `2s` | Go duration, 0 or greater |
| `--stt-language` | `VOICE_STT_LANGUAGE` | `es` | language code for Whisper, not empty |
| `--record-max` | `VOICE_RECORD_MAX` | `8s` | greater than 0, at most `30s` (one Whisper window) |
| `--record-start-timeout` | `VOICE_RECORD_START_TIMEOUT` | `3s` | greater than 0, at most `--record-max` |
| `--record-end-silence` | `VOICE_RECORD_END_SILENCE` | `1s` | greater than 0 |
| `--speech-level` | `VOICE_SPEECH_LEVEL` | `-40` | dBFS, from `-90` to below `0` |
| `--tts-voice` | `VOICE_TTS_VOICE` | `es_ES-sharvard-medium` | a Piper voice name, not empty |
| `--tts-speaker` | `VOICE_TTS_SPEAKER` | `F` (female) | a speaker of `--tts-voice`, checked with Piper; `--tts-speaker ""` for a voice with one speaker |
| `--ha-url` | `VOICE_HA_URL` | empty (no Home Assistant) | `http://host:port` or `https://host:port`; the exposed entities become a safe action catalog, unresolved sentences go to the conversation agent, and exposed states/forecasts become read-only model context |
| (no flag) | `VOICE_HA_TOKEN` | empty | a Home Assistant long-lived access token; necessary with `--ha-url` |
| `--ha-agent` | `VOICE_HA_AGENT` | empty (Home Assistant default agent) | conversation agent entity ID, for example `conversation.ollama`; it handles sentences that the action catalog does not resolve, and questions |
| `--llm-url` | `VOICE_LLM_URL` | `http://127.0.0.1:8080` | `http://host:port` or `https://host:port` of the llama.cpp command/grounded-reply model; `VOICE_LLM_MODE=off` disables it |
| `--llm-mode` | `VOICE_LLM_MODE` | `fallback` | `off` or `fallback` (unresolved sentences) |
| `--llm-safety` | `VOICE_LLM_SAFETY` | `guard` | `guard` (the named device or catalog entity/action, or a matching physical state), `literal` (any model action) or `states` (only a physical state) |
| `--llm-order` | `VOICE_LLM_ORDER` | `ha-first` | `ha-first` (Home Assistant before the model) or `model-first` (model before Home Assistant) for an unresolved sentence; a deterministic catalog match always runs first |
| `--llm-timeout` | `VOICE_LLM_TIMEOUT` | `30s` | Go duration, greater than 0; limit for one model answer |
| `--room` | `VOICE_ROOM` | empty (unknown) | room where the assistant is, for example `Salon`; lets the model resolve "aquí" and ask about the right light |
| `--learning-file` | `VOICE_LEARNING_FILE` | `client/data/learning.json` | JSON file with learned sentences and their feedback; `off` turns learning off |
| `--actions-file` | `VOICE_ACTIONS_FILE` | `client/data/actions.json` | JSON file where `voice-client actions` writes the generated catalog; the directory is git-ignored; the assistant regenerates the catalog at runtime and does not read this file |
| `--log-level` | `VOICE_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `--log-transcripts` | `VOICE_LOG_TRANSCRIPTS` | `false` | `true` logs what Whisper heard at info level (`msg=heard text=...`); `false` logs it only at `debug` |
| `--log-http-addr` | `VOICE_LOG_HTTP_ADDR` | empty (off) | `host:port` for the log web page of `run`, for example `0.0.0.0:8081`; not 8080, which is the command model; see [Log web page](assistant.md#log-web-page) |
| `--text-addr` | `VOICE_TEXT_ADDR` | `127.0.0.1:8082` | `host:port` where `run` accepts text commands for the `text` command; empty turns the channel off; see [Text channel](assistant.md#text-channel) |
| `--connect-timeout` | `VOICE_CONNECT_TIMEOUT` | `3s` | Go duration, greater than 0 |
| `--operation-timeout` | `VOICE_OPERATION_TIMEOUT` | `30s` | Go duration, greater than 0; limit for one read or write on a service connection, and for one Home Assistant conversation request |
| `--config` | `VOICE_CONFIG` | empty (defaults) | path of a YAML configuration file; see "Configuration file" below |

## Env files

`make run` builds the client and runs the assistant with the settings in `client/.env`; `make run ENV=dev` uses `client/.env.dev`:

```sh
make run                          # client/.env: your Home Assistant
make run ENV=dev                  # client/.env.dev: the development Home Assistant (make ha-up)
make run ARGS="--log-level info"  # flags still win over the file
```

- `client/.env` is git-ignored. Copy it from `client/.env.example`, set `VOICE_HA_URL` and `VOICE_HA_TOKEN`, and run `chmod 600 client/.env`. The example also enables the local command model and sets `VOICE_ROOM=Salon`: change it to the room where the assistant is, or leave it empty. Set `VOICE_LLM_MODE=off` to turn off the local command model; the deterministic catalog and Home Assistant still handle the exposed devices and questions. Set `VOICE_HA_AGENT` to use a conversation agent other than the default.
- `client/.env.dev` is committed and has no secrets; use it with `make run ENV=dev`. When it has no `VOICE_HA_TOKEN`, `make run ENV=dev` uses the token in `server/homeassistant/dev-token`.
- The settings come only from the file: `make run` first removes the `VOICE_*` variables of your shell, so a development run never uses a production token by mistake.
- The file is read by the shell (`set -a; . file`): put values with spaces in quotes.

## Configuration file

`--config` (`VOICE_CONFIG`) names a YAML file with the settings that have no flag or variable. Empty means the defaults. `client/.env.example` and `client/.env.dev` set `VOICE_CONFIG=client/config.yaml`, the committed file with every key and its rules. A key that is not in the file keeps its default; an unknown key or a bad value stops the client, like a bad flag. `config-check` shows the file and the values.

| Key | Default | Rule |
|---|---|---|
| `wake_word.beep.enabled` | `true` | `true` or `false` |
| `wake_word.beep.notes` | `[880, 1318.5]` (A5, E6) | 1 to 8 frequencies, from 20 to 20000 Hz |
| `wake_word.beep.note_duration` | `70ms` | greater than 0, at most `1s` |
| `wake_word.beep.fade` | `8ms` | 0 to half of `note_duration` |
| `wake_word.beep.level` | `-14` | peak in dBFS, from -60 to below 0 |
| `commands.matching.accept` | `0.8` | score greater than 0, at most 1; 1 uses only fixed commands that sound the same |
| `commands.matching.margin` | `0.1` | from 0 to below 1 |

An invalid configuration stops the client before it starts and shows each bad value with its flag and variable name:

```text
$ VOICE_WAKEWORD_COOLDOWN=soon client/bin/voice-client config-check --tts-addr piper
invalid configuration:
VOICE_WAKEWORD_COOLDOWN="soon": not a duration; use a value such as 500ms or 5s
--tts-addr (VOICE_TTS_ADDR) "piper": use host:port, for example 127.0.0.1:10200
```

The wake-word sensitivity (threshold) is not a client setting. See [wake-word.md](wake-word.md).
