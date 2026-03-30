package env

import (
	"context"
	"fmt"
)

// DaemonProvider delegates environment management to the grove daemon (groved).
// Used for built-in providers like "native" and "docker".
type DaemonProvider struct {
	client DaemonEnvClient
}

// NewDaemonProvider wraps a DaemonEnvClient into a Provider.
func NewDaemonProvider(client DaemonEnvClient) *DaemonProvider {
	return &DaemonProvider{client: client}
}

func (p *DaemonProvider) Up(ctx context.Context, req EnvRequest) (*EnvResponse, error) {
	req.Action = "up"
	resp, err := p.client.EnvUp(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("daemon provider failed to start environment: %w", err)
	}
	if resp.Error != "" {
		return resp, fmt.Errorf("daemon environment error: %s", resp.Error)
	}
	return resp, nil
}

func (p *DaemonProvider) Down(ctx context.Context, req EnvRequest) error {
	req.Action = "down"
	resp, err := p.client.EnvDown(ctx, req)
	if err != nil {
		return fmt.Errorf("daemon provider failed to stop environment: %w", err)
	}
	if resp != nil && resp.Error != "" {
		return fmt.Errorf("daemon environment error: %s", resp.Error)
	}
	return nil
}
