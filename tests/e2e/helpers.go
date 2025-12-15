package main

import (
	"fmt"
)

// findCoreBinary finds the grove-core binary under test by using grove.yml.
func findCoreBinary() (string, error) {
	path, err := FindProjectBinary()
	if err != nil {
		return "", fmt.Errorf("could not find 'core' binary via grove.yml: %w", err)
	}
	return path, nil
}
