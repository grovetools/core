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
// Entry. Update holds this lock for the full cycle.
//
// This mutex is process-local and does NOT make the registry safe across
// processes: Reconcile Saves (anchor-heal and adopt), and the daemon is a
// different process from every CLI and TUI writer. The destructive half of
// that gap — adopt replacing a populated entry with a structural default — is
// closed by SaveIfAbsent. Concurrent read-modify-write between processes is
// still last-write-wins on the whole Entry; closing that needs flock and is an
// open question.
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

// SaveIfAbsent persists entry only if no registry file exists yet, reporting
// whether it created one. An existing entry is left untouched: losing the race
// is a normal outcome, not an error.
//
// Save is an unconditional whole-Entry overwrite — the wrong primitive for a
// caller that only means to CREATE. os.Link fails with EEXIST rather than
// replacing, making the check and the write one atomic step that is safe
// across processes without a lock. Content is staged in a temp file first so
// readers never observe a partial entry, matching Save's guarantee.
func SaveIfAbsent(entry *Entry) (bool, error) {
	if entry.AbsPath == "" {
		return false, fmt.Errorf("registry entry AbsPath must be non-empty")
	}
	registryMu.Lock()
	defer registryMu.Unlock()

	id := pathutil.WorktreeID(entry.AbsPath)
	dir := registryDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, fmt.Errorf("create registry dir: %w", err)
	}

	entry.LastActive = time.Now().UTC()
	data, err := json.Marshal(entry)
	if err != nil {
		return false, fmt.Errorf("marshal registry entry %s: %w", id, err)
	}

	// A unique temp name: two processes adopting the same path concurrently
	// must not collide on the staging file itself.
	tmp, err := os.CreateTemp(dir, id+".*.tmp")
	if err != nil {
		return false, fmt.Errorf("create registry tmp %s: %w", id, err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return false, fmt.Errorf("write registry tmp %s: %w", id, err)
	}
	if err := tmp.Close(); err != nil {
		return false, fmt.Errorf("close registry tmp %s: %w", id, err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return false, fmt.Errorf("chmod registry tmp %s: %w", id, err)
	}

	if err := os.Link(tmpPath, entryPath(id)); err != nil {
		if os.IsExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("link registry entry %s: %w", id, err)
	}
	return true, nil
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

// Tombstone flips the registry entry for id to StatusFinished instead of
// deleting it, so finishing a plan stops destroying the only record that binds
// a worktree to its plan, repos and labels. It:
//
//   - sets Status=StatusFinished, FinishedAt=now and SchemaVersion;
//   - records finals as the per-repo final SHAs (nil leaves any existing set
//     untouched, so a re-tombstone cannot blank what the first one captured);
//   - STRIPS SessionState. A session payload is ephemeral working state, not
//     provenance; it is checkpointed separately by the review-packet path and
//     must never fossilize in a record that now lives forever.
//
// It is idempotent: re-tombstoning an already-finished entry keeps the original
// FinishedAt. A missing entry returns Load's error unchanged, so callers can
// test it with os.IsNotExist and distinguish "nothing to record" from "the
// record was lost".
//
// Delete remains for callers that genuinely want the entry gone.
func Tombstone(id string, finals []RepoFinalState) (*Entry, error) {
	registryMu.Lock()
	defer registryMu.Unlock()

	entry, err := Load(id)
	if err != nil {
		return nil, err
	}

	entry.SchemaVersion = EntrySchemaVersion
	if !entry.IsFinished() {
		entry.Status = StatusFinished
		entry.FinishedAt = time.Now().UTC()
	}
	if len(finals) > 0 {
		entry.FinalSHAs = finals
	}
	entry.SessionState = nil

	if err := Save(entry); err != nil {
		return nil, err
	}
	return entry, nil
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

// DeleteUnlessFinished removes the registry entry for id unless it has been
// tombstoned, reporting whether the entry was kept. Teardown paths that used to
// end in Delete call this instead: once finish has recorded a worktree's story
// as a tombstone, the very next teardown step must not erase it. An entry that
// is absent, unreadable or still active is deleted exactly as Delete would.
func DeleteUnlessFinished(id string) (kept bool, err error) {
	registryMu.Lock()
	defer registryMu.Unlock()

	if entry, loadErr := Load(id); loadErr == nil && entry.IsFinished() {
		return true, nil
	}
	return false, Delete(id)
}

// ListAll returns every valid ACTIVE registry entry. Tombstoned (finished)
// entries are excluded: they describe worktrees that no longer exist, and every
// existing caller of this function — skill sync, settings propagation, plan
// binding, orphan pruning — is asking about live worktrees. Callers that want
// history call ListAllIncludingFinished.
//
// Entries with unparseable JSON are silently skipped (treat as corrupt /
// being-written). Returns nil slice and nil error when the registry directory
// does not yet exist.
func ListAll() ([]*Entry, error) {
	return listAll(false)
}

// ListAllIncludingFinished returns every valid registry entry, tombstones
// included. This is the explicit opt-in for provenance queries ("which work
// came from which worktree, including worktrees that are gone").
func ListAllIncludingFinished() ([]*Entry, error) {
	return listAll(true)
}

func listAll(includeFinished bool) ([]*Entry, error) {
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
		if !includeFinished && entry.IsFinished() {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
