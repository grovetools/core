package daemon

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/workspace"
)

// redirectFixture is the hermetic environment for the newAutoStart host
// redirect tests (the scoped-daemon auto-start class fix, plan perf-audit
// job 28):
//
//   - GROVE_HOME pinned to a SHORT /tmp dir — both so a developer's live
//     treemux registration cannot leak into the registry lookup, and so
//     paths.SocketPath stays under macOS's ~104-char sun_path limit.
//   - a git-inited scope dir, so workspace.ResolveScope resolves it to a
//     non-empty scope (a bare temp dir resolves to "" = global, which the
//     redirect deliberately never touches).
//   - a sentinel `groved` script FIRST on PATH that records any spawn
//     attempt in a marker file. The real PATH stays appended so the scope
//     resolution's `git` subprocess keeps working.
type redirectFixture struct {
	scopeDir   string
	scope      string
	scopedSock string
	markerPath string
}

func newRedirectFixture(t *testing.T) *redirectFixture {
	t.Helper()

	groveHome := shortTmpDir(t, "gfr-home")
	t.Setenv("GROVE_HOME", groveHome)
	t.Setenv(HostSocketEnv, "")
	_ = os.Unsetenv(HostSocketEnv)
	t.Setenv(groveScopeEnv, "")
	_ = os.Unsetenv(groveScopeEnv)

	scopeDir := shortTmpDir(t, "gfr-scope")
	gitInit := exec.Command("git", "init", "-q", scopeDir)
	if out, err := gitInit.CombinedOutput(); err != nil {
		t.Fatalf("git init scope dir: %v\n%s", err, out)
	}

	f := &redirectFixture{
		scopeDir:   scopeDir,
		markerPath: filepath.Join(groveHome, "spawn-attempted"),
	}

	sentinelDir := filepath.Join(groveHome, "sentinel-bin")
	if err := os.MkdirAll(sentinelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf("#!/bin/sh\n: > %q\nexit 1\n", f.markerPath)
	if err := os.WriteFile(filepath.Join(sentinelDir, "groved"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", sentinelDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// The redirect only exists for scoped resolution; if scope resolution
	// ever returned "" here the whole table would silently test the global
	// path instead.
	f.scope = workspace.ResolveScope(scopeDir)
	if f.scope == "" {
		t.Fatalf("scope dir %s resolved to the global scope; git init did not take", scopeDir)
	}
	f.scopedSock = paths.SocketPath(f.scope)
	return f
}

// shortTmpDir mirrors shortSocketPath's rationale: t.TempDir() paths are too
// long for sun_path once a socket name is appended.
func shortTmpDir(t *testing.T, prefix string) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", prefix)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// listenStub binds a bare accepting unix listener on socketPath — enough for
// tryConnect's dial probe and for NewRemoteClient, which constructs without
// issuing requests.
func listenStub(t *testing.T, socketPath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix %s: %v", socketPath, err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	t.Cleanup(func() { ln.Close() })
}

func (f *redirectFixture) spawnAttempted() bool {
	_, err := os.Stat(f.markerPath)
	return err == nil
}

func (f *redirectFixture) assertRemoteOn(t *testing.T, client Client, wantSocket string) {
	t.Helper()
	rc, ok := client.(*RemoteClient)
	if !ok {
		t.Fatalf("factory returned %T, want *RemoteClient", client)
	}
	if rc.SocketPath() != wantSocket {
		t.Fatalf("client socket = %q, want %q", rc.SocketPath(), wantSocket)
	}
}

// registerLiveHost plants a registry entry for a live pid (this test process)
// whose daemon socket is a fresh short stub listener, and returns that socket.
func registerLiveHost(t *testing.T, name string) string {
	t.Helper()
	sock := filepath.Join(shortTmpDir(t, "gfr-host"), "host.sock")
	listenStub(t, sock)
	writeUIHostRegistrationFile(t, name+".json", uiHostRegistration{
		PID: os.Getpid(), Program: "treemux", Scope: "", SocketPath: sock, StartedAt: time.Now().UTC(),
	})
	return sock
}

// TestAutoStartRedirectsToRegisteredHost is the class fix itself: scoped
// daemon absent, a live registered UI host covering the scope → the factory
// returns the HOST daemon's client and never spawns a scoped groved.
func TestAutoStartRedirectsToRegisteredHost(t *testing.T) {
	f := newRedirectFixture(t)
	hostSock := registerLiveHost(t, "host")

	client := NewWithAutoStart(f.scopeDir)
	defer client.Close()

	f.assertRemoteOn(t, client, hostSock)
	if f.spawnAttempted() {
		t.Fatal("a scoped groved was spawned despite a live registered host")
	}
	if diag := LastConnectDiagnosis(); diag != nil {
		t.Fatalf("redirect recorded a connect diagnosis (%+v); it is not a failure", diag)
	}
}

// TestAutoStartRedirectEnvBeatsRegistry pins the precedence: agent shells
// carry GROVE_HOST_DAEMON_SOCKET and must redirect there even when a registry
// entry points elsewhere (or is absent entirely — satellites, sandboxes).
func TestAutoStartRedirectEnvBeatsRegistry(t *testing.T) {
	f := newRedirectFixture(t)
	registerLiveHost(t, "registered")

	envSock := filepath.Join(shortTmpDir(t, "gfr-env"), "env.sock")
	listenStub(t, envSock)
	t.Setenv(HostSocketEnv, envSock)

	client := NewWithAutoStart(f.scopeDir)
	defer client.Close()

	f.assertRemoteOn(t, client, envSock)
	if f.spawnAttempted() {
		t.Fatal("a scoped groved was spawned despite a published host endpoint")
	}
}

// TestAutoStartLiveScopedDaemonStillWins guards LOCKED semantics #1: the
// scoped-socket dial stays first, so a running scoped daemon keeps winning
// even when a host is registered — only the spawn decision redirects.
func TestAutoStartLiveScopedDaemonStillWins(t *testing.T) {
	f := newRedirectFixture(t)
	registerLiveHost(t, "host")
	listenStub(t, f.scopedSock)

	client := NewWithAutoStart(f.scopeDir)
	defer client.Close()

	f.assertRemoteOn(t, client, f.scopedSock)
	if f.spawnAttempted() {
		t.Fatal("groved spawned despite a live scoped daemon")
	}
}

// TestAutoStartSpawnsWithoutAnyHost keeps the no-host path byte-identical to
// today: nothing published, nothing registered → the spawn path is reached.
func TestAutoStartSpawnsWithoutAnyHost(t *testing.T) {
	f := newRedirectFixture(t)

	client := NewWithAutoStart(f.scopeDir)
	defer client.Close()

	if !f.spawnAttempted() {
		t.Fatal("spawn path not reached with no host anywhere")
	}
}

// TestAutoStartOptOutsReachSpawnPath guards LOCKED semantics #4: callers that
// require a genuinely scoped daemon (treemux's own startup via EarlyReady,
// NewPaired/PairWith, and the explicit RequireScopedDaemon option) must reach
// the spawn path even with a live registered host.
func TestAutoStartOptOutsReachSpawnPath(t *testing.T) {
	cases := []struct {
		name string
		dial func(f *redirectFixture) Client
	}{
		{"PairWith", func(f *redirectFixture) Client {
			return NewWithAutoStartOpts(f.scopeDir, PairWith(os.Getpid()), SuppressStartNotice())
		}},
		{"EarlyReady", func(f *redirectFixture) Client {
			return NewWithAutoStartOpts(f.scopeDir, EarlyReady(), SuppressStartNotice())
		}},
		{"RequireScopedDaemon", func(f *redirectFixture) Client {
			return NewWithAutoStartOpts(f.scopeDir, RequireScopedDaemon(), SuppressStartNotice())
		}},
		{"NewPaired", func(f *redirectFixture) Client {
			return NewPaired(f.scopeDir, os.Getpid())
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRedirectFixture(t)
			registerLiveHost(t, "host")

			client := tc.dial(f)
			defer client.Close()

			if !f.spawnAttempted() {
				t.Fatalf("%s did not reach the spawn path with a live registered host", tc.name)
			}
		})
	}
}

// TestAutoStartFallsThroughOnDeadOrUnreachableHost: a registry entry whose
// host pid is gone, or whose daemon socket nobody answers, must not swallow
// the client — the spawn path is reached exactly as today.
func TestAutoStartFallsThroughOnDeadOrUnreachableHost(t *testing.T) {
	t.Run("dead host pid", func(t *testing.T) {
		f := newRedirectFixture(t)
		sock := filepath.Join(shortTmpDir(t, "gfr-dead"), "dead.sock")
		listenStub(t, sock) // daemon alive, but its host UI is gone
		writeUIHostRegistrationFile(t, "dead.json", uiHostRegistration{
			PID: 1 << 30, Scope: "", SocketPath: sock, StartedAt: time.Now().UTC(),
		})

		client := NewWithAutoStart(f.scopeDir)
		defer client.Close()

		if !f.spawnAttempted() {
			t.Fatal("dead host pid did not fall through to the spawn path")
		}
	})

	t.Run("undialable host socket", func(t *testing.T) {
		f := newRedirectFixture(t)
		writeUIHostRegistrationFile(t, "silent.json", uiHostRegistration{
			PID: os.Getpid(), Scope: "", SocketPath: "/tmp/gfr-nobody-listens.sock", StartedAt: time.Now().UTC(),
		})

		client := NewWithAutoStart(f.scopeDir)
		defer client.Close()

		if !f.spawnAttempted() {
			t.Fatal("undialable host socket did not fall through to the spawn path")
		}
	})
}
