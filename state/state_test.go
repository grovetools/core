package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateOperations(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "grove-state-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Change to temp directory
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	t.Run("Load empty state", func(t *testing.T) {
		state, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if state == nil {
			t.Fatal("Load() returned nil state")
		}
		if len(state) != 0 {
			t.Errorf("Load() returned non-empty state: %v", state)
		}
	})

	t.Run("Set and Get string value", func(t *testing.T) {
		key := "test.key"
		value := "test-value"

		if err := Set(key, value); err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		got, err := GetString(key)
		if err != nil {
			t.Fatalf("GetString() error = %v", err)
		}
		if got != value {
			t.Errorf("GetString() = %v, want %v", got, value)
		}
	})

	t.Run("Get with generic Get function", func(t *testing.T) {
		key := "test.another"
		value := "another-value"

		if err := Set(key, value); err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		got, ok, err := Get(key)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if !ok {
			t.Fatal("Get() returned ok=false")
		}
		if got != value {
			t.Errorf("Get() = %v, want %v", got, value)
		}
	})

	t.Run("Get non-existent key", func(t *testing.T) {
		got, ok, err := Get("non.existent")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if ok {
			t.Error("Get() returned ok=true for non-existent key")
		}
		if got != nil {
			t.Errorf("Get() = %v, want nil", got)
		}
	})

	t.Run("Delete key", func(t *testing.T) {
		key := "test.delete"
		value := "to-be-deleted"

		// Set a value
		if err := Set(key, value); err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		// Verify it exists
		_, ok, err := Get(key)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if !ok {
			t.Fatal("Get() returned ok=false after Set()")
		}

		// Delete it
		if err := Delete(key); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		// Verify it's gone
		_, ok, err = Get(key)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if ok {
			t.Error("Get() returned ok=true after Delete()")
		}
	})

	t.Run("Set multiple keys", func(t *testing.T) {
		keys := map[string]interface{}{
			"flow.active_plan": "my-plan",
			"flow.model":       "claude-3-5-sonnet",
			"notebook.count":   42,
		}

		for k, v := range keys {
			if err := Set(k, v); err != nil {
				t.Fatalf("Set(%q, %v) error = %v", k, v, err)
			}
		}

		// Verify all keys exist
		state, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		for k, want := range keys {
			got, ok := state[k]
			if !ok {
				t.Errorf("state[%q] not found", k)
				continue
			}
			if got != want {
				t.Errorf("state[%q] = %v, want %v", k, got, want)
			}
		}
	})

	t.Run("State file location", func(t *testing.T) {
		// Set a value to ensure state file is created
		if err := Set("test.location", "value"); err != nil {
			t.Fatalf("Set() error = %v", err)
		}

		// Check that state file exists in .grove/state.yml
		statePath := filepath.Join(tmpDir, ".grove", "state.yml")
		if _, err := os.Stat(statePath); os.IsNotExist(err) {
			t.Errorf("state file not found at %s", statePath)
		}
	})
}
