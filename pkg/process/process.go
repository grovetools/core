package process

import (
	"os"
	"syscall"
)

// IsProcessAlive checks if a process with the given PID is still running.
// It uses a signal-sending method that is cross-platform for Unix-like systems (macOS, Linux).
func IsProcessAlive(pid int) bool {
	// PID 0 or less is invalid.
	if pid <= 0 {
		return false
	}

	// Find the process. This doesn't fail on Unix if the process doesn't exist.
	process, err := os.FindProcess(pid)
	if err != nil {
		return false // Should not happen on Unix-like systems.
	}

	// On Unix, sending signal 0 to a process checks for its existence without actually sending a signal.
	// If the process exists and we have permission, err will be nil.
	// If the process exists but we don't have permission, err will be EPERM, but it's still alive.
	// If the process does not exist, err will be ESRCH.
	err = process.Signal(syscall.Signal(0))

	// err == nil means process is alive and we have permission.
	// os.IsPermission(err) means process is alive but we don't have permission (e.g., owned by root).
	return err == nil || os.IsPermission(err)
}
