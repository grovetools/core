package workspace_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/grovetools/core/pkg/workspace"
)

func writeHistoryFile(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "access-history.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestLoadAccessHistory_CorruptStateFallsBackToLegacy verifies that a corrupt
// state-dir file is skipped in favour of the healthy legacy config-dir file.
func TestLoadAccessHistory_CorruptStateFallsBackToLegacy(t *testing.T) {
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "config")

	// Write a corrupt file at the state dir path (simulates the real on-disk bug).
	stateDir := filepath.Join(tmp, "state", "grove")
	t.Setenv("GROVE_HOME", "")
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
	writeHistoryFile(t, filepath.Join(stateDir, "gmux"), `{"projects":{}}}`) // extra }

	// Write valid history at the legacy config-dir path.
	legacyJSON := `{"projects":{"/a/project":{"path":"/a/project","last_accessed":"2026-06-01T12:00:00Z","access_count":3}}}`
	writeHistoryFile(t, filepath.Join(configDir, "gmux"), legacyJSON)

	h, err := workspace.LoadAccessHistory(configDir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(h.Projects) != 1 {
		t.Fatalf("expected 1 project from legacy fallback, got %d", len(h.Projects))
	}
	if _, ok := h.Projects["/a/project"]; !ok {
		t.Fatalf("expected /a/project in history, got %v", h.Projects)
	}
}

// TestLoadAccessHistory_BothCorruptReturnsEmpty verifies that when both state
// and legacy paths are unreadable, an empty history is returned (not an error).
func TestLoadAccessHistory_BothCorruptReturnsEmpty(t *testing.T) {
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "config")

	stateDir := filepath.Join(tmp, "state", "grove")
	t.Setenv("GROVE_HOME", "")
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
	writeHistoryFile(t, filepath.Join(stateDir, "gmux"), `{bad json}}`)
	writeHistoryFile(t, filepath.Join(configDir, "gmux"), `{also bad}}`)

	h, err := workspace.LoadAccessHistory(configDir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(h.Projects) != 0 {
		t.Fatalf("expected empty history, got %d projects", len(h.Projects))
	}
}

// TestLoadAccessHistory_ValidStateDir verifies the normal path: valid state-dir
// file is read and legacy is ignored.
func TestLoadAccessHistory_ValidStateDir(t *testing.T) {
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "config")

	stateDir := filepath.Join(tmp, "state", "grove")
	t.Setenv("GROVE_HOME", "")
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))

	accessTime := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	validJSON := `{"projects":{"/state/project":{"path":"/state/project","last_accessed":"2026-06-10T12:00:00Z","access_count":5}}}`
	writeHistoryFile(t, filepath.Join(stateDir, "gmux"), validJSON)

	// Legacy also has an entry; it should be ignored.
	legacyJSON := `{"projects":{"/legacy/project":{"path":"/legacy/project","last_accessed":"2026-06-01T12:00:00Z","access_count":1}}}`
	writeHistoryFile(t, filepath.Join(configDir, "gmux"), legacyJSON)

	h, err := workspace.LoadAccessHistory(configDir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(h.Projects) != 1 {
		t.Fatalf("expected 1 project from state dir, got %d", len(h.Projects))
	}
	p := h.Projects["/state/project"]
	if p == nil {
		t.Fatal("expected /state/project in history")
	}
	if !p.LastAccessed.Equal(accessTime) {
		t.Fatalf("time mismatch: %v vs %v", p.LastAccessed, accessTime)
	}
}
