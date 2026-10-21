package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"jarvis/client/internal/audio"
	"jarvis/client/internal/config"
	"jarvis/client/internal/logging"
)

const (
	commandVersion     = "version"
	commandConfigCheck = "config-check"
	commandDevices     = "devices"
	commandMicTest     = "mic-test"
	commandLoopback    = "loopback"
	commandWakeTest    = "wake-test"
	commandSTTTest     = "stt-test"
	commandSay         = "say"
	commandRun         = "run"

	flagDuration = "duration"

	exitOK     = 0
	exitError  = 1
	exitUsage  = 2
	exitConfig = 3
)

type durationRange struct {
	initial time.Duration
	max     time.Duration
}

// The loopback range bounds the recording that it keeps in memory.
var audioTestDurations = map[string]durationRange{
	commandMicTest:  {initial: 10 * time.Second, max: time.Hour},
	commandLoopback: {initial: 5 * time.Second, max: 30 * time.Second},
}

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr, os.Getenv)
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	if len(args) == 0 {
		printUsage(stderr)
		return exitUsage
	}
	command := args[0]
	switch command {
	case commandVersion:
		fmt.Fprintln(stdout, version)
		return exitOK
	case commandDevices:
		return listDevices(stdout, stderr)
	case commandConfigCheck, commandRun, commandMicTest, commandLoopback, commandWakeTest, commandSTTTest, commandSay:
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", command)
		printUsage(stderr)
		return exitUsage
	}

	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(stderr)
	var duration time.Duration
	durations, isAudioTest := audioTestDurations[command]
	if isAudioTest {
		fs.DurationVar(&duration, flagDuration, durations.initial, fmt.Sprintf("test length, at most %v", durations.max))
	}
	var lines texts
	if command == commandSay {
		fs.Var(&lines, flagText, "text to speak; repeat the flag to speak several texts in order")
	}
	cfg, err := config.Load(fs, args[1:], getenv)
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}
	if err == nil && isAudioTest && (duration <= 0 || duration > durations.max) {
		err = fmt.Errorf("--%s %v: use a value greater than 0 and at most %v", flagDuration, duration, durations.max)
	}
	if err == nil && command == commandSay {
		err = lines.validate()
	}
	if err != nil {
		fmt.Fprintf(stderr, "invalid configuration:\n%v\n", err)
		return exitConfig
	}

	// Only the assistant serves its log: the other commands print to the
	// console, and must not take the port of a running assistant.
	serveLog := command == commandRun && cfg.LogHTTPAddr != ""
	logOut := stderr
	if serveLog {
		ring := logging.NewRing(logLines)
		if err := logging.Serve(ctx, cfg.LogHTTPAddr, ring); err != nil {
			fmt.Fprintln(stderr, err)
			return exitError
		}
		logOut = io.MultiWriter(stderr, ring)
	}
	logger := logging.New(logOut, cfg.LogLevel)
	if serveLog {
		logger.Info("log web page", "addr", cfg.LogHTTPAddr, "lines", logLines)
	}
	switch command {
	case commandConfigCheck:
		printConfig(stdout, cfg)
		return exitOK
	case commandMicTest:
		return micTest(ctx, cfg, duration, stdout, logger)
	case commandLoopback:
		return loopback(ctx, cfg, duration, logger)
	case commandWakeTest:
		return wakeTest(ctx, cfg, stdout, logger)
	case commandSTTTest:
		return sttTest(ctx, cfg, stdout, logger)
	case commandSay:
		return say(ctx, cfg, lines, stdout, logger)
	}
	return runAssistant(ctx, cfg, logger)
}

// logLines is how many log lines the log web page keeps, about 300 KB.
const logLines = 2000

func printUsage(w io.Writer) {
	fmt.Fprintf(w, `usage: voice-client <command> [flags]

commands:
  %-13s print the client version
  %-13s validate the configuration and print it
  %-13s list audio input and output devices
  %-13s capture from the microphone and show the level each second
  %-13s record from the microphone, then play the recording
  %-13s listen for the wake word and print each detection
  %-13s wake word, then print the command that Whisper recognizes
  %-13s speak --text with Piper on the output device
  %-13s start the assistant

Use "voice-client <command> -h" to see the flags of a command.
`, commandVersion, commandConfigCheck, commandDevices, commandMicTest, commandLoopback, commandWakeTest, commandSTTTest, commandSay, commandRun)
}

func beepState(cfg config.Config) string {
	if !cfg.BeepEnabled {
		return "off"
	}
	b := cfg.Beep
	return fmt.Sprintf("notes %v Hz, %v each, fade %v, level %v dBFS", b.Notes, b.NoteDuration, b.Fade, b.Level)
}

func printConfig(w io.Writer, cfg config.Config) {
	settings := []struct {
		name  string
		value any
	}{
		{"wakeword-addr", cfg.WakeWordAddr},
		{"stt-addr", cfg.STTAddr},
		{"tts-addr", cfg.TTSAddr},
		{"input-device", fmt.Sprintf("%q", cfg.InputDevice)},
		{"output-device", fmt.Sprintf("%q", cfg.OutputDevice)},
		{"wakeword-model", cfg.WakeWordModel},
		{"wakeword-cooldown", cfg.WakeWordCooldown},
		{"stt-language", cfg.STTLanguage},
		{"record-max", cfg.RecordMax},
		{"record-start-timeout", cfg.RecordStartTimeout},
		{"record-end-silence", cfg.RecordEndSilence},
		{"speech-level", fmt.Sprintf("%v dBFS", cfg.SpeechLevel)},
		{"tts-voice", cfg.TTSVoice},
		{"tts-speaker", fmt.Sprintf("%q", cfg.TTSSpeaker)},
		{"ha-url", fmt.Sprintf("%q", cfg.HAURL)},
		{"ha-token", tokenState(cfg.HAToken)},
		{"log-level", cfg.LogLevel},
		{"log-transcripts", cfg.LogTranscripts},
		{"log-http-addr", fmt.Sprintf("%q", cfg.LogHTTPAddr)},
		{"connect-timeout", cfg.ConnectTimeout},
		{"operation-timeout", cfg.OperationTimeout},
		{"config", fmt.Sprintf("%q", cfg.ConfigFile)},
		{"beep", beepState(cfg)},
		{"matching", fmt.Sprintf("accept %v, suggest %v, margin %v", cfg.Matching.Accept, cfg.Matching.Suggest, cfg.Matching.Margin)},
	}
	for _, s := range settings {
		fmt.Fprintf(w, "%-21s %v\n", s.name+":", s.value)
	}
	fmt.Fprintln(w, "configuration is valid")
}

// tokenState never shows the token itself.
func tokenState(token string) string {
	if token == "" {
		return "(not set)"
	}
	return "(set)"
}

func listDevices(stdout, stderr io.Writer) int {
	backend, err := audio.Open(nil)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}
	defer backend.Close()
	devices, err := backend.Devices()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}
	printDevices(stdout, "input devices", devices.Inputs)
	printDevices(stdout, "output devices", devices.Outputs)
	return exitOK
}

func printDevices(w io.Writer, title string, devices []audio.Device) {
	fmt.Fprintf(w, "%s:\n", title)
	if len(devices) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	for _, d := range devices {
		marker := ""
		if d.Default {
			marker = " (default)"
		}
		fmt.Fprintf(w, "  %s%s\n", d.Label, marker)
	}
}
