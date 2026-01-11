package pathutil

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// NormalizeForLookup creates a canonical, case-normalized path suitable for use as a map key or in comparisons.
// It performs the following steps:
// 1. Makes the path absolute.
// 2. Evaluates any symbolic links.
// 3. On case-insensitive OSes (macOS, Windows), converts the path to lowercase.
func NormalizeForLookup(path string) (string, error) {
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
