package main

import (
	"context"
	"errors"
	"fmt"
)

var (
	// ErrServiceNotFound marks an unknown service or one without a container in
	// the configured compose project.
	ErrServiceNotFound = errors.New("service not found")
	// ErrServiceRestartNotAllowed marks a known service that the dashboard must
	// not restart.
	ErrServiceRestartNotAllowed = errors.New("service cannot be restarted from the dashboard")
)

// RestartService restarts the container of one allowlisted compose service. It
// never accepts a container ID from the caller, only a service ID that is in
// the registry and has a container in the configured compose project.
func (c *collector) RestartService(ctx context.Context, id string) error {
	definition, ok := registryByID(id)
	if !ok || definition.ExternalHA {
		return ErrServiceNotFound
	}
	// The dashboard serves this request, so restarting itself would cut the
	// response before the browser receives it.
	if id == "dashboard" {
		return ErrServiceRestartNotAllowed
	}

	containers, err := c.docker.ListContainers(ctx)
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}
	var target *dockerContainer
	for index := range containers {
		container := containers[index]
		if c.cfg.ComposeProject != "" && container.Labels["com.docker.compose.project"] != c.cfg.ComposeProject {
			continue
		}
		if container.Labels["com.docker.compose.service"] != id {
			continue
		}
		if target == nil || (target.State != "running" && container.State == "running") {
			target = &containers[index]
		}
	}
	if target == nil {
		return ErrServiceNotFound
	}
	if err := c.docker.RestartContainer(ctx, target.ID); err != nil {
		return fmt.Errorf("restart %s: %w", id, err)
	}
	// Refresh the cached snapshot so the next status request already shows the
	// new state instead of waiting for the next sampler tick.
	c.storeSnapshot(c.buildSnapshot(ctx))
	return nil
}
