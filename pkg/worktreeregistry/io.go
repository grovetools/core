package worktreeregistry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/util/pathutil"
)

// registryDir returns the directory that holds per-worktree JSON files:
//
//	paths.StateDir()/worktrees/
func registryDir() string {
	return filepath.Join(paths.StateDir(), "worktrees")
}

// entryPath returns the full path for the JSON file with the given id.
func entryPath(id string) string {
	return filepath.Join(registryDir(), id+".json")
}

// Load reads and parses the registry entry for id. Returns a non-nil error
// when the file is absent or malformed. Callers that want a "not found" zero
// value should check os.IsNotExist on the returned error.
func Load(id string) (*Entry, error) {
	data, err := os.ReadFile(entryPath(id))
	if err != nil {
		return nil, err
	}
	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("unmarshal registry entry %s: %w", id, err)
	}
	return &entry, nil
}

// Save atomically persists entry. The registry ID is derived from
// entry.AbsPath using pathutil.WorktreeID so all callers agree on the key.
// Write is atomic: JSON is written to <id>.json.tmp then os.Renamed to
// <id>.json, so readers never observe a partial file.
func Save(entry *Entry) error {
	if entry.AbsPath == "" {
		return fmt.Errorf("registry entry AbsPath must be non-empty")
	}
	id := pathutil.WorktreeID(entry.AbsPath)
	entry.LastActive = time.Now().UTC()

	dir := registryDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create registry dir: %w", err)
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal registry entry %s: %w", id, err)
	}

	tmpPath := filepath.Join(dir, id+".json.tmp")
	finalPath := entryPath(id)

	if err := os.WriteFile(tmpPath, data, 0o644); err != nil { //nolint:gosec // registry data is not sensitive
		return fmt.Errorf("write registry tmp %s: %w", id, err)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath) // best-effort cleanup of orphaned tmp
		return fmt.Errorf("rename registry entry %s: %w", id, err)
	}
	return nil
}

// Delete removes the registry entry for id. Returns nil when the file was
// already absent (idempotent).
func Delete(id string) error {
	err := os.Remove(entryPath(id))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete registry entry %s: %w", id, err)
	}
	return nil
}

// ListAll returns every valid registry entry. Entries with unparseable JSON
// are silently skipped (treat as corrupt / being-written). Returns nil slice
// and nil error when the registry directory does not yet exist.
func ListAll() ([]*Entry, error) {
	files, err := os.ReadDir(registryDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list registry: %w", err)
	}
	var entries []*Entry
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".json" {
			continue
		}
		// Skip in-progress atomic writes.
		if strings.HasSuffix(f.Name(), ".json.tmp") {
			continue
		}
		id := strings.TrimSuffix(f.Name(), ".json")
		entry, err := Load(id)
		if err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
