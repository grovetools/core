package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	
	"github.com/mattsolo1/grove-core/docker"
)

const (
	// CustomAgentDockerfile is the name of the Dockerfile in XDG config
	CustomAgentDockerfile = "agent.Dockerfile"
	// CustomAgentImageName is the tag for the custom built image
	CustomAgentImageName = "grove-agent-custom:latest"
)

// GetCustomAgentDockerfilePath returns the path to the custom agent Dockerfile
func GetCustomAgentDockerfilePath() string {
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		return filepath.Join(xdgConfig, "grove", CustomAgentDockerfile)
	}
	
	if homeDir, err := os.UserHomeDir(); err == nil {
		return filepath.Join(homeDir, ".config", "grove", CustomAgentDockerfile)
	}
	
	return ""
}

// HasCustomAgentDockerfile checks if a custom agent Dockerfile exists
func HasCustomAgentDockerfile() bool {
	dockerfilePath := GetCustomAgentDockerfilePath()
	if dockerfilePath == "" {
		return false
	}
	
	info, err := os.Stat(dockerfilePath)
	return err == nil && !info.IsDir()
}

// BuildCustomAgentImage builds the custom agent image if Dockerfile exists
func BuildCustomAgentImage(ctx context.Context, dockerClient docker.Client) error {
	dockerfilePath := GetCustomAgentDockerfilePath()
	if dockerfilePath == "" {
		return fmt.Errorf("cannot determine XDG config directory")
	}
	
	if !HasCustomAgentDockerfile() {
		return fmt.Errorf("custom agent Dockerfile not found at %s", dockerfilePath)
	}
	
	fmt.Printf("🔨 Building custom agent image from %s...\n", dockerfilePath)
	
	// Find the monorepo root to locate the binary
	repoRoot, err := findGroveEcosystemRoot()
	if err != nil {
		return fmt.Errorf("could not find grove monorepo root to locate canopy binary: %w", err)
	}
	
	// Define the source and destination paths
	canopyBinaryName := "canopy-aarch64" // Target the aarch64 binary
	sourceBinaryPath := filepath.Join(repoRoot, "grove-canopy", "bin", canopyBinaryName)
	buildContextPath := filepath.Dir(dockerfilePath)
	destBinaryPath := filepath.Join(buildContextPath, "canopy") // Destination inside the build context
	
	// Copy the binary into the build context
	fmt.Printf("➡️  Copying canopy binary to build context...\n")
	binaryBytes, err := os.ReadFile(sourceBinaryPath)
	if err != nil {
		return fmt.Errorf("failed to read canopy binary at '%s'. Please ensure it's built (e.g., 'make build-canopy'): %w", sourceBinaryPath, err)
	}
	
	if err := os.WriteFile(destBinaryPath, binaryBytes, 0755); err != nil {
		return fmt.Errorf("failed to copy canopy binary to build context: %w", err)
	}
	
	// Defer cleanup of the copied binary
	defer func() {
		fmt.Printf("⬅️  Cleaning up temporary canopy binary from build context...\n")
		os.Remove(destBinaryPath)
	}()
	
	// Build the custom image using the Docker client
	if err := dockerClient.BuildImage(ctx, buildContextPath, dockerfilePath, CustomAgentImageName); err != nil {
		return fmt.Errorf("failed to build custom agent image: %w", err)
	}
	
	fmt.Printf("✅ Successfully built custom agent image: %s\n", CustomAgentImageName)
	return nil
}

// GetAgentImage returns the appropriate agent image to use
func GetAgentImage(configuredImage string) string {
	// If a custom Dockerfile exists, use the custom image
	if HasCustomAgentDockerfile() {
		return CustomAgentImageName
	}
	
	// Otherwise use the configured image
	if configuredImage != "" {
		return configuredImage
	}
	
	// Default to the standard grove agent image
	return BaseAgentImageName
}

// NeedsCustomImageBuild checks if the custom image needs to be built
func NeedsCustomImageBuild(ctx context.Context, dockerClient docker.Client) (bool, error) {
	if !HasCustomAgentDockerfile() {
		return false, nil
	}
	
	// Check if the custom image already exists
	exists, err := dockerClient.ImageExists(ctx, CustomAgentImageName)
	if err != nil {
		return false, fmt.Errorf("failed to check for existing image: %w", err)
	}
	
	// If image doesn't exist, it needs to be built
	return !exists, nil
}

// CreateExampleAgentDockerfile creates an example agent Dockerfile
func CreateExampleAgentDockerfile() (string, error) {
	dockerfilePath := GetCustomAgentDockerfilePath()
	if dockerfilePath == "" {
		return "", fmt.Errorf("cannot determine XDG config directory")
	}
	
	// Ensure the directory exists
	if err := os.MkdirAll(filepath.Dir(dockerfilePath), 0755); err != nil {
		return "", fmt.Errorf("failed to create config directory: %w", err)
	}
	
	exampleContent := `# Custom Grove Agent Image
# This Dockerfile extends the base Grove agent with your customizations

FROM grove-claude-agent:latest

# Switch to root to install packages
USER root

# Common development tools (Alpine-based)
# These essential tools are included by default:
RUN apk add --no-cache \
    git \
    vim \
    curl \
    jq \
    tree \
    ripgrep

# Additional tools - uncomment the ones you need:

# Text editors and terminal tools
# RUN apk add --no-cache \
#     nano \
#     tmux \
#     screen \
#     htop \
#     ncdu

# More development tools
# RUN apk add --no-cache \
#     wget \
#     yq \
#     fd \
#     fzf \
#     bat \
#     eza \
#     zsh \
#     fish

# Build tools and compilers
# RUN apk add --no-cache \
#     make \
#     gcc \
#     g++ \
#     musl-dev \
#     linux-headers

# Database clients
# RUN apk add --no-cache \
#     postgresql-client \
#     mysql-client \
#     redis \
#     sqlite

# Network and debugging tools
# RUN apk add --no-cache \
#     openssh-client \
#     netcat-openbsd \
#     bind-tools \
#     iputils \
#     tcpdump \
#     strace

# Python development
# RUN apk add --no-cache python3-dev py3-pip && \
#     pip3 install --no-cache-dir \
#     ipython \
#     black \
#     ruff \
#     mypy \
#     pytest \
#     requests \
#     pandas \
#     numpy

# Node.js development (if not already included)
# RUN apk add --no-cache nodejs npm && \
#     npm install -g \
#     typescript \
#     @types/node \
#     eslint \
#     prettier \
#     nodemon \
#     pnpm \
#     yarn

# Go development
# RUN apk add --no-cache go gopls

# Rust development
# RUN apk add --no-cache cargo rust rust-analyzer

# Container tools
# RUN apk add --no-cache \
#     docker-cli \
#     docker-cli-compose \
#     dive

# Cloud CLI tools
# RUN apk add --no-cache \
#     aws-cli \
#     azure-cli \
#     google-cloud-sdk

# Example: Custom shell configuration
# COPY .bashrc /root/.bashrc
# COPY .zshrc /root/.zshrc
# COPY .vimrc /root/.vimrc
# COPY .tmux.conf /root/.tmux.conf

# Add custom binaries
# If you have pre-compiled binaries, you can copy them into the image.
# Example for adding the 'canopy' CLI:
#
# USER root
# COPY canopy /usr/local/bin/canopy
# RUN chmod +x /usr/local/bin/canopy

# Switch back to the claude user (required for --dangerously-skip-permissions)
USER claude

# The base image already sets the correct WORKDIR and ENTRYPOINT
`
	
	if err := os.WriteFile(dockerfilePath, []byte(exampleContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write Dockerfile: %w", err)
	}
	
	return dockerfilePath, nil
}

// findGroveEcosystemRoot attempts to find the Grove ecosystem repository root directory
func findGroveEcosystemRoot() (string, error) {
	// Start from current directory and walk up
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	
	for {
		// Check if this is the Grove ecosystem root (has grove-canopy, grove-core, etc. as subdirectories)
		if _, err := os.Stat(filepath.Join(dir, "grove-canopy")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "grove-core")); err == nil {
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
	
	// If not found from current directory, check the specific known location
	knownPath := "/Users/solom4/Code/grove-ecosystem"
	if _, err := os.Stat(filepath.Join(knownPath, "grove-canopy")); err == nil {
		if _, err := os.Stat(filepath.Join(knownPath, "grove-core")); err == nil {
			return knownPath, nil
		}
	}
	
	return "", fmt.Errorf("Grove ecosystem repository root not found")
}