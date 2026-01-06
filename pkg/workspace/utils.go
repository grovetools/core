package workspace

import (
	"os"
	"path/filepath"
)

// IsNotebookRepo checks if a given path is a notebook repository by looking for a marker file.
func IsNotebookRepo(path string) bool {
	markerPath := filepath.Join(path, ".grove", "notebook.yml")
	if _, err := os.Stat(markerPath); err == nil {
		return true
	}
	return false
}
