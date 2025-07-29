package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	
	"github.com/mattsolo1/grove-core/docker"
)

const (
	// BaseAgentImageName is the name of the base Grove agent image
	BaseAgentImageName = "grove-claude-agent:latest"
	// BaseAgentDockerfilePath is the path to the base agent Dockerfile relative to repo root
	BaseAgentDockerfilePath = "images/agent"
)

// EnsureBaseAgentImage checks if the base Grove agent image exists and builds it if needed
func EnsureBaseAgentImage(ctx context.Context, dockerClient docker.Client) error {
	// Check if the base image exists
	exists, err := dockerClient.ImageExists(ctx, BaseAgentImageName)
	if err != nil {
		return fmt.Errorf("failed to check for base agent image: %w", err)
	}
	
	// If image exists, we're done
	if exists {
		return nil
	}
	
	// Try to find the Grove repository root first (for development)
	repoRoot, err := findGroveRepoRoot()
	if err == nil {
		// Check if the base Dockerfile exists in the repo
		dockerfilePath := filepath.Join(repoRoot, BaseAgentDockerfilePath, "Dockerfile")
		if _, err := os.Stat(dockerfilePath); err == nil {
			// Build from source files
			fmt.Println("🔨 Building Grove base agent image from source...")
			buildContextPath := filepath.Join(repoRoot, BaseAgentDockerfilePath)
			
			if err := dockerClient.BuildImage(ctx, buildContextPath, dockerfilePath, BaseAgentImageName); err != nil {
				return fmt.Errorf("failed to build base agent image: %w", err)
			}
			
			fmt.Println("✅ Successfully built base agent image")
			return nil
		}
	}
	
	// Fall back to embedded files (for binary distribution)
	return EnsureBaseAgentImageEmbedded(ctx, dockerClient)
}

// findGroveRepoRoot attempts to find the Grove repository root directory
func findGroveRepoRoot() (string, error) {
	// Start from current directory and walk up
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	
	for {
		// Check if this is the Grove repo root (has go.mod with module github.com/grove-cloud/grove)
		goModPath := filepath.Join(dir, "go.mod")
		if data, err := os.ReadFile(goModPath); err == nil {
			if strings.Contains(string(data), "module github.com/grove-cloud/grove") {
				return dir, nil
			}
		}
		
		// Check if we've reached the root
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	
	// If not found from current directory, check common locations
	homeDir, _ := os.UserHomeDir()
	commonPaths := []string{
		filepath.Join(homeDir, "Code", "grove"),
		filepath.Join(homeDir, "code", "grove"),
		filepath.Join(homeDir, "src", "grove"),
		filepath.Join(homeDir, "projects", "grove"),
		"/Users/solom4/Code/grove", // Your specific path as fallback
	}
	
	for _, path := range commonPaths {
		goModPath := filepath.Join(path, "go.mod")
		if data, err := os.ReadFile(goModPath); err == nil {
			if strings.Contains(string(data), "module github.com/grove-cloud/grove") {
				return path, nil
			}
		}
	}
	
	return "", fmt.Errorf("Grove repository root not found")
}