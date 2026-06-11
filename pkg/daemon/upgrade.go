// Package daemon provides upgrade functionality for zero-downtime daemon updates.
// PHASE 2: Contains the UpgradeRunning function that signals and replaces the daemon.
package daemon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// UpgradeRunning signals the running daemon to enter drain mode, waits for the socket
// to be unlinked, and starts a new daemon binary.
// PHASE 2: Implements zero-downtime upgrade by signaling SIGUSR1 to the old daemon,
// which unlinks the socket and continues serving in-flight requests. The new daemon
// then binds to the freed socket immediately.
func UpgradeRunning(ctx context.Context, scope string) error {
	// Find the running daemon's PID
	pidFilePath := getPidfilePath(scope)
	pidData, err := os.ReadFile(pidFilePath)
	if err != nil {
		return fmt.Errorf("failed to read pidfile: %w", err)
	}

	var oldPID int
	if _, err := fmt.Sscanf(string(pidData), "%d", &oldPID); err != nil {
		return fmt.Errorf("failed to parse PID from pidfile: %w", err)
	}

	// Signal the old daemon to enter drain mode
	if err := syscall.Kill(oldPID, syscall.SIGUSR1); err != nil {
		return fmt.Errorf("failed to signal daemon PID %d: %w", oldPID, err)
	}

	fmt.Printf("Sent SIGUSR1 to daemon (PID %d) - entering drain mode\n", oldPID)

	// Wait for the socket to be unlinked (indicating drain mode is active)
	socketPath := getSocketPath(scope)
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	for {
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("timeout waiting for socket to be unlinked")
		case <-time.After(100 * time.Millisecond):
			if _, err := os.Stat(socketPath); os.IsNotExist(err) {
				fmt.Println("Socket unlinked - old daemon in drain mode")
				goto socketUnlinked
			}
		}
	}

socketUnlinked:
	// Start the new daemon in the background
	newDaemon := exec.Command("groved", "start", "--socket", socketPath, "--pidfile", pidFilePath)
	if scope != "" {
		newDaemon.Args = append(newDaemon.Args, "--scope", scope)
	}

	if err := newDaemon.Start(); err != nil {
		return fmt.Errorf("failed to start new daemon: %w", err)
	}

	fmt.Printf("Started new daemon (PID %d)\n", newDaemon.Process.Pid)
	return nil
}

// getPidfilePath returns the path to the daemon's pidfile based on scope.
func getPidfilePath(scope string) string {
	// This mirrors the logic in core/pkg/paths package
	if scope == "" {
		return os.ExpandEnv("$HOME/.grovetools/groved.pid")
	}
	return os.ExpandEnv(fmt.Sprintf("$HOME/.grovetools/groved-%s.pid", scope))
}

// getSocketPath returns the path to the daemon's socket based on scope.
func getSocketPath(scope string) string {
	// This mirrors the logic in core/pkg/paths package
	if scope == "" {
		return os.ExpandEnv("$HOME/.grovetools/groved.sock")
	}
	return os.ExpandEnv(fmt.Sprintf("$HOME/.grovetools/groved-%s.sock", scope))
}
