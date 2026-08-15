package claudetrust

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// IsTrusted reports whether path's exact entry in ~/.claude.json has
// hasTrustDialogAccepted set to the JSON boolean true.
//
// Like SeedTrust, IsTrusted expects path to already be in the canonical form
// used by the launcher. It does not clean, resolve, or canonicalize path, and
// trust does not inherit between parent and child paths.
//
// A missing ~/.claude.json, projects map, project entry, or true trust flag is
// reported as (false, nil). A malformed or unreadable file is reported as
// (false, error). IsTrusted is read-only and is not affected by the trust
// seeding environment gate.
func IsTrusted(path string) (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, fmt.Errorf("locate home dir: %w", err)
	}
	configPath := filepath.Join(home, ".claude.json")

	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", configPath, err)
	}

	root := map[string]any{}
	if err := json.Unmarshal(data, &root); err != nil {
		return false, fmt.Errorf("parse %s: %w", configPath, err)
	}

	projects, ok := root["projects"].(map[string]any)
	if !ok {
		return false, nil
	}
	entry, ok := projects[path].(map[string]any)
	if !ok {
		return false, nil
	}
	trusted, ok := entry[trustKey].(bool)
	return ok && trusted, nil
}
