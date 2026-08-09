package config

import (
	"os"
	"path/filepath"
	"slices"
	"sync"
)

// LoadFromWithLogger's per-startDir memoization.
//
// The hierarchical load walks ancestors for a project config, reads the global
// XDG config plus its fragment and plugin globs, sniffs for an ecosystem
// config, resolves a notebook-stored config, merges override files, and folds
// in machine.toml — ~10 file reads, as many TOML parses, and a jsonschema
// validation per call. groved's plan-binding resolution runs that across every
// registry entry on every refresh, which is what pinned the daemon heap at its
// GOMEMLIMIT in the 2026-08-09 audit. The predecessor here was a 2-second TTL
// cache: correct, but useless for callers spaced ~30s apart, so the daemon
// paid ~45 cold hierarchical loads per refresh.
//
// So LoadFromWithLogger memoizes per absolute startDir, and a cached entry is
// served only while every input the previous load consulted is provably
// unchanged. The load records its inputs into a loadFromTrace as it goes:
//
//   - every file it read (or found unreadable), by (mtime, size) stamped
//     BEFORE the read, plus the values of the env vars that file's ${VAR}
//     references expand from;
//   - every candidate path it checked and did NOT find (override files),
//     so a newly created file invalidates;
//   - every directory glob's membership (config fragments, plugins);
//   - every discovery step whose answer is a path — the project-config
//     search, the XDG resolution, the ecosystem walk, the notebook lookup,
//     the git-root fallback — as a re-runnable closure whose result must
//     still match.
//
// Freshness costs a few dozen stats (plus re-running the cheap discovery
// closures); a miss costs the full hierarchical load. Correctness comes from
// those checks, never from wall time. Failures are not cached: an error path
// behaves exactly as it did before the memo existed.
//
// The IMMUTABILITY CONTRACT from filecache.go applies unchanged: the *Config
// returned by LoadFrom/LoadFromWithLogger is shared between all callers with
// the same startDir and must never be mutated.

// hierDep pins one file the hierarchical load consulted: present with a
// stamp, or absent (the zero stamp). envNames/envValues carry the environment
// the file's ${VAR} references expanded from, exactly as in fileCacheEntry.
type hierDep struct {
	path      string
	stamp     fileStamp
	envNames  []string
	envValues []string
}

// hierGlob pins a directory glob's membership. Per-file content changes are
// carried by the hierDep of each matched file; this catches files appearing
// in or vanishing from the globbed directory.
type hierGlob struct {
	pattern string
	files   []string
}

// hierLookup pins the result of a discovery step. redo re-runs the step
// against the same inputs; a differing answer means the cascade would be
// anchored on different files than the memoized load's.
type hierLookup struct {
	result string
	redo   func() string
}

// loadFromTrace accumulates every input one hierarchical load consulted. All
// methods are nil-safe so helpers shared with untraced callers can thread a
// nil trace through unchanged.
type loadFromTrace struct {
	deps    []hierDep
	globs   []hierGlob
	lookups []hierLookup
}

func (t *loadFromTrace) dep(path string, stamp fileStamp, envNames []string) {
	if t == nil {
		return
	}
	envValues := make([]string, len(envNames))
	for i, name := range envNames {
		envValues[i] = os.Getenv(name)
	}
	t.deps = append(t.deps, hierDep{path: path, stamp: stamp, envNames: envNames, envValues: envValues})
}

// readFile stamps path, reads it, and records the dependency. The stamp is
// taken BEFORE the read for the same reason Load's memo does it: a mid-load
// rewrite then caches newer content under an older stamp, which the next
// freshness check invalidates — the opposite order would pin stale content to
// a stamp nothing will ever invalidate. A failed read still records the stamp,
// so a change to the offending path retries the load.
func (t *loadFromTrace) readFile(path string) ([]byte, error) {
	if t == nil {
		return os.ReadFile(path)
	}
	stamp := stampFile(path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.dep(path, stamp, nil)
		return nil, err
	}
	t.dep(path, stamp, envRefs(string(data)))
	return data, nil
}

// absent records that path was checked and must stay absent: creating it would
// change which file a cascade layer reads.
func (t *loadFromTrace) absent(path string) {
	t.dep(path, fileStamp{}, nil)
}

// stat records path's current state — present or absent — without reading it.
func (t *loadFromTrace) stat(path string) {
	t.dep(path, stampFile(path), nil)
}

func (t *loadFromTrace) glob(pattern string, files []string) {
	if t == nil {
		return
	}
	t.globs = append(t.globs, hierGlob{pattern: pattern, files: slices.Clone(files)})
}

func (t *loadFromTrace) lookup(result string, redo func() string) {
	if t == nil {
		return
	}
	t.lookups = append(t.lookups, hierLookup{result: result, redo: redo})
}

// loadFromEntry memoizes one startDir's hierarchical load together with the
// trace of every input it depended on. The mutex is per entry, so a slow load
// of one directory never blocks another, and concurrent callers asking for the
// SAME directory collapse onto a single load — the singleflight the daemon's
// per-registry-entry fan-out needs.
type loadFromEntry struct {
	mu sync.Mutex

	valid bool
	cfg   *Config
	trace *loadFromTrace
}

// fresh reports whether the memoized config is still what a full hierarchical
// load would produce. Callers hold e.mu.
func (e *loadFromEntry) fresh() bool {
	if !e.valid || e.cfg == nil || e.trace == nil {
		return false
	}
	for _, l := range e.trace.lookups {
		if l.redo() != l.result {
			return false
		}
	}
	for _, g := range e.trace.globs {
		files, err := filepath.Glob(g.pattern)
		if err != nil || !slices.Equal(files, g.files) {
			return false
		}
	}
	for _, d := range e.trace.deps {
		if !d.stamp.equal(stampFile(d.path)) {
			return false
		}
		for i, name := range d.envNames {
			if os.Getenv(name) != d.envValues[i] {
				return false
			}
		}
	}
	return true
}

// store records a freshly loaded config and its trace. Callers hold e.mu.
func (e *loadFromEntry) store(cfg *Config, trace *loadFromTrace) {
	e.cfg = cfg
	e.trace = trace
	e.valid = true
}

// loadFromCache maps absolute startDir -> *loadFromEntry. Entries are never
// evicted by age: the set of directories a process loads from is bounded by
// the workspaces it knows about, and an entry is a pointer plus its trace.
var loadFromCache sync.Map

// resetLoadFromCache drops every memoized hierarchical load. See ResetLoadCache.
func resetLoadFromCache() {
	loadFromCache.Range(func(key, _ any) bool {
		loadFromCache.Delete(key)
		return true
	})
}
