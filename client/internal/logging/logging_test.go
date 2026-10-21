package logging

import (
	"log/slog"
	"strings"
	"testing"
)

func TestNewFiltersBelowLevel(t *testing.T) {
	var out strings.Builder
	logger := New(&out, slog.LevelInfo)

	logger.Debug("hidden")
	logger.Info("shown", "key", "value")

	got := out.String()
	if strings.Contains(got, "hidden") {
		t.Errorf("debug record written at info level: %q", got)
	}
	if !strings.Contains(got, "msg=shown key=value") {
		t.Errorf("info record missing: %q", got)
	}
}
