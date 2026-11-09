package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"jarvis/client/internal/actions"
	"jarvis/client/internal/config"
	"jarvis/client/internal/homeassistant"
)

// generateActions reads the exposed Home Assistant entities, generates the
// action catalog and writes it to cfg.ActionsFile. It also prints the catalog,
// so the action names the language model sees are easy to review.
func generateActions(ctx context.Context, cfg config.Config, stdout, stderr io.Writer) int {
	if cfg.HAURL == "" || cfg.HAToken == "" {
		fmt.Fprintln(stderr, "the actions command needs --ha-url and VOICE_HA_TOKEN")
		return exitConfig
	}
	if cfg.ActionsFile == "" {
		fmt.Fprintln(stderr, "the actions command needs --actions-file")
		return exitConfig
	}
	ha := homeassistant.New(homeassistant.Config{
		URL:     cfg.HAURL,
		Token:   cfg.HAToken,
		Timeout: haTimeout,
	})
	entities, err := ha.Entities(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "read Home Assistant entities: %v\n", err)
		return exitError
	}
	catalog := actions.Generate(entities, time.Now())
	if err := actions.Save(cfg.ActionsFile, catalog); err != nil {
		fmt.Fprintf(stderr, "save action catalog: %v\n", err)
		return exitError
	}
	fmt.Fprintf(stdout, "%d actions from %d exposed entities written to %s\n", len(catalog.Actions), len(entities), cfg.ActionsFile)
	for _, action := range catalog.Actions {
		fmt.Fprintf(stdout, "  %-52s %s\n", action.ID, action.Name)
	}
	return exitOK
}
