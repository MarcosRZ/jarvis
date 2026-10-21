# Wyoming protocol

How the client talks to the three voice services.

## Wyoming client

Package `client/internal/wyoming` implements the framing in "Protocol contracts" below, as the reference implementation (`wyoming` 1.8.0 and 1.10.2) does:

- The client sends `version` `1.8.0` (the oldest version of the selected services).
- When `data` is inline in the header and also in the data block, the two are merged and the data block wins.
- Payloads are binary. They are never read as text.
- Size limits: header line 64 KiB, data 1 MiB (the Piper `info` data is about 86 KB), payload 4 MiB. A larger event is an error and no memory is allocated for it.
- Errors: `ErrMalformed` (invalid header, data that is not a JSON object, size over a limit), `io.ErrUnexpectedEOF` (the stream ends inside an event), `ErrConnectionClosed` (the service closed the connection between events), `*ServiceError` (the service sent an `error` event), `ErrUnexpectedEvent` (an event type that the caller did not expect).
- Each read and write has a deadline from `--operation-timeout` and from the context. When the context ends, a blocked read or write returns at once. After an error, close the connection: the stream position is unknown. A malformed event also makes the reference server close its side.

`make integration` sends `describe` to all three services, and sends Spanish speech from Piper to Whisper.

## Protocol contracts (verified on 2026-10-20)

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
