package state

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// seedEcosystem creates dir with a grove.toml marker and returns the
// symlink-resolved path.
func seedEcosystem(t *testing.T, prefix string) string {
	t.Helper()
	dir, err := os.MkdirTemp("", prefix)
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if resolved, rerr := filepath.EvalSymlinks(dir); rerr == nil {
		dir = resolved
	}
	if err := os.WriteFile(filepath.Join(dir, "grove.toml"), []byte("# test\n"), 0o644); err != nil {
		t.Fatalf("seed grove.toml: %v", err)
	}
	return dir
}

// TestStateIsEcosystemScoped asserts that a plan written under ecosystem dirA
// is NOT visible when reading from a different ecosystem dirB (no cross-ecosystem
// leak), which is the core bug this change fixes.
func TestStateIsEcosystemScoped(t *testing.T) {
	dirA := seedEcosystem(t, "grove-state-eco-a-*")
	dirB := seedEcosystem(t, "grove-state-eco-b-*")

	if err := Set(dirA, "flow.active_plan", "plan-a"); err != nil {
		t.Fatalf("Set(dirA): %v", err)
	}
	if err := Set(dirB, "flow.active_plan", "plan-b"); err != nil {
		t.Fatalf("Set(dirB): %v", err)
	}

	gotA, err := GetString(dirA, "flow.active_plan")
	if err != nil {
		t.Fatalf("GetString(dirA): %v", err)
	}
	if gotA != "plan-a" {
		t.Errorf("dirA active plan = %q, want plan-a", gotA)
	}

	gotB, err := GetString(dirB, "flow.active_plan")
	if err != nil {
		t.Fatalf("GetString(dirB): %v", err)
	}
	if gotB != "plan-b" {
		t.Errorf("dirB active plan = %q, want plan-b (leak from dirA?)", gotB)
	}
}

// TestWriteFromNoEcosystemErrors asserts a WRITE from a dir with no ecosystem
// root returns ErrNoEcosystemRoot and creates no state file.
func TestWriteFromNoEcosystemErrors(t *testing.T) {
	bare, err := os.MkdirTemp("", "grove-state-bare-*")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	defer os.RemoveAll(bare)
	if resolved, rerr := filepath.EvalSymlinks(bare); rerr == nil {
		bare = resolved
	}

	err = Set(bare, "flow.active_plan", "should-not-write")
	if err == nil {
		t.Fatalf("Set from no-ecosystem dir: expected error, got nil")
	}
	if !errors.Is(err, ErrNoEcosystemRoot) {
		t.Errorf("Set error = %v, want ErrNoEcosystemRoot", err)
	}

	// No .grove/state.yml may have been created under the bare dir.
	if _, statErr := os.Stat(filepath.Join(bare, ".grove", "state.yml")); statErr == nil {
		t.Errorf("state file was created under no-ecosystem dir")
	}
}

// TestReadFromNoEcosystemReturnsEmpty asserts a READ from a dir with no
// ecosystem root returns empty state and NO error.
func TestReadFromNoEcosystemReturnsEmpty(t *testing.T) {
	bare, err := os.MkdirTemp("", "grove-state-bare-read-*")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	defer os.RemoveAll(bare)
	if resolved, rerr := filepath.EvalSymlinks(bare); rerr == nil {
		bare = resolved
	}

	st, err := Load(bare)
	if err != nil {
		t.Fatalf("Load from no-ecosystem dir: expected nil error, got %v", err)
	}
	if len(st) != 0 {
		t.Errorf("Load from no-ecosystem dir = %v, want empty", st)
	}

	got, err := GetString(bare, "flow.active_plan")
	if err != nil {
		t.Fatalf("GetString from no-ecosystem dir: expected nil error, got %v", err)
	}
	if got != "" {
		t.Errorf("GetString from no-ecosystem dir = %q, want empty", got)
	}
}
