# Development

Build, tests, platforms, CI and releases.

## Prerequisites

- Go 1.27 or later.
- A C compiler (cgo), for the native audio library `github.com/gen2brain/malgo` v0.11.26 (miniaudio, public domain). On macOS: Xcode Command Line Tools.

## Build and check

```sh
make build              # client/bin/voice-client, VERSION=x.y.z sets the version
make test               # go test ./...; needs no voice service and no audio device
make integration        # tests against the running Docker services (see [voice-services.md](voice-services.md))
make lint               # gofmt check and go vet ./...
make clean
```

## Platforms

| Platform | Build | Unit tests | Real microphone and speaker |
|---|---|---|---|
| macOS arm64 | yes | yes | Capture: real sound (peak -30 dBFS while speaking), 16000 Hz from a 48000 Hz device. Playback: `loopback` recording heard on the built-in speakers. Verified 2026-10-20. |
| Linux arm64 | yes (`golang:1.27` container) | yes | Not tested. The container has no sound system, so miniaudio used its null device. |
| Windows amd64 | yes (`make windows`, and CI) | no | Capture and playback work on an Intel N100 mini PC (2026-10-21): the assistant runs there. See [windows.md](windows.md). |
| Linux amd64, macOS amd64, Windows arm64 | not tried | no | Not tested. |

Native build requirements:

- macOS: Xcode Command Line Tools. miniaudio uses Core Audio.
- Linux: `gcc`. miniaudio loads ALSA, PulseAudio or JACK at run time, so no `-dev` package is necessary to build. At run time, install `libasound2` or PulseAudio/PipeWire. With no sound system, miniaudio uses a null device that captures silence.
- Windows: a MinGW-w64 C compiler (`x86_64-w64-mingw32-gcc` for a cross-build, `CC=x86_64-w64-mingw32-gcc CGO_ENABLED=1 GOOS=windows GOARCH=amd64`). miniaudio uses WASAPI.

## CI and releases

`.github/workflows/build.yml` runs on GitHub Actions, with the actions pinned by commit:

- every push to `main` and every pull request: `make lint`, `make test`, and `voice-client.exe` built with MinGW on Ubuntu, kept 30 days as the workflow artifact `voice-client-windows` (download it from the run page);
- a tag `v*`, for example `git tag v0.2.0 && git push origin v0.2.0`: the same, and a GitHub release with `voice-client.exe` and `voice-client.exe.sha256`. `voice-client version` prints the tag.

The repository is public, so `run.ps1 -Update` downloads the release without a login, from `https://github.com/<owner>/<repo>/releases/latest/download/voice-client.exe`.
