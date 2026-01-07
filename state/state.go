package state

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mattsolo1/grove-core/pkg/workspace"
	"gopkg.in/yaml.v3"
)

// State represents the local Grove state as a generic map of key-value pairs.
// This allows any Grove tool to store arbitrary state data.
type State map[string]interface{}

// stateFilePath returns the path to the state file.
// The state file is located in .grove/state.yml in the current working directory.
// This allows each worktree to have its own independent state.
//
// Returns an error if the current directory is inside a notebook, directing
// the user to run the command from the project directory instead.
func stateFilePath() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get current directory: %w", err)
	}

	// Check if we're inside a notebook - if so, block state writes with helpful error
	notebookRoot := workspace.FindNotebookRoot(cwd)
	if notebookRoot != "" {
		// Extract workspace name for the error message
		workspaceName := workspace.ExtractWorkspaceNameFromNotebookPath(cwd, notebookRoot)
		if workspaceName == "" || workspaceName == "global" {
			return "", fmt.Errorf("cannot write state from notebook directory")
		}

		// Use reverse lookup to find the associated project (discovery is fine here since this is an error path)
		project, _, _ := workspace.GetProjectFromNotebookPath(cwd)

		if project != nil {
			return "", fmt.Errorf("you are in the notebook directory for '%s'.\n"+
				"Run this command from the project directory instead:\n\n"+
				"  cd %s", workspaceName, project.Path)
		}

		return "", fmt.Errorf("you are in a notebook directory for '%s'.\n"+
			"Run this command from the associated project directory instead.", workspaceName)
	}

	return filepath.Join(cwd, ".grove", "state.yml"), nil
}

// Load loads the state from the state file.
// Returns an empty state if the file doesn't exist.
func Load() (State, error) {
	path, err := stateFilePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty state if file doesn't exist
			return make(State), nil
		}
		return nil, fmt.Errorf("read state file: %w", err)
	}

	var state State
	if err := yaml.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse state file: %w", err)
	}

	if state == nil {
		state = make(State)
	}

	return state, nil
}

// Save saves the state to the state file.
func Save(state State) error {
	path, err := stateFilePath()
	if err != nil {
		return err
	}

	// Ensure .grove directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}

	data, err := yaml.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}

	return nil
}

// Get retrieves a value from the state by key.
// Returns the value and true if found, nil and false otherwise.
func Get(key string) (interface{}, bool, error) {
	state, err := Load()
	if err != nil {
		return nil, false, err
	}

	val, ok := state[key]
	return val, ok, nil
}

// GetString is a convenience function to get a string value from state.
// Returns empty string if the key doesn't exist or the value is not a string.
func GetString(key string) (string, error) {
	val, ok, err := Get(key)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", nil
	}

	str, ok := val.(string)
	if !ok {
		return "", nil
	}

	return str, nil
}

// Set sets a value in the state.
func Set(key string, value interface{}) error {
	state, err := Load()
	if err != nil {
		return err
	}

	state[key] = value
	return Save(state)
}

// Delete removes a key from the state.
func Delete(key string) error {
	state, err := Load()
	if err != nil {
		return err
	}

	delete(state, key)
	return Save(state)
}
