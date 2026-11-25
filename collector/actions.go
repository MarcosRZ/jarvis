package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	actioncatalog "jarvis/actions"
)

var (
	// ErrActionNotFound means that the ID is not in the current catalog, so it
	// may not be run.
	ErrActionNotFound = errors.New("action not found")
	// ErrActionsUnavailable means that no catalog has ever been generated.
	ErrActionsUnavailable = errors.New("action catalog unavailable")
)

const (
	actionsInterval   = 30 * time.Second
	exposureAssistant = "conversation"
)

var actionEntityID = regexp.MustCompile(`^([a-z0-9_]+)\.([a-z0-9_]+)$`)

// actionService is the shape of a Home Assistant service name that the
// collector may call: one domain and one service, both lowercase.
var actionService = regexp.MustCompile(`^([a-z0-9_]+)\.([a-z0-9_]+)$`)

// Actions returns the last generated action catalog. A background sampler
// refreshes it every 30 seconds, which is the same interval that the voice
// client uses for its in-memory catalog.
func (c *collector) Actions(ctx context.Context) ActionsSnapshot {
	c.actionsMu.Lock()
	cached := c.actions
	if cached != nil {
		snapshot := cloneActions(*cached)
		c.actionsMu.Unlock()
		return snapshot
	}
	c.actionsMu.Unlock()

	snapshot := c.buildActions(ctx)
	c.storeActions(snapshot)
	return cloneActions(snapshot)
}

// RunAction calls the fixed Home Assistant service of one catalog action. The
// action must come from the current catalog, so the dashboard can only run the
// services that the voice assistant may run and never an arbitrary one. Home
// Assistant is the authority: the collector does not change the state itself.
func (c *collector) RunAction(ctx context.Context, id string) (actioncatalog.Action, error) {
	if c.home == nil {
		return actioncatalog.Action{}, ErrActionsUnavailable
	}
	catalog := c.Actions(ctx)
	if catalog.Source.State == SourceDisabled || catalog.Source.State == SourceUnavailable {
		return actioncatalog.Action{}, ErrActionsUnavailable
	}
	var action actioncatalog.Action
	found := false
	for _, candidate := range catalog.Actions {
		if candidate.ID == id {
			action, found = candidate, true
			break
		}
	}
	if !found {
		return actioncatalog.Action{}, ErrActionNotFound
	}
	// A catalog action is data, not a command: the service must belong to the
	// same domain as the entity, and both names must be plain identifiers.
	if !actionEntityID.MatchString(action.EntityID) {
		return actioncatalog.Action{}, fmt.Errorf("action %q has an invalid entity %q", action.ID, action.EntityID)
	}
	service := actionService.FindStringSubmatch(action.Service)
	if service == nil || service[1] != action.Domain {
		return actioncatalog.Action{}, fmt.Errorf("action %q has an invalid service %q", action.ID, action.Service)
	}
	requestCtx, cancel := context.WithTimeout(ctx, c.cfg.RequestTimeout)
	defer cancel()
	if err := c.home.CallService(requestCtx, action.Service, action.EntityID, action.Data); err != nil {
		return actioncatalog.Action{}, err
	}
	return action, nil
}

func (c *collector) storeActions(snapshot ActionsSnapshot) {
	c.actionsMu.Lock()
	c.actions = &snapshot
	c.actionsMu.Unlock()
}

func (c *collector) runActionsSampler(ctx context.Context) {
	c.storeActions(c.buildActions(ctx))
	ticker := time.NewTicker(actionsInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.storeActions(c.buildActions(ctx))
		}
	}
}

func (c *collector) buildActions(ctx context.Context) ActionsSnapshot {
	now := time.Now().UTC()
	if c.home == nil {
		return ActionsSnapshot{
			Version:     actioncatalog.Version,
			GeneratedAt: now.Format(time.RFC3339Nano),
			Source: SourceStatus{
				State:   SourceDisabled,
				Message: ptr("Set VOICE_HA_URL and VOICE_HA_TOKEN in server/.env to read the exposed entities."),
			},
			Actions: []actioncatalog.Action{},
		}
	}

	requestCtx, cancel := context.WithTimeout(ctx, c.cfg.RequestTimeout)
	entities, err := c.catalogEntities(requestCtx)
	cancel()
	if err != nil {
		return c.staleActions(now, err)
	}

	catalog := actioncatalog.Generate(entities, now)
	actions := catalog.Actions
	if actions == nil {
		actions = []actioncatalog.Action{}
	}
	generated := now.Format(time.RFC3339Nano)
	snapshot := ActionsSnapshot{
		Version:     catalog.Version,
		GeneratedAt: generated,
		Source:      SourceStatus{State: SourceOK, LastSuccessAt: &generated},
		EntityCount: len(entities),
		Actions:     actions,
	}

	c.actionsMu.Lock()
	lastGood := cloneActions(snapshot)
	c.actionsLastGood = &lastGood
	c.actionsMu.Unlock()

	return snapshot
}

// staleActions keeps the last usable catalog when Home Assistant is
// unreachable, so the dashboard shows recent data and an honest source state
// instead of an empty list.
func (c *collector) staleActions(now time.Time, cause error) ActionsSnapshot {
	c.actionsMu.Lock()
	lastGood := c.actionsLastGood
	if lastGood != nil {
		previous := cloneActions(*lastGood)
		lastGood = &previous
	}
	c.actionsMu.Unlock()

	message := ptr(cause.Error())
	if lastGood != nil {
		lastGood.Source = SourceStatus{
			State:         SourceDegraded,
			LastSuccessAt: lastGood.Source.LastSuccessAt,
			Message:       message,
		}
		return *lastGood
	}

	return ActionsSnapshot{
		Version:     actioncatalog.Version,
		GeneratedAt: now.Format(time.RFC3339Nano),
		Source:      SourceStatus{State: SourceUnavailable, Message: message},
		Actions:     []actioncatalog.Action{},
	}
}

func (c *collector) catalogEntities(ctx context.Context) ([]actioncatalog.Entity, error) {
	exposed, err := c.home.ExposedEntities(ctx)
	if err != nil {
		return nil, fmt.Errorf("read exposed entities: %w", err)
	}
	states, err := c.home.States(ctx)
	if err != nil {
		return nil, fmt.Errorf("read entity states: %w", err)
	}

	entities := make([]actioncatalog.Entity, 0, len(states))
	for _, state := range states {
		m := actionEntityID.FindStringSubmatch(state.EntityID)
		if m == nil || !exposed[state.EntityID][exposureAssistant] {
			continue
		}
		attributes := make(map[string]any, len(state.Attributes))
		for key, raw := range state.Attributes {
			var value any
			if err := json.Unmarshal(raw, &value); err != nil {
				continue
			}
			attributes[key] = value
		}
		name := strings.TrimSpace(stringAttribute(attributes, "friendly_name"))
		if name == "" {
			name = strings.ReplaceAll(m[2], "_", " ")
		}
		entities = append(entities, actioncatalog.Entity{
			EntityID:   state.EntityID,
			Domain:     m[1],
			Name:       name,
			State:      state.State,
			Attributes: attributes,
		})
	}
	return entities, nil
}

func stringAttribute(attributes map[string]any, key string) string {
	text, _ := attributes[key].(string)
	return text
}

func cloneActions(snapshot ActionsSnapshot) ActionsSnapshot {
	clone := snapshot
	clone.Actions = make([]actioncatalog.Action, len(snapshot.Actions))
	for i, action := range snapshot.Actions {
		clone.Actions[i] = action
		if action.Data != nil {
			data := make(map[string]any, len(action.Data))
			for key, value := range action.Data {
				data[key] = value
			}
			clone.Actions[i].Data = data
		}
		if action.Aliases != nil {
			clone.Actions[i].Aliases = append([]string(nil), action.Aliases...)
		}
	}
	return clone
}
