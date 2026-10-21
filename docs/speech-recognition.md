# Speech recognition

The `whisper` service turns the recorded command into text. Despite its name, it can run Whisper or NVIDIA Parakeet (see "Speech engine").

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

**Speech engine.** `WHISPER_STT_LIBRARY` in `server/.env` sets `--stt-library` of the image (default `faster-whisper`). `WHISPER_STT_LIBRARY=sherpa` with `WHISPER_MODEL=auto` uses NVIDIA Parakeet TDT 0.6B v3 (int8, about 640 MB, downloaded at the first start). Measured on 2026-10-21 on the Mac: 98 ms instead of 315 ms (Whisper base int8) for 7 phrases from Piper, both with 18 of 21 exact; with the user's voice, 7 of 7 exact in 90 to 121 ms instead of about 450 ms. Parakeet ignores `WHISPER_INITIAL_PROMPT`, and it finds the language itself, so check short commands such as "Cancela." The other engines of the image were slower (Qwen3-ASR, 291 ms) or less accurate (streaming Zipformer, 15 of 21), or have no Spanish model.

**Initial prompt.** Whisper takes a text that steers it toward expected words (`--initial-prompt`, set with `WHISPER_INITIAL_PROMPT` in `server/.env`). The default is the command list: `¿Qué hora es? Repite lo que he dicho. Cancela. ¿Qué puedes hacer?`. Measured on 2026-10-20 with `base`: in the end-to-end test, "¿Qué hora es?" was correct in 5 of 5 runs with the prompt and in 1 of 5 without it ("¿Qué orais?"). All five STT samples were exact with the prompt, also the long sentence that is not in it. The prompt adds about 40 ms. When Phase 9 adds commands, add their phrases to the prompt. To turn the prompt off, set `WHISPER_INITIAL_PROMPT=` (empty) and run `make services-up`.
