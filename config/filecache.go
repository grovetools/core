package config

import (
	"os"
	"strings"
	"sync"
	"time"
)

// Load's per-file memoization.
//
// Every workspace classification (workspace.findGroveConfig ->
// classifyWorkspaceRoot) calls Load on a repo's grove.toml, and groved's
// plan-stats pass runs that across hundreds of workspaces every few seconds.
// A single Load is ~170us / ~1200 allocations: the TOML body is unmarshalled
// twice (struct + raw map for Extensions) and the result is walked by
// jsonschema/v5 in validateAndWarn. Repeating that for files that have not
// changed since the last call is pure garbage — enough of it to drive the
// daemon's GC, which is what showed up as CPU in the 2026-08-07 audit.
//
// So Load memoizes per absolute path, and a cached entry is served only while
// every input the result depends on is provably unchanged:
//
//   - the config file itself, by (mtime, size);
//   - ~/.config/grove/machine.toml, by path and (mtime, size) — compileMachineGroves
//     folds its subscriptions into every loaded config, and its ABSENCE is
//     equally load-bearing, so a stamp records "no file here" as a state;
//   - the environment variables the file's ${VAR} references expand from,
//     by value.
//
// Freshness costs two stats and a handful of os.Getenv calls; a miss costs the
// full read+parse+validate. Failures are never cached: an unreadable or
// unparseable path takes exactly the code path it took before this cache
// existed, so ENOENT/permission behaviour is bit-for-bit unchanged.
//
// IMMUTABILITY CONTRACT: the *Config returned by Load is shared between all
// callers holding the same path. Nothing in the ecosystem mutates it today
// (every call site reads Workspaces/Extensions/Context or funnels through
// UnmarshalExtension, which decodes into the caller's own struct) and nothing
// should start: a mutation would leak sideways into unrelated callers. The
// same contract already applies to LoadFromWithLogger's cache.

// fileStamp is the cheap identity of a file version: what a stat returns, plus
// whether the file is there at all.
type fileStamp struct {
	exists  bool
	modTime time.Time
	size    int64
}

// equal compares stamps. time.Time must be compared with Equal rather than ==
// (the monotonic/wall/location fields make == unreliable).
func (s fileStamp) equal(other fileStamp) bool {
	if s.exists != other.exists {
		return false
	}
	if !s.exists {
		return true
	}
	return s.size == other.size && s.modTime.Equal(other.modTime)
}

// stampFile stats path. A path that does not exist, cannot be statted, or is a
// directory all collapse to the zero stamp — "nothing usable here" — which is
// a legitimate state to pin an entry against.
func stampFile(path string) fileStamp {
	if path == "" {
		return fileStamp{}
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return fileStamp{}
	}
	return fileStamp{exists: true, modTime: info.ModTime(), size: info.Size()}
}

// fileCacheEntry memoizes one path's Load result together with every input
// that result depended on. The mutex is per entry, so a slow parse of one
// config never blocks Load of a different one, and concurrent callers asking
// for the SAME path collapse onto a single parse instead of racing through it
// N times (the singleflight the daemon's fan-out needs).
type fileCacheEntry struct {
	mu sync.Mutex

	valid bool
	cfg   *Config

	self        fileStamp
	machinePath string
	machine     fileStamp
	envNames    []string
	envValues   []string
}

// fresh reports whether the memoized config is still a correct answer for a
// config file whose current stamp is self. Callers hold e.mu.
func (e *fileCacheEntry) fresh(self fileStamp) bool {
	if !e.valid || e.cfg == nil {
		return false
	}
	if !e.self.equal(self) {
		return false
	}
	if e.machinePath != MachineConfigPath() {
		return false
	}
	if !e.machine.equal(stampFile(e.machinePath)) {
		return false
	}
	for i, name := range e.envNames {
		if os.Getenv(name) != e.envValues[i] {
			return false
		}
	}
	return true
}

// store records a freshly parsed config and the inputs it was derived from.
// Callers hold e.mu.
func (e *fileCacheEntry) store(self fileStamp, cfg *Config, envNames []string) {
	envValues := make([]string, len(envNames))
	for i, name := range envNames {
		envValues[i] = os.Getenv(name)
	}

	machinePath := MachineConfigPath()
	e.self = self
	e.machinePath = machinePath
	e.machine = stampFile(machinePath)
	e.envNames = envNames
	e.envValues = envValues
	e.cfg = cfg
	e.valid = true
}

// fileCache maps absolute config path -> *fileCacheEntry. Entries are never
// evicted by age: the set of config files a process touches is bounded by the
// workspaces it knows about, and an entry is a pointer plus two stamps.
var fileCache sync.Map

// envRefs returns the environment variable names expandEnvVars would consult
// for this content, in first-appearance order and deduplicated. It mirrors
// expandEnvVars' parsing exactly — including the ${VAR:-default} form, where
// only the part before ":-" is a variable name — because a divergence here
// would let an env change go unnoticed by the cache.
func envRefs(content string) []string {
	matches := envVarRegex.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		name := m[1]
		if parts := strings.SplitN(name, ":-", 2); len(parts) > 1 {
			name = parts[0]
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}

// resetFileCache drops every memoized Load result. See ResetLoadCache.
func resetFileCache() {
	fileCache.Range(func(key, _ any) bool {
		fileCache.Delete(key)
		return true
	})
}
