package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/grovetools/core/pkg/paths"
)

// ProjectAccess tracks when a project was last accessed
type ProjectAccess struct {
	Path         string    `json:"path"`
	LastAccessed time.Time `json:"last_accessed"`
	AccessCount  int       `json:"access_count"`
}

// AccessHistory manages project access history
type AccessHistory struct {
	Projects map[string]*ProjectAccess `json:"projects"`
}

// GetAccessHistoryPath returns the path to the access history file.
// Access history is runtime state, not configuration, so it lives under the
// state directory. The configDir argument is kept for the legacy fallback
// location used by older installs.
func GetAccessHistoryPath(configDir string) string {
	if stateDir := paths.StateDir(); stateDir != "" {
		return filepath.Join(stateDir, "gmux", "access-history.json")
	}
	return legacyAccessHistoryPath(configDir)
}

// legacyAccessHistoryPath is the pre-StateDir location under the config dir.
func legacyAccessHistoryPath(configDir string) string {
	return filepath.Join(configDir, "gmux", "access-history.json")
}

// LoadAccessHistory loads the access history from disk
func LoadAccessHistory(configDir string) (*AccessHistory, error) {
	historyFile := GetAccessHistoryPath(configDir)

	data, err := os.ReadFile(historyFile)
	if err != nil && os.IsNotExist(err) {
		// Migrate-on-read: fall back to the legacy config-dir location; the
		// next Save writes to the state dir.
		if legacy := legacyAccessHistoryPath(configDir); legacy != historyFile {
			data, err = os.ReadFile(legacy)
		}
	}
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty history if file doesn't exist
			return &AccessHistory{
				Projects: make(map[string]*ProjectAccess),
			}, nil
		}
		return nil, err
	}

	// Return empty history if file is empty
	if len(data) == 0 {
		return &AccessHistory{
			Projects: make(map[string]*ProjectAccess),
		}, nil
	}

	var history AccessHistory
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, err
	}

	if history.Projects == nil {
		history.Projects = make(map[string]*ProjectAccess)
	}

	return &history, nil
}

// Save saves the access history to disk
func (h *AccessHistory) Save(configDir string) error {
	historyFile := GetAccessHistoryPath(configDir)
	if err := os.MkdirAll(filepath.Dir(historyFile), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(historyFile, data, 0o644) //nolint:gosec // history file is not sensitive
}

// RecordAccess records that a project was accessed
func (h *AccessHistory) RecordAccess(path string) {
	if h.Projects == nil {
		h.Projects = make(map[string]*ProjectAccess)
	}

	if access, exists := h.Projects[path]; exists {
		access.LastAccessed = time.Now()
		access.AccessCount++
	} else {
		h.Projects[path] = &ProjectAccess{
			Path:         path,
			LastAccessed: time.Now(),
			AccessCount:  1,
		}
	}
}

// GetLastAccessed returns the last accessed time for a project
func (h *AccessHistory) GetLastAccessed(path string) *time.Time {
	if access, exists := h.Projects[path]; exists {
		return &access.LastAccessed
	}
	return nil
}

// UpdateAccessHistory is a convenience function that loads, updates, and saves access history
func UpdateAccessHistory(configDir, workspacePath string) error {
	history, err := LoadAccessHistory(configDir)
	if err != nil {
		return fmt.Errorf("failed to load access history: %w", err)
	}

	history.RecordAccess(workspacePath)

	if err := history.Save(configDir); err != nil {
		return fmt.Errorf("failed to save access history: %w", err)
	}

	return nil
}

// LoadAccessHistoryAsMap is a convenience function that returns a simple map of path -> lastAccessed time
func LoadAccessHistoryAsMap(configDir string) (map[string]time.Time, error) {
	history, err := LoadAccessHistory(configDir)
	if err != nil {
		return nil, err
	}

	result := make(map[string]time.Time)
	for path, access := range history.Projects {
		result[path] = access.LastAccessed
	}
	return result, nil
}
