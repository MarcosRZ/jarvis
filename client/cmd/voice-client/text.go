package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"jarvis/client/internal/config"
	"jarvis/client/internal/textinput"
)

// textTimeout bounds one text request, including the time spent waiting for an
// answer that the assistant speaks.
const textTimeout = 2 * time.Minute

// sendTexts sends each text to the running assistant and prints its reply.
func sendTexts(ctx context.Context, cfg config.Config, lines texts, stdout, stderr io.Writer) int {
	if cfg.TextAddr == "" {
		fmt.Fprintln(stderr, "--text-addr is empty: the text channel is off; start run with an address such as 127.0.0.1:8082")
		return exitConfig
	}
	failed := false
	for _, text := range lines {
		reqCtx, cancel := context.WithTimeout(ctx, textTimeout)
		reply, err := textinput.Send(reqCtx, cfg.TextAddr, text)
		cancel()
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return exitOK
			}
			fmt.Fprintln(stderr, err)
			failed = true
			continue
		}
		fmt.Fprintln(stdout, reply)
	}
	if failed {
		return exitError
	}
	return exitOK
}
