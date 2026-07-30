package mux

import (
	"os"

	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/tuimux"
)

type MuxType string

const (
	MuxNone   MuxType = ""
	MuxTmux   MuxType = "tmux"
	MuxTuimux MuxType = "tuimux"

	EnvGroveMux = "GROVE_MUX"
	EnvTmux     = "TMUX"
	// EnvGroveTerminal marks a process spawned by a grove terminal. It is not a
	// mux marker: ActiveMux ignores it, and only the agent-target derivation
	// reads it — see agent_target.go for why it ranks below tuimux.
	EnvGroveTerminal     = "GROVE_TERMINAL"
	EnvTuimuxPTY         = "TUIMUX_PTY"
	EnvTuimuxSession     = "TUIMUX_SESSION"
	EnvGroveTmuxSocket   = "GROVE_TMUX_SOCKET"
	EnvGroveTuimuxSocket = "GROVE_TUIMUX_SOCKET"
)

// ActiveMux returns which multiplexer the current process is running inside.
func ActiveMux() MuxType {
	if os.Getenv(EnvTuimuxPTY) != "" {
		return MuxTuimux
	}
	if os.Getenv(EnvTmux) != "" {
		return MuxTmux
	}
	return MuxNone
}

// GetTmuxSocketPath returns the tmux socket name from GROVE_TMUX_SOCKET, or empty for default.
func GetTmuxSocketPath() string {
	return os.Getenv(EnvGroveTmuxSocket)
}

// GetTuimuxSocketPath returns the tuimux socket path. GROVE_TUIMUX_SOCKET is
// the explicit override; otherwise the ambient GROVE_SCOPE — normalized
// through workspace.ResolveScope — selects the scoped socket, and with no
// scope at all the cwd is classified, mirroring ResolveRecordSocket.
//
// Normalization is load-bearing: the scoped socket name embeds a hash of the
// literal scope string, and groved binds its tuimuxd at the NORMALIZED
// (ecosystem/worktree root) scope. Hashing a raw GROVE_SCOPE that points at a
// repo subdir resolved a socket no daemon ever bound, so callers concluded
// "tuimux not running" while a healthy tuimuxd served the worktree.
func GetTuimuxSocketPath() string {
	if s := os.Getenv(EnvGroveTuimuxSocket); s != "" {
		return s
	}
	if s := os.Getenv(EnvGroveScope); s != "" {
		return tuimux.ScopedSocketPath(workspace.ResolveScope(s))
	}
	return tuimux.ScopedSocketPath(workspace.ResolveScope(""))
}

// PingTuimuxSocket checks if a tuimux daemon is reachable at the given socket path.
func PingTuimuxSocket(socketPath string) error {
	return tuimux.NewApiClient(socketPath).Ping()
}
