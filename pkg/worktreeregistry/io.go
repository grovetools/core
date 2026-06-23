package worktreeregistry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/util/pathutil"
)

// registryMu serializes read-modify-write cycles through Update so concurrent
// in-process writers cannot clobber each other. Save writes the WHOLE Entry
// atomically (tmp-file + os.Rename, no torn reads), but a bare
// Load → mutate → Save from two goroutines is last-write-wins on the whole
// Entry. Update holds this lock for the full cycle. The daemon only Reconciles
// (reads/merges) the registry — it does not Save — so in-process
// serialization is sufficient and no on-disk lock (flock) is warranted, which
// also avoids adding a file-locking dependency the ecosystem does not
// currently use.
var registryMu sync.Mutex

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

// Update performs a serialized read-modify-write on the registry entry for id.
// It locks registryMu for the entire Load(id) → mutate(&entry) → Save(entry)
// cycle, so concurrent callers see each other's changes instead of clobbering
// the whole Entry. mutate receives the freshly loaded entry and should modify
// it in place (e.g. set keys in SessionState).
//
// If the entry does not exist, Load's error is returned unchanged (callers can
// test it with os.IsNotExist). Any error from mutate's resulting Save is
// returned too. The id must be the registry id (the <id>.json filename, i.e.
// pathutil.WorktreeID of the entry's AbsPath) so the post-mutate Save — which
// re-derives the id from entry.AbsPath — writes back to the same file.
func Update(id string, mutate func(*Entry)) error {
	registryMu.Lock()
	defer registryMu.Unlock()

	entry, err := Load(id)
	if err != nil {
		return err
	}
	mutate(entry)
	return Save(entry)
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
