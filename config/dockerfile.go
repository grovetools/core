package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DockerfileParser extracts information from Dockerfiles
type DockerfileParser struct {
	workdirRegex *regexp.Regexp
}

// NewDockerfileParser creates a new Dockerfile parser
func NewDockerfileParser() *DockerfileParser {
	return &DockerfileParser{
		workdirRegex: regexp.MustCompile(`(?i)^WORKDIR\s+(.+)$`),
	}
}

// FindWorkdir finds the WORKDIR in a Dockerfile
func (p *DockerfileParser) FindWorkdir(dockerfilePath string) (string, error) {
	file, err := os.Open(dockerfilePath)
	if err != nil {
		return "", fmt.Errorf("failed to open Dockerfile: %w", err)
	}
	defer file.Close()

	var lastWorkdir string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for WORKDIR instruction
		if matches := p.workdirRegex.FindStringSubmatch(line); len(matches) > 1 {
			workdir := strings.TrimSpace(matches[1])
			// Remove quotes if present
			workdir = strings.Trim(workdir, `"'`)
			lastWorkdir = workdir
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading Dockerfile: %w", err)
	}

	return lastWorkdir, nil
}

// FindDockerfile locates the Dockerfile for a service
func (p *DockerfileParser) FindDockerfile(servicePath string, buildContext map[string]interface{}) (string, error) {
	// Check if Dockerfile is specified in build context
	if buildContext != nil {
		if dockerfile, ok := buildContext["dockerfile"].(string); ok {
			return filepath.Join(servicePath, dockerfile), nil
		}
	}

	// Try common Dockerfile names
	dockerfiles := []string{"Dockerfile", "dockerfile", "Dockerfile.dev", "Dockerfile.development"}
	for _, name := range dockerfiles {
		path := filepath.Join(servicePath, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("no Dockerfile found in %s", servicePath)
}