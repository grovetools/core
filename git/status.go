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

	// AheadMainCount is the number of commits ahead of the local main/master branch
	AheadMainCount int `json:"ahead_main_count"`

	// BehindMainCount is the number of commits behind the local main/master branch
	BehindMainCount int `json:"behind_main_count"`
}

// GetStatus returns detailed git status information for the repository at the given path
func GetStatus(path string) (*StatusInfo, error) {
	cmdBuilder := command.NewSafeBuilder()
	status := &StatusInfo{}

	// Use git status --porcelain=v2 --branch for a single, efficient call
	cmd, err := cmdBuilder.Build(context.Background(), "git", "status", "--porcelain=v2", "--branch", "--ignore-submodules")
	if err != nil {
		return nil, fmt.Errorf("failed to build command: %w", err)
	}
	execCmd := cmd.Exec()
	execCmd.Dir = path
	output, err := execCmd.Output()
	if err != nil {
		// Check if it's not a git repository
		outputStr := string(output)
		if strings.Contains(outputStr, "not a git repository") {
			return nil, fmt.Errorf("not a git repository: %s", path)
		}
		// This can happen in a new repo before the first commit. Return a valid but empty status.
		if strings.Contains(outputStr, "No commits yet") {
			// Try to get branch name separately for new repos
			branchCmd, buildErr := cmdBuilder.Build(context.Background(), "git", "rev-parse", "--abbrev-ref", "HEAD")
			if buildErr == nil {
				branchExec := branchCmd.Exec()
				branchExec.Dir = path
				branchOutput, runErr := branchExec.Output()
				if runErr == nil {
					status.Branch = strings.TrimSpace(string(branchOutput))
				}
			}
			return status, nil
		}
		return nil, fmt.Errorf("failed to get git status: %w, output: %s", err, outputStr)
	}

	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		// Parse header lines (start with '#')
		if strings.HasPrefix(line, "# ") {
			parts := strings.Fields(line)
			if len(parts) < 3 {
				continue
			}
			switch parts[1] {
			case "branch.head":
				status.Branch = parts[2]
			case "branch.upstream":
				status.HasUpstream = true
			case "branch.ab":
				// format is +<ahead> -<behind>
				if len(parts) > 2 {
					aheadStr := strings.TrimPrefix(parts[2], "+")
					status.AheadCount, _ = strconv.Atoi(aheadStr)
				}
				if len(parts) > 3 {
					behindStr := strings.TrimPrefix(parts[3], "-")
					status.BehindCount, _ = strconv.Atoi(behindStr)
				}
			}
			continue
		}

		// Parse file status lines
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		switch parts[0] {
		case "?": // Untracked
			status.UntrackedCount++
		case "1", "2": // Changed entries (1 for normal, 2 for rename/copy)
			if len(parts) < 2 {
				continue
			}
			xy := parts[1]
			if len(xy) < 2 {
				continue
			}
			staged := xy[0]
			working := xy[1]

			// Staged changes are indicated by any letter other than '.'
			if staged != '.' {
				status.StagedCount++
			}
			// Modified changes in the working tree (. means unchanged)
			if working != '.' {
				status.ModifiedCount++
			}
		case "u", "U": // Unmerged
			status.StagedCount++
			status.ModifiedCount++
		}
	}

	status.IsDirty = status.ModifiedCount > 0 || status.UntrackedCount > 0 || status.StagedCount > 0

	return status, nil
}

// GetCommitsDivergenceFromMain returns the number of commits a branch is ahead of and behind the local main/master branch.
func GetCommitsDivergenceFromMain(repoPath, currentBranch string) (ahead int, behind int) {
	cmdBuilder := command.NewSafeBuilder()

	// Determine if main or master exists
	mainBranch := ""
	for _, branchName := range []string{"main", "master"} {
		cmd, err := cmdBuilder.Build(context.Background(), "git", "show-ref", "--verify", "--quiet", "refs/heads/"+branchName)
		if err != nil {
			continue // Should not happen, but defensive
		}
		execCmd := cmd.Exec()
		execCmd.Dir = repoPath
		if execCmd.Run() == nil {
			mainBranch = branchName
			break
		}
	}

	if mainBranch == "" || currentBranch == mainBranch {
		return 0, 0
	}

	// git rev-list --left-right --count main...HEAD
	cmd, err := cmdBuilder.Build(context.Background(), "git", "rev-list", "--left-right", "--count", mainBranch+"...HEAD")
	if err != nil {
		return 0, 0
	}

	execCmd := cmd.Exec()
	execCmd.Dir = repoPath
	output, err := execCmd.Output()
	if err != nil {
		return 0, 0
	}

	parts := strings.Fields(string(output))
	if len(parts) == 2 {
		// output is: <behind> <ahead>
		behind, _ = strconv.Atoi(parts[0])
		ahead, _ = strconv.Atoi(parts[1])
	}

	return ahead, behind
}

// GetCommitsDivergenceFromRemoteMain returns the number of commits the local main/master branch
// is ahead of and behind origin/main or origin/master.
func GetCommitsDivergenceFromRemoteMain(repoPath, currentBranch string) (ahead int, behind int) {
	cmdBuilder := command.NewSafeBuilder()

	// Check if origin/main or origin/master exists
	remoteRef := ""
	for _, branchName := range []string{"main", "master"} {
		refPath := "refs/remotes/origin/" + branchName
		cmd, err := cmdBuilder.Build(context.Background(), "git", "show-ref", "--verify", "--quiet", refPath)
		if err != nil {
			continue
		}
		execCmd := cmd.Exec()
		execCmd.Dir = repoPath
		if execCmd.Run() == nil {
			remoteRef = "origin/" + branchName
			break
		}
	}

	if remoteRef == "" {
		return 0, 0
	}

	// git rev-list --left-right --count HEAD...origin/main
	cmd, err := cmdBuilder.Build(context.Background(), "git", "rev-list", "--left-right", "--count", "HEAD..."+remoteRef)
	if err != nil {
		return 0, 0
	}

	execCmd := cmd.Exec()
	execCmd.Dir = repoPath
	output, err := execCmd.Output()
	if err != nil {
		return 0, 0
	}

	parts := strings.Fields(string(output))
	if len(parts) == 2 {
		// output is: <ahead> <behind> (HEAD on left side)
		ahead, _ = strconv.Atoi(parts[0])
		behind, _ = strconv.Atoi(parts[1])
	}

	return ahead, behind
}