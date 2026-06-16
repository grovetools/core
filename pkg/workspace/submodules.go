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
//
// gitRoot is the root of the repository that owns worktreePath. It must be
// passed explicitly: the worktree's location does not imply its owner.
func SetupSubmodules(ctx context.Context, worktreePath, gitRoot, branchName string, repos []string, provider *Provider, setupHandlers ...func(worktreePath, gitRoot string) error) error {
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

	// When specific repos are explicitly requested, treat that list as
	// authoritative: pre-populate the project map with any requested repo not
	// already discovered. This covers sibling/direct-child repos that lack a
	// grove.toml (and thus aren't in localWorkspaces) and aren't .gitmodules
	// entries either — e.g. a `workspaces=["*"]` ecosystem of independent repos.
	// For direct-child repos the relative path equals the repo name.
	//
	// This is gated on len(repos) > 0 so the empty case keeps its existing
	// "empty == all discovered projects" semantics (see the early return below).
	if len(repos) > 0 {
		for _, repo := range repos {
			// Unified-container case: the standalone repo IS the source root
			// (gitRoot itself), not a child of it. Map its source location to
			// gitRoot so the child worktree below is created from the right
			// place (worktree add of gitRoot INTO worktreePath/<repo>).
			if repo == filepath.Base(gitRoot) {
				projects[repo] = repo
				localWorkspaces[repo] = gitRoot
				continue
			}
			if _, alreadyPresent := projects[repo]; !alreadyPresent {
				projects[repo] = repo
			}
		}
	}

	// Parse .gitmodules to find uninitialized submodules not yet discovered
	// by the workspace provider (no grove.toml on disk yet).
	gitmodulesPath := filepath.Join(worktreePath, ".gitmodules")
	gitmoduleNames := make(map[string]bool)
	if submodulePaths, err := parseGitmodules(gitmodulesPath); err == nil {
		for name, path := range submodulePaths {
			gitmoduleNames[name] = true
			if _, alreadyDiscovered := projects[name]; !alreadyDiscovered {
				projects[name] = path
			}
		}
	}

	if len(projects) == 0 && len(repos) == 0 {
		return nil
	}

	var uninitializedSubmodules []string

	// addWorktree creates a linked worktree of memberRoot into targetPath. It
	// prunes stale/dangling worktree registrations in memberRoot BEFORE the add
	// so a leftover entry (e.g. one left by an `rm -rf` cleanup that never ran
	// `git worktree prune`, reported as "gitdir file points to non-existent
	// location") doesn't block a clean create. It returns the `git worktree add`
	// error so the caller can collect it — a swallowed add error here is exactly
	// what silently produced incomplete, non-hermetic containers.
	addWorktree := func(memberRoot, targetPath string) error {
		_ = os.MkdirAll(filepath.Dir(targetPath), 0o755)
		os.RemoveAll(targetPath)
		// Prune stale registrations in the member repo before adding. Best-effort:
		// a prune failure shouldn't block the add (the add error below is the
		// authoritative signal), but log it so the cause is visible.
		cmdPrune := exec.CommandContext(ctx, "git", "worktree", "prune")
		cmdPrune.Dir = memberRoot
		if out, err := cmdPrune.CombinedOutput(); err != nil {
			fmt.Printf("    Warning: failed to prune stale worktrees in %s: %v: %s\n", memberRoot, err, strings.TrimSpace(string(out)))
		}
		cmdWorktree := exec.CommandContext(ctx, "git", "worktree", "add", targetPath, "-B", branchName)
		cmdWorktree.Dir = memberRoot
		if out, err := cmdWorktree.CombinedOutput(); err != nil {
			return fmt.Errorf("%s", strings.TrimSpace(string(out)))
		}
		for _, handler := range setupHandlers {
			if err := handler(targetPath, memberRoot); err != nil {
				fmt.Printf("    Warning: setup handler failed for worktree %s: %v\n", targetPath, err)
			}
		}
		return nil
	}

	// failedMembers collects repos whose linked-worktree creation failed so we
	// can fail loud at the end rather than silently returning an incomplete,
	// non-hermetic container (the original bug: a missing repo was swallowed).
	var failedMembers []string

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
			if err := addWorktree(mainProjectPath, targetPath); err != nil {
				fmt.Printf("    Error: failed to create worktree for %s: %v\n", projectName, err)
				failedMembers = append(failedMembers, projectName)
			}
			continue
		}

		// Try via provider lookup (project may be elsewhere on disk)
		if localRepoPath, hasLocal := localWorkspaces[projectName]; hasLocal {
			fmt.Printf("%s: creating linked worktree\n", projectName)
			if err := addWorktree(localRepoPath, targetPath); err != nil {
				fmt.Printf("    Error: failed to create worktree for %s: %v\n", projectName, err)
				failedMembers = append(failedMembers, projectName)
			}
			continue
		}

		// Not found locally. If this repo was EXPLICITLY requested and is also
		// not a real .gitmodules entry, it can't be an uninitialized submodule
		// either — it's a typo or a missing repo. Hard-error rather than
		// silently `git submodule update`-ing nothing and leaving an empty dir.
		// A legitimate-but-uninitialized submodule (present in .gitmodules) still
		// falls through to the submodule-update path below.
		if repoFilter[projectName] && !gitmoduleNames[projectName] {
			return fmt.Errorf("requested repo %q not found at %s or in local workspaces", projectName, filepath.Join(gitRoot, projectPath))
		}

		// Not found locally — must be an uninitialized submodule
		uninitializedSubmodules = append(uninitializedSubmodules, projectPath)
	}

	// Initialize any submodules that weren't available locally
	for _, submodulePath := range uninitializedSubmodules {
		targetPath := filepath.Join(worktreePath, submodulePath)
		cmdUpdate := exec.CommandContext(ctx, "git", "submodule", "update", "--init", "--recursive", "--", submodulePath)
		cmdUpdate.Dir = worktreePath
		if err := cmdUpdate.Run(); err != nil {
			_ = os.MkdirAll(targetPath, 0o755)
		}
	}

	// Fail loud: if any member repo's linked worktree could not be created, the
	// resulting container is incomplete and non-hermetic (it won't build). The
	// original bug silently swallowed these failures and returned a partial
	// container with no signal. Return an error naming exactly which repos were
	// dropped so the caller (e.g. flow plan init) exits non-zero and the user
	// sees what's missing.
	if len(failedMembers) > 0 {
		return fmt.Errorf("incomplete worktree: failed to create linked worktree(s) for %d repo(s): %s",
			len(failedMembers), strings.Join(failedMembers, ", "))
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
		} else if currentName != "" && (strings.HasPrefix(line, "path =") || strings.HasPrefix(line, "path=")) {
			value := line
			value = strings.TrimPrefix(value, "path")
			value = strings.TrimSpace(value)
			value = strings.TrimPrefix(value, "=")
			value = strings.TrimSpace(value)
			value = strings.Trim(value, "\"")
			submodules[currentName] = value
		}
	}
	return submodules, scanner.Err()
}
