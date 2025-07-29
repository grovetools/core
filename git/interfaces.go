package git

import "context"

// WorktreeProvider defines the interface for git worktree operations
type WorktreeProvider interface {
	// Worktree operations
	ListWorktrees(ctx context.Context, repoPath string) ([]WorktreeInfo, error)
	GetCurrentWorktree(ctx context.Context, path string) (*WorktreeInfo, error)
	CreateWorktree(ctx context.Context, basePath, worktreePath, branch string, createBranch bool) error
	RemoveWorktree(ctx context.Context, basePath, worktreePath string) error
	GetWorktreeRoot(ctx context.Context, path string) (string, error)
}

// HookProvider defines the interface for git hook operations
type HookProvider interface {
	// Hook management
	InstallHooks(ctx context.Context, repoPath string) error
	UninstallHooks(ctx context.Context, repoPath string) error
}

// RepositoryProvider defines the interface for general git repository operations
type RepositoryProvider interface {
	// Repository information
	GetRepoInfo(ctx context.Context, dir string) (repo string, branch string, err error)
	IsGitRepo(ctx context.Context, dir string) bool
	GetGitRoot(ctx context.Context, dir string) (string, error)
	GetEnvironmentVars(ctx context.Context, workDir string) (*EnvironmentVars, error)
}