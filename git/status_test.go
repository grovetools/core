package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGetStatus(t *testing.T) {
	// Skip if not in a git repository
	cwd, err := os.Getwd()
	if err != nil {
		t.Skip("Could not get current directory")
	}
	
	if !IsGitRepo(cwd) {
		t.Skip("Not in a git repository")
	}
	
	// Test getting status of current directory
	status, err := GetStatus(cwd)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	
	// Verify we got basic information
	if status.Branch == "" {
		t.Error("Expected branch name to be non-empty")
	}
	
	// Log the status for debugging
	t.Logf("Git Status for %s:", cwd)
	t.Logf("  Branch: %s", status.Branch)
	t.Logf("  Has Upstream: %v", status.HasUpstream)
	if status.HasUpstream {
		t.Logf("  Ahead: %d, Behind: %d", status.AheadCount, status.BehindCount)
	}
	t.Logf("  Modified: %d, Untracked: %d, Staged: %d", 
		status.ModifiedCount, status.UntrackedCount, status.StagedCount)
	t.Logf("  Is Dirty: %v", status.IsDirty)
}

func TestGetStatusInvalidPath(t *testing.T) {
	// Test with non-existent directory
	_, err := GetStatus("/non/existent/path")
	if err == nil {
		t.Error("Expected error for non-existent path")
	}
	
	// Test with non-git directory
	tempDir := t.TempDir()
	_, err = GetStatus(tempDir)
	if err == nil {
		t.Error("Expected error for non-git directory")
	}
}

func TestGetStatusCleanRepo(t *testing.T) {
	// Create a temporary git repo
	tempDir := t.TempDir()
	
	// Initialize git repo
	gitInit := exec.Command("git", "init")
	gitInit.Dir = tempDir
	if err := gitInit.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}
	
	// Configure git user for the test repo
	gitConfig := exec.Command("git", "config", "user.email", "test@example.com")
	gitConfig.Dir = tempDir
	gitConfig.Run()
	
	gitConfig = exec.Command("git", "config", "user.name", "Test User")
	gitConfig.Dir = tempDir
	gitConfig.Run()
	
	// Create and commit a file
	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	
	gitAdd := exec.Command("git", "add", "test.txt")
	gitAdd.Dir = tempDir
	if err := gitAdd.Run(); err != nil {
		t.Fatalf("Failed to add file: %v", err)
	}
	
	gitCommit := exec.Command("git", "commit", "-m", "Initial commit")
	gitCommit.Dir = tempDir
	if err := gitCommit.Run(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}
	
	// Get status of clean repo
	status, err := GetStatus(tempDir)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}
	
	// Verify clean repo status
	if status.IsDirty {
		t.Error("Expected clean repo to not be dirty")
	}
	if status.ModifiedCount != 0 {
		t.Errorf("Expected 0 modified files, got %d", status.ModifiedCount)
	}
	if status.UntrackedCount != 0 {
		t.Errorf("Expected 0 untracked files, got %d", status.UntrackedCount)
	}
	if status.StagedCount != 0 {
		t.Errorf("Expected 0 staged files, got %d", status.StagedCount)
	}
}