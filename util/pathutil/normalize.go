package pathutil

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// normalizeCacheTTL caps how long a memoized NormalizeForLookup result
// is served before the underlying filesystem is re-consulted. 2s is
// short enough that a user who deletes and recreates a directory will
// see fresh results on the next render cycle, yet long enough to
// absorb the thundering herd of Lstat syscalls that a single TUI
// render frame or daemon collector tick used to trigger (observed at
// 82% CPU in the sluggish-terminal pprof, 2026-04-08). Tune via the
// SetNormalizeCacheTTL helper in tests; the production default stays 2s.
const normalizeCacheTTL = 2 * time.Second

type normalizeCacheEntry struct {
	normalized string
	err        error
	stored     time.Time
}

// normalizeCache memoizes NormalizeForLookup results. sync.Map is the
// right fit: write-once-read-many under contention, and the key space
// (absolute paths a process references) is bounded in practice. We
// deliberately do not evict old entries — a short TTL combined with a
// long-lived process won't accumulate enough paths to matter.
var (
	normalizeCache sync.Map
	// cacheTTL is package-var not const so tests can shrink it.
	cacheTTL = normalizeCacheTTL
)

// NormalizeForLookup creates a canonical, case-normalized path suitable for use as a map key or in comparisons.
// It performs the following steps:
// 1. Makes the path absolute.
// 2. Evaluates any symbolic links.
// 3. On case-insensitive OSes (macOS, Windows), converts the path to lowercase.
//
// Results are memoized per input path for a short TTL to absorb the
// cost of repeated EvalSymlinks -> Lstat loops in TUI render frames
// and daemon collector ticks.
func NormalizeForLookup(path string) (string, error) {
	if cached, ok := normalizeCache.Load(path); ok {
		entry := cached.(normalizeCacheEntry)
		if time.Since(entry.stored) < cacheTTL {
			return entry.normalized, entry.err
		}
	}

	normalized, err := normalizeForLookupUncached(path)
	normalizeCache.Store(path, normalizeCacheEntry{
		normalized: normalized,
		err:        err,
		stored:     time.Now(),
	})
	return normalized, err
}

// normalizeForLookupUncached is the raw (uncached) normalization logic.
// Split out so the cache wrapper above stays small and easy to audit.
func normalizeForLookupUncached(path string) (string, error) {
	// Step 1 & 2: Get the absolute, canonical path.
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	canonicalPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		// If symlink evaluation fails (e.g., path doesn't exist yet),
		// fall back to the absolute path.
		canonicalPath = absPath
	}

	// Step 3: Normalize case on insensitive filesystems.
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		return strings.ToLower(canonicalPath), nil
	}

	return canonicalPath, nil
}

// ComparePaths checks if two paths refer to the same location, respecting OS case sensitivity.
func ComparePaths(path1, path2 string) (bool, error) {
	norm1, err := NormalizeForLookup(path1)
	if err != nil {
		return false, err
	}
	norm2, err := NormalizeForLookup(path2)
	if err != nil {
		return false, err
	}
	return norm1 == norm2, nil
}

// CanonicalPath returns the absolute path with correct filesystem case.
// Unlike NormalizeForLookup, this preserves the actual case from the filesystem,
// which is important for matching paths used by external tools (e.g., Claude's project paths).
// On macOS, this ensures /users/foo becomes /Users/foo to match the real directory name.
//
// Note: filepath.EvalSymlinks does NOT canonicalize case on macOS, so we walk
// each path component and look up the correct case from the filesystem.
func CanonicalPath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	// First resolve symlinks
	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		// If symlink evaluation fails (e.g., path doesn't exist yet),
		// fall back to the absolute path.
		resolved = absPath
	}

	// On non-macOS/Windows, case is preserved by the filesystem
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		return resolved, nil
	}

	// Handle root
	if resolved == "/" {
		return "/", nil
	}

	// Split into components, handling leading /
	var parts []string
	isAbsolute := strings.HasPrefix(resolved, "/")
	for _, p := range strings.Split(resolved, string(filepath.Separator)) {
		if p != "" {
			parts = append(parts, p)
		}
	}

	// Build up the canonical path component by component
	var result string
	if isAbsolute {
		result = "/"
	}

	for _, part := range parts {
		// Try to read the current directory and find the correct case
		entries, err := os.ReadDir(result)
		if err != nil {
			// Directory doesn't exist or can't be read, append as-is
			result = filepath.Join(result, part)
			continue
		}

		found := false
		for _, entry := range entries {
			if strings.EqualFold(entry.Name(), part) {
				result = filepath.Join(result, entry.Name())
				found = true
				break
			}
		}

		if !found {
			result = filepath.Join(result, part)
		}
	}

	return result, nil
}
