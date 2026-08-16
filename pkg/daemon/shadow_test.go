package daemon

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/grovetools/core/pkg/process"
)

// ownerMap builds the pidfile-reader seam from a path→pid map.
func ownerMap(m map[string]int) func(string) (bool, int) {
	return func(path string) (bool, int) {
		pid, ok := m[path]
		return ok && pid > 0, pid
	}
}

// TestFindShadowDaemonsReproducesTheAuditPair is the 2026-08-15 finding as
// data: two unscoped daemons with identical socket/pidfile arguments, one of
// which the pidfile names. Only the other must be reported.
func TestFindShadowDaemonsReproducesTheAuditPair(t *testing.T) {
	stateDir := "/home/u/.local/state/grove"
	pidPath := filepath.Join(stateDir, "groved.pid")
	sock := "/home/u/.local/state/grove/groved.sock"
	argv := "groved start --socket " + sock + " --pidfile " + pidPath

	procs := []process.Entry{
		{PID: 25659, PPID: 1, Elapsed: 142 * time.Minute, Args: argv},
		{PID: 25658, PPID: 913, Elapsed: 142 * time.Minute, Args: argv + " --ready-fd 3"},
	}

	got := findShadowDaemons(procs, stateDir, pidPath, ownerMap(map[string]int{pidPath: 25659}))

	if len(got) != 1 {
		t.Fatalf("expected exactly the shadow, got %d: %+v", len(got), got)
	}
	if got[0].PID != 25658 {
		t.Errorf("reported pid %d, want the process that owns no pidfile (25658)", got[0].PID)
	}
	if got[0].OwnerPID != 25659 {
		t.Errorf("owner reported as %d, want 25659", got[0].OwnerPID)
	}
	if !strings.Contains(got[0].Reason(), "25659") {
		t.Errorf("reason should name the real owner, got %q", got[0].Reason())
	}
}

func TestFindShadowDaemonsIgnoresTheHealthyFleet(t *testing.T) {
	stateDir := "/home/u/.local/state/grove"
	globalPid := filepath.Join(stateDir, "groved.pid")
	scopedPid := filepath.Join(stateDir, "groved-plan-a1b2c3d4.pid")

	procs := []process.Entry{
		// The global daemon, started by hand with no arguments at all.
		{PID: 100, Args: "groved start"},
		// A scoped daemon from the auto-start factory.
		{PID: 200, Args: "groved start --auto-shutdown --scope /w/plan --socket /run/grove/groved-plan-a1b2c3d4.sock --pidfile " + scopedPid},
		// Client subcommands that merely mention a socket.
		{PID: 300, Args: "groved status"},
		{PID: 301, Args: "groved stop"},
		{PID: 302, Args: "groved kill scoped"},
		// Something else entirely.
		{PID: 400, Args: "tuimux daemon --socket /run/grove/tuimux.sock"},
	}

	got := findShadowDaemons(procs, stateDir, globalPid, ownerMap(map[string]int{
		globalPid: 100,
		scopedPid: 200,
	}))

	if len(got) != 0 {
		t.Fatalf("healthy fleet reported %d shadows: %+v", len(got), got)
	}
}

func TestFindShadowDaemonsSkipsForeignAndFixtureDaemons(t *testing.T) {
	stateDir := "/home/u/.local/state/grove"
	globalPid := filepath.Join(stateDir, "groved.pid")

	procs := []process.Entry{
		// A tend fixture inside a sandbox namespace.
		{PID: 10, Args: "groved start --socket /tmp/tend-abc-1/grove/groved.sock --pidfile /tmp/tend-abc-1/state/groved.pid"},
		// A daemon in someone else's GROVE_HOME.
		{PID: 11, Args: "groved start --socket /other/home/groved.sock --pidfile /other/home/groved.pid"},
		// Scoped, with no --pidfile: its path needs the process's cwd.
		{PID: 12, Args: "groved start --scope /w/plan"},
	}

	got := findShadowDaemons(procs, stateDir, globalPid, ownerMap(nil))
	if len(got) != 0 {
		t.Fatalf("expected no adjudicable candidates, got %+v", got)
	}
}

// TestFindShadowDaemonsReportsAnUnownedPidfile covers the other shape: a groved
// running while the pidfile it serves under names nobody live at all.
func TestFindShadowDaemonsReportsAnUnownedPidfile(t *testing.T) {
	stateDir := "/home/u/.local/state/grove"
	globalPid := filepath.Join(stateDir, "groved.pid")

	procs := []process.Entry{{PID: 77, Args: "groved start"}}

	got := findShadowDaemons(procs, stateDir, globalPid, ownerMap(nil))
	if len(got) != 1 {
		t.Fatalf("expected 1 shadow, got %d", len(got))
	}
	if got[0].OwnerPID != 0 {
		t.Errorf("owner should be 0 when the pidfile names nobody, got %d", got[0].OwnerPID)
	}
	if !strings.Contains(got[0].Reason(), "no live daemon") {
		t.Errorf("unexpected reason %q", got[0].Reason())
	}
}

// TestFindShadowDaemonsIgnoresANonGrovedOwner keeps a recycled PID from
// vouching for a shadow: the pidfile names a live process, but that process is
// not a groved.
func TestFindShadowDaemonsIgnoresANonGrovedOwner(t *testing.T) {
	stateDir := "/home/u/.local/state/grove"
	globalPid := filepath.Join(stateDir, "groved.pid")

	procs := []process.Entry{{PID: 77, Args: "groved start"}}

	got := findShadowDaemons(procs, stateDir, globalPid, ownerMap(map[string]int{globalPid: 555}))
	if len(got) != 1 || got[0].OwnerPID != 0 {
		t.Fatalf("expected 1 shadow with no live groved owner, got %+v", got)
	}
}

func TestIsGrovedStart(t *testing.T) {
	for _, tc := range []struct {
		cmdline string
		want    bool
	}{
		{"groved start", true},
		{"/usr/local/bin/groved start --socket /x.sock", true},
		{"groved --scope /w start", true},
		{"groved status", false},
		{"groved stop", false},
		{"groved upgrade --global", false},
		{"groved", false},
		{"tuimux daemon", false},
		{"grep groved start", false},
		{"", false},
	} {
		if got := isGrovedStart(tc.cmdline); got != tc.want {
			t.Errorf("isGrovedStart(%q) = %v, want %v", tc.cmdline, got, tc.want)
		}
	}
}

func TestFlagValue(t *testing.T) {
	const cmdline = "groved start --socket /a.sock --pidfile=/b.pid --ready-fd 3"
	if got := flagValue(cmdline, "--socket"); got != "/a.sock" {
		t.Errorf("--socket: got %q", got)
	}
	if got := flagValue(cmdline, "--pidfile"); got != "/b.pid" {
		t.Errorf("--pidfile=: got %q", got)
	}
	if got := flagValue(cmdline, "--scope"); got != "" {
		t.Errorf("absent flag: got %q", got)
	}
}

func TestFormatShadowDaemons(t *testing.T) {
	if got := FormatShadowDaemons(nil); got != "none\n" {
		t.Errorf("empty census: got %q", got)
	}
	out := FormatShadowDaemons([]ShadowDaemon{{
		PID: 25658, PPID: 913, Elapsed: 142 * time.Minute,
		PidPath: "/s/groved.pid", OwnerPID: 25659,
		Cmdline: "groved start --ready-fd 3",
	}})
	for _, want := range []string{"25658", "913", "25659", "groved start --ready-fd 3"} {
		if !strings.Contains(out, want) {
			t.Errorf("census output missing %q:\n%s", want, out)
		}
	}
}
