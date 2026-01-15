package git

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mattsolo1/grove-core/command"
)

// ExtendedGitStatus holds git status including line changes.
type ExtendedGitStatus struct {
	*StatusInfo
	LinesAdded   int `json:"lines_added"`
	LinesDeleted int `json:"lines_deleted"`
}

// GetExtendedStatus fetches detailed git status including line changes.
func GetExtendedStatus(path string) (*ExtendedGitStatus, error) {
	cleanPath := filepath.Clean(path)
	if !IsGitRepo(cleanPath) {
		return nil, fmt.Errorf("not a git repository: %s", path)
	}

	status, err := GetStatus(cleanPath)
	if err != nil {
		return nil, err
	}

	extStatus := &ExtendedGitStatus{StatusInfo: status}

	if status.Branch != "main" && status.Branch != "master" {
		ahead, behind := GetCommitsDivergenceFromMain(cleanPath, status.Branch)
		status.AheadMainCount = ahead
		status.BehindMainCount = behind
	} else if !status.HasUpstream {
		// When on main/master without an upstream set, compare against origin/main or origin/master
		ahead, behind := GetCommitsDivergenceFromRemoteMain(cleanPath, status.Branch)
		status.AheadMainCount = ahead
		status.BehindMainCount = behind
	}

	cmdBuilder := command.NewSafeBuilder()
	cmd, err := cmdBuilder.Build(context.Background(), "git", "diff", "--numstat")
	if err != nil {
		return nil, fmt.Errorf("failed to build diff command: %w", err)
	}

	execCmd := cmd.Exec()
	execCmd.Dir = cleanPath
	output, err := execCmd.Output()
	if err == nil {
		extStatus.LinesAdded, extStatus.LinesDeleted = parseNumstat(string(output))
	}

	cmd, err = cmdBuilder.Build(context.Background(), "git", "diff", "--cached", "--numstat")
	if err != nil {
		return nil, fmt.Errorf("failed to build cached diff command: %w", err)
	}

	execCmd = cmd.Exec()
	execCmd.Dir = cleanPath
	output, err = execCmd.Output()
	if err == nil {
		stagedAdded, stagedDeleted := parseNumstat(string(output))
		extStatus.LinesAdded += stagedAdded
		extStatus.LinesDeleted += stagedDeleted
	}

	return extStatus, nil
}

func parseNumstat(output string) (added, deleted int) {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			if fields[0] != "-" {
				if a, err := strconv.Atoi(fields[0]); err == nil {
					added += a
				}
			}
			if fields[1] != "-" {
				if d, err := strconv.Atoi(fields[1]); err == nil {
					deleted += d
				}
			}
		}
	}
	return added, deleted
}
