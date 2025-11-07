package pathutil

import (
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
