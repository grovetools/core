package sessions

import (
	"os"
	"testing"
)

func TestFileSystemRegistryIsAlive(t *testing.T) {
	registry := &FileSystemRegistry{baseDir: t.TempDir()}

	t.Run("live process", func(t *testing.T) {
		meta := SessionMetadata{
			SessionID: "live-session",
			PID:       os.Getpid(),
		}
		if err := registry.Register(meta); err != nil {
			t.Fatalf("Register failed: %v", err)
		}

		alive, err := registry.IsAlive("live-session")
		if err != nil {
			t.Fatalf("IsAlive returned error: %v", err)
		}
		if !alive {
			t.Errorf("IsAlive = false for the current process (pid %d), want true", os.Getpid())
		}
	})

	t.Run("dead process", func(t *testing.T) {
		meta := SessionMetadata{
			SessionID: "dead-session",
			PID:       99999999, // absurd PID, cannot exist
		}
		if err := registry.Register(meta); err != nil {
			t.Fatalf("Register failed: %v", err)
		}

		alive, err := registry.IsAlive("dead-session")
		if err != nil {
			t.Fatalf("IsAlive returned error: %v", err)
		}
		if alive {
			t.Error("IsAlive = true for an absurd PID, want false")
		}
	})

	t.Run("missing session", func(t *testing.T) {
		alive, err := registry.IsAlive("no-such-session")
		if err != nil {
			t.Fatalf("IsAlive returned error: %v", err)
		}
		if alive {
			t.Error("IsAlive = true for a missing session, want false")
		}
	})
}
