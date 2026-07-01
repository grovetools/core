package workspace

import "path/filepath"

// resolveDir symlink-resolves a temp root so tests compare against the
// canonical paths discovery now produces (macOS /tmp -> /private/tmp).
func resolveDir(d string) string {
	if r, err := filepath.EvalSymlinks(d); err == nil {
		return r
	}
	return d
}
