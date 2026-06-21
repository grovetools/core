package state

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/pkg/worktreeregistry"
	"github.com/grovetools/core/util/pathutil"
)

// State represents the local Grove state as a generic map of key-value pairs.
// This allows any Grove tool to store arbitrary state data.
type State map[string]interface{}

// ErrNoEcosystemRoot is returned by writes when the given dir does not resolve
// to an ecosystem/worktree root. Writes are refused (rather than falling back
// to a home-global ~/.grove/state.yml) to keep state strictly per-ecosystem.
var ErrNoEcosystemRoot = fmt.Errorf("no grove ecosystem root found for directory")

// ecosystemRootForDir walks up from dir to the nearest ecosystem/worktree root.
//
// A directory qualifies as an ecosystem root when it contains a grove config
// marker (grove.toml/grove.yml and dotted variants) or a .grove/ directory.
// dir itself is eligible (a dir that IS an ecosystem root resolves to itself).
//
// Returns ("", false) if no ecosystem root is found up to the filesystem root.
// There is intentionally NO home-global fallback: a process whose dir is
// outside any ecosystem (e.g. $HOME) resolves to no root.
func ecosystemRootForDir(dir string) (string, bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}

	current := abs
	for {
		if dirIsEcosystemRoot(current) {
			return current, true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		current = parent
	}
}

// dirIsEcosystemRoot reports whether dir contains a grove ecosystem marker.
func dirIsEcosystemRoot(dir string) bool {
	markers := []string{
		"grove.toml",
		"grove.yml",
		"grove.yaml",
		".grove.toml",
		".grove.yml",
		".grove.yaml",
	}
	for _, m := range markers {
		if info, err := os.Stat(filepath.Join(dir, m)); err == nil && !info.IsDir() {
			return true
		}
	}
	if info, err := os.Stat(filepath.Join(dir, ".grove")); err == nil && info.IsDir() {
		return true
	}
	return false
}

// stateRoot resolves the directory whose .grove/state.yml should be used for
// the given dir. It mirrors the registry-primary resolution used by Load/Save:
// when dir is inside a grove worktree the worktree root is preferred; otherwise
// it walks up to the nearest ecosystem root.
//
// Returns ("", false) when dir resolves to no ecosystem/worktree root.
func stateRoot(dir string) (string, bool) {
	if root, ok := workspace.WorktreeRootForPath(dir); ok {
		return root, true
	}
	return ecosystemRootForDir(dir)
}

// stateFilePath returns the path to the state file for dir.
//
// Resolution is anchored to dir (never os.Getwd()). The dir is resolved to its
// owning ecosystem/worktree root; .grove/state.yml under that root is returned.
//
// Returns an error if dir is inside a notebook (directing the user to run the
// command from the project directory instead), or ErrNoEcosystemRoot if dir
// resolves to no ecosystem/worktree root (writes must refuse rather than create
// a home-global file).
func stateFilePath(dir string) (string, error) {
	// Check if we're inside a notebook - if so, block state writes with helpful error
	notebookRoot := workspace.FindNotebookRoot(dir)
	if notebookRoot != "" {
		// Extract workspace name for the error message
		workspaceName := workspace.ExtractWorkspaceNameFromNotebookPath(dir, notebookRoot)
		if workspaceName == "" || workspaceName == "global" {
			return "", fmt.Errorf("cannot write state from notebook directory")
		}

		// Use reverse lookup to find the associated project (discovery is fine here since this is an error path)
		project, _, _ := workspace.GetProjectFromNotebookPath(dir)

		if project != nil {
			return "", fmt.Errorf("you are in the notebook directory for '%s'.\n"+
				"Run this command from the project directory instead:\n\n"+
				"  cd %s", workspaceName, project.Path)
		}

		return "", fmt.Errorf("you are in a notebook directory for '%s'.\n"+
			"Run this command from the associated project directory instead.", workspaceName)
	}

	root, ok := ecosystemRootForDir(dir)
	if !ok {
		return "", ErrNoEcosystemRoot
	}

	return filepath.Join(root, ".grove", "state.yml"), nil
}

// Load loads the state from the state file for dir.
// Returns an empty state if the file doesn't exist or dir resolves to no
// ecosystem/worktree root (graceful "no state" — never an error for reads).
//
// Registry-primary: when dir is inside a grove worktree, the registry entry's
// SessionState is returned if non-empty. Falls back to the .grove/state.yml
// file under dir's ecosystem root for missing/empty registry entries.
func Load(dir string) (State, error) {
	if root, ok := workspace.WorktreeRootForPath(dir); ok {
		id := pathutil.WorktreeID(root)
		if entry, rerr := worktreeregistry.Load(id); rerr == nil && len(entry.SessionState) > 0 {
			return State(entry.SessionState), nil
		}
	}

	root, ok := ecosystemRootForDir(dir)
	if !ok {
		// No ecosystem root: graceful empty state (no home-global fallback).
		return make(State), nil
	}

	path := filepath.Join(root, ".grove", "state.yml")

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

// Save saves the state to the state file for dir.
//
// Returns ErrNoEcosystemRoot (and writes nothing) when dir resolves to no
// ecosystem/worktree root, so a process outside any ecosystem (e.g. $HOME)
// can never create a home-global ~/.grove/state.yml.
//
// Dual-write: when dir is inside a grove worktree the session state is also
// persisted to the registry entry. The .grove/state.yml write is retained
// during the deprecation window so older tooling still works.
func Save(dir string, state State) error {
	// Enforce notebook guard and resolve path first.
	path, err := stateFilePath(dir)
	if err != nil {
		return err
	}

	// Registry dual-write: best-effort, non-fatal.
	if root, ok := workspace.WorktreeRootForPath(dir); ok {
		id := pathutil.WorktreeID(root)
		entry, _ := worktreeregistry.Load(id)
		if entry == nil {
			entry = &worktreeregistry.Entry{AbsPath: root}
		}
		entry.SessionState = map[string]interface{}(state)
		_ = worktreeregistry.Save(entry)
	}

	// .grove/state.yml write (deprecation-window dual-write).
	dirPath := filepath.Dir(path)
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}

	data, err := yaml.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil { //nolint:gosec // state file is not sensitive
		return fmt.Errorf("write state file: %w", err)
	}

	return nil
}

// Get retrieves a value from the state for dir by key.
// Returns the value and true if found, nil and false otherwise.
func Get(dir, key string) (interface{}, bool, error) {
	state, err := Load(dir)
	if err != nil {
		return nil, false, err
	}

	val, ok := state[key]
	return val, ok, nil
}

// GetString is a convenience function to get a string value from state for dir.
// Returns empty string if the key doesn't exist or the value is not a string.
func GetString(dir, key string) (string, error) {
	val, ok, err := Get(dir, key)
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

// Set sets a value in the state for dir.
func Set(dir, key string, value interface{}) error {
	state, err := Load(dir)
	if err != nil {
		return err
	}

	state[key] = value
	return Save(dir, state)
}

// Delete removes a key from the state for dir.
func Delete(dir, key string) error {
	state, err := Load(dir)
	if err != nil {
		return err
	}

	delete(state, key)
	return Save(dir, state)
}
