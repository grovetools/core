package tmux

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/grovetools/core/pkg/paths"
)

// GeneratedBindingsConfPath returns the absolute path of the master nav
// keybinding conf that bindings.GenerateTmuxConf writes. It is derived from
// paths.CacheDir() so it ALWAYS agrees with the generator (which writes to
// paths.CacheDir()/nav/generated-bindings.conf). Both the nav CLI and the
// daemon resolve the path through this single helper, so a GROVE_HOME /
// XDG_CACHE_HOME override can never make a reloader source-file a stale or
// wrong file.
func GeneratedBindingsConfPath() string {
	cache := paths.CacheDir()
	if cache == "" {
		return ""
	}
	return filepath.Join(cache, "nav", "generated-bindings.conf")
}

// ReloadAllServers source-files the freshly generated nav keybindings into
// every running tmux server so a binding change made anywhere (the nav CLI,
// or an embedded treemux mutation that goes through the daemon) reaches
// standalone tmux sessions immediately rather than only on the next tmux
// restart.
//
// It enumerates every server socket under the per-user tmux socket
// directories in /tmp (tmux-<uid>/<server>) and runs `tmux -L <server>
// source-file <conf>` against each, then also targets the default server
// (no -L). Failures are ignored: a socket may belong to a dead server, and a
// best-effort reload must never block or error the caller.
//
// This was extracted from nav/cmd/nav/key.go (which was package main and
// therefore unimportable by the daemon) so the daemon can own artifact
// generation AND the live reload atomically.
func ReloadAllServers() {
	conf := GeneratedBindingsConfPath()
	if conf == "" {
		return
	}
	if _, err := os.Stat(conf); err != nil {
		// Nothing generated yet — nothing to source.
		return
	}

	// List all tmux sockets and reload each live one. The liveness gate
	// matters: tmux never unlinks the socket of a server that died by
	// SIGKILL, and abandoned test fixtures accumulate dozens of dead
	// sockets. Forking a tmux client at each of them costs ~5ms apiece and
	// this function runs under the daemon's nav-bindings lock, so an
	// ungated sweep turns every bindings write into a multi-hundred-ms
	// stall. A direct dial answers in microseconds and also skips other
	// users' sockets (permission denied), which `-L` could never reach
	// anyway — it resolves against our own uid's socket directory.
	socketsDir := "/tmp"
	if entries, err := os.ReadDir(socketsDir); err == nil {
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), "tmux-") {
				continue
			}
			socketPath := filepath.Join(socketsDir, entry.Name())
			sockets, err := os.ReadDir(socketPath)
			if err != nil {
				continue
			}
			for _, sock := range sockets {
				serverName := sock.Name()
				if !socketAlive(filepath.Join(socketPath, serverName)) {
					continue
				}
				_ = exec.Command("tmux", "-L", serverName, "source-file", conf).Run()
			}
		}
	}

	// Also try the default server (no -L flag), gated the same way. Unlike
	// the enumeration above, tmux resolves this one via TMUX_TMPDIR, so the
	// gate must too — it is the only path that reaches servers living
	// outside /tmp.
	defaultDir := os.Getenv("TMUX_TMPDIR")
	if defaultDir == "" {
		defaultDir = socketsDir
	}
	if socketAlive(filepath.Join(defaultDir, fmt.Sprintf("tmux-%d", os.Getuid()), "default")) {
		_ = exec.Command("tmux", "source-file", conf).Run()
	}
}

// socketAlive reports whether a unix socket file has a listening server.
func socketAlive(path string) bool {
	conn, err := net.DialTimeout("unix", path, 250*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
