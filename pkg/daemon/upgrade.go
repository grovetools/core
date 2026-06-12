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

	"github.com/grovetools/core/pkg/paths"
)

// UpgradeRunning signals the running daemon to enter drain mode, waits for the socket
// to be unlinked, and starts a new daemon binary.
// PHASE 2: Implements zero-downtime upgrade by signaling SIGUSR1 to the old daemon,
// which unlinks the socket and continues serving in-flight requests. The new daemon
// then binds to the freed socket and adopts running detached agents by PID.
func UpgradeRunning(ctx context.Context, scope string) error {
	// Find the running daemon's PID
	pidFilePath := paths.PidFilePath(scope)
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
	socketPath := paths.SocketPath(scope)
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
	// The binary running this command is the one we're upgrading to; fall back
	// to PATH lookup if the executable path can't be resolved.
	newBinary, err := os.Executable()
	if err != nil {
		newBinary = "groved"
	}

	args := []string{"start", "--socket", socketPath, "--pidfile", pidFilePath}
	if scope != "" {
		args = append(args, "--scope", scope)
	}

	// Detach into its own session so the new daemon survives this terminal's
	// exit. Without Setsid it shares the terminal's process group and receives
	// SIGHUP when the terminal closes, which triggers ptyManager.Shutdown()
	// and kills every agent PTY the daemon owns (see factory.go).
	newDaemon := exec.Command(newBinary, args...)
	newDaemon.Stdout = nil
	newDaemon.Stderr = nil
	newDaemon.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := newDaemon.Start(); err != nil {
		return fmt.Errorf("failed to start new daemon: %w", err)
	}

	fmt.Printf("Started new daemon (PID %d)\n", newDaemon.Process.Pid)

	// Confirm the new daemon bound the socket before declaring success.
	bindCtx, cancelBind := context.WithTimeout(ctx, 10*time.Second)
	defer cancelBind()
	for {
		select {
		case <-bindCtx.Done():
			return fmt.Errorf("new daemon (PID %d) did not bind %s in time", newDaemon.Process.Pid, socketPath)
		case <-time.After(100 * time.Millisecond):
			if _, err := os.Stat(socketPath); err == nil {
				fmt.Printf("New daemon bound %s - upgrade complete\n", socketPath)
				return nil
			}
		}
	}
}
