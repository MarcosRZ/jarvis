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
| `--wakeword-model` | `VOICE_WAKEWORD_MODEL` | `alexa` (say "Aleksei") | a model that openWakeWord lists |
| `--wakeword-cooldown` | `VOICE_WAKEWORD_COOLDOWN` | `2s` | Go duration, 0 or greater |
| `--stt-language` | `VOICE_STT_LANGUAGE` | `es` | language code for Whisper, not empty |
| `--record-max` | `VOICE_RECORD_MAX` | `8s` | greater than 0, at most `30s` (one Whisper window) |
| `--record-start-timeout` | `VOICE_RECORD_START_TIMEOUT` | `3s` | greater than 0, at most `--record-max` |
| `--record-end-silence` | `VOICE_RECORD_END_SILENCE` | `1s` | greater than 0 |
| `--speech-level` | `VOICE_SPEECH_LEVEL` | `-40` | dBFS, from `-90` to below `0` |
| `--tts-voice` | `VOICE_TTS_VOICE` | `es_ES-sharvard-medium` | a Piper voice name, not empty |
| `--tts-speaker` | `VOICE_TTS_SPEAKER` | `F` (female) | a speaker of `--tts-voice`, checked with Piper; `--tts-speaker ""` for a voice with one speaker |
| `--ha-url` | `VOICE_HA_URL` | empty (no device control) | `http://host:port` or `https://host:port` |
| (no flag) | `VOICE_HA_TOKEN` | empty | a Home Assistant long-lived access token; necessary with `--ha-url` |
| `--log-level` | `VOICE_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `--log-transcripts` | `VOICE_LOG_TRANSCRIPTS` | `false` | `true` logs what Whisper heard at info level (`msg=heard text=...`); `false` logs it only at `debug` |
| `--log-http-addr` | `VOICE_LOG_HTTP_ADDR` | empty (off) | `host:port` for the log web page of `run`, for example `0.0.0.0:8080`; see [Log web page](assistant.md#log-web-page) |
| `--connect-timeout` | `VOICE_CONNECT_TIMEOUT` | `3s` | Go duration, greater than 0 |
| `--operation-timeout` | `VOICE_OPERATION_TIMEOUT` | `30s` | Go duration, greater than 0; limit for one read or write on a service connection |
| `--config` | `VOICE_CONFIG` | empty (defaults) | path of a YAML configuration file; see "Configuration file" below |

## Env files

`make run` builds the client and runs the assistant with the settings in `client/.env.$(ENV)`:

```sh
make run                          # client/.env.dev: the development Home Assistant (make ha-up)
make run ENV=prod                 # client/.env.prod: your own Home Assistant
make run ARGS="--log-level info"  # flags still win over the file
```

- `client/.env.dev` is committed and has no secrets. When it has no `VOICE_HA_TOKEN`, `make run` uses the token in `server/homeassistant/dev-token`.
- `client/.env.prod` is git-ignored. Copy it from `client/.env.prod.example`, set `VOICE_HA_URL` and `VOICE_HA_TOKEN`, and run `chmod 600 client/.env.prod`.
- The settings come only from the file: `make run` first removes the `VOICE_*` variables of your shell, so a development run never uses a production token by mistake.
- The file is read by the shell (`set -a; . file`): put values with spaces in quotes.

## Configuration file

`--config` (`VOICE_CONFIG`) names a YAML file with the settings that have no flag or variable. Empty means the defaults. `client/.env.dev` and `client/.env.prod.example` set `VOICE_CONFIG=client/config.yaml`, the committed file with every key and its rules. A key that is not in the file keeps its default; an unknown key or a bad value stops the client, like a bad flag. `config-check` shows the file and the values.

| Key | Default | Rule |
|---|---|---|
| `wake_word.beep.enabled` | `true` | `true` or `false` |
| `wake_word.beep.notes` | `[880, 1318.5]` (A5, E6) | 1 to 8 frequencies, from 20 to 20000 Hz |
| `wake_word.beep.note_duration` | `70ms` | greater than 0, at most `1s` |
| `wake_word.beep.fade` | `8ms` | 0 to half of `note_duration` |
| `wake_word.beep.level` | `-14` | peak in dBFS, from -60 to below 0 |
| `commands.matching.accept` | `0.8` | score greater than 0, at most 1; 1 uses only names that sound the same |
| `commands.matching.suggest` | `0.7` | score greater than 0, at most `accept` |
| `commands.matching.margin` | `0.1` | from 0 to below 1 |

An invalid configuration stops the client before it starts and shows each bad value with its flag and variable name:

```text
$ VOICE_WAKEWORD_COOLDOWN=soon client/bin/voice-client config-check --tts-addr piper
invalid configuration:
VOICE_WAKEWORD_COOLDOWN="soon": not a duration; use a value such as 500ms or 5s
--tts-addr (VOICE_TTS_ADDR) "piper": use host:port, for example 127.0.0.1:10200
```

The wake-word sensitivity (threshold) is not a client setting. See [wake-word.md](wake-word.md).
