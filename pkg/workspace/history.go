package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
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

// GetAccessHistoryPath returns the path to the access history file
func GetAccessHistoryPath(configDir string) string {
	return filepath.Join(configDir, "gmux", "access-history.json")
}

// LoadAccessHistory loads the access history from disk
func LoadAccessHistory(configDir string) (*AccessHistory, error) {
	historyFile := GetAccessHistoryPath(configDir)

	data, err := os.ReadFile(historyFile)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty history if file doesn't exist
			return &AccessHistory{
				Projects: make(map[string]*ProjectAccess),
			}, nil
		}
		return nil, err
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
	historyDir := filepath.Join(configDir, "gmux")
	if err := os.MkdirAll(historyDir, 0755); err != nil {
		return err
	}

	historyFile := GetAccessHistoryPath(configDir)

	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(historyFile, data, 0644)
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

