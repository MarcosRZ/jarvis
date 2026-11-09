# Assistant pipeline

How `run` connects the wake word, the recording, speech recognition, the commands and the spoken reply.

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

**Wake sound.** After the wake word, the assistant plays a short beep on the output device and then starts to record. Start speaking after the beep. It drops the microphone audio from the time of the beep, so the beep never starts the recording. With the default beep and the device drain, the recording starts about 0.4 s after the detection. When the beep cannot play, the log shows `wake sound failed` and the request goes on without it. The beep is generated in `audio.Beep` (no sound file); change it in `wake_word.beep` of the configuration file (see [Configuration file](configuration.md)).

**No self-hearing.** The wake-word detector runs only in `LISTENING`. When the assistant enters `LISTENING`, it first drops the microphone audio buffered meanwhile (up to 2 s, `audio.Capture.Discard`), so its own voice is never taken as a wake word or a command.

## Text channel

With `--text-addr` (`VOICE_TEXT_ADDR`, default `127.0.0.1:8082`), `run` also accepts text commands. `voice-client text --text "¿Qué hora es?"` sends one and prints the reply:

```text
$ client/bin/voice-client text --text "¿Qué hora es?"
Son las 10 y cuarto.
```

The text skips the wake word and speech recognition and enters the pipeline at `HANDLING_COMMAND`, the same point as a transcription. It uses the same command handler, action catalog, device allowlist, Home Assistant conversation fallback, language model, learning and speaker, and the assistant answers out loud as it does for a voice command. The state log shows `source=text` on the `heard` line and `source=text` on the state change:

```text
level=INFO msg=state from=LISTENING to=HANDLING_COMMAND after=3s source=text
level=INFO msg=state from=HANDLING_COMMAND to=SPEAKING after=0s
```

Both channels stay available. A text command that arrives while the assistant is listening stops the wake-word wait and is handled at once; a text command that arrives while a voice request is in progress waits until that request ends. The wake-word detector is not reading the microphone while a text reply plays, so the assistant does not hear itself.

The endpoint is `POST /text` with a JSON body `{"text": "..."}` and answers `{"reply": "..."}`:

```sh
curl -sS -X POST http://127.0.0.1:8082/text \
  -H 'Content-Type: application/json' \
  -d '{"text":"¿Qué puedes hacer?"}'
```

The default binds only `127.0.0.1`; use `0.0.0.0:8082` to allow the local network. Like the log page, the endpoint has no login and can control the exposed devices, so use it only in a trusted network. The text is logged like a transcript: at info level only with `--log-transcripts`, otherwise at `debug`. An empty text, a malformed body or a stopped assistant gives a clear HTTP error.

**Errors.** A failed request does not stop the assistant. It speaks a short message and listens again:

| Case | Reply |
|---|---|
| No speech after the wake word | "No te he oído." |
| Whisper returns no text | "No te he entendido." |
| Whisper not available, too slow, or an error | "Ahora no puedo entenderte. El servicio de voz no responde." |
| Unknown command | Home Assistant's answer, a grounded local-model answer, or "Todavía no sé hacer eso." when neither handles the sentence |
| Piper or the speaker fails | logged; the assistant listens again |

The assistant stops only for a microphone failure, an unknown wake-word model or speaker, or Ctrl+C. When openWakeWord is down, the detector reconnects (see [wake-word.md](wake-word.md)).

**Unresolved sentences.** A sentence that the fixed parser and learned memory do not resolve is tried against the generated action catalog. A known verb (encender/poner/activar, apagar, alternar, silenciar, subir el volumen, ...) plus the name or alias of exactly one entity runs at once, before Home Assistant and the model; each clause joined by "y" or "e" is matched, and all matched actions run in order. A single-action sentence is learned; a sentence with several actions is not stored as one association. An ambiguous catalog phrase goes to the Home Assistant conversation agent by default; `VOICE_LLM_ORDER=model-first` reverses the order between Home Assistant and the local model. The model receives the action catalog and a read-only snapshot of exposed entities and forecasts, so it can choose one or more actions (up to four) or answer a question. The default `VOICE_LLM_SAFETY=guard` discards an action when the sentence does not name the chosen entity/action and is not a matching physical state; for locks, covers, valves, climate, water heaters, scenes, scripts, automations, buttons, sirens, remotes and vacuums it also requires a known verb that matches the action. `literal` runs any model action, and `states` accepts only physical states. A weather question such as "¿Va a llover?" can be answered from the snapshot instead of toggling a device when Home Assistant does not answer it; with the 0.5B model this is best effort. Accepted actions run in the requested order, without a confirmation question; a reply is only spoken. The common light phrases ("no veo nada", "no hay luz", "está muy oscuro", "hay demasiada luz", "me sobra luz") are resolved deterministically for the `VOICE_ROOM` light, even if the language model is off. The local model can choose exposed catalog actions or an exposed light/switch, and it can still choose the wrong one; see [commands.md](commands.md#language-model). The info-level `msg=action` log line names the entity and action behind a local "Hecho."; `msg=catalog` marks a deterministic match, `msg=conversation` a sentence handled by Home Assistant, and `msg=reasoned` an accepted model action or reply.

**Other devices and questions.** With `--ha-url`, the exposed entities become the safe action catalog: covers, climate, media players, locks, scenes, scripts and more are controlled locally when the phrase matches an action. A sentence that the catalog does not resolve is sent to the Home Assistant conversation agent, which also answers questions such as the weather. If the agent does not handle a question, the local model can answer it from the read-only state and forecast snapshot. See [home-assistant.md](home-assistant.md).

**Learning.** A local action (a deterministic catalog match or an inferred action) counts as a success unless you say "mal" within 60 seconds. Learned sentences are stored in `VOICE_LEARNING_FILE` (default `client/data/learning.json`) and run without the model or Home Assistant, so repeated sentences become fast. "Mal" makes that association negative and the assistant stops repeating the punished action; a different action replaces it. Replies are not stored, because forecasts and states change; only actions are learned. See [Learning and feedback](commands.md#learning-and-feedback).

**Commands.** See [commands.md](commands.md).

**Privacy.** The command text (`msg=heard`, also when it is empty) is logged only with `--log-transcripts` or at `--log-level debug`. `client/.env.dev` turns it on; it is off by default. Recordings are cleared after the transcription. The learning file stores normalized command sentences on disk with mode 0600 even when transcripts are off; set `VOICE_LEARNING_FILE=off` to turn learning off.

**End-to-end test.** `make integration` runs the pipeline with the real services and the real detector, recorder, Whisper client, command handler and Piper speaker. Only the speaker output is a fake. The input is Piper speech (`es_ES-davefx-medium`) fed at real-time speed, as from a microphone: three times "Alexa", 1 s of silence and a command ("¿Qué hora es?", "¿Qué puedes hacer?", "Repite lo que he dicho."), each followed by 4 s of silence. Each reply must be correct, and the third must repeat the text of the second. Measured on 2026-10-20: 3 of 3 runs passed, about 21.6 s for the three requests (of which about 17 s is the input itself).

## Log web page

With `--log-http-addr` (`VOICE_LOG_HTTP_ADDR`), `run` keeps its last 2000 log lines (about 300 KB) in memory and serves them as plain text, read only. Nothing is written to disk, and the lines are gone when the client stops. The console gets the full log as before. Other commands do not serve their log, so `config-check` can run next to a running assistant.

```sh
curl http://<pc>:8081/            # all kept lines
curl http://<pc>:8081/?n=50       # the last 50
curl -i http://<pc>:8081/?after=120   # the lines after line 120; X-Log-Last is the number of the last line
```

`127.0.0.1:8081` serves the page only on the same computer; `0.0.0.0:8081` serves it in the local network. Do not use 8080: the local command model listens there. The page has no login and shows the transcripts when `--log-transcripts` is on, so use it only in a trusted home network. On Windows, the firewall asks once whether `voice-client.exe` may accept connections: allow it for private networks. Run `.\scripts\run.ps1` once in a window for that question; the autostart task is hidden. A busy port stops `run` at start with `log web page: listen tcp ...: address already in use`.
