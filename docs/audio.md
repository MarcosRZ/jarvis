# Audio

Audio devices, formats and checks on each platform.

## Audio design

- Internal format: signed 16-bit little-endian PCM, mono, 16000 Hz (`audio.SpeechFormat`). Playback accepts any rate, for example 22050 Hz from Piper.
- Each stream asks miniaudio for the internal format. When the device format is different, miniaudio converts the format, the channels and the rate (linear resampler). The log shows both formats for each stream, for example `rate=16000 device_rate=48000 converted=true`. When the formats are the same, miniaudio does no conversion.
- The audio callback does no network I/O and allocates no memory. It only copies samples into, or out of, a fixed-size ring buffer (capture: 2 s, playback: 1 s).
- Capture: when the reader is too slow, the ring drops the oldest audio and counts the dropped bytes. Playback: when the ring is empty, the device plays silence. A writer waits while the ring is full.
- When a device stops without a request (for example, it is disconnected), blocked reads and writes return `audio device stopped unexpectedly`. The client does not reconnect to the device automatically.

## Microphone permission on macOS

macOS gives an app with no microphone permission a stream of zero samples, not an error. `mic-test` and `loopback` then show `all captured samples are zero`. Give the terminal app (Terminal, iTerm2, the IDE) access in System Settings > Privacy & Security > Microphone, then restart the terminal app.

## Audio smoke test (each platform)

1. `voice-client devices` lists the microphone and the speaker.
2. `voice-client mic-test --duration 10s`: speak, and the peak goes up (above about -40 dBFS). Silence stays low.
3. `voice-client loopback --duration 5s`: speak, then you hear the recording.
4. `voice-client mic-test --duration 60s`: `dropped 0 bytes`, and the heap at the end is near the heap at the start.
5. Start `mic-test --duration 60s` with a USB or Bluetooth microphone in `--input-device`, and disconnect it: the command stops with `audio device stopped unexpectedly`.
