package config

import (
	"errors"
	"flag"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"jarvis/client/internal/audio"
	"jarvis/client/internal/commands"
)

func envFrom(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func load(args []string, getenv func(string) string, output io.Writer) (Config, error) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(output)
	return Load(fs, args, getenv)
}

func TestLoadParsesCallerFlags(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	seconds := fs.Int("seconds", 1, "")
	cfg, err := Load(fs, []string{"--seconds", "7", "--tts-addr", "piper:1"}, envFrom(nil))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if *seconds != 7 || cfg.TTSAddr != "piper:1" {
		t.Fatalf("seconds %d, tts %q", *seconds, cfg.TTSAddr)
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(nil, envFrom(nil), io.Discard)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Config{
		WakeWordAddr:       "127.0.0.1:10400",
		STTAddr:            "127.0.0.1:10300",
		TTSAddr:            "127.0.0.1:10200",
		WakeWordModel:      "alexa",
		WakeWordCooldown:   2 * time.Second,
		STTLanguage:        "es",
		RecordMax:          8 * time.Second,
		RecordStartTimeout: 3 * time.Second,
		RecordEndSilence:   time.Second,
		SpeechLevel:        -40,
		TTSVoice:           "es_ES-sharvard-medium",
		TTSSpeaker:         "F",
		LogLevel:           slog.LevelInfo,
		ConnectTimeout:     3 * time.Second,
		OperationTimeout:   30 * time.Second,
		BeepEnabled:        true,
		Beep:               audio.DefaultBeep,
		Matching:           commands.DefaultMatching,
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("got %+v, want %+v", cfg, want)
	}
}

func TestLoadPrecedence(t *testing.T) {
	env := envFrom(map[string]string{
		EnvWakeWordAddr:       "10.0.0.1:1",
		EnvSTTAddr:            "10.0.0.2:2",
		EnvTTSAddr:            "10.0.0.3:3",
		EnvInputDevice:        "env mic",
		EnvOutputDevice:       "env speaker",
		EnvWakeWordModel:      "okay_nabu",
		EnvWakeWordCooldown:   "5s",
		EnvSTTLanguage:        "ca",
		EnvRecordMax:          "10s",
		EnvRecordStartTimeout: "2s",
		EnvRecordEndSilence:   "700ms",
		EnvSpeechLevel:        "-45.5",
		EnvTTSVoice:           "es_MX-claude-high",
		EnvTTSSpeaker:         "M",
		EnvLogLevel:           "debug",
		EnvConnectTimeout:     "5s",
		EnvOperationTimeout:   "1m",
		EnvLogTranscripts:     "true",
		EnvLogHTTPAddr:        "0.0.0.0:8080",
	})
	args := []string{"--stt-addr", "host.local:4", "--wakeword-cooldown", "0s", "--input-device", "flag mic", "--connect-timeout", "250ms", "--record-max", "12s", "--speech-level", "-35", "--tts-speaker", "F"}

	cfg, err := load(args, env, io.Discard)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Config{
		WakeWordAddr:       "10.0.0.1:1",
		STTAddr:            "host.local:4",
		TTSAddr:            "10.0.0.3:3",
		InputDevice:        "flag mic",
		OutputDevice:       "env speaker",
		WakeWordModel:      "okay_nabu",
		WakeWordCooldown:   0,
		STTLanguage:        "ca",
		RecordMax:          12 * time.Second,
		RecordStartTimeout: 2 * time.Second,
		RecordEndSilence:   700 * time.Millisecond,
		SpeechLevel:        -35,
		TTSVoice:           "es_MX-claude-high",
		TTSSpeaker:         "F",
		LogLevel:           slog.LevelDebug,
		ConnectTimeout:     250 * time.Millisecond,
		OperationTimeout:   time.Minute,
		LogTranscripts:     true,
		LogHTTPAddr:        "0.0.0.0:8080",
		BeepEnabled:        true,
		Beep:               audio.DefaultBeep,
		Matching:           commands.DefaultMatching,
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("got %+v, want %+v", cfg, want)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		env      map[string]string
		mentions []string
	}{
		{"address without port", []string{"--wakeword-addr", "localhost"}, nil, []string{"--wakeword-addr", EnvWakeWordAddr}},
		{"address with empty host", []string{"--stt-addr", ":10300"}, nil, []string{"--stt-addr", EnvSTTAddr}},
		{"port zero", []string{"--tts-addr", "localhost:0"}, nil, []string{"--tts-addr", "port"}},
		{"port too large", []string{"--tts-addr", "localhost:70000"}, nil, []string{"--tts-addr", "port"}},
		{"port not a number", []string{"--tts-addr", "localhost:http"}, nil, []string{"--tts-addr", "port"}},
		{"threshold is a server setting", []string{"--wakeword-threshold", "0.5"}, nil, []string{"wakeword-threshold"}},
		{"model empty", []string{"--wakeword-model", ""}, nil, []string{"--wakeword-model", EnvWakeWordModel}},
		{"cooldown negative", []string{"--wakeword-cooldown", "-1s"}, nil, []string{"--wakeword-cooldown", EnvWakeWordCooldown}},
		{"cooldown env not a duration", nil, map[string]string{EnvWakeWordCooldown: "soon"}, []string{EnvWakeWordCooldown, "soon"}},
		{"log level env unknown", nil, map[string]string{EnvLogLevel: "loud"}, []string{EnvLogLevel, "loud"}},
		{"log web page address", []string{"--log-http-addr", "8080"}, nil, []string{"--log-http-addr", "use host:port"}},
		{"log transcripts env not a bool", nil, map[string]string{EnvLogTranscripts: "sí"}, []string{EnvLogTranscripts, "use true or false"}},
		{"log level flag unknown", []string{"--log-level", "loud"}, nil, []string{"log-level"}},
		{"connect timeout zero", []string{"--connect-timeout", "0s"}, nil, []string{"--connect-timeout", EnvConnectTimeout}},
		{"operation timeout negative", []string{"--operation-timeout", "-1s"}, nil, []string{"--operation-timeout", EnvOperationTimeout}},
		{"timeout env not a duration", nil, map[string]string{EnvOperationTimeout: "soon"}, []string{EnvOperationTimeout, "soon"}},
		{"language empty", []string{"--stt-language", ""}, nil, []string{"--stt-language", EnvSTTLanguage}},
		{"record max zero", []string{"--record-max", "0s"}, nil, []string{"--record-max", EnvRecordMax}},
		{"record max above whisper window", []string{"--record-max", "31s"}, nil, []string{"--record-max", "30s"}},
		{"start timeout zero", []string{"--record-start-timeout", "0s"}, nil, []string{"--record-start-timeout", EnvRecordStartTimeout}},
		{"start timeout above max", []string{"--record-start-timeout", "9s"}, nil, []string{"--record-start-timeout", "--record-max"}},
		{"end silence zero", []string{"--record-end-silence", "0s"}, nil, []string{"--record-end-silence", EnvRecordEndSilence}},
		{"speech level zero", []string{"--speech-level", "0"}, nil, []string{"--speech-level", EnvSpeechLevel}},
		{"speech level too low", []string{"--speech-level", "-100"}, nil, []string{"--speech-level"}},
		{"speech level env not a number", nil, map[string]string{EnvSpeechLevel: "loud"}, []string{EnvSpeechLevel, "loud"}},
		{"record env not a duration", nil, map[string]string{EnvRecordMax: "long"}, []string{EnvRecordMax, "long"}},
		{"tts voice empty", []string{"--tts-voice", ""}, nil, []string{"--tts-voice", EnvTTSVoice}},
		{"ha url without token", []string{"--ha-url", "http://ha.local:8123"}, nil, []string{EnvHAToken}},
		{"ha url not http", []string{"--ha-url", "ftp://ha.local"}, map[string]string{EnvHAToken: "t"}, []string{"--ha-url", EnvHAURL}},
		{"ha url without host", []string{"--ha-url", "http://"}, map[string]string{EnvHAToken: "t"}, []string{"--ha-url"}},
		{"ha token is not a flag", []string{"--ha-token", "secret"}, nil, []string{"ha-token"}},
		{"unknown flag", []string{"--colour", "red"}, nil, []string{"colour"}},
		{"extra argument", []string{"surprise"}, nil, []string{"surprise"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := load(tt.args, envFrom(tt.env), io.Discard)
			if err == nil {
				t.Fatal("Load returned no error")
			}
			for _, m := range tt.mentions {
				if !strings.Contains(err.Error(), m) {
					t.Errorf("error %q does not mention %q", err, m)
				}
			}
		})
	}
}

func TestLoadHomeAssistant(t *testing.T) {
	env := envFrom(map[string]string{EnvHAURL: "http://127.0.0.1:8123/", EnvHAToken: "secret"})
	cfg, err := load(nil, env, io.Discard)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HAURL != "http://127.0.0.1:8123/" || cfg.HAToken != "secret" {
		t.Errorf("HAURL %q, HAToken %q", cfg.HAURL, cfg.HAToken)
	}
	if cfg, _ := load([]string{"--ha-url", "https://ha.example:8123"}, env, io.Discard); cfg.HAURL != "https://ha.example:8123" {
		t.Errorf("flag did not win over the environment: %q", cfg.HAURL)
	}
}

func TestLoadReportsAllInvalidValues(t *testing.T) {
	_, err := load([]string{"--wakeword-addr", "x", "--tts-addr", "y"}, envFrom(nil), io.Discard)
	if err == nil {
		t.Fatal("Load returned no error")
	}
	for _, m := range []string{"--wakeword-addr", "--tts-addr"} {
		if !strings.Contains(err.Error(), m) {
			t.Errorf("error %q does not mention %q", err, m)
		}
	}
}

func TestLoadHelp(t *testing.T) {
	var out strings.Builder
	_, err := load([]string{"-h"}, envFrom(nil), &out)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("got %v, want flag.ErrHelp", err)
	}
	if !strings.Contains(out.String(), EnvSTTAddr) {
		t.Errorf("help does not name %s:\n%s", EnvSTTAddr, out.String())
	}
}
