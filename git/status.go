package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/grovetools/core/command"
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

// FileStatus describes a single changed path from `git status --porcelain=v2`.
// Staged is the 'X' column (index/staged state) and Working is the 'Y' column
// (working-tree state) from the porcelain v2 status code. For untracked entries
// both are set to '?'.
type FileStatus struct {
	Path    string
	Staged  rune // The 'X' column from porcelain v2 (e.g., 'M', 'A', 'D', 'R', '.')
	Working rune // The 'Y' column from porcelain v2 (e.g., 'M', 'D', '?', '.')

	// LinesAdded / LinesDeleted are the per-file numstat counts merged in by the
	// Get* functions (the parsers leave them zero). Binary files and untracked
	// paths, which `git diff --numstat` does not report, stay at zero.
	LinesAdded   int
	LinesDeleted int
}

// GetChangedFiles returns the per-file change list for the repository at the
// given path. Unlike GetStatus (which only counts), this preserves each
// changed path along with its X/Y status code so callers can render a
// browsable change tree.
//
// It runs `git status --porcelain=v2 -z -uall --ignore-submodules`. The -z flag
// NUL-delimits records (and the two halves of a rename record), which makes
// paths containing spaces unambiguous. -uall (--untracked-files=all) lists each
// untracked file individually rather than collapsing a brand-new directory into
// a single `? dir/` record — without it a new file in an otherwise-empty
// directory never reaches the change tree (it would surface only as a childless
// directory node).
func GetChangedFiles(path string) ([]FileStatus, error) {
	cmdBuilder := command.NewSafeBuilder()
	cmd, err := cmdBuilder.Build(context.Background(), "git", "status", "--porcelain=v2", "-z", "-uall", "--ignore-submodules")
	if err != nil {
		return nil, fmt.Errorf("failed to build command: %w", err)
	}
	execCmd := cmd.Exec()
	execCmd.Dir = path
	output, err := execCmd.Output()
	if err != nil {
		outputStr := string(output)
		if strings.Contains(outputStr, "not a git repository") {
			return nil, fmt.Errorf("not a git repository: %s", path)
		}
		// A new repo before its first commit has no changes to enumerate.
		if strings.Contains(outputStr, "No commits yet") {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get git status: %w, output: %s", err, outputStr)
	}

	files := parseChangedFiles(string(output))

	// Merge per-file line counts. Working-tree changes span both the unstaged
	// (`git diff`) and staged (`git diff --cached`) sets, summed per path so a
	// file with both shows its combined churn — matching the aggregate the
	// sessionizer CHANGES column computes in GetExtendedStatus.
	stats := getNumstatZ(path)
	addNumstat(stats, getNumstatZ(path, "--cached"))
	applyNumstat(files, stats)

	return files, nil
}

// getNumstatZ runs `git diff --numstat -z [args...]` in repoPath and returns a
// map of repo-relative path -> {added, deleted}. It is best-effort: any failure
// yields an empty map so callers degrade to zero line counts rather than error.
func getNumstatZ(repoPath string, args ...string) map[string][2]int {
	cmdBuilder := command.NewSafeBuilder()
	full := append([]string{"diff", "--numstat", "-z"}, args...)
	cmd, err := cmdBuilder.Build(context.Background(), "git", full...)
	if err != nil {
		return map[string][2]int{}
	}
	execCmd := cmd.Exec()
	execCmd.Dir = repoPath
	output, err := execCmd.Output()
	if err != nil {
		return map[string][2]int{}
	}
	return parseNumstatZ(string(output))
}

// parseNumstatZ parses NUL-delimited `git diff --numstat -z` output into a map
// of path -> {added, deleted}. Each ordinary record is "<add>\t<del>\t<path>";
// a rename emits "<add>\t<del>\t" followed by two more NUL records (old path,
// new path), and the new path is kept. Binary files report "-" for the counts,
// which parse to zero. Split out from getNumstatZ for unit testing.
func parseNumstatZ(output string) map[string][2]int {
	result := make(map[string][2]int)
	records := strings.Split(output, "\x00")

	for i := 0; i < len(records); {
		record := records[i]
		if record == "" {
			i++
			continue
		}
		parts := strings.SplitN(record, "\t", 3)
		if len(parts) < 3 {
			i++
			continue
		}
		added := numstatCount(parts[0])
		deleted := numstatCount(parts[1])

		path := parts[2]
		if path == "" {
			// Rename/copy: the two following records are old then new path.
			if i+2 >= len(records) {
				break
			}
			path = records[i+2]
			i += 3
		} else {
			i++
		}
		result[path] = [2]int{added, deleted}
	}

	return result
}

// numstatCount parses a numstat count field, mapping the binary "-" sentinel to 0.
func numstatCount(field string) int {
	if field == "-" {
		return 0
	}
	n, err := strconv.Atoi(field)
	if err != nil {
		return 0
	}
	return n
}

// addNumstat sums src into dst per path (used to combine staged + unstaged).
func addNumstat(dst, src map[string][2]int) {
	for path, c := range src {
		cur := dst[path]
		dst[path] = [2]int{cur[0] + c[0], cur[1] + c[1]}
	}
}

// applyNumstat copies the merged per-path counts onto the matching FileStatus
// entries (paths are repo-relative with forward slashes in both sources).
func applyNumstat(files []FileStatus, stats map[string][2]int) {
	for i := range files {
		if c, ok := stats[files[i].Path]; ok {
			files[i].LinesAdded = c[0]
			files[i].LinesDeleted = c[1]
		}
	}
}

// parseChangedFiles parses NUL-delimited `git status --porcelain=v2 -z` output
// into a flat list of FileStatus. It is split out from GetChangedFiles so it
// can be unit-tested without invoking git.
func parseChangedFiles(output string) []FileStatus {
	records := strings.Split(output, "\x00")
	var files []FileStatus

	for i := 0; i < len(records); i++ {
		record := records[i]
		if record == "" {
			continue
		}

		switch {
		case strings.HasPrefix(record, "1 "):
			// Ordinary: "1 <XY> <sub> <mH> <mI> <mW> <hH> <hI> <path>"
			parts := strings.SplitN(record, " ", 9)
			if len(parts) < 9 {
				continue
			}
			files = append(files, fileStatusFromXY(parts[1], parts[8]))

		case strings.HasPrefix(record, "2 "):
			// Rename/copy: "2 <XY> <sub> <mH> <mI> <mW> <hH> <hI> <Xscore> <path>"
			// followed by a separate NUL-delimited record holding the ORIGINAL
			// path, which must be consumed so it isn't parsed on its own.
			parts := strings.SplitN(record, " ", 10)
			if len(parts) < 10 {
				continue
			}
			files = append(files, fileStatusFromXY(parts[1], parts[9]))
			i++ // skip the original-path record

		case strings.HasPrefix(record, "u "):
			// Unmerged: "u <XY> <sub> <m1> <m2> <m3> <mW> <h1> <h2> <h3> <path>"
			parts := strings.SplitN(record, " ", 11)
			if len(parts) < 11 {
				continue
			}
			files = append(files, fileStatusFromXY(parts[1], parts[10]))

		case strings.HasPrefix(record, "? "):
			// Untracked: "? <path>" — the whole remainder is the path.
			files = append(files, FileStatus{
				Path:    record[2:],
				Staged:  '?',
				Working: '?',
			})
		}
	}

	return files
}

// fileStatusFromXY builds a FileStatus from a porcelain v2 "XY" code and path.
func fileStatusFromXY(xy, path string) FileStatus {
	fs := FileStatus{Path: path}
	if len(xy) >= 2 {
		fs.Staged = rune(xy[0])
		fs.Working = rune(xy[1])
	}
	return fs
}

// localMainBranch returns "main" or "master" — whichever local branch exists
// in the repo — or "" if neither is present. It is the single source of truth
// for resolving the local main ref, shared by the divergence counters
// (AheadMainCount/BehindMainCount) and the since-main change list.
func localMainBranch(repoPath string) string {
	cmdBuilder := command.NewSafeBuilder()
	for _, branchName := range []string{"main", "master"} {
		cmd, err := cmdBuilder.Build(context.Background(), "git", "show-ref", "--verify", "--quiet", "refs/heads/"+branchName)
		if err != nil {
			continue // Should not happen, but defensive
		}
		execCmd := cmd.Exec()
		execCmd.Dir = repoPath
		if execCmd.Run() == nil {
			return branchName
		}
	}
	return ""
}

// GetChangedFilesSinceMain returns the per-file change list between the local
// main/master branch and the working tree — i.e. everything that differs from
// main, including work already committed on the current branch. This mirrors
// the "git status since local main" behavior used by neo-tree's <leader>gm.
//
// It runs `git diff --name-status -z <mainBranch>` (two-dot: main vs working
// tree). The main ref is resolved identically to the AheadMainCount/
// BehindMainCount counters via localMainBranch. If neither main nor master
// exists locally there is nothing to compare against, so it returns nil,nil.
func GetChangedFilesSinceMain(repoPath string) ([]FileStatus, error) {
	mainBranch := localMainBranch(repoPath)
	if mainBranch == "" {
		return nil, nil
	}

	cmdBuilder := command.NewSafeBuilder()
	cmd, err := cmdBuilder.Build(context.Background(), "git", "diff", "--name-status", "-z", mainBranch)
	if err != nil {
		return nil, fmt.Errorf("failed to build command: %w", err)
	}
	execCmd := cmd.Exec()
	execCmd.Dir = repoPath
	output, err := execCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to diff against %s: %w, output: %s", mainBranch, err, string(output))
	}

	files := parseDiffNameStatusZ(string(output))

	// Per-file churn for the since-main diff: a single numstat pass against the
	// same base (committed + working-tree delta from main).
	applyNumstat(files, getNumstatZ(repoPath, mainBranch))

	return files, nil
}

// parseDiffNameStatusZ parses NUL-delimited `git diff --name-status -z` output
// into a flat list of FileStatus. Unlike porcelain status there is no separate
// staged/working pair — diff emits a single status code per path — so Working
// holds the code and Staged is set to '.' (the icon mapping in gitStatusIcon
// keys off either column, so a single code renders correctly).
//
// Records are NUL-separated: ordinary changes emit "<status>\0<path>", while
// renames/copies emit "<status>\0<oldpath>\0<newpath>" — both halves must be
// consumed and the NEW path is kept. It is split out from GetChangedFilesSinceMain
// so it can be unit-tested without invoking git.
func parseDiffNameStatusZ(output string) []FileStatus {
	records := strings.Split(output, "\x00")
	var files []FileStatus

	for i := 0; i < len(records); {
		status := records[i]
		if status == "" {
			i++
			continue
		}
		statusChar := status[0]

		// Rename/copy: status token, then OLD path, then NEW path.
		if statusChar == 'R' || statusChar == 'C' {
			if i+2 >= len(records) {
				break
			}
			files = append(files, FileStatus{Path: records[i+2], Working: rune(statusChar), Staged: '.'})
			i += 3
			continue
		}

		// Ordinary: status token, then path.
		if i+1 >= len(records) {
			break
		}
		files = append(files, FileStatus{Path: records[i+1], Working: rune(statusChar), Staged: '.'})
		i += 2
	}

	return files
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
func GetCommitsDivergenceFromMain(repoPath, currentBranch string) (ahead, behind int) {
	cmdBuilder := command.NewSafeBuilder()

	// Determine if main or master exists (shared resolution).
	mainBranch := localMainBranch(repoPath)
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
func GetCommitsDivergenceFromRemoteMain(repoPath, currentBranch string) (ahead, behind int) {
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
