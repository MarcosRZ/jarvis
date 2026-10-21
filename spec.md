# Local Voice Assistant PoC: Implementation Plan

## 1. Objective

Build a local, cross-platform voice assistant using Go for the client and Docker Compose for speech processing.

The user should be able to say a wake word, issue a voice command in Spanish, receive a spoken response, and eventually execute commands against home devices.

The initial implementation must not require Python, Node.js, or any other application runtime installed on the client machine. Docker and the Go toolchain are development prerequisites. The distributed client should run as a native executable.

### Definition of success

A user can:

1. Start the voice services with Docker Compose.
2. Run the Go client on macOS, Linux, or Windows.
3. Activate the assistant by speaking a supported wake word.
4. Issue a command in Spanish.
5. Have the command transcribed locally.
6. Receive a spoken response generated locally.
7. Execute a predefined command through a safe, deterministic command handler.

All speech processing must remain on the local network. No external speech recognition, synthesis, or LLM API may be required.

## 2. Architecture and technology decisions

### Client

- Language: Go.
- Audio capture and playback: `github.com/gen2brain/malgo`, subject to verifying supported platforms, native build requirements, and licensing.
- Networking: Go standard library.
- CLI: Go standard library initially; introduce a CLI framework only if justified.
- Configuration: environment variables and command-line flags, with documented precedence.
- Logging: structured or consistently formatted logs using the standard library.
- Testing: Go testing package.

### Voice services

- Wake word: openWakeWord.
- Speech-to-text: faster-whisper through a verified Wyoming-compatible service.
- Text-to-speech: Piper through a verified Wyoming-compatible service.
- Deployment: Docker Compose.
- Communication: Wyoming protocol over TCP, after verifying each service's actual protocol implementation and supported message types.

Use official upstream documentation and repositories to verify image names, supported options, protocol behavior, model downloads, and licensing. Pin service image versions rather than relying on `latest`.

### Audio format

Use a consistent internal audio representation:

- Mono.
- Signed 16-bit PCM.
- 16,000 Hz for speech recognition and wake-word processing, unless a verified service requires a different rate.
- Explicit sample rate and channel metadata.
- Bounded audio buffers.

The capture layer must handle devices whose native sample rate differs from the internal format. Resampling must be explicit, tested, and performed only where necessary.

### Architectural boundaries

Keep the client organized into these responsibilities:

- `audio`: capture, playback, audio format conversion.
- `wyoming`: protocol framing, message encoding, transport.
- `wakeword`: wake-word detection client.
- `stt`: speech transcription client.
- `tts`: speech synthesis client.
- `pipeline`: orchestration and state transitions.
- `commands`: deterministic command interpretation and execution.
- `config`: configuration loading and validation.
- `logging`: diagnostic output.

Avoid unnecessary interfaces and abstractions. Introduce interfaces where they enable independent testing or isolate an external dependency.

## 3. Repository structure

```text
voice-assistant/
├── client/
│   ├── cmd/
│   │   └── voice-client/
│   │       └── main.go
│   ├── internal/
│   │   ├── audio/
│   │   ├── commands/
│   │   ├── config/
│   │   ├── logging/
│   │   ├── pipeline/
│   │   ├── stt/
│   │   ├── tts/
│   │   ├── wakeword/
│   │   └── wyoming/
│   ├── go.mod
│   └── go.sum
├── server/
│   ├── compose.yaml
│   ├── .env.example
│   └── models/
│       └── .gitkeep
├── scripts/
│   └── verify.sh
├── .gitignore
├── Makefile
└── README.md
```

The model directory should use persistent Docker volumes or an explicitly mounted local directory. Model files must not be committed to Git.

## 4. Phase 0: Verify dependencies and freeze the integration contract

### Objective

Resolve external integration risks before writing application logic.

### Tasks

1. Inspect the current upstream repositories and documentation for openWakeWord, faster-whisper, Piper, and Wyoming.
2. Identify maintained, compatible Docker images for all three services.
3. Confirm the actual network protocol and message types supported by each image.
4. Confirm how models are downloaded, configured, and persisted.
5. Determine the wake-word model available for the initial implementation.
6. Confirm the license of each selected image, model, and voice.
7. Document audio formats, endpoint addresses, startup requirements, and health-check behavior.
8. Verify that the selected components can run without an external API.

Do not assume all services support the same protocol features or that an image supports arbitrary wake-word models.

### Deliverables

- `server/compose.yaml` with verified image references.
- A dependency and compatibility table in `README.md`.
- Documented protocol contracts for each service.
- A list of known limitations and licensing considerations.

### Acceptance criteria

- Every selected image has a verified upstream source and pinned version.
- All three services can start and expose their documented interfaces.
- At least one Spanish-capable STT model and one Spanish TTS voice have been identified.
- The wake-word model and its language limitations are documented.
- No integration depends on undocumented assumptions.

### Verification

Start each service independently, inspect its logs, verify connectivity, and execute a minimal request using a suitable test client.

Do not proceed until the external contracts are established.

## 5. Phase 1: Create the Go client skeleton

### Objective

Establish a clean, buildable application with validated configuration and predictable shutdown.

### Tasks

1. Initialize the Go module.
2. Create the package structure.
3. Implement configuration for service addresses, audio settings, wake-word threshold, and logging level.
4. Add CLI commands:
   - `version`
   - `config-check`
   - `devices`
   - `run`
5. Implement configuration validation with useful error messages.
6. Handle SIGINT and SIGTERM using `context.Context`.
7. Add a Makefile with `build`, `test`, `lint`, and `clean` targets.
8. Add basic unit tests for configuration parsing and validation.

Configuration should allow service addresses to be overridden without recompilation.

### Acceptance criteria

- `go test ./...` passes.
- `go vet ./...` passes.
- `make build` produces a native executable.
- Invalid configuration fails immediately with an actionable message.
- Ctrl+C terminates the application cleanly.
- No voice service is required to run `version` or `config-check`.

## 6. Phase 2: Implement cross-platform audio

### Objective

Prove that the client can capture and reproduce audio independently of the voice services.

### Tasks

1. Integrate malgo behind a small audio interface.
2. Implement audio device enumeration.
3. Allow selecting an input and output device.
4. Implement microphone capture using bounded buffers and a producer-consumer model.
5. Implement audio playback.
6. Implement any required resampling and PCM conversion.
7. Handle device initialization failures and device disconnection.
8. Ensure the audio callback does not perform blocking network operations or expensive processing.
9. Provide CLI commands for listing devices and testing the microphone.
10. Add a loopback test mode that records a short sample and plays it back.

### Acceptance criteria

- The microphone can be opened and used on each supported operating system.
- Audio can be captured for at least 60 seconds without unbounded memory growth.
- Captured audio can be played back without application crashes.
- Input and output device selection works.
- Device initialization failures are reported clearly.
- Unit tests cover format conversion and buffer boundaries.
- No Python installation is needed.

### Verification

Test on macOS first. Validate Linux and Windows builds in CI, then perform actual microphone and playback tests on each operating system before claiming full platform support.

Document any native compiler or system-library prerequisites.

## 7. Phase 3: Implement the Wyoming protocol client

### Objective

Build a reusable protocol implementation before integrating the audio pipeline.

### Tasks

1. Implement TCP connection management.
2. Implement message framing according to the verified Wyoming specification.
3. Parse the JSON header and handle optional payload metadata.
4. Support binary payloads without assuming they are UTF-8 text.
5. Implement the message types required by the selected services.
6. Add configurable connection and operation timeouts.
7. Implement cancellation and connection cleanup.
8. Add useful errors for malformed messages, unexpected events, and closed connections.
9. Build fake TCP servers for protocol tests.
10. Add integration tests against the actual Docker services.

Do not hard-code assumptions about message ordering beyond the verified protocol contract.

### Acceptance criteria

- Protocol framing tests cover partial reads, multiple messages, malformed headers, and binary payloads.
- Network errors do not crash the process.
- Context cancellation releases blocked operations.
- Connections close cleanly.
- Tests run without real voice services where possible.
- An integration test verifies communication with at least one real service.

## 8. Phase 4: Deploy the voice services with Docker Compose

### Objective

Provide a reproducible local backend for voice processing.

### Tasks

1. Add the pinned openWakeWord, Whisper, and Piper services to `server/compose.yaml`.
2. Configure persistent model storage.
3. Add health checks where supported.
4. Expose only the necessary ports.
5. Keep service configuration in environment variables.
6. Add startup and shutdown instructions.
7. Document model download and initialization behavior.
8. Ensure a missing model produces a clear startup error.
9. Provide a command to inspect service status and logs.
10. Avoid exposing the service ports to external networks unnecessarily.

### Acceptance criteria

- `docker compose up -d` starts the selected services.
- `docker compose ps` shows their operational status.
- Models persist across container restarts.
- Service startup failures are diagnosable from logs.
- No secrets are required for the local configuration.
- No service requires an external API key.
- Repeated restarts do not redownload already persisted models unnecessarily.

## 9. Phase 5: Integrate openWakeWord

### Objective

Detect the activation phrase while continuously monitoring the microphone.

### Tasks

1. Implement the wake-word service adapter.
2. Stream audio to the verified openWakeWord endpoint using the required protocol.
3. Configure the activation model and detection threshold.
4. Add detection cooldown and duplicate-event protection.
5. Measure detection latency.
6. Implement a CLI mode that logs each detection without starting Whisper or Piper.
7. Test against recorded positive and negative audio samples.
8. Ensure detection events are surfaced to the pipeline as typed events.

The client may continuously stream microphone audio to the local wake-word service. This is acceptable for the initial PoC, provided the data remains on the local network. Do not send audio to external services.

### Acceptance criteria

- The client detects the selected wake word from live audio.
- Detection does not repeatedly trigger for one utterance.
- The detector recovers after temporary network failures.
- Audio buffering remains bounded.
- The threshold is configurable.
- A documented test procedure measures false positives and false negatives.
- Wake-word detection does not require Whisper to run on every audio block.

## 10. Phase 6: Integrate Whisper

### Objective

Convert captured commands into Spanish text.

### Tasks

1. Implement the STT adapter using the verified service contract.
2. Send a bounded recording after wake-word detection.
3. Configure the language as Spanish.
4. Handle empty transcriptions and transcription errors.
5. Add maximum recording duration and silence timeout.
6. Record transcription duration and total request latency.
7. Add tests using a small set of known audio samples.

The recording should end when the silence detector reaches its configured threshold or the maximum duration is reached. Use configurable defaults and document them.

### Acceptance criteria

- A Spanish command is transcribed through the local service.
- Empty or unintelligible audio does not crash the pipeline.
- Oversized recordings are rejected or truncated according to documented rules.
- The client handles timeouts and unavailable services.
- Transcription latency is measurable.
- Audio is not retained beyond the configured retention policy.

## 11. Phase 7: Integrate Piper

### Objective

Generate and play spoken responses.

### Tasks

1. Implement the TTS adapter.
2. Send text to Piper using the verified service protocol.
3. Receive the generated audio and decode its metadata correctly.
4. Convert the output to the format required by the audio playback layer.
5. Play the response through the selected output device.
6. Handle empty responses, malformed audio, and playback errors.
7. Add tests for the TTS adapter and playback lifecycle.

### Acceptance criteria

- Piper generates speech for a Spanish response.
- The client plays the generated audio.
- Audio metadata is handled correctly, including sample rate.
- Playback errors do not leave the application in a stuck state.
- The application can speak multiple responses sequentially.
- TTS works without external services.

## 12. Phase 8: Implement the end-to-end pipeline

### Objective

Connect wake-word detection, recording, transcription, response generation, and playback.

### State machine

```text
STARTING
    |
    v
LISTENING
    |
    | wake word detected
    v
RECORDING
    |
    | silence or timeout
    v
TRANSCRIBING
    |
    v
HANDLING_COMMAND
    |
    v
SPEAKING
    |
    v
LISTENING
```

Any state may transition to a controlled error-handling path, followed by recovery or shutdown.

### Tasks

1. Implement explicit state transitions.
2. Prevent overlapping command executions.
3. Suspend wake-word processing while the assistant is speaking.
4. Define behavior for timeouts, cancellation, and service failures.
5. Add a simple deterministic command handler.
6. Add a test command that returns the current time.
7. Generate a spoken response through Piper.
8. Add structured logs and timing measurements for each stage.
9. Add end-to-end tests using fake service adapters.
10. Test a complete live interaction.

### Acceptance criteria

- Saying the wake word and then "¿Qué hora es?" produces a spoken response.
- The system returns to listening after speaking.
- Errors in one request do not terminate the application.
- Duplicate events do not execute a command twice.
- The microphone is not unintentionally processed as a new command while TTS is playing.
- Every pipeline transition is observable in logs.
- The application shuts down without leaked goroutines or blocked audio operations.

## 13. Phase 9: Add safe command execution

### Objective

Establish a secure boundary between recognized text and actions.

### Tasks

1. Define a typed command model.
2. Implement a deterministic parser for a small set of Spanish commands.
3. Add an allowlist of supported commands.
4. Validate all command arguments.
5. Separate command interpretation from command execution.
6. Add an explicit unknown-command response.
7. Add a fake device adapter for tests.
8. Define a future interface for a Home Assistant adapter, without requiring Home Assistant in this phase.

Initial commands:

- "¿Qué hora es?"
- "Repite lo que he dicho."
- "Cancela."
- "¿Qué puedes hacer?"

Do not execute arbitrary shell commands based on transcribed text. Do not let an LLM invoke arbitrary network services or device actions.

### Acceptance criteria

- Known commands produce deterministic results.
- Unknown commands never trigger actions.
- Malformed arguments are rejected.
- Command execution is independently unit-testable.
- The fake device adapter can simulate success and failure.
- The system can be extended without changing the audio pipeline.

## 14. Phase 10: Packaging, portability, and documentation

### Objective

Make the PoC reproducible and usable outside the development environment.

### Tasks

1. Add cross-compilation targets for macOS, Linux, and Windows.
2. Verify architecture-specific native dependencies for malgo.
3. Build and test on supported operating systems.
4. Document microphone permissions on macOS.
5. Document audio device requirements on Linux and Windows.
6. Document the Docker server setup separately from client installation.
7. Add a configuration example and troubleshooting guide.
8. Add a basic CI pipeline for formatting, tests, vetting, and supported builds.
9. Pin dependency versions.
10. Document known limitations and model licenses.

Cross-compilation alone does not prove runtime compatibility. Native audio capture and playback must be tested on real operating systems.

### Acceptance criteria

- A developer can follow the README from a clean environment.
- The client can be distributed as a native executable.
- Python and Node.js are not runtime requirements.
- Supported platforms are explicitly documented.
- CI verifies all checks possible without physical audio hardware.
- At least one real-device smoke test is documented for each claimed platform.

## 15. Non-functional requirements

### Privacy

- No external speech APIs.
- No external LLM APIs.
- Audio is transmitted only to explicitly configured local services.
- No persistent audio recording by default.
- Logs must not include raw audio or sensitive command content by default.

### Reliability

- All buffers have explicit limits.
- Network operations support cancellation and timeouts.
- Service failures do not crash the client.
- Shutdown closes network connections and audio devices.
- The pipeline never executes the same detected command twice due to a retry.

### Performance

Record the following measurements:

- Wake-word detection latency.
- Time to finish recording after wake-word detection.
- STT processing duration.
- TTS processing duration.
- Total time from end of command to start of spoken response.
- Memory usage during a prolonged listening session.

Do not establish strict performance targets before measuring the selected models on the actual hardware.

### Maintainability

- Business logic must not depend directly on malgo or Wyoming implementation details.
- Audio and service adapters should be replaceable in tests.
- Prefer the Go standard library unless an additional dependency materially improves correctness or maintainability.
- Avoid introducing an LLM before deterministic command handling works.

## 16. Recommended implementation workflow for a coding agent

Implement one phase at a time, in numerical order.

For each phase:

1. Inspect the existing repository and current upstream documentation relevant to that phase.
2. Implement only the scope of the current phase.
3. Add or update tests.
4. Run formatting, unit tests, vetting, and any applicable integration tests.
5. Fix failures before proceeding.
6. Update the README with newly supported functionality and exact commands.
7. Summarize changed files, decisions, test results, and remaining limitations.
8. Stop at the phase boundary and report whether its acceptance criteria are met.

Never report a test as passing if it was not executed. If a platform or service is unavailable for testing, state that explicitly.

Do not silently change architectural decisions. If an upstream dependency makes a planned integration impossible, explain the issue and propose the smallest compatible alternative before proceeding.

## 17. First implementation milestone

Start with **Phase 0**, followed by **Phase 1**.

The first deliverable should be a repository that contains:

- Verified service choices and protocol contracts.
- A pinned Docker Compose configuration.
- A compiling Go client.
- Configuration validation.
- `version`, `config-check`, and `devices` commands.
- Unit tests and a reproducible build command.

Do not implement the full voice assistant in one pass. The objective is a sequence of small, executable, verifiable milestones that can be reviewed independently.