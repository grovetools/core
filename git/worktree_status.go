package git

import (
	"context"
	"fmt"
)

// WorktreeWithStatus contains worktree info plus its git status
type WorktreeWithStatus struct {
	WorktreeInfo
	Status *StatusInfo
}

// ListWorktreesWithStatus returns worktrees with their git status
func ListWorktreesWithStatus(repoPath string) ([]WorktreeWithStatus, error) {
	ctx := context.Background()
	manager := NewWorktreeManager()
	
	// Get all worktrees
	worktrees, err := manager.ListWorktrees(ctx, repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}
	
	// Get status for each worktree
	var result []WorktreeWithStatus
	for _, wt := range worktrees {
		wtWithStatus := WorktreeWithStatus{
			WorktreeInfo: wt,
		}
		
		// Get git status for this worktree
		status, err := GetStatus(wt.Path)
		if err == nil {
			wtWithStatus.Status = status
		}
		// Continue even if status fails - worktree might be in unusual state
		
		result = append(result, wtWithStatus)
	}
	
	return result, nil
}