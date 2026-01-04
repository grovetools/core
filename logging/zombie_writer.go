package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// zombieAwareWriter is an io.WriteCloser that validates the log path before each write.
// If it detects that the target worktree has been deleted, it redirects logs to the main project root.
type zombieAwareWriter struct {
	mu             sync.Mutex
	originalPath   string
	redirectedPath string
	writer         io.WriteCloser
}

// newZombieAwareWriter creates a new writer for the given log file path.
func newZombieAwareWriter(logFilePath string) *zombieAwareWriter {
	return &zombieAwareWriter{
		originalPath: logFilePath,
	}
}

// Write implements the io.Writer interface.
func (w *zombieAwareWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	writer, err := w.getWriter()
	if err != nil {
		// Log to stderr as a last resort
		fmt.Fprintf(os.Stderr, "zombieAwareWriter: failed to get writer: %v\n", err)
		return 0, err
	}

	return writer.Write(p)
}

// Close implements the io.Closer interface.
func (w *zombieAwareWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.writer != nil {
		err := w.writer.Close()
		w.writer = nil
		return err
	}
	return nil
}

// getWriter is the core logic. It checks for zombie worktrees and returns a valid writer.
func (w *zombieAwareWriter) getWriter() (io.WriteCloser, error) {
	currentPath := w.originalPath
	if w.redirectedPath != "" {
		currentPath = w.redirectedPath
	}

	// Check if the path points to a zombie worktree.
	if strings.Contains(currentPath, ".grove-worktrees") {
		// Extract the worktree root from the log path.
		// e.g., /path/to/repo/.grove-worktrees/my-feature/.grove/logs/file.log
		parts := strings.Split(currentPath, ".grove-worktrees")
		if len(parts) > 1 {
			gitRoot := parts[0]
			remaining := parts[1]
			worktreeName := strings.Split(strings.TrimPrefix(remaining, "/"), "/")[0]
			worktreeRoot := filepath.Join(gitRoot, ".grove-worktrees", worktreeName)

			// A valid worktree must have a .git FILE. If it's missing, the worktree was deleted.
			if _, err := os.Stat(filepath.Join(worktreeRoot, ".git")); os.IsNotExist(err) {
				// Zombie detected! Redirect logs to the main project root.
				// The new path should be in the main git root, not the deleted worktree.
				newLogDir := filepath.Join(gitRoot, ".grove", "logs")
				dateStr := time.Now().Format("2006-01-02")
				newLogPath := filepath.Join(newLogDir, fmt.Sprintf("workspace-%s.log", dateStr))

				// If we haven't already redirected, log a warning and update the path.
				if w.redirectedPath == "" {
					fmt.Fprintf(os.Stderr, "grove-log: Stale worktree path detected: %s. Redirecting logs to: %s\n", currentPath, newLogPath)
					w.redirectedPath = newLogPath
					if w.writer != nil {
						w.writer.Close()
						w.writer = nil
					}
				}
				currentPath = newLogPath
			}
		}
	}

	// If writer is nil (first write, or after redirection), open the file.
	if w.writer == nil {
		if err := os.MkdirAll(filepath.Dir(currentPath), 0755); err != nil {
			return nil, fmt.Errorf("creating log directory: %w", err)
		}
		file, err := os.OpenFile(currentPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			return nil, fmt.Errorf("opening log file: %w", err)
		}
		w.writer = file
	}

	return w.writer, nil
}
