package main

import (
	"fmt"
	"os/exec"
)

// findCoreBinary finds the grove-core binary under test.
// It relies on the Makefile setting the PATH to include the local ./bin directory.
func findCoreBinary() (string, error) {
	path, err := exec.LookPath("core")
	if err != nil {
		return "", fmt.Errorf("could not find 'core' binary in PATH. Ensure 'make test-e2e' is used")
	}
	return path, nil
}
