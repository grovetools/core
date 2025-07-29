package git

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mattsolo1/grove-core/command"
)

// EnvironmentVars contains git-based environment variables
type EnvironmentVars struct {
	Repo         string
	Branch       string
	Commit       string
	CommitShort  string
	Author       string
	AuthorEmail  string
	WorktreePath string
	IsDirty      bool
}

// GetEnvironmentVars returns git-based environment variables
func GetEnvironmentVars(workDir string) (*EnvironmentVars, error) {
	vars := &EnvironmentVars{}

	// Get repository name
	repo, branch, err := GetRepoInfo(workDir)
	if err != nil {
		return nil, err
	}
	vars.Repo = repo
	vars.Branch = branch

	// Get commit hash
	commit, err := getGitOutput(workDir, "rev-parse", "HEAD")
	if err == nil {
		vars.Commit = commit
		if len(commit) >= 7 {
			vars.CommitShort = commit[:7]
		}
	}

	// Get author info
	author, err := getGitOutput(workDir, "config", "user.name")
	if err == nil {
		vars.Author = author
	}

	email, err := getGitOutput(workDir, "config", "user.email")
	if err == nil {
		vars.AuthorEmail = email
	}

	// Get worktree path
	wtManager := NewWorktreeManager()
	if wt, err := wtManager.GetCurrentWorktree(context.Background(), workDir); err == nil {
		vars.WorktreePath = wt.Path
	}

	// Check if working directory is dirty
	status, err := getGitOutput(workDir, "status", "--porcelain")
	vars.IsDirty = err == nil && len(status) > 0

	return vars, nil
}

// ToMap converts environment vars to a map
func (v *EnvironmentVars) ToMap() map[string]string {
	return map[string]string{
		"GROVE_REPO":          v.Repo,
		"GROVE_BRANCH":        v.Branch,
		"GROVE_COMMIT":        v.Commit,
		"GROVE_COMMIT_SHORT":  v.CommitShort,
		"GROVE_AUTHOR":        v.Author,
		"GROVE_AUTHOR_EMAIL":  v.AuthorEmail,
		"GROVE_WORKTREE_PATH": v.WorktreePath,
		"GROVE_IS_DIRTY":      fmt.Sprintf("%t", v.IsDirty),

		// Convenience aliases
		"REPO":   v.Repo,
		"BRANCH": v.Branch,
		"COMMIT": v.CommitShort,
	}
}

// SetEnvironment sets git-based environment variables
func (v *EnvironmentVars) SetEnvironment() {
	for key, value := range v.ToMap() {
		os.Setenv(key, value)
	}
}

// getGitOutput runs a git command and returns output
func getGitOutput(workDir string, args ...string) (string, error) {
	cmdBuilder := command.NewSafeBuilder()
	cmd, err := cmdBuilder.Build(context.Background(), "git", args...)
	if err != nil {
		return "", fmt.Errorf("failed to build command: %w", err)
	}

	execCmd := cmd.Exec()
	execCmd.Dir = workDir

	output, err := execCmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}