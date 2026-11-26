# Audio filters and diagnostic tails

The client has two audio diagnosis tools:

- an optional high-pass filter for the wake-word audio;
- short microphone recordings (tails) after each reply.

Both are off by default. They are useful when a microphone picks up mains hum or
desk vibration, or when openWakeWord fires without anybody saying the wake word.

## Signal path

| Path | Audio |
|---|---|
| Wake word | microphone, optional high-pass, openWakeWord |
| Command recording and speech recognition | microphone, unfiltered |
| Diagnostic tail | microphone, unfiltered, 3 s after the reply |

The filter only changes the copy sent to the wake-word service. The command
recording, Whisper transcription and diagnostic tails keep the original signal,
so filtering the wake word cannot degrade a transcript or hide the noise that
caused a false wake.

## Wake-word high-pass

`VOICE_WAKEWORD_HIGHPASS` (or `--wakeword-highpass`) sets the cutoff in Hz. The
filter is two cascaded second-order high-pass sections, 24 dB per octave.

| Setting | Use |
|---|---|
| `0` | Off. The default. |
| `120` | Recommended starting point when the microphone picks up mains hum or low-frequency rumble. |
| `100` | If the wake word stops being detected after enabling the filter. |
| `150`–`180` | If a false wake persists with `120`; test that the wake word is still detected. |

The maximum is `1000`. Raising the cutoff removes more low-frequency noise and
also removes more of the voice fundamental, so always test the wake word after
changing it.

The filter does not replace fixing a ground loop, a bad cable or a vibrating
mount. It only removes the part of the noise that reaches the wake-word model.

A high-pass is not a universal fix. Removing the hum can expose low-level noise or
a mechanical resonance that the model then accepts. The pre-roll (`wake-*.wav`)
is the raw audio, so compare it with the same file after the filter: if the
filtered version triggers when the raw one does not, lower the cutoff or set
`VOICE_WAKEWORD_HIGHPASS=0`.

## Diagnostic tails

The client can save two kinds of 3-second microphone recordings:

- `tail-...wav`: after every spoken reply.
- `wake-...wav`: the audio **before** every wake-word detection. This is the one
  that catches a false wake during a long silence, because the post-reply tail
  starts only after the reply that follows it.

| Setting | Default | Use |
|---|---|---|
| `VOICE_TAIL_DIR` | empty | Directory for the WAV files. Empty turns the recordings off. For example `client/data/tails`. |
| `VOICE_TAIL_SECONDS` | `3s` | Length of each recording, greater than 0 and at most 10 s. |
| `VOICE_TAIL_KEEP` | `10` | Total number of files kept, from 1 to 100. Older files are deleted after every new one. |

The recorder uses a second ring fed by the same microphone callback, so it never
takes audio from the wake word or the command. The pre-roll is a snapshot of
that ring taken as soon as the detector fires, before the beep. A post-reply
tail that overlaps the next reply is skipped rather than mixing two recordings;
the pre-roll is always saved. Files are written with mode `0600` in a directory
created with mode `0700`; `client/data/` is git-ignored. With the defaults, ten
files use about 1 MB.

The recordings contain room audio and can include private speech. Keep the
directory local, delete it when the investigation ends, and leave
`VOICE_TAIL_DIR` empty when the tails are not needed.

### Checking a false wake

1. For a false wake, use the `wake tail saved` line that appears just after the
   detection and before the beep. For a reply, use the `tail saved` line.
2. Play the WAV, for example `pw-play client/data/tails/wake-....wav`.
3. Optionally send the same file to the wake-word service and check whether it
   detects the wake word again. A file that triggers the service while Whisper
   transcribes nothing is a false positive from noise, not from speech.
4. Inspect the spectrum of the quiet part before the beep. A strong peak at
   50 Hz and its harmonics points to mains hum; a broad rise at low frequencies
   points to rumble or vibration.
5. Apply the high-pass, or fix the mount/cable, and compare a new recording.

On one affected setup the raw tail triggered `alexa` at 1360 ms without any
speech; the 50 Hz hum was 3 dB lower after suspending the microphone, but the
false wake remained. A notch at the first mains harmonics was not enough for
the remaining broadband rumble; a 120 Hz high-pass removed the false wake, and a
16 kHz English "Alexa" sample still triggered. The exact cutoff is
microphone-dependent.

## Long-lived sessions

openWakeWord keeps streaming state for as long as the connection is open. After
hours of silence that state can drift and produce a detection whose pre-roll
audio does not trigger when replayed on a fresh connection. That pattern points
to the session, not to the sound.

`VOICE_WAKEWORD_SESSION_MAX` reconnects the service periodically to reset it.
`0` keeps the connection; `2m` is a useful test. The capture ring buffers the
audio during the short reconnect, so no wake word is lost. A periodic reset does
not change the model or the threshold.

## Related settings

- `WAKEWORD_THRESHOLD` (server `.env`) controls the openWakeWord sensitivity.
  Raising it reduces false wakes but can also miss the real wake word; it does
  not remove a hum that the model keeps interpreting as speech.
- `VOICE_WAKEWORD_COOLDOWN` only ignores repeated detections for a short time;
  it does not help with a false wake that comes after a reply.
- `VOICE_SPEECH_LEVEL` controls the command recording, not the wake-word filter.
