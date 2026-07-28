package daemon

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestScopeFromPidFilename(t *testing.T) {
	cases := map[string]string{
		"groved.pid":                        "",
		"groved-env-continued-e2435831.pid": "env-continued",
		"groved-perf-audit-0bd46c64.pid":    "perf-audit",
		"groved-solo.pid":                   "solo",
		"tuimuxd.pid":                       "",
	}
	for in, want := range cases {
		if got := ScopeFromPidFilename(in); got != want {
			t.Errorf("ScopeFromPidFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestScopeHashFromPidFilename(t *testing.T) {
	cases := map[string]string{
		"groved.pid":                        "",
		"groved-env-continued-e2435831.pid": "e2435831",
		"groved-solo.pid":                   "",
		"other.pid":                         "",
	}
	for in, want := range cases {
		if got := ScopeHashFromPidFilename(in); got != want {
			t.Errorf("ScopeHashFromPidFilename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEnumerateDaemonsInReportsLiveAndStale(t *testing.T) {
	dir := t.TempDir()

	// A live daemon: our own pid, with a .scope sidecar.
	live := filepath.Join(dir, "groved-perf-audit-0bd46c64.pid")
	if err := os.WriteFile(live, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ScopeSidecarPath(live), []byte("/home/u/work/perf-audit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A stale daemon: a pid that cannot be alive.
	stale := filepath.Join(dir, "groved.pid")
	if err := os.WriteFile(stale, []byte("2147483646\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Noise that must not be enumerated.
	if err := os.WriteFile(filepath.Join(dir, "tuimuxd.pid"), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A distinct runtime dir, as under GROVE_HOME / XDG_RUNTIME_DIR — the
	// socket must land here, not next to the pidfile.
	runDir := t.TempDir()

	entries, err := enumerateDaemonsIn(dir, runDir)
	if err != nil {
		t.Fatalf("enumerateDaemonsIn: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(entries), entries)
	}

	byScope := map[string]FleetEntry{}
	for _, e := range entries {
		byScope[e.Scope] = e
	}

	got, ok := byScope["perf-audit"]
	if !ok {
		t.Fatalf("no perf-audit entry: %+v", entries)
	}
	if !got.Running || got.PID != os.Getpid() {
		t.Errorf("live entry: running=%v pid=%d, want true/%d", got.Running, got.PID, os.Getpid())
	}
	if got.ExactScope != "/home/u/work/perf-audit" {
		t.Errorf("ExactScope = %q", got.ExactScope)
	}
	// The socket must share the pidfile's stem (never a re-hashed label) and
	// live in the runtime dir, not the state dir.
	if want := filepath.Join(runDir, "groved-perf-audit-0bd46c64.sock"); got.SockPath != want {
		t.Errorf("SockPath = %q, want %q", got.SockPath, want)
	}

	global, ok := byScope[""]
	if !ok {
		t.Fatalf("no unscoped entry: %+v", entries)
	}
	if global.Running {
		t.Errorf("stale entry reported running")
	}
	if global.ExactScope != "" {
		t.Errorf("stale entry ExactScope = %q, want empty", global.ExactScope)
	}
}

func TestEnumerateDaemonsInEmptyDir(t *testing.T) {
	entries, err := enumerateDaemonsIn(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatalf("enumerateDaemonsIn: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("got %d entries, want 0", len(entries))
	}
}

func TestDisplayScope(t *testing.T) {
	if got := DisplayScope(""); got != "(unscoped)" {
		t.Errorf("DisplayScope(\"\") = %q", got)
	}
	if got := DisplayScope("nav"); got != "nav" {
		t.Errorf("DisplayScope(\"nav\") = %q", got)
	}
}
