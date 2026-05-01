package mux

import (
	"os"

	"github.com/grovetools/tuimux"
)

type MuxType string

const (
	MuxNone   MuxType = ""
	MuxTmux   MuxType = "tmux"
	MuxTuimux MuxType = "tuimux"

	EnvGroveMux          = "GROVE_MUX"
	EnvTmux              = "TMUX"
	EnvTuimuxPTY         = "TUIMUX_PTY"
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

// GetTuimuxSocketPath returns the tuimux socket path, falling back to tuimux.DefaultSocketPath().
func GetTuimuxSocketPath() string {
	if s := os.Getenv(EnvGroveTuimuxSocket); s != "" {
		return s
	}
	return tuimux.DefaultSocketPath()
}

// PingTuimuxSocket checks if a tuimux daemon is reachable at the given socket path.
func PingTuimuxSocket(socketPath string) error {
	return tuimux.NewApiClient(socketPath).Ping()
}
