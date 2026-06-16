package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to initialize a git repository
func initGitRepo(t *testing.T, dir string) {
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	err := cmd.Run()
	require.NoError(t, err, "Failed to initialize git repo")

	// Configure git user for commits
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = dir
	err = cmd.Run()
	require.NoError(t, err, "Failed to configure git user.email")

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = dir
	err = cmd.Run()
	require.NoError(t, err, "Failed to configure git user.name")
}

// Helper function to add and commit files
func commitFiles(t *testing.T, dir, message string) {
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = dir
	err := cmd.Run()
	require.NoError(t, err, "Failed to add files")

	cmd = exec.Command("git", "commit", "-m", message)
	cmd.Dir = dir
	err = cmd.Run()
	require.NoError(t, err, "Failed to commit files")
}

// Helper function to create a git worktree
func createWorktree(t *testing.T, repoDir, worktreePath, branchName string) {
	cmd := exec.Command("git", "worktree", "add", "-b", branchName, worktreePath)
	cmd.Dir = repoDir
	err := cmd.Run()
	require.NoError(t, err, "Failed to create worktree")
}

// Helper function to create a mock Provider with workspaces
func createMockProvider(workspaces map[string]string) *Provider {
	var projects []Project
	for name, path := range workspaces {
		projects = append(projects, Project{
			Name: name,
			Path: path,
			Workspaces: []DiscoveredWorkspace{
				{
					Name:              "main",
					Path:              path,
					Type:              WorkspaceTypePrimary,
					ParentProjectPath: path,
				},
			},
		})
	}

	result := &DiscoveryResult{
		Projects: projects,
	}
	return NewProvider(result)
}

func TestSetupSubmodules(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}

	ctx := context.Background()

	t.Run("setup all submodules", func(t *testing.T) {
		// Create temporary directories
		tempDir := t.TempDir()
		superprojectDir := filepath.Join(tempDir, "superproject")
		localSubmoduleDir := filepath.Join(tempDir, "local-sub")

		// Initialize repositories
		require.NoError(t, os.MkdirAll(superprojectDir, 0o755))
		initGitRepo(t, superprojectDir)

		require.NoError(t, os.MkdirAll(localSubmoduleDir, 0o755))
		initGitRepo(t, localSubmoduleDir)

		// Create content in local submodule
		readmePath := filepath.Join(localSubmoduleDir, "README.md")
		err := os.WriteFile(readmePath, []byte("local submodule"), 0o644)
		require.NoError(t, err)
		commitFiles(t, localSubmoduleDir, "initial commit")

		// Create .gitmodules in superproject
		gitmodulesContent := `[submodule "local-sub"]
	path = local-sub
	url = ../local-sub
[submodule "remote-sub"]
	path = remote-sub
	url = https://github.com/example/remote-sub.git
`
		gitmodulesPath := filepath.Join(superprojectDir, ".gitmodules")
		err = os.WriteFile(gitmodulesPath, []byte(gitmodulesContent), 0o644)
		require.NoError(t, err)

		// Create initial commit in superproject
		err = os.WriteFile(filepath.Join(superprojectDir, "README.md"), []byte("superproject"), 0o644)
		require.NoError(t, err)
		commitFiles(t, superprojectDir, "initial commit")

		// Create a worktree
		worktreePath := filepath.Join(tempDir, "test-wt")
		createWorktree(t, superprojectDir, worktreePath, "feature-branch")

		// Create a mock Provider with the local-sub as a discovered workspace
		mockProvider := createMockProvider(map[string]string{
			"local-sub": localSubmoduleDir,
		})

		// Test SetupSubmodules with all repos
		err = SetupSubmodules(ctx, worktreePath, superprojectDir, "feature-branch", nil, mockProvider)
		require.NoError(t, err)

		// Verify local-sub exists as a directory (linked worktree)
		localSubPath := filepath.Join(worktreePath, "local-sub")
		info, err := os.Stat(localSubPath)
		assert.NoError(t, err, "local-sub should exist")
		assert.True(t, info.IsDir(), "local-sub should be a directory")

		// Verify remote-sub exists (it should be created as empty dir or cloned)
		remoteSubPath := filepath.Join(worktreePath, "remote-sub")
		info, err = os.Stat(remoteSubPath)
		assert.NoError(t, err, "remote-sub should exist")
		assert.True(t, info.IsDir(), "remote-sub should be a directory")
	})

	t.Run("setup with repos filter", func(t *testing.T) {
		// Create temporary directories
		tempDir := t.TempDir()
		superprojectDir := filepath.Join(tempDir, "superproject")
		localSubmoduleDir := filepath.Join(tempDir, "local-sub")

		// Initialize repositories
		require.NoError(t, os.MkdirAll(superprojectDir, 0o755))
		initGitRepo(t, superprojectDir)

		require.NoError(t, os.MkdirAll(localSubmoduleDir, 0o755))
		initGitRepo(t, localSubmoduleDir)

		// Create content in local submodule
		readmePath := filepath.Join(localSubmoduleDir, "README.md")
		err := os.WriteFile(readmePath, []byte("local submodule"), 0o644)
		require.NoError(t, err)
		commitFiles(t, localSubmoduleDir, "initial commit")

		// Create .gitmodules in superproject with multiple submodules
		gitmodulesContent := `[submodule "local-sub"]
	path = local-sub
	url = ../local-sub
[submodule "excluded-sub"]
	path = excluded-sub
	url = https://github.com/example/excluded-sub.git
`
		gitmodulesPath := filepath.Join(superprojectDir, ".gitmodules")
		err = os.WriteFile(gitmodulesPath, []byte(gitmodulesContent), 0o644)
		require.NoError(t, err)

		// Create initial commit in superproject
		err = os.WriteFile(filepath.Join(superprojectDir, "README.md"), []byte("superproject"), 0o644)
		require.NoError(t, err)
		commitFiles(t, superprojectDir, "initial commit")

		// Create a worktree
		worktreePath := filepath.Join(tempDir, "test-wt")
		createWorktree(t, superprojectDir, worktreePath, "feature-branch")

		// Create a mock Provider
		mockProvider := createMockProvider(map[string]string{
			"local-sub": localSubmoduleDir,
		})

		// Test SetupSubmodules with repos filter (only local-sub)
		err = SetupSubmodules(ctx, worktreePath, superprojectDir, "feature-branch", []string{"local-sub"}, mockProvider)
		require.NoError(t, err)

		// Verify local-sub exists
		localSubPath := filepath.Join(worktreePath, "local-sub")
		info, err := os.Stat(localSubPath)
		assert.NoError(t, err, "local-sub should exist")
		assert.True(t, info.IsDir(), "local-sub should be a directory")

		// Verify excluded-sub does NOT exist or is empty
		excludedSubPath := filepath.Join(worktreePath, "excluded-sub")
		_, err = os.Stat(excludedSubPath)
		// It might exist as an empty directory or not exist at all
		if err == nil {
			// If it exists, verify it's empty
			entries, err := os.ReadDir(excludedSubPath)
			assert.NoError(t, err)
			assert.Empty(t, entries, "excluded-sub should be empty")
		} else {
			// It's OK if it doesn't exist
			assert.True(t, os.IsNotExist(err), "excluded-sub should not exist")
		}
	})

	t.Run("xdg-shaped worktree path uses passed gitRoot", func(t *testing.T) {
		sandboxXDG(t)

		tempDir := t.TempDir()
		superprojectDir := filepath.Join(tempDir, "superproject")
		require.NoError(t, os.MkdirAll(superprojectDir, 0o755))
		initGitRepo(t, superprojectDir)
		require.NoError(t, os.WriteFile(filepath.Join(superprojectDir, "README.md"), []byte("superproject"), 0o644))
		commitFiles(t, superprojectDir, "initial commit")

		// Sibling repo as a DIRECT CHILD of the ecosystem root (untracked,
		// like a gitignored private repo) — the direct-child filter must
		// match it against the PASSED gitRoot, not the worktree location.
		localSubmoduleDir := filepath.Join(superprojectDir, "local-sub")
		require.NoError(t, os.MkdirAll(localSubmoduleDir, 0o755))
		initGitRepo(t, localSubmoduleDir)
		require.NoError(t, os.WriteFile(filepath.Join(localSubmoduleDir, "README.md"), []byte("local submodule"), 0o644))
		commitFiles(t, localSubmoduleDir, "initial commit")

		// Ecosystem worktree at the XDG location (outside the repo tree).
		worktreePath := ResolveNewWorktreePath(superprojectDir, "wt1", true)
		require.NoError(t, os.MkdirAll(filepath.Dir(worktreePath), 0o755))
		createWorktree(t, superprojectDir, worktreePath, "feature-branch")

		mockProvider := createMockProvider(map[string]string{
			"local-sub": localSubmoduleDir,
		})

		err := SetupSubmodules(ctx, worktreePath, superprojectDir, "feature-branch", nil, mockProvider)
		require.NoError(t, err)

		// mainProjectPath resolved against gitRoot: local-sub is a real
		// linked worktree (worktree .git pointer FILE), not a placeholder.
		gitPointer := filepath.Join(worktreePath, "local-sub", ".git")
		info, err := os.Stat(gitPointer)
		require.NoError(t, err, "local-sub should be a linked worktree at the XDG location")
		assert.False(t, info.IsDir(), "local-sub/.git should be a worktree pointer file")
	})

	t.Run("handle missing submodules", func(t *testing.T) {
		// Create temporary directories
		tempDir := t.TempDir()
		superprojectDir := filepath.Join(tempDir, "superproject")

		// Initialize repository
		require.NoError(t, os.MkdirAll(superprojectDir, 0o755))
		initGitRepo(t, superprojectDir)

		// Create .gitmodules with non-existent submodule
		gitmodulesContent := `[submodule "missing-sub"]
	path = missing-sub
	url = ../missing-sub
`
		gitmodulesPath := filepath.Join(superprojectDir, ".gitmodules")
		err := os.WriteFile(gitmodulesPath, []byte(gitmodulesContent), 0o644)
		require.NoError(t, err)

		// Create initial commit in superproject
		err = os.WriteFile(filepath.Join(superprojectDir, "README.md"), []byte("superproject"), 0o644)
		require.NoError(t, err)
		commitFiles(t, superprojectDir, "initial commit")

		// Create a worktree
		worktreePath := filepath.Join(tempDir, "test-wt")
		createWorktree(t, superprojectDir, worktreePath, "feature-branch")

		// Create empty mock Provider (no workspaces found)
		mockProvider := createMockProvider(map[string]string{})

		// Test SetupSubmodules - should handle missing submodule gracefully
		err = SetupSubmodules(ctx, worktreePath, superprojectDir, "feature-branch", nil, mockProvider)
		require.NoError(t, err)

		// Verify missing-sub directory exists (created as placeholder)
		missingSubPath := filepath.Join(worktreePath, "missing-sub")
		info, err := os.Stat(missingSubPath)
		assert.NoError(t, err, "missing-sub should exist as placeholder")
		assert.True(t, info.IsDir(), "missing-sub should be a directory")
	})

	t.Run("handle complex gitmodules", func(t *testing.T) {
		// Create temporary directories
		tempDir := t.TempDir()
		superprojectDir := filepath.Join(tempDir, "superproject")

		// Initialize repository
		require.NoError(t, os.MkdirAll(superprojectDir, 0o755))
		initGitRepo(t, superprojectDir)

		// Create complex .gitmodules with various formats
		gitmodulesContent := `# Comment line
[submodule "sub1"]
	path = path/to/sub1
	url = https://github.com/example/sub1.git
	branch = main

[submodule "sub2"]
	# Another comment
	path = sub2
	url = ../sub2.git
	
[submodule "sub3"]
	path=sub3
	url=git@github.com:example/sub3.git
	
# Empty lines and spaces
	
[submodule "sub with spaces"]
	path = "path with spaces/sub"
	url = https://example.com/sub.git
`
		gitmodulesPath := filepath.Join(superprojectDir, ".gitmodules")
		err := os.WriteFile(gitmodulesPath, []byte(gitmodulesContent), 0o644)
		require.NoError(t, err)

		// Create initial commit in superproject
		err = os.WriteFile(filepath.Join(superprojectDir, "README.md"), []byte("superproject"), 0o644)
		require.NoError(t, err)
		commitFiles(t, superprojectDir, "initial commit")

		// Create a worktree
		worktreePath := filepath.Join(tempDir, "test-wt")
		createWorktree(t, superprojectDir, worktreePath, "feature-branch")

		// Create empty mock Provider
		mockProvider := createMockProvider(map[string]string{})

		// Test SetupSubmodules with complex gitmodules
		err = SetupSubmodules(ctx, worktreePath, superprojectDir, "feature-branch", nil, mockProvider)
		require.NoError(t, err)

		// Verify all submodule directories are created
		expectedPaths := []string{
			"path/to/sub1",
			"sub2",
			"sub3",
			"path with spaces/sub",
		}

		for _, p := range expectedPaths {
			subPath := filepath.Join(worktreePath, p)
			info, err := os.Stat(subPath)
			assert.NoError(t, err, "Submodule path %s should exist", p)
			if err == nil {
				assert.True(t, info.IsDir(), "Submodule path %s should be a directory", p)
			}
		}
	})

	t.Run("handle empty repos list", func(t *testing.T) {
		// Create temporary directories
		tempDir := t.TempDir()
		superprojectDir := filepath.Join(tempDir, "superproject")

		// Initialize repository
		require.NoError(t, os.MkdirAll(superprojectDir, 0o755))
		initGitRepo(t, superprojectDir)

		// Create .gitmodules
		gitmodulesContent := `[submodule "sub1"]
	path = sub1
	url = https://github.com/example/sub1.git
`
		gitmodulesPath := filepath.Join(superprojectDir, ".gitmodules")
		err := os.WriteFile(gitmodulesPath, []byte(gitmodulesContent), 0o644)
		require.NoError(t, err)

		// Create initial commit in superproject
		err = os.WriteFile(filepath.Join(superprojectDir, "README.md"), []byte("superproject"), 0o644)
		require.NoError(t, err)
		commitFiles(t, superprojectDir, "initial commit")

		// Create a worktree
		worktreePath := filepath.Join(tempDir, "test-wt")
		createWorktree(t, superprojectDir, worktreePath, "feature-branch")

		// Create empty mock Provider
		mockProvider := createMockProvider(map[string]string{})

		// Test with empty repos list (should setup all submodules)
		err = SetupSubmodules(ctx, worktreePath, superprojectDir, "feature-branch", []string{}, mockProvider)
		require.NoError(t, err)

		// Verify submodule directory is created
		sub1Path := filepath.Join(worktreePath, "sub1")
		info, err := os.Stat(sub1Path)
		assert.NoError(t, err, "sub1 should exist")
		assert.True(t, info.IsDir(), "sub1 should be a directory")
	})
}

// hasStaleWorktreeRegistration reports whether `git worktree prune --dry-run`
// in repoDir would remove anything — i.e. a dangling/stale registration exists.
func hasStaleWorktreeRegistration(t *testing.T, repoDir string) bool {
	t.Helper()
	cmd := exec.Command("git", "worktree", "prune", "--dry-run", "-v")
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "prune --dry-run failed: %s", out)
	return len(strings.TrimSpace(string(out))) > 0
}

// TestSetupSubmodulesStaleWorktreeRegistration is the regression test for the
// inbox bug "worktree create silently yields incomplete ecosystem on stale
// git-worktree state". A member repo carries a stale/dangling git-worktree
// registration left by an `rm -rf` cleanup that never ran `git worktree prune`
// (reported by git as "gitdir file points to non-existent location"). Before
// the fix, `git worktree add` for that member failed, the failure was swallowed,
// and the member was silently dropped from the container — yielding an
// incomplete, non-hermetic worktree with NO error. The fix prunes stale
// registrations before add (so the create succeeds) AND collects any genuine
// add failures so SetupSubmodules fails loud instead of returning silently
// incomplete.
func TestSetupSubmodulesStaleWorktreeRegistration(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}
	ctx := context.Background()

	t.Run("pre-create prune clears stale entry; container is complete", func(t *testing.T) {
		tempDir := t.TempDir()

		// Ecosystem-of-repos layout: the ecosystem root holds member repos as
		// direct-child dirs (the real `flow plan init --worktree` case).
		ecoRoot := filepath.Join(tempDir, "eco")
		require.NoError(t, os.MkdirAll(ecoRoot, 0o755))
		initGitRepo(t, ecoRoot)
		require.NoError(t, os.WriteFile(filepath.Join(ecoRoot, "grove.toml"), []byte("workspaces = [\"*\"]\n"), 0o644))
		commitFiles(t, ecoRoot, "ecosystem root")

		// Member repo as a direct child of the ecosystem root.
		memberDir := filepath.Join(ecoRoot, "member")
		require.NoError(t, os.MkdirAll(memberDir, 0o755))
		initGitRepo(t, memberDir)
		require.NoError(t, os.WriteFile(filepath.Join(memberDir, "README.md"), []byte("member"), 0o644))
		commitFiles(t, memberDir, "member initial")

		// SEED a stale/dangling git-worktree registration in the member, exactly
		// as a prior `rm -rf` cleanup would leave it: create a worktree, then
		// remove its directory WITHOUT `git worktree remove`/`prune`.
		staleWtDir := filepath.Join(tempDir, "stale-member-wt")
		createWorktree(t, memberDir, staleWtDir, "leftover-branch")
		require.NoError(t, os.RemoveAll(staleWtDir))
		require.True(t, hasStaleWorktreeRegistration(t, memberDir),
			"precondition: member should carry a stale worktree registration after rm -rf")

		// The container worktree of the ecosystem root.
		worktreePath := filepath.Join(tempDir, "container")
		createWorktree(t, ecoRoot, worktreePath, "feature-branch")

		// Provider discovers the member as a local workspace under the ecosystem.
		mockProvider := createMockProvider(map[string]string{
			"member": memberDir,
		})

		err := SetupSubmodules(ctx, worktreePath, ecoRoot, "feature-branch", []string{"member"}, mockProvider)
		// Fail-before: without the pre-create prune, the member's `git worktree
		// add` fails on the stale entry and (pre-fix) is swallowed -> no error
		// but missing member. Pass-after: prune clears the entry, add succeeds.
		require.NoError(t, err, "stale registration should have been pruned, allowing a complete container")

		// Assert the container is COMPLETE: the member's linked worktree exists.
		memberWt := filepath.Join(worktreePath, "member")
		info, statErr := os.Stat(filepath.Join(memberWt, ".git"))
		require.NoError(t, statErr, "member worktree must be present in the container (complete, hermetic)")
		_ = info
		// And the stale entry is gone.
		assert.False(t, hasStaleWorktreeRegistration(t, memberDir),
			"pre-create prune should have cleared the stale registration")
	})

	t.Run("genuine add failure fails loud and names the repo", func(t *testing.T) {
		tempDir := t.TempDir()

		ecoRoot := filepath.Join(tempDir, "eco")
		require.NoError(t, os.MkdirAll(ecoRoot, 0o755))
		initGitRepo(t, ecoRoot)
		require.NoError(t, os.WriteFile(filepath.Join(ecoRoot, "grove.toml"), []byte("workspaces = [\"*\"]\n"), 0o644))
		commitFiles(t, ecoRoot, "ecosystem root")

		memberDir := filepath.Join(ecoRoot, "member")
		require.NoError(t, os.MkdirAll(memberDir, 0o755))
		initGitRepo(t, memberDir)
		require.NoError(t, os.WriteFile(filepath.Join(memberDir, "README.md"), []byte("member"), 0o644))
		commitFiles(t, memberDir, "member initial")

		// Make the add genuinely impossible: the target branch is ALREADY checked
		// out in a separate live worktree of the member. `git worktree add -B
		// feature-branch` then refuses ("already used by worktree at ..."), and a
		// prune cannot clear a live worktree. This is the "member truly cannot be
		// added" path — it must surface a clear error naming the repo, never a
		// silently-incomplete container.
		liveWtDir := filepath.Join(tempDir, "live-feature-branch")
		createWorktree(t, memberDir, liveWtDir, "feature-branch")

		worktreePath := filepath.Join(tempDir, "container")
		createWorktree(t, ecoRoot, worktreePath, "feature-branch")

		mockProvider := createMockProvider(map[string]string{
			"member": memberDir,
		})

		err := SetupSubmodules(ctx, worktreePath, ecoRoot, "feature-branch", []string{"member"}, mockProvider)
		require.Error(t, err, "a member that cannot be added must produce a loud error, not a silent incomplete container")
		assert.Contains(t, err.Error(), "member", "the error must name the dropped repo")
		assert.Contains(t, err.Error(), "incomplete", "the error must flag the container as incomplete")
	})
}

func TestParseGitmodules(t *testing.T) {
	t.Run("parse standard gitmodules", func(t *testing.T) {
		gitmodulesContent := `[submodule "grove-core"]
	path = grove-core
	url = https://github.com/grovetools/core.git
[submodule "grove-flow"]
	path = grove-flow
	url = https://github.com/grovetools/flow.git
`
		tmpDir := t.TempDir()
		gitmodulesPath := filepath.Join(tmpDir, ".gitmodules")
		err := os.WriteFile(gitmodulesPath, []byte(gitmodulesContent), 0o644)
		require.NoError(t, err)

		submodules, err := parseGitmodules(gitmodulesPath)
		require.NoError(t, err)

		assert.Equal(t, 2, len(submodules))
		assert.Equal(t, "grove-core", submodules["grove-core"])
		assert.Equal(t, "grove-flow", submodules["grove-flow"])
	})

	t.Run("parse gitmodules with different paths", func(t *testing.T) {
		gitmodulesContent := `[submodule "mylib"]
	path = libs/mylib
	url = https://github.com/example/mylib.git
[submodule "tool"]
	path = tools/mytool
	url = https://github.com/example/tool.git
`
		tmpDir := t.TempDir()
		gitmodulesPath := filepath.Join(tmpDir, ".gitmodules")
		err := os.WriteFile(gitmodulesPath, []byte(gitmodulesContent), 0o644)
		require.NoError(t, err)

		submodules, err := parseGitmodules(gitmodulesPath)
		require.NoError(t, err)

		assert.Equal(t, 2, len(submodules))
		assert.Equal(t, "libs/mylib", submodules["mylib"])
		assert.Equal(t, "tools/mytool", submodules["tool"])
	})

	t.Run("handle missing gitmodules file", func(t *testing.T) {
		tmpDir := t.TempDir()
		gitmodulesPath := filepath.Join(tmpDir, ".gitmodules")

		submodules, err := parseGitmodules(gitmodulesPath)
		assert.Error(t, err)
		assert.Nil(t, submodules)
	})

	t.Run("handle malformed gitmodules", func(t *testing.T) {
		gitmodulesContent := `[submodule "incomplete"
	url = https://github.com/example/incomplete.git
[submodule "nourl"]
	path = nourl
`
		tmpDir := t.TempDir()
		gitmodulesPath := filepath.Join(tmpDir, ".gitmodules")
		err := os.WriteFile(gitmodulesPath, []byte(gitmodulesContent), 0o644)
		require.NoError(t, err)

		submodules, err := parseGitmodules(gitmodulesPath)
		require.NoError(t, err)

		// Only "nourl" should be parsed (has path but no url)
		assert.Equal(t, 1, len(submodules))
		assert.Equal(t, "nourl", submodules["nourl"])
	})
}

// Variable to allow mocking in tests
// var discoverLocalWorkspacesFunc = DiscoverLocalWorkspaces
