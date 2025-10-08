package state

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// State represents the local Grove state as a generic map of key-value pairs.
// This allows any Grove tool to store arbitrary state data.
type State map[string]interface{}

// stateFilePath returns the path to the state file.
// The state file is located in .grove/state.yml in the current working directory.
// This allows each worktree to have its own independent state.
func stateFilePath() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get current directory: %w", err)
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
