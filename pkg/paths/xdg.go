// Package paths provides XDG-compliant path resolution for Grove.
//
// Resolution order:
// 1. GROVE_HOME (portable root) → $GROVE_HOME/{config,data,state,cache}
// 2. XDG env vars → $XDG_*_HOME/grove
// 3. Platform defaults → ~/.config/grove, ~/.local/share/grove, etc.
package paths

import (
	"os"
	"path/filepath"
)

// getConfigHome returns the base config home directory.
func getConfigHome() string {
	if groveHome := os.Getenv("GROVE_HOME"); groveHome != "" {
		return filepath.Join(groveHome, "config")
	}
	if xdgConfigHome := os.Getenv("XDG_CONFIG_HOME"); xdgConfigHome != "" {
		return xdgConfigHome
	}
	if homeDir, err := os.UserHomeDir(); err == nil {
		return filepath.Join(homeDir, ".config")
	}
	return ""
}

// getDataHome returns the base data home directory.
func getDataHome() string {
	if groveHome := os.Getenv("GROVE_HOME"); groveHome != "" {
		return filepath.Join(groveHome, "data")
	}
	if xdgDataHome := os.Getenv("XDG_DATA_HOME"); xdgDataHome != "" {
		return xdgDataHome
	}
	if homeDir, err := os.UserHomeDir(); err == nil {
		return filepath.Join(homeDir, ".local", "share")
	}
	return ""
}

// getStateHome returns the base state home directory.
func getStateHome() string {
	if groveHome := os.Getenv("GROVE_HOME"); groveHome != "" {
		return filepath.Join(groveHome, "state")
	}
	if xdgStateHome := os.Getenv("XDG_STATE_HOME"); xdgStateHome != "" {
		return xdgStateHome
	}
	if homeDir, err := os.UserHomeDir(); err == nil {
		return filepath.Join(homeDir, ".local", "state")
	}
	return ""
}

// getCacheHome returns the base cache home directory.
func getCacheHome() string {
	if groveHome := os.Getenv("GROVE_HOME"); groveHome != "" {
		return filepath.Join(groveHome, "cache")
	}
	if xdgCacheHome := os.Getenv("XDG_CACHE_HOME"); xdgCacheHome != "" {
		return xdgCacheHome
	}
	if homeDir, err := os.UserHomeDir(); err == nil {
		return filepath.Join(homeDir, ".cache")
	}
	return ""
}

// ConfigDir returns the Grove configuration directory.
// Used for config files like grove.yml.
func ConfigDir() string {
	base := getConfigHome()
	if base == "" {
		return ""
	}
	return filepath.Join(base, "grove")
}

// DataDir returns the Grove data directory.
// Used for binaries, versions, plugins, notebooks.
func DataDir() string {
	base := getDataHome()
	if base == "" {
		return ""
	}
	return filepath.Join(base, "grove")
}

// StateDir returns the Grove state directory.
// Used for runtime state, DBs, logs.
func StateDir() string {
	base := getStateHome()
	if base == "" {
		return ""
	}
	return filepath.Join(base, "grove")
}

// CacheDir returns the Grove cache directory.
// Used for temporary/regenerable data.
func CacheDir() string {
	base := getCacheHome()
	if base == "" {
		return ""
	}
	return filepath.Join(base, "grove")
}

// BinDir returns the Grove binary directory.
// Resolution order:
// 1. GROVE_BIN env var (explicit override for demos/testing)
// 2. DataDir()/bin (standard location)
func BinDir() string {
	// Allow explicit override for demos/testing where GROVE_HOME
	// is set but binaries should still come from the real location
	if binDir := os.Getenv("GROVE_BIN"); binDir != "" {
		return binDir
	}
	data := DataDir()
	if data == "" {
		return ""
	}
	return filepath.Join(data, "bin")
}

// RuntimeDir returns the Grove runtime directory for sockets and pipes.
// Uses XDG_RUNTIME_DIR when available (Linux), falls back to StateDir (macOS).
func RuntimeDir() string {
	if groveHome := os.Getenv("GROVE_HOME"); groveHome != "" {
		return filepath.Join(groveHome, "run")
	}
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, "grove")
	}
	// Fallback: use state dir for socket on macOS/systems without XDG_RUNTIME_DIR
	return StateDir()
}

// SocketPath returns the path to the grove daemon unix socket.
func SocketPath() string {
	return filepath.Join(RuntimeDir(), "groved.sock")
}

// PidFilePath returns the path to the grove daemon PID file.
func PidFilePath() string {
	return filepath.Join(StateDir(), "groved.pid")
}

// EnsureDirs creates all Grove directories if they don't exist.
func EnsureDirs() error {
	dirs := []string{
		ConfigDir(),
		DataDir(),
		StateDir(),
		CacheDir(),
		BinDir(),
		RuntimeDir(),
	}

	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}
