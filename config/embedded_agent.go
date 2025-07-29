package config

import (
	_ "embed"
	"context"
	"fmt"
	"os"
	"path/filepath"
	
	"github.com/mattsolo1/grove-core/docker"
)

// Embed the base agent Dockerfile and entrypoint script
//go:embed agent_dockerfile.txt
var embeddedAgentDockerfile string

//go:embed agent_entrypoint.txt
var embeddedAgentEntrypoint string

// EnsureBaseAgentImageEmbedded checks if the base Grove agent image exists and builds it if needed
// This version uses embedded files for binary distribution
func EnsureBaseAgentImageEmbedded(ctx context.Context, dockerClient docker.Client) error {
	// Check if the base image exists
	exists, err := dockerClient.ImageExists(ctx, BaseAgentImageName)
	if err != nil {
		return fmt.Errorf("failed to check for base agent image: %w", err)
	}
	
	// If image exists, we're done
	if exists {
		return nil
	}
	
	// Create a temporary directory for the build
	tmpDir, err := os.MkdirTemp("", "grove-agent-build-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	
	// Write the embedded Dockerfile
	dockerfilePath := filepath.Join(tmpDir, "Dockerfile")
	if err := os.WriteFile(dockerfilePath, []byte(embeddedAgentDockerfile), 0644); err != nil {
		return fmt.Errorf("failed to write Dockerfile: %w", err)
	}
	
	// Write the embedded entrypoint script
	entrypointPath := filepath.Join(tmpDir, "entrypoint.sh")
	if err := os.WriteFile(entrypointPath, []byte(embeddedAgentEntrypoint), 0755); err != nil {
		return fmt.Errorf("failed to write entrypoint.sh: %w", err)
	}
	
	// Build the base image
	fmt.Println("🔨 Building Grove base agent image...")
	if err := dockerClient.BuildImage(ctx, tmpDir, dockerfilePath, BaseAgentImageName); err != nil {
		return fmt.Errorf("failed to build base agent image: %w", err)
	}
	
	fmt.Println("✅ Successfully built base agent image")
	return nil
}

// WriteBaseAgentDockerfile writes the embedded base agent Dockerfile to a specified path
// This is useful for users who want to inspect or modify the base Dockerfile
func WriteBaseAgentDockerfile(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	
	// Write Dockerfile
	if err := os.WriteFile(path, []byte(embeddedAgentDockerfile), 0644); err != nil {
		return fmt.Errorf("failed to write Dockerfile: %w", err)
	}
	
	// Write entrypoint.sh in the same directory
	entrypointPath := filepath.Join(dir, "entrypoint.sh")
	if err := os.WriteFile(entrypointPath, []byte(embeddedAgentEntrypoint), 0755); err != nil {
		return fmt.Errorf("failed to write entrypoint.sh: %w", err)
	}
	
	return nil
}