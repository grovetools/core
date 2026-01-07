package workspace

import (
	"os"
	"path/filepath"
)

// IsNotebookRepo checks if a given path is a notebook repository by looking for a marker file.
func IsNotebookRepo(path string) bool {
	// Check new location first (top-level)
	if _, err := os.Stat(filepath.Join(path, "notebook.yml")); err == nil {
		return true
	}
	// Fall back to legacy location for existing notebooks
	if _, err := os.Stat(filepath.Join(path, ".grove", "notebook.yml")); err == nil {
		return true
	}
	return false
}
