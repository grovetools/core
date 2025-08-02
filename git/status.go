package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/mattsolo1/grove-core/command"
)

// StatusInfo contains detailed git status information for a repository
type StatusInfo struct {
	// Branch is the current branch name
	Branch string `json:"branch"`
	
	// AheadCount is the number of commits ahead of the upstream branch
	AheadCount int `json:"ahead_count"`
	
	// BehindCount is the number of commits behind the upstream branch  
	BehindCount int `json:"behind_count"`
	
	// ModifiedCount is the number of modified files
	ModifiedCount int `json:"modified_count"`
	
	// UntrackedCount is the number of untracked files
	UntrackedCount int `json:"untracked_count"`
	
	// StagedCount is the number of staged files
	StagedCount int `json:"staged_count"`
	
	// IsDirty indicates if there are any uncommitted changes
	IsDirty bool `json:"is_dirty"`
	
	// HasUpstream indicates if the branch has an upstream tracking branch
	HasUpstream bool `json:"has_upstream"`
}

// GetStatus returns detailed git status information for the repository at the given path
func GetStatus(path string) (*StatusInfo, error) {
	cmdBuilder := command.NewSafeBuilder()
	status := &StatusInfo{}
	
	// Get current branch
	cmd, err := cmdBuilder.Build(context.Background(), "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("failed to build command: %w", err)
	}
	execCmd := cmd.Exec()
	execCmd.Dir = path
	output, err := execCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get current branch: %w", err)
	}
	status.Branch = strings.TrimSpace(string(output))
	
	// Check if branch has upstream
	cmd, err = cmdBuilder.Build(context.Background(), "git", "rev-parse", "--abbrev-ref", "@{u}")
	if err != nil {
		return nil, fmt.Errorf("failed to build command: %w", err)
	}
	execCmd = cmd.Exec()
	execCmd.Dir = path
	_, err = execCmd.Output()
	status.HasUpstream = err == nil
	
	// Get ahead/behind counts if upstream exists
	if status.HasUpstream {
		// Get ahead count
		cmd, err = cmdBuilder.Build(context.Background(), "git", "rev-list", "--count", "@{u}..HEAD")
		if err != nil {
			return nil, fmt.Errorf("failed to build command: %w", err)
		}
		execCmd = cmd.Exec()
		execCmd.Dir = path
		output, err = execCmd.Output()
		if err == nil {
			count, _ := strconv.Atoi(strings.TrimSpace(string(output)))
			status.AheadCount = count
		}
		
		// Get behind count
		cmd, err = cmdBuilder.Build(context.Background(), "git", "rev-list", "--count", "HEAD..@{u}")
		if err != nil {
			return nil, fmt.Errorf("failed to build command: %w", err)
		}
		execCmd = cmd.Exec()
		execCmd.Dir = path
		output, err = execCmd.Output()
		if err == nil {
			count, _ := strconv.Atoi(strings.TrimSpace(string(output)))
			status.BehindCount = count
		}
	}
	
	// Get status information using porcelain format
	cmd, err = cmdBuilder.Build(context.Background(), "git", "status", "--porcelain", "-uno")
	if err != nil {
		return nil, fmt.Errorf("failed to build command: %w", err)
	}
	execCmd = cmd.Exec()
	execCmd.Dir = path
	output, err = execCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get git status: %w", err)
	}
	
	// Parse porcelain output
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if len(line) < 2 {
			continue
		}
		
		// First character is staged status, second is working tree status
		staged := line[0]
		working := line[1]
		
		// Count staged files
		if staged != ' ' && staged != '?' {
			status.StagedCount++
		}
		
		// Count modified files in working tree
		if working == 'M' || working == 'D' {
			status.ModifiedCount++
		}
	}
	
	// Get untracked files count
	cmd, err = cmdBuilder.Build(context.Background(), "git", "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, fmt.Errorf("failed to build command: %w", err)
	}
	execCmd = cmd.Exec()
	execCmd.Dir = path
	output, err = execCmd.Output()
	if err == nil && len(output) > 0 {
		untrackedFiles := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, file := range untrackedFiles {
			if file != "" {
				status.UntrackedCount++
			}
		}
	}
	
	// Set IsDirty flag
	status.IsDirty = status.ModifiedCount > 0 || status.UntrackedCount > 0 || status.StagedCount > 0
	
	return status, nil
}