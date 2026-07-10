package workspace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to check if git is available
func skipIfNoGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}
}

// Helper to setup a git repository for testing
func setupTestRepo(t *testing.T, dir string) {
	// Pin the initial branch to main explicitly. With HOME sandboxed (below),
	// there is no global git config, so a bare `git init` would fall back to
	// `master` and later `git checkout main` steps would fail. -b main makes the
	// default branch independent of the developer's ~/.gitconfig.
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = dir
	err := cmd.Run()
	require.NoError(t, err)

	// Configure git user
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = dir
	err = cmd.Run()
	require.NoError(t, err)

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = dir
	err = cmd.Run()
	require.NoError(t, err)

	// Create initial commit
	readmePath := filepath.Join(dir, "README.md")
	err = os.WriteFile(readmePath, []byte("test repo"), 0o644)
	require.NoError(t, err)

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = dir
	err = cmd.Run()
	require.NoError(t, err)

	cmd = exec.Command("git", "commit", "-m", "initial commit")
	cmd.Dir = dir
	err = cmd.Run()
	require.NoError(t, err)
}

func TestPrepare(t *testing.T) {
	skipIfNoGit(t)

	// Sandbox HOME so Prepare's claudetrust.SeedTrust writes folder-trust into a
	// throwaway ~/.claude.json instead of polluting the developer's real one.
	// Each subtest creates worktrees under t.TempDir() that are then deleted, so
	// without this every run leaked orphan projects[] keys into the real file
	// (the leak the PruneOrphanTrust sweep can't reach — those paths live outside
	// WorktreesDir). Inherited by all subtests below.
	t.Setenv("HOME", t.TempDir())

	t.Run("single repo workspace creation", func(t *testing.T) {
		// Setup test repository
		tempDir := resolveDir(t.TempDir())
		repoDir := filepath.Join(tempDir, "test-repo")
		require.NoError(t, os.MkdirAll(repoDir, 0o755))
		setupTestRepo(t, repoDir)

		ctx := context.Background()
		opts := PrepareOptions{
			GitRoot:      repoDir,
			WorktreeName: "test-workspace",
			BranchName:   "feature/test",
		}

		// Create workspace
		worktreePath, err := Prepare(ctx, opts)
		require.NoError(t, err)
		assert.NotEmpty(t, worktreePath)

		// Verify worktree exists
		expectedPath := filepath.Join(repoDir, ".grove-worktrees", "test-workspace")
		assert.Equal(t, expectedPath, worktreePath)

		info, err := os.Stat(worktreePath)
		require.NoError(t, err)
		assert.True(t, info.IsDir())

		// Verify branch was created
		cmd := exec.Command("git", "branch", "--list", "feature/test")
		cmd.Dir = repoDir
		output, err := cmd.Output()
		require.NoError(t, err)
		assert.Contains(t, string(output), "feature/test")
	})

	t.Run("ecosystem worktree with repos", func(t *testing.T) {
		// Setup ecosystem repository with submodules
		tempDir := resolveDir(t.TempDir())
		ecosystemDir := filepath.Join(tempDir, "ecosystem")
		require.NoError(t, os.MkdirAll(ecosystemDir, 0o755))
		setupTestRepo(t, ecosystemDir)

		// Create .gitmodules file
		gitmodulesContent := `[submodule "repo1"]
	path = repo1
	url = https://github.com/example/repo1.git
[submodule "repo2"]
	path = repo2
	url = https://github.com/example/repo2.git
`
		gitmodulesPath := filepath.Join(ecosystemDir, ".gitmodules")
		err := os.WriteFile(gitmodulesPath, []byte(gitmodulesContent), 0o644)
		require.NoError(t, err)

		// Commit .gitmodules
		cmd := exec.Command("git", "add", ".gitmodules")
		cmd.Dir = ecosystemDir
		err = cmd.Run()
		require.NoError(t, err)

		cmd = exec.Command("git", "commit", "-m", "add gitmodules")
		cmd.Dir = ecosystemDir
		err = cmd.Run()
		require.NoError(t, err)

		ctx := context.Background()
		opts := PrepareOptions{
			GitRoot:           ecosystemDir,
			WorktreeName:      "eco-workspace",
			BranchName:        "feature/ecosystem",
			SiblingWorkspaces: []string{"repo1"}, // Only setup repo1
		}

		// Mock DiscoverLocalWorkspaces to avoid actual discovery
		// oldFunc := discoverLocalWorkspacesFunc
		// discoverLocalWorkspacesFunc = func(ctx context.Context) (map[string]string, error) {
		// 	return map[string]string{}, nil
		// }
		// defer func() { discoverLocalWorkspacesFunc = oldFunc }()

		// Create workspace
		worktreePath, err := Prepare(ctx, opts)
		require.NoError(t, err)
		assert.NotEmpty(t, worktreePath)

		// Verify worktree exists
		expectedPath := filepath.Join(ecosystemDir, ".grove-worktrees", "eco-workspace")
		assert.Equal(t, expectedPath, worktreePath)

		// Verify go.work was created
		goWorkPath := filepath.Join(worktreePath, "go.work")
		_, err = os.Stat(goWorkPath)
		// go.work might not exist if there are no Go modules
		if err == nil {
			content, err := os.ReadFile(goWorkPath)
			require.NoError(t, err)
			assert.Contains(t, string(content), "go 1.")
		}
	})

	t.Run("xdg worktree with owner marker", func(t *testing.T) {
		sandboxXDG(t)

		tempDir := resolveDir(t.TempDir())
		repoDir := filepath.Join(tempDir, "test-repo")
		require.NoError(t, os.MkdirAll(repoDir, 0o755))
		setupTestRepo(t, repoDir)

		// repo1 is a real direct-child repo so the (now authoritative) sibling
		// setup can create a linked worktree for it. An explicitly-requested
		// repo that exists nowhere is a hard error under the new contract.
		repo1Dir := filepath.Join(repoDir, "repo1")
		require.NoError(t, os.MkdirAll(repo1Dir, 0o755))
		setupTestRepo(t, repo1Dir)

		ctx := context.Background()
		opts := PrepareOptions{
			GitRoot:           repoDir,
			WorktreeName:      "xdg-workspace",
			BranchName:        "feature/xdg",
			SiblingWorkspaces: []string{"repo1"},
			UseXDGWorktrees:   true,
		}

		worktreePath, err := Prepare(ctx, opts)
		require.NoError(t, err)

		// Lands at the XDG location, not under <repo>/.grove-worktrees.
		expectedPath := ResolveNewWorktreePath(repoDir, "xdg-workspace", true)
		assert.Equal(t, expectedPath, worktreePath)
		_, err = os.Stat(filepath.Join(repoDir, ".grove-worktrees", "xdg-workspace"))
		assert.True(t, os.IsNotExist(err))

		// Marker keeps the frozen ecosystem:/repos: keys and gains owner:.
		marker, err := os.ReadFile(filepath.Join(worktreePath, ".grove", "workspace"))
		require.NoError(t, err)
		absRepo, err := filepath.Abs(repoDir)
		require.NoError(t, err)
		assert.Contains(t, string(marker), "owner: "+absRepo+"\n")
		assert.Contains(t, string(marker), "ecosystem: true\n")
		assert.Contains(t, string(marker), "repos:\n  - repo1\n")

		// Re-preparing reuses the XDG worktree. Normalize to handle the
		// /var vs /private/var symlink on macOS (git reports realpaths).
		again, err := Prepare(ctx, opts)
		require.NoError(t, err)
		resolvedFirst, _ := filepath.EvalSymlinks(worktreePath)
		resolvedAgain, _ := filepath.EvalSymlinks(again)
		assert.Equal(t, resolvedFirst, resolvedAgain)
	})

	t.Run("container has no superrepo .git", func(t *testing.T) {
		// Regression: an ecosystem worktree container must NOT be a git worktree
		// of the ecosystem superrepo. A top-level .git makes the container track
		// submodule gitlinks, forcing submodule bumps and blocking clean per-repo
		// rebasing. Prepare must always build a synthetic container (a plain dir +
		// grove.toml `workspaces = ["*"]`) whose members are the only git
		// worktrees, for anchored AND non-anchored ecosystem worktrees alike.
		sandboxXDG(t)

		tempDir := resolveDir(t.TempDir())
		repoDir := filepath.Join(tempDir, "eco-root")
		require.NoError(t, os.MkdirAll(repoDir, 0o755))
		setupTestRepo(t, repoDir)

		// A real direct-child repo so the authoritative sibling setup can create a
		// linked worktree for it.
		memberDir := filepath.Join(repoDir, "member1")
		require.NoError(t, os.MkdirAll(memberDir, 0o755))
		setupTestRepo(t, memberDir)

		ctx := context.Background()
		opts := PrepareOptions{
			GitRoot:           repoDir,
			WorktreeName:      "no-superrepo",
			BranchName:        "feature/no-superrepo",
			SiblingWorkspaces: []string{"member1"},
			UseXDGWorktrees:   true,
		}

		worktreePath, err := Prepare(ctx, opts)
		require.NoError(t, err)

		// THE regression assertion: no top-level .git in the container.
		_, statErr := os.Stat(filepath.Join(worktreePath, ".git"))
		assert.True(t, os.IsNotExist(statErr), "container must not have a top-level .git (superrepo worktree)")

		// It IS a synthetic container: grove.toml with workspaces = ["*"].
		groveToml, err := os.ReadFile(filepath.Join(worktreePath, "grove.toml"))
		require.NoError(t, err)
		assert.Contains(t, string(groveToml), `workspaces = ["*"]`)

		// The member repo IS a linked git worktree (its .git is a gitdir FILE,
		// not a directory), proving members carry the git state, not the container.
		memberGit := filepath.Join(worktreePath, "member1", ".git")
		info, err := os.Stat(memberGit)
		require.NoError(t, err, "member repo should be checked out as a linked worktree")
		assert.False(t, info.IsDir(), "member .git should be a gitdir file (linked worktree), not a directory")
	})

	t.Run("error cases", func(t *testing.T) {
		ctx := context.Background()

		// Test with invalid git root
		opts := PrepareOptions{
			GitRoot:      "/nonexistent/path",
			WorktreeName: "test",
			BranchName:   "test",
		}

		_, err := Prepare(ctx, opts)
		assert.Error(t, err)

		// Test with empty worktree name
		tempDir := resolveDir(t.TempDir())
		opts = PrepareOptions{
			GitRoot:      tempDir,
			WorktreeName: "",
			BranchName:   "test",
		}

		_, err = Prepare(ctx, opts)
		assert.Error(t, err)
	})

	t.Run("prepare with existing branch", func(t *testing.T) {
		// Setup test repository
		tempDir := resolveDir(t.TempDir())
		repoDir := filepath.Join(tempDir, "test-repo")
		require.NoError(t, os.MkdirAll(repoDir, 0o755))
		setupTestRepo(t, repoDir)

		// Create an existing branch
		cmd := exec.Command("git", "checkout", "-b", "existing-branch")
		cmd.Dir = repoDir
		err := cmd.Run()
		require.NoError(t, err)

		// Switch back to main
		cmd = exec.Command("git", "checkout", "main")
		cmd.Dir = repoDir
		err = cmd.Run()
		require.NoError(t, err)

		ctx := context.Background()
		opts := PrepareOptions{
			GitRoot:      repoDir,
			WorktreeName: "test-workspace",
			BranchName:   "existing-branch",
		}

		// Should use existing branch without error
		worktreePath, err := Prepare(ctx, opts)
		require.NoError(t, err)
		assert.NotEmpty(t, worktreePath)
	})

	t.Run("prepare with branch already checked out in worktree", func(t *testing.T) {
		// Setup test repository
		tempDir := resolveDir(t.TempDir())
		repoDir := filepath.Join(tempDir, "test-repo")
		require.NoError(t, os.MkdirAll(repoDir, 0o755))
		setupTestRepo(t, repoDir)

		ctx := context.Background()

		// First, create a worktree with a branch
		opts1 := PrepareOptions{
			GitRoot:      repoDir,
			WorktreeName: "first-workspace",
			BranchName:   "shared-branch",
		}

		firstWorktreePath, err := Prepare(ctx, opts1)
		require.NoError(t, err)
		assert.NotEmpty(t, firstWorktreePath)

		// Now try to prepare another worktree for the same branch
		// This should return the existing worktree path instead of failing
		opts2 := PrepareOptions{
			GitRoot:      repoDir,
			WorktreeName: "second-workspace",
			BranchName:   "shared-branch",
		}

		secondWorktreePath, err := Prepare(ctx, opts2)
		require.NoError(t, err)
		// Normalize to handle /var vs /private/var symlink on macOS
		resolvedFirst, _ := filepath.EvalSymlinks(firstWorktreePath)
		resolvedSecond, _ := filepath.EvalSymlinks(secondWorktreePath)
		assert.Equal(t, resolvedFirst, resolvedSecond, "Should return existing worktree path when branch is already checked out")
	})
}
