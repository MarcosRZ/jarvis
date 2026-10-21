// Package logging builds the client logger.
package logging

import (
	"io"
	"log/slog"
)

func New(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}))
}
