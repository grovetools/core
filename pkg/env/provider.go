package env

import (
	"context"
)

// Provider abstracts the environment provisioning backend.
type Provider interface {
	Up(ctx context.Context, req EnvRequest) (*EnvResponse, error)
	Down(ctx context.Context, req EnvRequest) error
}

// DaemonEnvClient is the subset of the daemon client needed for built-in providers.
// This avoids an import cycle between env and daemon packages.
type DaemonEnvClient interface {
	EnvUp(ctx context.Context, req EnvRequest) (*EnvResponse, error)
	EnvDown(ctx context.Context, req EnvRequest) (*EnvResponse, error)
}

// ResolveProvider returns the appropriate Provider implementation based on the name.
// For built-in providers (native, docker), a DaemonEnvClient must be supplied.
// For exec plugins, the client parameter is ignored. If command is non-empty,
// it is used as the binary path instead of searching PATH for grove-env-<name>.
func ResolveProvider(name string, client DaemonEnvClient, command string) Provider {
	switch name {
	case "native", "docker", "terraform":
		return NewDaemonProvider(client)
	default:
		return NewExecProvider(name, command)
	}
}
