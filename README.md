# Local Voice Assistant PoC

A local voice assistant: a native Go client and three Wyoming voice services in Docker Compose. The plan is in `spec.md`.

Status: Phases 0 to 9 are done (integration contract, Go client skeleton, cross-platform audio, Wyoming protocol client, Docker voice services, wake word, speech to text, speech output, end-to-end pipeline, safe commands). `run` is the assistant: say "Aleksei", then "¿Qué hora es?", "Repite lo que he dicho", "Cancela" or "¿Qué puedes hacer?". With Home Assistant (`--ha-url`), it also switches the lights and switches that are exposed to voice assistants in Home Assistant, for example "Enciende la luz de la cocina". A new device needs no rebuild and no restart.

## Client

### Prerequisites

- Go 1.27 or later.
- A C compiler (cgo), for the native audio library `github.com/gen2brain/malgo` v0.11.26 (miniaudio, public domain). On macOS: Xcode Command Line Tools.

### Build and check

```sh
make build              # client/bin/voice-client, VERSION=x.y.z sets the version
make test               # go test ./...; needs no voice service and no audio device
make integration        # tests against the running Docker services (see "Voice services")
make lint               # gofmt check and go vet ./...
make clean
```

### Commands

```sh
client/bin/voice-client version
client/bin/voice-client config-check [flags]   # validate and print the effective configuration
client/bin/voice-client devices                # list audio input and output devices
client/bin/voice-client mic-test [--duration 10s] [flags]   # capture and show the peak level each second
client/bin/voice-client loopback [--duration 5s] [flags]    # record, then play the recording
client/bin/voice-client wake-test [flags]      # listen for the wake word and print each detection
client/bin/voice-client stt-test [flags]       # wake word, then print the command that Whisper recognizes
client/bin/voice-client say --text "..." [--text "..."] [flags]   # speak each text in order with Piper
client/bin/voice-client run [flags]            # the assistant; stop with Ctrl+C (SIGINT) or SIGTERM
client/bin/voice-client <command> -h           # list the flags of a command
```

`version` and `config-check` need no voice service and no audio device. Exit codes: 0 OK, 1 runtime error, 2 usage error, 3 invalid configuration. `mic-test` accepts up to `1h`. `loopback` accepts up to `30s`, because it keeps the recording in memory.

`devices` shows each device with the name that `--input-device` and `--output-device` accept. When two devices of one kind have the same name, the second one gets ` #2`, the third ` #3`, in the order of the system list:

```text
output devices:
  MSI G32CQ4 E2
  MSI G32CQ4 E2 #2
  Altavoces del MacBook Pro (default)
```

`mic-test` ends with a summary. Use it to check memory and lost audio over a long capture:

```text
captured 1m0.03s of audio in 1m0.001s, dropped 0 bytes, heap 0.3 MiB at start and 0.4 MiB at end
```

### Audio design

- Internal format: signed 16-bit little-endian PCM, mono, 16000 Hz (`audio.SpeechFormat`). Playback accepts any rate, for example 22050 Hz from Piper.
- Each stream asks miniaudio for the internal format. When the device format is different, miniaudio converts the format, the channels and the rate (linear resampler). The log shows both formats for each stream, for example `rate=16000 device_rate=48000 converted=true`. When the formats are the same, miniaudio does no conversion.
- The audio callback does no network I/O and allocates no memory. It only copies samples into, or out of, a fixed-size ring buffer (capture: 2 s, playback: 1 s).
- Capture: when the reader is too slow, the ring drops the oldest audio and counts the dropped bytes. Playback: when the ring is empty, the device plays silence. A writer waits while the ring is full.
- When a device stops without a request (for example, it is disconnected), blocked reads and writes return `audio device stopped unexpectedly`. The client does not reconnect to the device automatically.

### Configuration

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
| `--log-http-addr` | `VOICE_LOG_HTTP_ADDR` | empty (off) | `host:port` for the log web page of `run`, for example `0.0.0.0:8080`; see "Log web page" |
| `--connect-timeout` | `VOICE_CONNECT_TIMEOUT` | `3s` | Go duration, greater than 0 |
| `--operation-timeout` | `VOICE_OPERATION_TIMEOUT` | `30s` | Go duration, greater than 0; limit for one read or write on a service connection |
| `--config` | `VOICE_CONFIG` | empty (defaults) | path of a YAML configuration file; see "Configuration file" |

**Env files.** `make run` builds the client and runs the assistant with the settings in `client/.env.$(ENV)`:

```sh
make run                          # client/.env.dev: the development Home Assistant (make ha-up)
make run ENV=prod                 # client/.env.prod: your own Home Assistant
make run ARGS="--log-level info"  # flags still win over the file
```

- `client/.env.dev` is committed and has no secrets. When it has no `VOICE_HA_TOKEN`, `make run` uses the token in `server/homeassistant/dev-token`.
- `client/.env.prod` is git-ignored. Copy it from `client/.env.prod.example`, set `VOICE_HA_URL` and `VOICE_HA_TOKEN`, and run `chmod 600 client/.env.prod`.
- The settings come only from the file: `make run` first removes the `VOICE_*` variables of your shell, so a development run never uses a production token by mistake.
- The file is read by the shell (`set -a; . file`): put values with spaces in quotes.

**Configuration file.** `--config` (`VOICE_CONFIG`) names a YAML file with the settings that have no flag or variable. Empty means the defaults. `client/.env.dev` and `client/.env.prod.example` set `VOICE_CONFIG=client/config.yaml`, the committed file with every key and its rules. A key that is not in the file keeps its default; an unknown key or a bad value stops the client, like a bad flag. `config-check` shows the file and the values.

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

The wake-word sensitivity (threshold) is not a client setting. See "Wake word".

### Wake word

`wake-test` streams the microphone to openWakeWord and prints each detection. It does not use Whisper or Piper.

```text
$ client/bin/voice-client wake-test
say the wake word (alexa); press Ctrl+C to stop
21:50:12.480  detected alexa  latency 112ms
^C
1 detections in 25s, dropped 0 bytes of audio
```

**Set the sensitivity (threshold).** openWakeWord compares each 80 ms of audio with a threshold from 0 to 1. The service has one threshold for all clients, and the Wyoming protocol cannot change it, so it is set on the server, not in the client:

1. Edit `server/.env` (copy it from `server/.env.example` first). Set `WAKEWORD_THRESHOLD`, for example `WAKEWORD_THRESHOLD=0.6`.
   - A lower value (for example `0.3`) detects a quiet or unclear wake word more easily, but gives more false detections.
   - A higher value (for example `0.7`) gives fewer false detections, but you must say the wake word more clearly.
   - The default is `0.5`.
2. Apply it: `make services-up`. Compose creates the openwakeword container again with the new value.
3. Check it: `cd server && docker compose exec openwakeword cat /proc/1/cmdline | tr '\0' ' '` shows `... --threshold 0.6 --refractory-seconds 2`.
4. Test it with `wake-test` (see the test procedure below).

**Wake word: "Aleksei".** openWakeWord has no Spanish models, and training a custom model is separate work. The built-in English `alexa` model reacts to the "a-lek-s..." sound, so it detects "Aleksei" in Spanish and in English. Test with Piper voices (2026-10-20):

| Phrase | `alexa` model |
|---|---|
| "Aleksei.", "Oye, Aleksei.", "Aleksei, ¿qué hora es?" (Spanish and English voice) | detected |
| "Alexa.", "Alexis.", "Alex, pásame la sal." | detected: these names also wake the assistant |
| "Alejandro viene mañana.", "Es una lección difícil.", "El texto es largo.", "Hay que elegir una opción.", "A lo mejor llueve.", "Los alelíes del jardín.", "Buenos días, ¿cómo estás?" | not detected |

Real voice (macOS, built-in microphone, threshold `0.5`, 2026-10-20): the user said "Aleksei" in `wake-test` and it was detected. No counts were recorded; use the test procedure below for numbers.

To use only "Aleksei", or another word, train a custom model and mount it with `--custom-model-dir`.

**Model.** `--wakeword-model` selects the model. At each connection the client asks openWakeWord with `describe` whether it offers that model, and stops with the list of available models when it does not. Without this check, openWakeWord would silently listen for `okay_nabu`.

**Double triggers.** openWakeWord ignores the same model for `WAKEWORD_REFRACTORY_SECONDS` (default `2`) after a detection, but this restarts on each new connection. The client also ignores detections for `--wakeword-cooldown` (default `2s`) after a detection, also across reconnects.

**Service failure.** When the connection fails, the client logs a warning and connects again after 0.5 s, then 1 s, 2 s, and up to 30 s. Microphone audio from that time stays in the 2 s capture buffer (older audio is dropped), and the client sends it after it connects again.

**Latency.** The client sends audio in 80 ms chunks. `latency` is the time from sending the chunk where openWakeWord found the wake word, to receiving the detection. It does not include the time you spoke before that chunk ended (at most 80 ms) or the capture buffering. Measured with `make integration`: about 112 ms.

**Test procedure: false negatives and false positives.**

1. False negatives: run `wake-test`. Say the wake word 20 times, with a pause of 5 s or more between them. Do this at 0.5 m and at 3 m from the microphone. Missed = 20 minus the number of detections.
2. False positives: run `wake-test` for 30 minutes or more with normal room sound (talking, TV, music) and no wake word. Count the detections.
3. Write down the threshold, the microphone, the distance and the results. Change `WAKEWORD_THRESHOLD` and repeat if necessary.

`make integration` also runs a fixed check. Piper says "Aleksei" (Spanish voice, then English voice), and each must give exactly one detection. Piper then says a Spanish sentence without the wake word, and it must give none.

### Speech to text

`stt-test` waits for the wake word, records one command, sends it to Whisper and prints the text. Then it waits for the wake word again. Stop it with Ctrl+C.

```text
$ client/bin/voice-client stt-test
say the wake word (alexa), then a command; press Ctrl+C to stop
22:10:03.112  wake word (latency 108ms), listening
    recorded 1.7s (end: silence), Whisper 418ms, total 431ms: "¿Qué hora es?"
```

**When the recording ends.** Whisper cannot find the end of speech itself (`requires_external_vad`), so the client does it. It measures the RMS level of each 20 ms of audio. Audio at `--speech-level` (default -40 dBFS) or louder is speech. The recording ends at the first of these:

| End | Rule | Result |
|---|---|---|
| `silence` | speech started, then `--record-end-silence` (1 s) with no speech | sent to Whisper |
| `max-duration` | `--record-max` (8 s) of audio | cut at 8 s and sent to Whisper |
| `no-speech` | no speech in the first `--record-start-timeout` (3 s) | not sent; `stt-test` prints `no speech within 3s` |

The times count audio, not clock time. A pause shorter than 1 s does not end the recording. If the room is noisy and recordings always run to 8 s, raise `--speech-level` (for example `-35`). If quiet speech is cut, lower it (for example `-45`). Use `mic-test` to see the levels of your room and your voice.

**Audio retention.** The recording is in one memory buffer of `--record-max` size. The client clears it after the transcription, also when there was no speech. No audio is written to disk. The `run` command logs the text only with `--log-transcripts` or at `--log-level debug`; `stt-test` prints it.

**Errors.** When Whisper is not available, is too slow (`--operation-timeout`) or sends an error, `stt-test` logs the error, prints `transcription failed` and waits for the wake word again. An empty transcript prints `nothing recognized`.

**Timing.** `Whisper` is the time from the end of the upload to the text (processing). `total` also includes the connection and the upload. The time from the end of your speech to the text is about `--record-end-silence` plus `total`.

**Whisper model: `base` or `small`.** Measured on 2026-10-20 (Mac with Apple silicon, Docker, CPU), with Piper's Spanish voice, 3 runs of each command, through `audio.Record`:

| Command | `base` (3 runs) | `small` (3 runs) |
|---|---|---|
| "Buenos días. Por favor, enciende la luz de la cocina." | correct 3 | correct 2, "Buenos Diras" 1 |
| "¿Qué hora es?" | correct 2, "¡Qué horas!" 1 | correct 1, "¡Qué horas!" 2 |
| "Repite lo que he dicho." | correct 2, "perdíte" 1 | correct 3 |
| "Cancela." | correct 3 | correct 3 |
| "¿Qué puedes hacer?" | correct 1, "y puedes hacer" 2 | correct 3 |
| Whisper time per short command | about 0.42 s | about 1.40 s |
| Memory of the container | about 470 MiB | about 1.5 GiB |
| Download | about 145 MB | about 480 MB |

Real voice (macOS, built-in microphone, 2026-10-20): the user ran `stt-test` and the commands were recognized. The project keeps `base`, because it is about 3 times faster.

Short commands from your own voice can give other results. To use `small`, set `WHISPER_MODEL=small` in `server/.env` and run `make services-up`. The `small` model is already in `server/models/whisper` from this measurement.

`make integration` sends the sentence and the four commands through `audio.Record` and Whisper. The sentence must contain "cocina". The commands are logged with their times, and fail only when the text is empty.

**Whisper speed.** `WHISPER_COMPUTE_TYPE`, `WHISPER_BEAM_SIZE` and `WHISPER_CPU_THREADS` in `server/.env` set `--compute-type`, `--beam-size` and `--cpu-threads`. The defaults keep the image behavior (`default`, `0`, `4`): the `base` model is float16, which Whisper computes as float32 on a CPU. Measured on 2026-10-21 on the Mac, 15 transcriptions each: `int8` took 276 ms on average instead of 482 ms, with the same text for the commands. `--beam-size 1` was not faster (456 ms with float32) and gave "¡Tempe, Céla!" for "Cancela." once. On a slow CPU, set `WHISPER_COMPUTE_TYPE=int8` and `WHISPER_CPU_THREADS` to the number of cores, then run `make services-up`.

**Initial prompt.** Whisper takes a text that steers it toward expected words (`--initial-prompt`, set with `WHISPER_INITIAL_PROMPT` in `server/.env`). The default is the command list: `¿Qué hora es? Repite lo que he dicho. Cancela. ¿Qué puedes hacer?`. Measured on 2026-10-20 with `base`: in the end-to-end test, "¿Qué hora es?" was correct in 5 of 5 runs with the prompt and in 1 of 5 without it ("¿Qué orais?"). All five STT samples were exact with the prompt, also the long sentence that is not in it. The prompt adds about 40 ms. When Phase 9 adds commands, add their phrases to the prompt. To turn the prompt off, set `WHISPER_INITIAL_PROMPT=` (empty) and run `make services-up`.

### Speech output

`say` sends each `--text` to Piper and plays the speech on `--output-device`, one after the other:

```text
$ client/bin/voice-client say --text "Hola, soy Aleksei." --text "Son las diez y cuarto."
1: 1.36s of speech, Piper 120ms, playback 1.554s
2: 1.16s of speech, Piper 56ms, playback 1.359s
```

**Format.** The client reads the sample rate and channels from Piper's `audio-start` event. It does not assume a rate. `es_ES-sharvard-medium` gives 22050 Hz mono. The output device is opened in that format for each response, and miniaudio converts it to the device format (for example 48000 Hz stereo).

**Checks on the audio.** The client accepts 16-bit samples only, one format for the whole response, and whole frames in each chunk. A response with no audio, or with more than 60 s of audio, is an error. These limits keep a bad response from using unbounded memory.

**Playback lifecycle.** For each response: synthesize, open the output, write, wait until the device has played everything, close. The output is always closed, also after an error. Playback has a deadline of the audio length plus 2 s, so a stuck device ends with `playback did not finish in time` and does not block the client. `playback` in the output is about 0.2 s longer than the speech, because the client waits for the device buffer to empty.

**Errors.** Empty text is not sent to Piper. An unknown voice gives `service error VoiceNotFoundError: <voice>`. An unknown output device, or Piper not available, gives a clear error. `say` continues with the next text and exits with code 1 when one text failed.

**Voice.** `--tts-voice` selects the Piper voice. Piper downloads a voice that is not on disk on its first use (see "Models"). Measured: Piper takes 30 to 120 ms for a short sentence.

Spanish voices with a female speaker (the Piper voice list has no gender field):

| Voice | Speaker | Accent | Dataset license |
|---|---|---|---|
| `es_ES-sharvard-medium` | `F` (the voice also has `M`) | Spain | CC BY 3.0 |
| `es_AR-daniela-high` | (one speaker) | Argentina | CC BY-SA 4.0 |
| `es_MX-claude-high` | (one speaker) | Mexico | Apache-2.0 |

**Speaker.** `--tts-speaker` selects one speaker of a voice with several, for example `--tts-voice es_ES-sharvard-medium --tts-speaker F`. Piper uses the default speaker for an unknown name and only logs a warning, so the client checks the speaker with `describe` before it speaks. It stops with `speaker not offered by the voice: "X" for "es_ES-sharvard-medium"; available: M, F`.

The assistant voice is `es_ES-sharvard-medium` with speaker `F`, chosen by the user on 2026-10-20. `PIPER_VOICE` in `server/.env` should name the same voice: Piper loads it at start, and its health check needs it on disk. To use a voice with one speaker, also give `--tts-speaker ""` (an empty `VOICE_TTS_SPEAKER` counts as not set, so use the flag), for example `say --tts-voice es_AR-daniela-high --tts-speaker "" --text "Hola"`. The integration tests still use `es_ES-davefx-medium` to make Spanish test speech.

`make integration` asks Piper for three sentences in a row and checks the format (22050 Hz mono) and the length. It also checks that an unknown voice gives `VoiceNotFoundError`.

### Assistant

`run` connects all parts. It needs the three voice services and a microphone and speaker. Example log (the times are typical values, they change with each request):

```text
$ client/bin/voice-client run
level=INFO msg="assistant started" version=dev wake_word_model=alexa voice=es_ES-sharvard-medium
level=INFO msg=state from=STARTING to=LISTENING after=0s
level=INFO msg=state from=LISTENING to=RECORDING after=8.2s wake_latency=108ms
level=INFO msg=state from=RECORDING to=TRANSCRIBING after=1.7s recorded=1.7s end=silence
level=INFO msg=state from=TRANSCRIBING to=HANDLING_COMMAND after=462ms stt_processing=448ms stt_total=462ms
level=INFO msg=state from=HANDLING_COMMAND to=SPEAKING after=0s
level=INFO msg=spoke response_latency=590ms synthesis=120ms audio=1.36s playback=1.55s
level=INFO msg=state from=SPEAKING to=LISTENING after=1.68s
```

**States.** `STARTING → LISTENING → RECORDING → TRANSCRIBING → HANDLING_COMMAND → SPEAKING → LISTENING`, and `STOPPED` at the end. Each change is logged with `from`, `to` and `after` (the time in the previous state), and with the measures of that step. `response_latency` is the time from the end of the recording to the start of the spoken reply.

**One request at a time.** One goroutine runs all states, so a new wake word cannot start a second request while one is in progress. Each recording is handled once; nothing is retried.

**Wake sound.** After the wake word, the assistant plays a short beep on the output device and then starts to record. Start speaking after the beep. It drops the microphone audio from the time of the beep, so the beep never starts the recording. With the default beep and the device drain, the recording starts about 0.4 s after the detection. When the beep cannot play, the log shows `wake sound failed` and the request goes on without it. The beep is generated in `audio.Beep` (no sound file); change it in `wake_word.beep` of the configuration file (see "Configuration file").

**No self-hearing.** The wake-word detector runs only in `LISTENING`. When the assistant enters `LISTENING`, it first drops the microphone audio buffered meanwhile (up to 2 s, `audio.Capture.Discard`), so its own voice is never taken as a wake word or a command.

**Errors.** A failed request does not stop the assistant. It speaks a short message and listens again:

| Case | Reply |
|---|---|
| No speech after the wake word | "No te he oído." |
| Whisper returns no text | "No te he entendido." |
| Whisper not available, too slow, or an error | "Ahora no puedo entenderte. El servicio de voz no responde." |
| Unknown command | "Todavía no sé hacer eso." |
| Piper or the speaker fails | logged; the assistant listens again |

The assistant stops only for a microphone failure, an unknown wake-word model or speaker, or Ctrl+C. When openWakeWord is down, the detector reconnects (see "Wake word").

**Commands.** See "Commands" below.

**Privacy.** The command text (`msg=heard`, also when it is empty) is logged only with `--log-transcripts` or at `--log-level debug`. `client/.env.dev` turns it on; it is off by default. Recordings are cleared after the transcription.

**End-to-end test.** `make integration` runs the pipeline with the real services and the real detector, recorder, Whisper client, command handler and Piper speaker. Only the speaker output is a fake. The input is Piper speech (`es_ES-davefx-medium`) fed at real-time speed, as from a microphone: three times "Aleksei", 1 s of silence and a command ("¿Qué hora es?", "¿Qué puedes hacer?", "Repite lo que he dicho."), each followed by 4 s of silence. Each reply must be correct, and the third must repeat the text of the second. Measured on 2026-10-20: 3 of 3 runs passed, about 21.6 s for the three requests (of which about 17 s is the input itself).

### Commands

`internal/commands` is the boundary between the recognized text and any action:

1. **Parse** (`Handler.Parse`): the text is normalized (lowercase, no accents, no punctuation, single spaces) and compared with a fixed list of phrases. The result is a typed `Command` (`Kind`, and a `Device` for device commands). Parse has no side effects.
2. **Execute** (`Handler.Execute`): runs the command and returns the spoken reply. Only `turn-on` and `turn-off` with a device from the allowlist reach a device controller.

The text never selects a program, a shell command, a URL or an ID. There is no language model.

| Kind | Phrases (after normalization) | Reply |
|---|---|---|
| `time` | "que hora es", "que horas son", "que hora son", "dime la hora", "sabes que hora es", "que hora es ya", and the Whisper errors "que horas", "que ahora es" | "Son las 10 y cuarto.", "Es la una en punto." (12-hour spoken Spanish) |
| `repeat` | "repite lo que he dicho", "repite", "repitelo" | "Has dicho: <your previous text>", or "Todavía no has dicho nada." |
| `cancel` | "cancela", "cancelar", "olvidalo" | "Vale, cancelado." |
| `help` | "que puedes hacer", "que sabes hacer", "ayuda" | the list of commands, and the allowlisted devices if there are any |
| `turn-on`, `turn-off` | "enciende", "prende" or "apaga", then a device name from the allowlist | "Hecho." or "No he podido encender la luz de la cocina." |
| `invalid` | "enciende", "prende" or "apaga" with no device, an unknown device, or extra words | "No me has dicho qué dispositivo." or `No conozco el dispositivo "horno".` No action. |
| `unknown` | everything else | "Todavía no sé hacer eso." No action. |

"Repite" repeats the last text that was not itself a repeat, so two repeats give the same text.

**Device allowlist.** A device has an `ID` for the controller (for example `light.kitchen`) and a spoken `Name` with its article (for example `la luz de la cocina`). `commands.New` rejects an allowlist with an empty ID or name, a name with characters other than letters, digits and spaces, a name that starts with a command word, or two names that are the same after normalization. The parser ignores a leading article, so "apaga luz del salón" and "apaga la luz del salón" both match. A device phrase that is not a name exactly goes through the misheard-word correction below: "enciende la luz de la cocina; rm -rf /" is `invalid`, with the kitchen light only as a suggestion.

**Misheard words.** When the text is not a command, a verb or a device name exactly, the parser takes the one that sounds most alike (`commands/match.go`). It compares the phrases without spaces, because Whisper joins and splits words, and after Spanish sound rules (z/c→s, v→b, silent h, ll→y, final m→n, and more). The score is 1 minus the edit distance divided by the longer length. Only names from the allowlist and the fixed phrases can be chosen.

| Score of the best guess | Result | Example (development names) |
|---|---|---|
| `accept` (0.8) or more, and `margin` (0.1) above the next device | the command runs | "Enciende el Udelsalom" → la luz del salón (0.82) |
| `suggest` (0.7) or more | `invalid`; the reply suggests one or two devices and nothing runs | "Enciende la luz del baño" → "No conozco el dispositivo "luz del bano". ¿Querías decir la luz del salón o la luz del techo?" |
| below `suggest` | `invalid` or `unknown`, as before | "Enciende el horno" |

`suggest` is above 0.69, the score of a wrong light that shares "luz del" with the right one. Change the values in `commands.matching` of the configuration file. Each guess is logged as `msg=guess heard=... meant=... score=... used=...`, at info level with `--log-transcripts` and at debug level otherwise.

**Device controller.** `commands.DeviceController` has one method, `SetPower(ctx, id, on)`. It receives only IDs from the allowlist. The unit tests use a fake controller that records each call and can fail. Without `--ha-url`, `run` has no allowlist and no controller, and device commands reply "Todavía no puedo controlar dispositivos." With `--ha-url`, the allowlist and the controller come from Home Assistant (see "Home Assistant").

**Allowlist from a source.** `Handler.UseSource` makes `Handle` read the allowlist again from a `DeviceSource` when the last read is older than a set time. When the source fails, the last list stays in use. `Handler.SetDevices` leaves out a device with a bad name (for example `Entrance Color + White Lights`) and reports it once per new list; the other devices stay usable. A name that starts with an article is spoken with a lowercase article: "La luz de la cocina" gives "No he podido encender la luz de la cocina."

### Home Assistant

With `--ha-url` and `VOICE_HA_TOKEN`, the assistant switches lights and switches in Home Assistant.

**Which devices.** The allowlist is the list of `light` and `switch` entities that are exposed to the Home Assistant voice assistant (Settings > Voice assistants > Expose; in the exposure settings this assistant is `conversation`). The spoken name is the entity name in Home Assistant. Give each device a name with its article for natural replies, for example "La luz de la cocina". Locks, covers, scripts and all other domains are never controlled, also when they are exposed. A device in the state `unavailable` (for example, unplugged) stays in the allowlist, but "¿Qué puedes hacer?" leaves it out; the log line `Home Assistant devices` marks it `(unavailable)`.

**New devices need no rebuild and no restart.** At start, and then before a command when the last read is older than 30 s, the client reads the exposed devices again. A device that you expose (or hide) in Home Assistant is usable (or rejected) at the latest 30 s later. The log line `Home Assistant devices` at start shows the count and the names.

**Security.**

- The client reads the exposure list over the WebSocket API (`homeassistant/expose_entity/list`) and the names over `GET /api/states`.
- It switches a device only with `POST /api/services/<light|switch>/<turn_on|turn_off>` and `{"entity_id": "<id>"}`. It checks the ID form (`domain.object_id`, lowercase) and the domain before each call.
- The token comes only from `VOICE_HA_TOKEN`, so it is not in the process list or the shell history. `config-check` shows only `(set)` or `(not set)`.
- A wrong token stops `run` at start with `Home Assistant rejected the access token`. When Home Assistant is down, `run` starts anyway, logs a warning, and tries again at the next command.
- Each Home Assistant request has a limit of 5 s.

**Development Home Assistant.** `server/compose.yaml` has a Home Assistant service (`ghcr.io/home-assistant/home-assistant:2026.10.2`, pinned by digest) in the profile `homeassistant`. It has the `demo` integration, which gives 6 lights and 2 switches. `make services-up` does not start it.

```sh
make ha-up      # start it, and on the first start: create the user, the token and the Spanish names
make run        # build, then run the assistant with client/.env.dev and the dev-token
make ha-down    # stop and remove the container; the data stays in server/homeassistant
make ha-reset   # also delete the data, except configuration.yaml, for a new first start
```

On the first start, `client/cmd/ha-dev-setup`:

1. Creates the user `dev` with a random password, and completes the onboarding.
2. Saves a long-lived token (3650 days) to `server/homeassistant/dev-token` and the password to `server/homeassistant/dev-password`, both with mode 0600. Both are git-ignored, and the tool prints neither.
3. Gives the demo devices Spanish names: La luz de la cocina, La luz del salón, La luz del dormitorio, La luz del techo, La luz del despacho, La luz de la entrada, Las luces decorativas, El aire acondicionado.

On the next starts it only checks that the saved token works. Open http://127.0.0.1:8123 and log in as `dev` with the password from `dev-password` to change the exposure or the names.

**Tests.** `make integration` runs the Home Assistant tests when `server/homeassistant/dev-token` exists, and skips them otherwise:

- the exposed devices are listed;
- a device that is un-exposed is gone from the list, and comes back when it is exposed again, with no restart;
- `SetPower` switches a light off, on and off, checked with its state;
- end to end: Piper says "Aleksei" and "Enciende la luz de la cocina", and the reply is "Hecho." with the light on in Home Assistant.

**Add a command.** Add a `Kind`, its phrases and its case in `Execute`, with tests. The pipeline does not change: it only calls `Handle(ctx, text)`.

### Wyoming client

Package `client/internal/wyoming` implements the framing in "Protocol contracts" below, as the reference implementation (`wyoming` 1.8.0 and 1.10.2) does:

- The client sends `version` `1.8.0` (the oldest version of the selected services).
- When `data` is inline in the header and also in the data block, the two are merged and the data block wins.
- Payloads are binary. They are never read as text.
- Size limits: header line 64 KiB, data 1 MiB (the Piper `info` data is about 86 KB), payload 4 MiB. A larger event is an error and no memory is allocated for it.
- Errors: `ErrMalformed` (invalid header, data that is not a JSON object, size over a limit), `io.ErrUnexpectedEOF` (the stream ends inside an event), `ErrConnectionClosed` (the service closed the connection between events), `*ServiceError` (the service sent an `error` event), `ErrUnexpectedEvent` (an event type that the caller did not expect).
- Each read and write has a deadline from `--operation-timeout` and from the context. When the context ends, a blocked read or write returns at once. After an error, close the connection: the stream position is unknown. A malformed event also makes the reference server close its side.

`make integration` sends `describe` to all three services, and sends Spanish speech from Piper to Whisper.

### Platforms

| Platform | Build | Unit tests | Real microphone and speaker |
|---|---|---|---|
| macOS arm64 | yes | yes | Capture: real sound (peak -30 dBFS while speaking), 16000 Hz from a 48000 Hz device. Playback: `loopback` recording heard on the built-in speakers. Verified 2026-10-20. |
| Linux arm64 | yes (`golang:1.27` container) | yes | Not tested. The container has no sound system, so miniaudio used its null device. |
| Windows amd64 | yes (`make windows`, MinGW in a container) | no | Not tested. See "Windows". |
| Linux amd64, macOS amd64, Windows arm64 | not tried | no | Not tested. |

Native build requirements:

- macOS: Xcode Command Line Tools. miniaudio uses Core Audio.
- Linux: `gcc`. miniaudio loads ALSA, PulseAudio or JACK at run time, so no `-dev` package is necessary to build. At run time, install `libasound2` or PulseAudio/PipeWire. With no sound system, miniaudio uses a null device that captures silence.
- Windows: a MinGW-w64 C compiler (`x86_64-w64-mingw32-gcc` for a cross-build, `CC=x86_64-w64-mingw32-gcc CGO_ENABLED=1 GOOS=windows GOARCH=amd64`). miniaudio uses WASAPI.

### Windows

The voice services run in Docker Desktop (WSL 2), as on the Mac. The client runs as `voice-client.exe`, not in Docker: a Linux container on Windows has no direct access to the Windows microphone and speaker. The client on Windows is not tested with a real microphone yet; test it step by step (below).

**On the Mac:** `make windows` builds `client/bin/voice-client.exe` in a `golang:1.27.2-bookworm` container with MinGW-w64. The file needs only `KERNEL32.dll` and `msvcrt.dll`; miniaudio loads WASAPI at run time.

**On the Windows PC, once:**

1. Docker Desktop with the WSL 2 engine, and "Start Docker Desktop when you sign in" on.
2. `git clone` this repository. `.gitattributes` keeps LF line endings, also on Windows.
3. Copy `client/bin/voice-client.exe` from the Mac. Create `client/.env.prod` from `client/.env.prod.example` (better with a new Home Assistant token for this PC). Optionally copy `server/models/` (about 1 GB) to skip the first download.
4. Settings > Privacy & security > Microphone: turn on "Let desktop apps access your microphone".
5. Allow local scripts once: `Set-ExecutionPolicy -Scope CurrentUser RemoteSigned`.

**Run and test** (PowerShell, in the repository folder). `scripts/run.ps1` is the `make run` of Windows: it waits for Docker, starts the voice services, loads `client/.env.prod` (`-Env dev` for the other file; the `VOICE_*` variables of the system are removed first) and runs the client with the given command, `run` by default.

```powershell
.\scripts\run.ps1 devices      # microphone and speaker names, for VOICE_INPUT_DEVICE and VOICE_OUTPUT_DEVICE
.\scripts\run.ps1 audio-test   # records and plays back
.\scripts\run.ps1 wake-test    # say "Aleksei"
.\scripts\run.ps1 stt-test     # say a command
.\scripts\run.ps1              # the assistant
```

**Start at log-on.** `.\scripts\install-autostart.ps1` registers the scheduled task "Jarvis voice assistant": at log-on it runs `run.ps1 -KeepRunning` in a hidden window and starts the client again 5 s after it stops. The window is hidden, so read the log over the network (see "Log web page"). A Windows service would have no access to the microphone, so it is a task in your session; turn on the automatic log-on of Windows. `-Uninstall` removes the task.

**Update:** `git pull`, copy a new `voice-client.exe` when the Go code changed, and restart the task (or log off and on).

### Log web page

With `--log-http-addr` (`VOICE_LOG_HTTP_ADDR`), `run` keeps its last 2000 log lines (about 300 KB) in memory and serves them as plain text, read only. Nothing is written to disk, and the lines are gone when the client stops. The console gets the full log as before. Other commands do not serve their log, so `config-check` can run next to a running assistant.

```sh
curl http://<pc>:8080/            # all kept lines
curl http://<pc>:8080/?n=50       # the last 50
curl -i http://<pc>:8080/?after=120   # the lines after line 120; X-Log-Last is the number of the last line
```

`127.0.0.1:8080` serves the page only on the same computer; `0.0.0.0:8080` serves it in the local network. The page has no login and shows the transcripts when `--log-transcripts` is on, so use it only in a trusted home network. On Windows, the firewall asks once whether `voice-client.exe` may accept connections: allow it for private networks. Run `.\scripts\run.ps1` once in a window for that question; the autostart task is hidden. A busy port stops `run` at start with `log web page: listen tcp ...: address already in use`.

### Microphone permission on macOS

macOS gives an app with no microphone permission a stream of zero samples, not an error. `mic-test` and `loopback` then show `all captured samples are zero`. Give the terminal app (Terminal, iTerm2, the IDE) access in System Settings > Privacy & Security > Microphone, then restart the terminal app.

### Audio smoke test (each platform)

1. `voice-client devices` lists the microphone and the speaker.
2. `voice-client mic-test --duration 10s`: speak, and the peak goes up (above about -40 dBFS). Silence stays low.
3. `voice-client loopback --duration 5s`: speak, then you hear the recording.
4. `voice-client mic-test --duration 60s`: `dropped 0 bytes`, and the heap at the end is near the heap at the start.
5. Start `mic-test --duration 60s` with a USB or Bluetooth microphone in `--input-device`, and disconnect it: the command stops with `audio device stopped unexpectedly`.

## Voice services

Prerequisite: Docker with Compose v2.20 or later (for `start_interval` and `!reset`). No API key or secret is necessary.

### Start, stop, inspect

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

### Models

| Service | Folder | First start | Next starts |
|---|---|---|---|
| Whisper | `server/models/whisper` (Hugging Face cache) | Downloads `Systran/faster-whisper-base` (about 145 MB) | Uses the cache, no network necessary |
| Piper | `server/models/piper` | Downloads `PIPER_VOICE` (`.onnx` and `.onnx.json`, about 77 MB for `es_ES-sharvard-medium`) | Uses the files, no network necessary |
| openWakeWord | inside the image | Nothing to download | Nothing to download |

The folders are bind mounts, so the models stay after `make services-down` and after a new image. Verified: after `down` and `up`, the services were healthy in 6 s and the logs had no download. `server/models/*` is in `.gitignore`.

Piper downloads other voices on the first `synthesize` that names them, into the same folder.

### When a model is missing

| Case | What you see |
|---|---|
| Unknown `WHISPER_MODEL` | Whisper exits with `Invalid model size '...', expected one of: ...`. `docker compose ps` shows `Restarting`. |
| Whisper model not on disk and no network | Whisper exits with `LocalEntryNotFoundError ... check your internet connection`. Shows `Restarting`. |
| Unknown `PIPER_VOICE` | Piper exits with `VoiceNotFoundError: <voice>`. Shows `Restarting`. |
| Piper voice not on disk and no network | Piper logs `URLError` and starts. The health check fails with `unhealthy: voice file /data/<voice>.onnx is missing; see the piper logs`. |

`make services-status` prints the last log lines of a service that is not running, and the health check output of a service that is `unhealthy`.

If a first start stops during the download, Whisper can fail with `Unable to open file 'model.bin'`. Remove `server/models/whisper/models--Systran--faster-whisper-base` and start again.

### Dependency and compatibility table

| Service | Image (pinned) | Upstream source | Port | Model or voice | License |
|---|---|---|---|---|---|
| Wake word | `rhasspy/wyoming-openwakeword:2.1.0@sha256:52cb1168…d42b` | github.com/rhasspy/wyoming-openwakeword, github.com/dscripka/openWakeWord | 10400 | `alexa` v0.1 (built in), used for "Aleksei" | Code: Apache-2.0. Pre-trained models: CC BY-NC-SA 4.0 (non-commercial) |
| STT | `rhasspy/wyoming-whisper:3.8.1@sha256:ba6fcb60…0d80` | github.com/rhasspy/wyoming-faster-whisper, github.com/SYSTRAN/faster-whisper | 10300 | `Systran/faster-whisper-base` (multilingual, `es` set) | Wrapper: MIT. faster-whisper: MIT. Model: MIT |
| TTS | `rhasspy/wyoming-piper:2.5.2@sha256:7d39aafa…af7a` | github.com/rhasspy/wyoming-piper, github.com/OHF-Voice/piper1-gpl | 10200 | `es_ES-sharvard-medium`, speaker `F` (22050 Hz) | Wrapper: MIT. Piper engine: GPL-3.0. Voice dataset: CC BY 3.0 (attribution required) |

The full digests are in `server/compose.yaml`. No service needs an API key or an external API at run time. Internet access is only necessary for the first model download (Hugging Face).

### Protocol contracts (verified on 2026-10-20)

All three services use Wyoming over TCP. A frame is one JSON header line, then `data_length` bytes of JSON data, then `payload_length` bytes of binary payload. Header fields: `type`, `data_length`, `payload_length`, optional `version`. Older peers can put `data` inline in the header. The services reply with protocol version 1.8.0 (openWakeWord) and 1.10.2 (Whisper, Piper).

Audio events:

- `audio-start` data: `{rate, width, channels}`.
- `audio-chunk` data: `{rate, width, channels}`, payload: raw PCM, little-endian.
- `audio-stop` data: `{}`.

| Service | Client sends | Service replies |
|---|---|---|
| Any | `describe` | `info` with `wake`, `asr` or `tts` entries |
| openWakeWord | `detect {names: ["alexa"]}`, then `audio-start`, `audio-chunk`..., `audio-stop` | `detection {name, timestamp}` for each hit (ms from stream start), `not-detected` after `audio-stop` with no hit |
| Whisper | `transcribe {language: "es"}`, then `audio-start`, `audio-chunk`..., `audio-stop` | `transcript {text}` after `audio-stop` |
| Piper | `synthesize {text, voice: {name}}` | `audio-start`, `audio-chunk`..., `audio-stop` |

Audio formats:

- openWakeWord: 16000 Hz, 16-bit, mono input.
- Whisper: 16000 Hz, 16-bit, mono input (used in the tests). `requires_external_vad: true`: the client must find the end of speech. `supports_transcript_streaming: false`.
- Piper: output is 22050 Hz, 16-bit, mono for this voice. The client must read the rate from `audio-start`.

Verification done on macOS (arm64), Docker Desktop:

- `describe` on all three ports returns `info`.
- Piper synthesized "¿Qué hora es?" in 0.65 s. Whisper transcribed the resampled audio (16 kHz) to "que hora es." in 0.43 s. In a second run the result was "que ahora es".
- Synthesized "Hey Jarvis" (`en_US-lessac-medium`) gave `detection {name: hey_jarvis}`. Spanish speech gave `not-detected`.
- After `docker compose restart` the services were ready with no new download.

### Health checks

Each health check sends `describe` and checks the `info` reply, so it fails when the service accepts connections but does not answer.

- Whisper: the image check (`wyoming_faster_whisper.health_check`).
- Piper: `server/compose.yaml` first checks that the voice file is on disk, then runs the image check (`wyoming_piper.health_check`). The image check alone stays healthy when the voice download failed.
- openWakeWord: the image has no check. `server/compose.yaml` adds the same `describe` check, with the `wyoming` library of the image. It also checks that the `info` reply has a wake program.
- During the start period, the checks run every 5 s, so `make services-up` returns as soon as the services are ready.

## Known limitations

- Wake word: there is no Spanish wake-word model in the image. The five built-in models (`okay_nabu`, `hey_jarvis`, `hey_mycroft`, `alexa`, `hey_rhasspy`) are English. The PoC uses `alexa` with the spoken word "Aleksei" (see "Wake word"), so "Alex", "Alexis" and "Alexa" also wake it. An Amazon Echo near the microphone can also react. A custom model needs separate training and a `--custom-model-dir` mount.
- Wake-word model license is CC BY-NC-SA 4.0: non-commercial use only.
- openWakeWord ignores an unknown `--preload-model` name with no error, so the compose file does not preload a model. The client gives the model name in each `detect` event.
- Whisper `base` is weak on short Spanish phrases. "¿Qué hora es?" from Piper gave "que hora es", "que ahora es", "¡Qué oras!" and "¡Qué odes!" in different runs. A larger model (`small`, `medium`) is more accurate but slower. Measure on the real hardware before you change it.
- Piper engine is GPL-3.0. It runs in a separate container and the client talks to it over the network only.
- Piper `describe` lists all known voices, also voices that are not on disk. Piper downloads a requested voice on the first `synthesize`. `scripts/verify.sh` checks that the voice is listed, not that it is on disk.
- Whisper needs the client to detect the end of speech (no server VAD).
- The wake-word threshold is one server value for all clients (`WAKEWORD_THRESHOLD`). The Wyoming `detect` event has no threshold field, and `detection` has no score.
- The wake-word integration test uses synthesized English speech. Real voices, accents and rooms need the manual test procedure in "Wake word".
- Device suffixes (` #2`) follow the order of the system device list. If that order changes (for example after a reconnect), a suffix can point to the other device.
- Device disconnection was not tested with real hardware. The unit tests cover the error path of the ring buffer only.
- After a device disconnects, the client does not reopen it. The pipeline phases must decide how to recover.
- In `run` and `stt-test`, the wake-word stream reads the microphone until the detection arrives (about 100 to 200 ms after the wake word). A command said with no pause after "Aleksei" can lose its first syllable. Make a short pause after the wake word.
- The end of speech is found with a fixed level (`--speech-level`), not with a voice detector. Loud steady noise (a fan near the microphone) counts as speech and keeps the recording open until `--record-max`.
- Device names are not in `WHISPER_INITIAL_PROMPT`, because the prompt is a fixed server setting and the devices come from Home Assistant. "Enciende la luz de la cocina" was recognized correctly in the end-to-end test; unusual device names can be recognized worse. Add the phrases of your devices to `WHISPER_INITIAL_PROMPT` if necessary.
- A device name must have only letters, digits and spaces (for example not "Luz + LED"). A device with another name is left out, with a warning in the log.
- Device names are matched by sound (see "Misheard words"), not by meaning: "el despacho" scores only 0.54 for "la luz del despacho", so it does not match. Use the full name, or a Home Assistant name that you say. Home Assistant aliases are not used yet.
- The development Home Assistant is for development only: its login and token are in plain files in `server/homeassistant`.
