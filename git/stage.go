package git

import (
	"context"
	"fmt"
	"strings"

	"github.com/grovetools/core/command"
)

// Stage stages the working-tree changes for filePath in the repository at
// repoPath by running `git add -- <filePath>`. filePath is repo-relative.
// It is the per-file counterpart to the bulk staging the daemon/CLI perform;
// the git-viewer staging page calls it for a single file leaf.
func Stage(repoPath, filePath string) error {
	return runFileGitCommand(repoPath, filePath, "add", "--", filePath)
}

// Unstage removes filePath from the index (reverting it to its HEAD state in
// the index only, leaving the working tree untouched) by running
// `git restore --staged -- <filePath>`. filePath is repo-relative.
func Unstage(repoPath, filePath string) error {
	return runFileGitCommand(repoPath, filePath, "restore", "--staged", "--", filePath)
}

// Discard throws away the unstaged working-tree changes for a tracked file by
// running `git checkout -- <filePath>`, restoring it from the index. filePath
// is repo-relative. This is destructive and irreversible. It does NOT remove
// untracked files (git checkout errors for a path it does not know) and does
// NOT unstage already-staged changes — callers that want a full reset should
// Unstage first, then Discard.
func Discard(repoPath, filePath string) error {
	return runFileGitCommand(repoPath, filePath, "checkout", "--", filePath)
}

// GetBlobHash returns the git blob object hash for the working-tree file at
// filePath (repo-relative) within repoPath, via `git hash-object -- <filePath>`.
// The hash is content-addressed, so it changes whenever the file's bytes
// change. That makes it a stable key for per-file review state that
// auto-invalidates on re-edit (review:<repo>/<path>@<blob-hash>): a re-edited
// file produces a new hash and thus un-reviews itself. The file must exist on
// disk; hash-object reads the working-tree file, not the index or HEAD.
func GetBlobHash(repoPath, filePath string) (string, error) {
	cmdBuilder := command.NewSafeBuilder()
	if err := cmdBuilder.Validate("fileName", filePath); err != nil {
		return "", fmt.Errorf("invalid file path: %w", err)
	}
	cmd, err := cmdBuilder.Build(context.Background(), "git", "hash-object", "--", filePath)
	if err != nil {
		return "", fmt.Errorf("failed to build command: %w", err)
	}
	execCmd := cmd.Exec()
	execCmd.Dir = repoPath
	output, err := execCmd.Output()
	if err != nil {
		return "", fmt.Errorf("git hash-object failed for %s: %w", filePath, err)
	}
	return strings.TrimSpace(string(output)), nil
}

// runFileGitCommand validates filePath with the shared "fileName" validator
// (rejecting traversal and shell metacharacters) and runs the given git
// subcommand in repoPath. It centralizes the exec.Command wiring used by the
// per-file staging primitives, mirroring the SafeBuilder pattern in worktree.go.
func runFileGitCommand(repoPath, filePath string, args ...string) error {
	cmdBuilder := command.NewSafeBuilder()
	if err := cmdBuilder.Validate("fileName", filePath); err != nil {
		return fmt.Errorf("invalid file path: %w", err)
	}
	cmd, err := cmdBuilder.Build(context.Background(), "git", args...)
	if err != nil {
		return fmt.Errorf("failed to build command: %w", err)
	}
	execCmd := cmd.Exec()
	execCmd.Dir = repoPath
	if output, err := execCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %s failed: %w, output: %s", strings.Join(args, " "), err, string(output))
	}
	return nil
}
