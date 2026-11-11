# Development

## Prerequisites

- Go 1.27 or later.
- A C compiler (cgo) for the audio library. On macOS: Xcode Command Line Tools. On Linux: `gcc`.

## Build and test

```sh
make build          # client/bin/voice-client; VERSION=x.y.z sets the version
make test           # unit tests; no services or audio device needed
make integration    # tests against the running Docker services
make lint           # gofmt check and go vet
make clean
```

`make integration` needs `make services-up`. The model test is skipped if llama.cpp is not ready.

## Platforms

| Platform | Build | Unit tests | Real audio |
|---|---|---|---|
| macOS arm64 | Yes | Yes | Tested |
| Linux arm64 | Yes | Yes | Not tested |
| Windows amd64 | Yes, with `make windows` and CI | Not run locally | Tested |
| Other combinations | Not tried | No | Not tested |

`make windows` cross-builds `client/bin/voice-client.exe` in a container with MinGW. Native Windows builds need a MinGW-w64 compiler.

## CI and releases

`.github/workflows/build.yml` runs on every push to `main` and every pull request:

- `make lint`
- `make test`
- a Windows build, kept as the artifact `voice-client-windows`

A tag such as `v0.2.0` also publishes a GitHub release with `voice-client.exe` and its checksum. `voice-client version` prints the tag.
