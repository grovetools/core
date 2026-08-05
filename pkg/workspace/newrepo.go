package workspace

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ReadmeFileName is the file CreateRepo writes so a fresh repository has
// something to commit and something to show.
const ReadmeFileName = "README.md"

// ValidateRepoName reports whether name is usable as a new repository's
// directory name: one path segment, no separators, no leading dot or dash.
func ValidateRepoName(name string) error {
	switch {
	case name == "":
		return fmt.Errorf("repository name cannot be empty")
	case name == "." || name == "..":
		return fmt.Errorf("repository name %q is not a directory name", name)
	case strings.ContainsAny(name, `/\`):
		return fmt.Errorf("repository name must be one path segment, got %q", name)
	case filepath.Base(name) != name:
		return fmt.Errorf("repository name must be one path segment, got %q", name)
	case strings.HasPrefix(name, "."):
		return fmt.Errorf("repository name cannot start with a dot: %q", name)
	case strings.HasPrefix(name, "-"):
		return fmt.Errorf("repository name cannot start with a dash: %q", name)
	}
	return nil
}

// CreateRepo initializes a minimal git repository at parentDir/name: a `.git`
// and an empty README, nothing else.
//
// Deliberately no grove.toml. An ecosystem whose `workspaces` glob covers the
// new directory already promotes a plain git repo to a discovered project (see
// the promotion pass in discover.go), so the marker would only be ceremony —
// and a repo that needs project config can grow one later.
//
// The initial commit is best-effort: a machine with no git identity configured
// still gets a usable repository, just with the README staged in the working
// tree rather than committed. committed reports which happened. A failure
// AFTER the directory is created removes it again rather than leaving a
// half-initialized directory behind.
func CreateRepo(parentDir, name string) (path string, committed bool, err error) {
	if err := ValidateRepoName(name); err != nil {
		return "", false, err
	}
	info, statErr := os.Stat(parentDir)
	if statErr != nil {
		return "", false, fmt.Errorf("parent directory unavailable: %w", statErr)
	}
	if !info.IsDir() {
		return "", false, fmt.Errorf("parent path is not a directory: %s", parentDir)
	}

	repoPath := filepath.Join(parentDir, name)
	if _, err := os.Stat(repoPath); err == nil {
		return "", false, fmt.Errorf("%s already exists", repoPath)
	} else if !os.IsNotExist(err) {
		return "", false, fmt.Errorf("checking %s: %w", repoPath, err)
	}

	if err := os.Mkdir(repoPath, 0o755); err != nil {
		return "", false, fmt.Errorf("creating %s: %w", repoPath, err)
	}
	cleanup := func(cause error) (string, bool, error) {
		_ = os.RemoveAll(repoPath)
		return "", false, cause
	}

	if out, err := runIn(repoPath, "git", "init", "-b", "main"); err != nil {
		// Git before 2.28 has no -b. Retry without it rather than refusing.
		if out2, err2 := runIn(repoPath, "git", "init"); err2 != nil {
			return cleanup(fmt.Errorf("git init failed: %w: %s", err, strings.TrimSpace(out+out2)))
		}
	}

	if err := os.WriteFile(filepath.Join(repoPath, ReadmeFileName), nil, 0o644); err != nil {
		return cleanup(fmt.Errorf("writing %s: %w", ReadmeFileName, err))
	}

	if _, err := runIn(repoPath, "git", "add", ReadmeFileName); err == nil {
		if _, err := runIn(repoPath, "git", "commit", "-m", "Initial commit"); err == nil {
			committed = true
		}
	}
	return repoPath, committed, nil
}

func runIn(dir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
