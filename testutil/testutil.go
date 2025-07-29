package testutil

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// RequireDocker skips the test if Docker is not available
func RequireDocker(t *testing.T) {
	t.Helper()

	cmd := exec.Command("docker", "version")
	if err := cmd.Run(); err != nil {
		t.Skip("Docker not available")
	}

	cmd = exec.Command("docker-compose", "version")
	if err := cmd.Run(); err != nil {
		t.Skip("Docker Compose not available")
	}
}

// InitGitRepo initializes a git repository in the given directory
func InitGitRepo(t *testing.T, dir string) {
	t.Helper()

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Configure git user
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to configure git user.name: %v", err)
	}

	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to configure git user.email: %v", err)
	}

	// Create initial commit
	testFile := filepath.Join(dir, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test Project\n"), 0600); err != nil {
		t.Fatalf("Failed to create README: %v", err)
	}

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git add: %v", err)
	}

	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to git commit: %v", err)
	}

	// Ensure we have a main branch (rename from master if needed)
	cmd = exec.Command("git", "branch", "-m", "main")
	cmd.Dir = dir
	_ = cmd.Run() // Ignore error as branch might already be named main
}

// CreateBranch creates and checks out a new git branch
func CreateBranch(t *testing.T, dir, branch string) {
	t.Helper()

	cmd := exec.Command("git", "checkout", "-b", branch)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create branch %s: %v", branch, err)
	}
}

// RandomString generates a random string of the specified length
func RandomString(length int) string {
	bytes := make([]byte, length/2+1)
	if _, err := rand.Read(bytes); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes)[:length]
}

// RunGitCommand runs a git command in the given directory
func RunGitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to run git %v: %v", args, err)
	}
}

// CreateCommit creates a file and commits it
func CreateCommit(t *testing.T, dir, filename, content string) {
	t.Helper()

	filePath := filepath.Join(dir, filename)
	if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create file %s: %v", filename, err)
	}

	RunGitCommand(t, dir, "add", filename)
	RunGitCommand(t, dir, "commit", "-m", "Add "+filename)
}

// SetupTestNetwork creates a temporary Docker network for a test and ensures it's cleaned up.
func SetupTestNetwork(t *testing.T, networkName string) {
	t.Helper()
	cmd := exec.Command("docker", "network", "create", networkName)
	err := cmd.Run()
	require.NoError(t, err, "failed to create test network %s", networkName)

	t.Cleanup(func() {
		// Use --force to ignore errors if the network is already gone
		_ = exec.Command("docker", "network", "rm", "-f", networkName).Run()
	})
}

// CreateTestAgentYAML creates a minimal grove.yml with agent enabled
func CreateTestAgentYAML(projectName string) string {
	return `version: "1.0"
services:
  test:
    image: alpine:latest
agent:
  enabled: true
  image: alpine:latest
  logs_path: ~/.claude/projects
settings:
  project_name: ` + projectName + `
  network_name: ` + projectName + `-net
  domain_suffix: localhost
  traefik_enabled: false
`
}

// WaitForAgentContainer waits for agent container to be running using docker commands
func WaitForAgentContainer(t *testing.T, projectName string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	containerName := projectName + "-grove-agent"

	for time.Now().Before(deadline) {
		cmd := exec.Command("docker", "inspect", "--format={{.State.Status}}", containerName)
		output, err := cmd.Output()
		if err == nil && string(output) == "running\n" {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}

	t.Fatalf("Agent container %s did not start within %v", containerName, timeout)
}

// CleanupAgentContainer removes agent container and cleanup
func CleanupAgentContainer(t *testing.T, projectName string) {
	t.Helper()
	containerName := projectName + "-grove-agent"

	// Stop and remove the container
	_ = exec.Command("docker", "stop", containerName).Run()
	_ = exec.Command("docker", "rm", "-f", containerName).Run()
}

// SetupAgentTest creates a complete agent test environment
func SetupAgentTest(t *testing.T) (string, string, func()) {
	t.Helper()

	// Require Docker
	RequireDocker(t)

	// Create temporary directory
	tmpDir := t.TempDir()
	InitGitRepo(t, tmpDir)

	// Create unique project name
	projectName := "grove-test-agent-" + RandomString(8)

	// Setup network
	SetupTestNetwork(t, projectName+"-net")

	// Create grove.yml with agent
	groveYAML := CreateTestAgentYAML(projectName)
	groveFile := filepath.Join(tmpDir, "grove.yml")
	require.NoError(t, os.WriteFile(groveFile, []byte(groveYAML), 0600))

	// Cleanup function
	cleanup := func() {
		CleanupAgentContainer(t, projectName)
	}

	return tmpDir, projectName, cleanup
}

// AssertAgentRunning checks if the agent is running
func AssertAgentRunning(t *testing.T, projectName string) {
	t.Helper()
	containerName := projectName + "-grove-agent"

	cmd := exec.Command("docker", "inspect", "--format={{.State.Status}}", containerName)
	output, err := cmd.Output()
	require.NoError(t, err, "Failed to inspect agent container")
	require.Equal(t, "running\n", string(output), "Agent container should be running")
}

// AssertAgentNotRunning checks if the agent is not running
func AssertAgentNotRunning(t *testing.T, projectName string) {
	t.Helper()
	containerName := projectName + "-grove-agent"

	cmd := exec.Command("docker", "inspect", "--format={{.State.Status}}", containerName)
	output, err := cmd.Output()
	if err == nil {
		// Container exists, check if it's stopped
		require.NotEqual(t, "running\n", string(output), "Agent container should not be running")
	}
	// If error occurs, container likely doesn't exist, which is fine
}
