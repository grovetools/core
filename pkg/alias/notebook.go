package alias

import (
	"fmt"
	"strings"
)

// ParseResourceAlias parses a string in the format "workspace:path/to/resource".
// It separates the workspace name from the relative path within that workspace's notebook.
// Note: The "nb:" prefix should already be stripped before calling this function.
func ParseResourceAlias(alias string) (workspace, resourcePath string, err error) {
	parts := strings.SplitN(alias, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid resource alias format: expected 'workspace:path/to/resource', got '%s'", alias)
	}
	return parts[0], parts[1], nil
}
