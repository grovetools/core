package workspace

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

// SetupSubmodules creates linked worktrees for ecosystem sub-projects.
// The workspace provider is the primary source of truth for what projects exist.
// .gitmodules is only used as a fallback to initialize submodules that haven't
// been cloned yet (and thus aren't discoverable by the provider).
func SetupSubmodules(ctx context.Context, worktreePath, branchName string, repos []string, provider *Provider, setupHandlers ...func(worktreePath, gitRoot string) error) error {
	// If no provider is given, create a temporary one
	if provider == nil {
		logger := logrus.New()
		logger.SetOutput(os.Stderr)
		logger.SetLevel(logrus.WarnLevel)
		ds := NewDiscoveryService(logger)
		result, err := ds.DiscoverAll()
		if err != nil {
			return fmt.Errorf("failed to discover workspaces: %w", err)
		}
		provider = NewProvider(result)
	}

	gitRoot := filepath.Dir(filepath.Dir(worktreePath))
	localWorkspaces := provider.LocalWorkspaces()

	repoFilter := make(map[string]bool)
	if len(repos) > 0 {
		for _, repo := range repos {
			repoFilter[repo] = true
		}
	}

	// Build the project list from discovered workspaces.
	// This includes both submodules and non-submodule projects (e.g., private
	// repos that are gitignored but present locally).
	projects := make(map[string]string) // name -> relative path within ecosystem
	for name, localPath := range localWorkspaces {
		// Only include projects that are direct children of this ecosystem root.
		// Use case-insensitive comparison for macOS where /Users/x/Code and
		// /Users/x/code refer to the same directory but have different cases
		// depending on how the path was resolved.
		if strings.EqualFold(filepath.Dir(localPath), gitRoot) {
			// Use filepath.Base instead of filepath.Rel since we already know
			// it's a direct child. filepath.Rel fails when paths differ only
			// in case (common on macOS case-insensitive filesystems).
			projects[name] = filepath.Base(localPath)
		}
	}

	// Parse .gitmodules to find uninitialized submodules not yet discovered
	// by the workspace provider (no grove.toml on disk yet).
	gitmodulesPath := filepath.Join(worktreePath, ".gitmodules")
	if submodulePaths, err := parseGitmodules(gitmodulesPath); err == nil {
		for name, path := range submodulePaths {
			if _, alreadyDiscovered := projects[name]; !alreadyDiscovered {
				projects[name] = path
			}
		}
	}

	if len(projects) == 0 && len(repos) == 0 {
		return nil
	}

	var uninitializedSubmodules []string

	for projectName, projectPath := range projects {
		if len(repoFilter) > 0 && !repoFilter[projectName] {
			fmt.Printf("%s: skipping (not in repos filter)\n", projectName)
			continue
		}

		targetPath := filepath.Join(worktreePath, projectPath)
		mainProjectPath := filepath.Join(gitRoot, projectPath)

		// Skip if worktree already exists at target
		if _, err := os.Stat(filepath.Join(targetPath, ".git")); err == nil {
			continue
		}

		// Try to create a linked worktree from the main checkout
		if _, err := os.Stat(filepath.Join(mainProjectPath, ".git")); err == nil {
			fmt.Printf("%s: creating linked worktree\n", projectName)
			os.MkdirAll(filepath.Dir(targetPath), 0o755)
			os.RemoveAll(targetPath)
			cmdWorktree := exec.CommandContext(ctx, "git", "worktree", "add", targetPath, "-B", branchName)
			cmdWorktree.Dir = mainProjectPath
			if err := cmdWorktree.Run(); err == nil {
				for _, handler := range setupHandlers {
					if err := handler(targetPath, mainProjectPath); err != nil {
						fmt.Printf("    Warning: setup handler failed for worktree %s: %v\n", targetPath, err)
					}
				}
			}
			continue
		}

		// Try via provider lookup (project may be elsewhere on disk)
		if localRepoPath, hasLocal := localWorkspaces[projectName]; hasLocal {
			fmt.Printf("%s: creating linked worktree\n", projectName)
			os.MkdirAll(filepath.Dir(targetPath), 0o755)
			os.RemoveAll(targetPath)
			cmdWorktree := exec.CommandContext(ctx, "git", "worktree", "add", targetPath, "-B", branchName)
			cmdWorktree.Dir = localRepoPath
			if err := cmdWorktree.Run(); err == nil {
				for _, handler := range setupHandlers {
					if err := handler(targetPath, localRepoPath); err != nil {
						fmt.Printf("    Warning: setup handler failed for worktree %s: %v\n", targetPath, err)
					}
				}
			}
			continue
		}

		// Not found locally — must be an uninitialized submodule
		uninitializedSubmodules = append(uninitializedSubmodules, projectPath)
	}

	// Initialize any submodules that weren't available locally
	for _, submodulePath := range uninitializedSubmodules {
		cmdUpdate := exec.CommandContext(ctx, "git", "submodule", "update", "--init", "--recursive", "--", submodulePath)
		cmdUpdate.Dir = worktreePath
		cmdUpdate.Run()
	}
	return nil
}

func parseGitmodules(gitmodulesPath string) (map[string]string, error) {
	file, err := os.Open(gitmodulesPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	submodules := make(map[string]string)
	var currentName string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[submodule") {
			start, end := strings.Index(line, "\""), strings.LastIndex(line, "\"")
			if start != -1 && end != -1 && start < end {
				currentName = line[start+1 : end]
			}
		} else if strings.HasPrefix(line, "path =") && currentName != "" {
			submodules[currentName] = strings.TrimSpace(strings.TrimPrefix(line, "path ="))
		}
	}
	return submodules, scanner.Err()
}
