package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/grovetools/core/pkg/notespace"
)

// configuredNotespaceIndex's memoization.
//
// Notespace name resolution is on groved's hottest loop: the unified watcher
// refreshes its watch set on a 15s ticker AND on every workspace/focus store
// update AND on every event batch containing a directory create, and the sync
// handler resolves a notespace root for every discovered workspace on each of
// those passes. Uncached, one resolution is a ReadDir of every recorded
// notebook's notespaces/ directory plus a read+TOML-parse of every stamp under
// them — twice over, since BuildIndex and the record sweep each loaded them
// independently. The 2026-08-15 profile attributed 27.7% of a 250%-CPU groved
// to that, with another 36.5% of the process in GC feeding on its allocations.
//
// So the index is memoized per root set, and a cached entry is served only
// while every input the result depends on is provably unchanged:
//
//   - each recorded notebook's notespaces/ directory, by (mtime, size) — which
//     is what changes when a notespace directory is created, removed or
//     renamed;
//   - each notespace's .notespace.toml, by (mtime, size) and by existence —
//     which is what changes when a stamp is minted, re-minted or edited;
//   - the legacy workspaces/ directory of any notebook that has no notespaces/
//     directory, because that is the input ValidateNotespaceLayout turns into a
//     migration error.
//
// Freshness therefore costs one stat per notebook and one per stamp — on this
// machine 13 + 76 of them, ~130us — against ~4.5ms and ~150 file reads for a
// full rebuild. Nothing is time-bounded: a stamp minted or a binding changed is
// picked up on the very next call, exactly as it was before this cache existed.
//
// Failures are never cached: an unreadable directory, a malformed stamp or a
// legacy layout takes the same code path it took before, every time.
//
// IMMUTABILITY CONTRACT: the *notespace.Index and the []notespace.Record
// returned are shared between all callers of the same root set. Both are
// read-only inputs to the resolvers in this package (Index exposes only
// copy-returning queries, and the record slice is scanned, never sorted in
// place). A caller that needs to mutate either must clone it first.

// pathStamp is the cheap identity of a filesystem entry version: what a stat
// returns, plus whether the entry is there at all. Directories are stamped the
// same way as files — a directory's mtime moves when its entry list changes,
// which is exactly the input that can add or remove a notespace.
type pathStamp struct {
	exists  bool
	isDir   bool
	modTime time.Time
	size    int64
}

// equal compares stamps. time.Time must be compared with Equal rather than ==
// (the monotonic/wall/location fields make == unreliable).
func (s pathStamp) equal(other pathStamp) bool {
	if s.exists != other.exists {
		return false
	}
	if !s.exists {
		return true
	}
	return s.isDir == other.isDir && s.size == other.size && s.modTime.Equal(other.modTime)
}

// statPath stats path. Anything that cannot be statted collapses to the zero
// stamp — "nothing usable here" — which is a legitimate state to pin an entry
// against: a notebook whose notespaces/ directory does not exist yet is a
// cacheable answer, and the directory appearing later changes the stamp.
func statPath(path string) pathStamp {
	if path == "" {
		return pathStamp{}
	}
	info, err := os.Stat(path)
	if err != nil {
		return pathStamp{}
	}
	return pathStamp{exists: true, isDir: info.IsDir(), modTime: info.ModTime(), size: info.Size()}
}

// stampedInput is one filesystem input the cached index was derived from.
type stampedInput struct {
	path  string
	stamp pathStamp
}

// notespaceIndexEntry memoizes one root set's index together with every input
// it depended on. The mutex is per entry, so a slow rebuild for one root set
// never blocks a lookup against another, and concurrent callers asking for the
// SAME set collapse onto a single rebuild instead of racing through it N times
// — the singleflight the daemon's per-workspace fan-out needs.
type notespaceIndexEntry struct {
	mu sync.Mutex

	valid   bool
	idx     *notespace.Index
	records []notespace.Record
	inputs  []stampedInput
}

// fresh reports whether the memoized index is still a correct answer. Callers
// hold e.mu.
func (e *notespaceIndexEntry) fresh() bool {
	if !e.valid {
		return false
	}
	for _, input := range e.inputs {
		if !statPath(input.path).equal(input.stamp) {
			return false
		}
	}
	return true
}

// notespaceIndexCache maps root-set key -> *notespaceIndexEntry. Entries are
// never evicted by age: the set of notebook root sets a process asks about is
// bounded by its recorded config, and an entry is one index plus a stamp per
// notespace.
var notespaceIndexCache sync.Map

// notespaceIndexKey identifies a root set. The roots are the RECORDED spellings
// (expansion happens during the build), sorted and joined with a separator no
// path can contain.
func notespaceIndexKey(roots []string) string {
	return strings.Join(roots, "\x00")
}

// ResetNotespaceIndexCache drops every memoized notespace index. Tests that
// rewrite a stamp in place within one filesystem timestamp tick — the one edit
// (mtime, size) cannot see — call this to force a rebuild.
func ResetNotespaceIndexCache() {
	notespaceIndexCache.Range(func(key, _ any) bool {
		notespaceIndexCache.Delete(key)
		return true
	})
}

// buildNotespaceIndex loads every stamped notespace under the recorded notebook
// roots, returning the index, the records, and every filesystem input the
// answer depended on.
//
// Each input is stamped BEFORE it is read, never after: a stamp taken after the
// read could pin a version newer than the bytes cached against it and hide the
// next change forever, whereas one taken before can only cause a spurious
// rebuild.
func buildNotespaceIndex(notebookRoots []string) (*notespace.Index, []notespace.Record, []stampedInput, error) {
	var (
		roots  []string
		inputs []stampedInput
	)
	for _, notebookRoot := range notebookRoots {
		expanded, err := expandCentralizedRoot(notebookRoot)
		if err != nil {
			return nil, nil, nil, err
		}
		notespacesDir := filepath.Join(expanded, NotespaceDirectory)
		dirStamp := statPath(notespacesDir)
		inputs = append(inputs, stampedInput{path: notespacesDir, stamp: dirStamp})
		if !dirStamp.exists {
			// expandCentralizedRoot turns a notebook that has workspaces/ but no
			// notespaces/ into a migration error, so that directory is an input
			// to this answer too — but only while notespaces/ is absent.
			legacy := filepath.Join(expanded, "workspaces")
			inputs = append(inputs, stampedInput{path: legacy, stamp: statPath(legacy)})
			continue
		}
		entries, err := os.ReadDir(notespacesDir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, nil, nil, fmt.Errorf("read notespaces in %s: %w", expanded, err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				roots = append(roots, filepath.Join(notespacesDir, entry.Name()))
			}
		}
	}
	sort.Strings(roots)

	records := make([]notespace.Record, 0, len(roots))
	for _, root := range roots {
		stampPath := notespace.NotespaceStampPath(root)
		inputs = append(inputs, stampedInput{path: stampPath, stamp: statPath(stampPath)})
		stamp, err := notespace.LoadNotespace(root)
		if err != nil {
			return nil, nil, nil, err
		}
		if stamp != nil {
			records = append(records, notespace.Record{Root: root, Stamp: *stamp})
		}
	}
	return notespace.NewIndex(records), records, inputs, nil
}
