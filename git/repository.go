package git

import (
	"context"

	"github.com/mattsolo1/grove-core/command"
)

// CLIRepository implements RepositoryProvider using git CLI
type CLIRepository struct {
	cmdBuilder *command.SafeBuilder
}

// Ensure it implements the interface
var _ RepositoryProvider = (*CLIRepository)(nil)

// NewCLIRepository creates a new CLI repository provider
func NewCLIRepository() *CLIRepository {
	return &CLIRepository{
		cmdBuilder: command.NewSafeBuilder(),
	}
}

// GetRepoInfo returns repository and branch information
func (r *CLIRepository) GetRepoInfo(ctx context.Context, dir string) (repo string, branch string, err error) {
	return GetRepoInfo(dir)
}

// IsGitRepo checks if a directory is a git repository
func (r *CLIRepository) IsGitRepo(ctx context.Context, dir string) bool {
	return IsGitRepo(dir)
}

// GetGitRoot returns the root directory of the git repository
func (r *CLIRepository) GetGitRoot(ctx context.Context, dir string) (string, error) {
	return GetGitRoot(dir)
}

// GetEnvironmentVars returns git-based environment variables
func (r *CLIRepository) GetEnvironmentVars(ctx context.Context, workDir string) (*EnvironmentVars, error) {
	return GetEnvironmentVars(workDir)
}