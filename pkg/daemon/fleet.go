package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/process"
)

// FleetEntry describes one groved discovered by scanning StateDir() for
// pidfiles. It is the shared shape behind `groved status`, `groved kill`,
// `groved resources`, `groved stats` and the inspector's Daemons (fleet) tab:
// every consumer of "which daemons exist right now" reads the same struct so
// scope labels and socket paths cannot drift between surfaces.
//
// This lived as daemon/cmd's unexported daemonEntry until R3 needed it from
// TUI code that cannot import the daemon binary's cmd package.
type FleetEntry struct {
	// Scope is the display label only ("" for the unscoped/global daemon):
	// the pidfile's middle segment, i.e. filepath.Base of the exact scope.
	Scope string
	// ExactScope is the daemon's exact resolved scope string, read from the
	// .scope sidecar next to its pidfile ("" when unscoped or sidecar-less).
	ExactScope string
	PidPath    string
	SockPath   string
	PID        int
	Running    bool
	// Age is how long ago the pidfile was last written (≈ daemon uptime).
	Age time.Duration
}

// ScopeSidecarPath returns the path to the .scope sidecar that sits next to a
// daemon's pidfile and records its exact resolved scope string. The pidfile
// stores only the PID (every reader Atoi's the whole file), so the exact scope
// — which a successor daemon needs as GROVE_SCOPE so its child clients
// reconnect to the same socket — lives in this sibling file instead.
func ScopeSidecarPath(pidPath string) string {
	return strings.TrimSuffix(pidPath, ".pid") + ".scope"
}

// EnumerateDaemons scans StateDir() for groved*.pid files and returns a
// summary of each. Stale entries (pidfile present, process gone) are returned
// with Running=false so callers can decide how to handle them; enumeration
// itself never fails on a dead daemon.
func EnumerateDaemons() ([]FleetEntry, error) {
	return enumerateDaemonsIn(paths.StateDir(), paths.RuntimeDir())
}

// enumerateDaemonsIn is EnumerateDaemons against explicit state and runtime
// directories (the test seam; the paths functions read process-wide env).
func enumerateDaemonsIn(stateDir, runtimeDir string) ([]FleetEntry, error) {
	matches, err := filepath.Glob(filepath.Join(stateDir, "groved*.pid"))
	if err != nil {
		return nil, err
	}

	entries := make([]FleetEntry, 0, len(matches))
	for _, pidPath := range matches {
		scope := ScopeFromPidFilename(filepath.Base(pidPath))
		// The socket shares the pidfile's stem but lives in RuntimeDir(),
		// which equals StateDir() only on plain macOS — under GROVE_HOME (or
		// XDG_RUNTIME_DIR on Linux) they diverge. Derive the name from the
		// real pidfile basename rather than re-hashing the extracted label:
		// the label is only filepath.Base(scope), so paths.SocketPath(label)
		// re-hashes the short label and yields a DIFFERENT hash than the
		// daemon's actual socket (which hashes the full scope path).
		sockName := strings.TrimSuffix(filepath.Base(pidPath), ".pid") + ".sock"
		sockPath := filepath.Join(runtimeDir, sockName)

		var exactScope string
		if data, err := os.ReadFile(ScopeSidecarPath(pidPath)); err == nil { //nolint:gosec // G304: path derived from our own state dir
			exactScope = strings.TrimSpace(string(data))
		}

		running, pid := pidfileState(pidPath)

		var age time.Duration
		if info, err := os.Stat(pidPath); err == nil {
			age = time.Since(info.ModTime()).Round(time.Second)
		}

		entries = append(entries, FleetEntry{
			Scope:      scope,
			ExactScope: exactScope,
			PidPath:    pidPath,
			SockPath:   sockPath,
			PID:        pid,
			Running:    running,
			Age:        age,
		})
	}

	return entries, nil
}

// pidfileState reads a pidfile and reports whether that process is alive.
// It mirrors daemon/internal/daemon/pidfile.IsRunning, which core cannot
// import (it lives under the daemon binary's internal/); both read a bare
// decimal PID and probe it with process.IsProcessAlive.
func pidfileState(pidPath string) (running bool, pid int) {
	content, err := os.ReadFile(pidPath) //nolint:gosec // G304: path derived from our own state dir
	if err != nil {
		return false, 0
	}
	pid = atoiSafe(strings.TrimSpace(string(content)))
	if pid <= 0 {
		return false, 0
	}
	return process.IsProcessAlive(pid), pid
}

// atoiSafe parses a non-negative decimal, returning 0 for anything else.
func atoiSafe(s string) int {
	n := 0
	if s == "" {
		return 0
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// ScopeFromPidFilename extracts the scope label from a pidfile basename.
//
//	"groved.pid"                        → ""
//	"groved-env-continued-e2435831.pid" → "env-continued"
//
// The hash suffix is exactly 8 hex chars (see paths.scopedPath).
func ScopeFromPidFilename(name string) string {
	name = strings.TrimSuffix(name, ".pid")
	if name == "groved" {
		return ""
	}
	if !strings.HasPrefix(name, "groved-") {
		return ""
	}
	rest := strings.TrimPrefix(name, "groved-")
	// Hash is the last 8 hex chars after a hyphen.
	idx := strings.LastIndex(rest, "-")
	if idx < 0 {
		return rest
	}
	return rest[:idx]
}

// ScopeHashFromPidFilename extracts the 8-hex scope hash suffix from a pidfile
// basename, or "" for the unscoped daemon / a malformed name.
//
//	"groved.pid"                        → ""
//	"groved-env-continued-e2435831.pid" → "e2435831"
func ScopeHashFromPidFilename(name string) string {
	name = strings.TrimSuffix(name, ".pid")
	if !strings.HasPrefix(name, "groved-") {
		return ""
	}
	rest := strings.TrimPrefix(name, "groved-")
	idx := strings.LastIndex(rest, "-")
	if idx < 0 || idx+1 >= len(rest) {
		return ""
	}
	return rest[idx+1:]
}

// DisplayScope returns a human-friendly scope label for display.
func DisplayScope(scope string) string {
	if scope == "" {
		return "(unscoped)"
	}
	return scope
}
