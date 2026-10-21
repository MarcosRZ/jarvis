# Speech output

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

**Voice.** `--tts-voice` selects the Piper voice. Piper downloads a voice that is not on disk on its first use (see [Models](voice-services.md#models)). Measured: Piper takes 30 to 120 ms for a short sentence.

Spanish voices with a female speaker (the Piper voice list has no gender field):

| Voice | Speaker | Accent | Dataset license |
|---|---|---|---|
| `es_ES-sharvard-medium` | `F` (the voice also has `M`) | Spain | CC BY 3.0 |
| `es_AR-daniela-high` | (one speaker) | Argentina | CC BY-SA 4.0 |
| `es_MX-claude-high` | (one speaker) | Mexico | Apache-2.0 |

**Speaker.** `--tts-speaker` selects one speaker of a voice with several, for example `--tts-voice es_ES-sharvard-medium --tts-speaker F`. Piper uses the default speaker for an unknown name and only logs a warning, so the client checks the speaker with `describe` before it speaks. It stops with `speaker not offered by the voice: "X" for "es_ES-sharvard-medium"; available: M, F`.

The assistant voice is `es_ES-sharvard-medium` with speaker `F`, chosen by the user on 2026-10-20. `PIPER_VOICE` in `server/.env` should name the same voice: Piper loads it at start, and its health check needs it on disk. To use a voice with one speaker, also give `--tts-speaker ""` (an empty `VOICE_TTS_SPEAKER` counts as not set, so use the flag), for example `say --tts-voice es_AR-daniela-high --tts-speaker "" --text "Hola"`. The integration tests still use `es_ES-davefx-medium` to make Spanish test speech.

`make integration` asks Piper for three sentences in a row and checks the format (22050 Hz mono) and the length. It also checks that an unknown voice gives `VoiceNotFoundError`.
