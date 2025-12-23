package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// findCoreBinary locates the core binary in the bin directory.
// This uses a relative path approach that works regardless of the current directory.
func findCoreBinary() (string, error) {
	// The binary should be at ../../bin/core relative to the test directory
	binPath := filepath.Join("..", "..", "bin", "core")
	absPath, err := filepath.Abs(binPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	if _, err := os.Stat(absPath); err != nil {
		return "", fmt.Errorf("core binary not found at %s: %w", absPath, err)
	}

	return absPath, nil
}
