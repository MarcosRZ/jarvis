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

**Errors.** A failed request does not stop the assistant. It speaks a short message and listens again:

| Case | Reply |
|---|---|
| No speech after the wake word | "No te he oído." |
| Whisper returns no text | "No te he entendido." |
| Whisper not available, too slow, or an error | "Ahora no puedo entenderte. El servicio de voz no responde." |
| Unknown command | "Todavía no sé hacer eso." |
| Piper or the speaker fails | logged; the assistant listens again |

The assistant stops only for a microphone failure, an unknown wake-word model or speaker, or Ctrl+C. When openWakeWord is down, the detector reconnects (see [wake-word.md](wake-word.md)).

**Physical states.** A sentence about a physical state ("hay demasiada luz aquí") runs an action at once, without a confirmation question. The common light phrases ("no veo nada", "no hay luz", "está muy oscuro", "hay demasiada luz", "me sobra luz") are resolved deterministically for the `VOICE_ROOM` light, even if the language model is off. Other states use the model. The model can only choose the exposed lights and switches, but it can still choose the wrong one; see [commands.md](commands.md#language-model). The info-level `msg=action` log line names the device and action behind "Hecho."

**Learning.** A state action counts as a success unless you say "mal" within 60 seconds. Learned sentences are stored in `VOICE_LEARNING_FILE` (default `client/data/learning.json`) and run without the model, so repeated states become fast. "Mal" makes that association negative and the assistant stops repeating the punished action; a different action replaces it. See [Learning and feedback](commands.md#learning-and-feedback).

**Commands.** See [commands.md](commands.md).

**Privacy.** The command text (`msg=heard`, also when it is empty) is logged only with `--log-transcripts` or at `--log-level debug`. `client/.env.dev` turns it on; it is off by default. Recordings are cleared after the transcription. The learning file stores normalized command sentences on disk with mode 0600 even when transcripts are off; set `VOICE_LEARNING_FILE=off` to turn learning off.

**End-to-end test.** `make integration` runs the pipeline with the real services and the real detector, recorder, Whisper client, command handler and Piper speaker. Only the speaker output is a fake. The input is Piper speech (`es_ES-davefx-medium`) fed at real-time speed, as from a microphone: three times "Aleksei", 1 s of silence and a command ("¿Qué hora es?", "¿Qué puedes hacer?", "Repite lo que he dicho."), each followed by 4 s of silence. Each reply must be correct, and the third must repeat the text of the second. Measured on 2026-10-20: 3 of 3 runs passed, about 21.6 s for the three requests (of which about 17 s is the input itself).

## Log web page

With `--log-http-addr` (`VOICE_LOG_HTTP_ADDR`), `run` keeps its last 2000 log lines (about 300 KB) in memory and serves them as plain text, read only. Nothing is written to disk, and the lines are gone when the client stops. The console gets the full log as before. Other commands do not serve their log, so `config-check` can run next to a running assistant.

```sh
curl http://<pc>:8081/            # all kept lines
curl http://<pc>:8081/?n=50       # the last 50
curl -i http://<pc>:8081/?after=120   # the lines after line 120; X-Log-Last is the number of the last line
```

`127.0.0.1:8081` serves the page only on the same computer; `0.0.0.0:8081` serves it in the local network. Do not use 8080: the local command model listens there. The page has no login and shows the transcripts when `--log-transcripts` is on, so use it only in a trusted home network. On Windows, the firewall asks once whether `voice-client.exe` may accept connections: allow it for private networks. Run `.\scripts\run.ps1` once in a window for that question; the autostart task is hidden. A busy port stops `run` at start with `log web page: listen tcp ...: address already in use`.
