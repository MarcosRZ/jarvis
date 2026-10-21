package main

import (
	"context"
	"strings"
	"testing"
)

func noEnv(string) string { return "" }

func runCLI(ctx context.Context, args ...string) (int, string, string) {
	var stdout, stderr strings.Builder
	code := run(ctx, args, &stdout, &stderr, noEnv)
	return code, stdout.String(), stderr.String()
}

func TestVersion(t *testing.T) {
	code, stdout, _ := runCLI(context.Background(), commandVersion)
	if code != exitOK {
		t.Fatalf("exit %d", code)
	}
	if strings.TrimSpace(stdout) != version {
		t.Errorf("stdout %q, want %q", stdout, version)
	}
}

func TestConfigCheckValid(t *testing.T) {
	code, stdout, stderr := runCLI(context.Background(), commandConfigCheck, "--stt-addr", "127.0.0.1:9999")
	if code != exitOK {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	if !strings.Contains(stdout, "127.0.0.1:9999") {
		t.Errorf("stdout does not show the effective config: %q", stdout)
	}
}

func TestConfigCheckHidesToken(t *testing.T) {
	env := map[string]string{"VOICE_HA_URL": "http://127.0.0.1:8123", "VOICE_HA_TOKEN": "very-secret-token"}
	var stdout, stderr strings.Builder
	code := run(context.Background(), []string{commandConfigCheck}, &stdout, &stderr, func(k string) string { return env[k] })
	if code != exitOK {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), "very-secret-token") {
		t.Fatal("config-check printed the token")
	}
	if !strings.Contains(stdout.String(), "(set)") {
		t.Errorf("config-check does not say that the token is set:\n%s", stdout.String())
	}
}

func TestConfigCheckInvalid(t *testing.T) {
	code, _, stderr := runCLI(context.Background(), commandConfigCheck, "--stt-addr", "nope")
	if code != exitConfig {
		t.Fatalf("exit %d, want %d", code, exitConfig)
	}
	if !strings.Contains(stderr, "--stt-addr") {
		t.Errorf("stderr does not name the bad flag: %q", stderr)
	}
}

func TestRunRejectsInvalidConfig(t *testing.T) {
	code, _, _ := runCLI(context.Background(), commandRun, "--wakeword-cooldown", "-1s")
	if code != exitConfig {
		t.Fatalf("exit %d, want %d", code, exitConfig)
	}
}

func TestUsageErrors(t *testing.T) {
	for _, args := range [][]string{nil, {"dance"}} {
		code, _, stderr := runCLI(context.Background(), args...)
		if code != exitUsage {
			t.Errorf("args %q: exit %d, want %d", args, code, exitUsage)
		}
		if !strings.Contains(stderr, commandConfigCheck) {
			t.Errorf("args %q: stderr has no usage: %q", args, stderr)
		}
	}
}

func TestAudioCommandsRejectInvalidInputBeforeOpeningDevices(t *testing.T) {
	tests := [][]string{
		{commandMicTest, "--duration", "0s"},
		{commandMicTest, "--duration", "2h"},
		{commandLoopback, "--duration", "-1s"},
		{commandLoopback, "--duration", "31s"},
		{commandLoopback, "--log-level", "loud"},
		{commandWakeTest, "--wakeword-model", ""},
		{commandWakeTest, "--wakeword-cooldown", "-1s"},
		{commandSTTTest, "--record-max", "0s"},
		{commandSTTTest, "--stt-language", ""},
		{commandSay},
		{commandSay, "--text", "  "},
		{commandSay, "--text", "Hola", "--tts-voice", ""},
	}
	for _, args := range tests {
		code, _, stderr := runCLI(context.Background(), args...)
		if code != exitConfig {
			t.Errorf("args %q: exit %d, want %d; stderr %q", args, code, exitConfig, stderr)
		}
	}
}

func TestHelpIsNotAnError(t *testing.T) {
	code, _, _ := runCLI(context.Background(), commandConfigCheck, "-h")
	if code != exitOK {
		t.Fatalf("exit %d", code)
	}
}
