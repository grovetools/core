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

	"github.com/grovetools/core/util/pathutil"
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

	// Resolve the CANONICAL ecosystem root that owns gitRoot. Member source
	// checkouts MUST be resolved against this root's repos so the new worktree
	// snapshots the CURRENT STATE OF THE ECOSYSTEM (each member at the commit the
	// ecosystem root currently has it at), regardless of how many other checkouts
	// of a repo exist on disk.
	//
	// Two cases:
	//   - gitRoot IS the ecosystem root (non-anchored ecosystem-root create): the
	//     root is gitRoot itself.
	//   - gitRoot is a SUB-REPO (anchored create, `--anchor <sub-repo>`): walk up
	//     to the owning ecosystem root via findRootEcosystemPath. `--anchor` only
	//     changes base-dir placement (already handled by the caller); it must NEVER
	//     change which commit each member starts from.
	//
	// Without this, member sources were resolved via the collision-prone
	// LocalWorkspaces() map, which returns an ARBITRARY checkout per repo name when
	// several exist (the normal case) — producing a Frankenstein mix of commits
	// from unrelated branches/efforts that does not build.
	rootEcosystemPath := gitRoot
	if rootEco := findRootEcosystemPath(gitRoot); rootEco != "" {
		rootEcosystemPath = rootEco
	}

	// localWorkspaces is the per-repo source-checkout map used to resolve where a
	// member's `git worktree add` runs from. Prefer the ecosystem-scoped map keyed
	// to the canonical root so name collisions across ecosystems/worktrees cannot
	// select a non-canonical checkout. Fall back to the (deprecated) global map
	// only for the no-provider / nothing-scoped case so existing behavior for
	// standalone roots is preserved.
	localWorkspaces := provider.LocalWorkspacesInEcosystem(rootEcosystemPath)
	if len(localWorkspaces) == 0 {
		localWorkspaces = provider.LocalWorkspaces()
	}

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
		// Only include projects that are direct children of the canonical
		// ecosystem root. For a non-anchored ecosystem-root create this is gitRoot;
		// for an anchored create (gitRoot is a sub-repo) it is the owning ecosystem
		// root, so members are still discovered.
		//
		// Compare via pathutil.ComparePaths, NOT strings.EqualFold: discovery keeps
		// the raw path spelling (e.g. /var/...) while the ecosystem root is derived
		// from git and may be a realpath (/private/var/...). A plain EqualFold then
		// matches NOTHING under a symlinked root and silently drops every sibling
		// repo. ComparePaths resolves symlinks AND normalizes case (macOS
		// /Users/x/Code vs /Users/x/code), covering both failure modes.
		if same, _ := pathutil.ComparePaths(filepath.Dir(localPath), rootEcosystemPath); same {
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

		// Resolve the CANONICAL source checkout for this member: the repo directly
		// under the ecosystem root. This is what makes the new worktree snapshot
		// the ecosystem's CURRENT state (the root's submodule pointer for each
		// member), rather than an arbitrary checkout among many on disk.
		//
		// Resolution order, all rooted at the canonical ecosystem root:
		//   1. The member directly under the ecosystem root: <rootEcosystemPath>/<projectPath>.
		//      This is the parent/main checkout and the authoritative source.
		//   2. provider.FindSubProjectByName — provider-aware canonical resolution
		//      (handles spelling/symlink differences; never returns a worktree copy
		//      or a copy living inside an ecosystem worktree).
		//   3. The ecosystem-scoped localWorkspaces map (already filtered to the
		//      canonical root above), as a final fallback.
		// NOTE: filepath.Join(gitRoot, projectPath) is deliberately NOT used as the
		// primary source: when gitRoot is an anchor sub-repo it points at a sibling
		// dir that does not exist, and historically fell through to an arbitrary
		// checkout — the root cause of the Frankenstein-mix bug.
		mainProjectPath := filepath.Join(rootEcosystemPath, projectPath)

		// Prefer discovery's raw spelling for this member when it resolves to the
		// same physical location. rootEcosystemPath may be a realpath (/private/var)
		// while discovery recorded the raw spelling (/var); joining the realpath
		// would cascade the symlink-resolved form into the `git worktree add` source
		// and later os.Stat/provider checks. Recovering localPath keeps the source
		// path consistent with the rest of grove (registry/discovery). Gated on a
		// same-physical-path check so anchor/sub-repo cases (where the member does
		// NOT live at <root>/<name>) fall through untouched to the resolvers below.
		if localPath, ok := localWorkspaces[projectName]; ok {
			if same, _ := pathutil.ComparePaths(mainProjectPath, localPath); same {
				mainProjectPath = localPath
			}
		}

		// Skip if worktree already exists at target
		if _, err := os.Stat(filepath.Join(targetPath, ".git")); err == nil {
			continue
		}

		// 1. Canonical member directly under the ecosystem root.
		if _, err := os.Stat(filepath.Join(mainProjectPath, ".git")); err == nil {
			fmt.Printf("%s: creating linked worktree\n", projectName)
			if err := addWorktree(mainProjectPath, targetPath); err != nil {
				fmt.Printf("    Error: failed to create worktree for %s: %v\n", projectName, err)
				failedMembers = append(failedMembers, projectName)
			}
			continue
		}

		// 2. Provider-aware canonical resolution (handles the anchor sub-repo,
		//    which is gitRoot itself and may not sit at <rootEcosystemPath>/<name>
		//    spelling-for-spelling, plus any spelling/symlink variance).
		if provider != nil {
			if node := provider.FindSubProjectByName(projectName, rootEcosystemPath); node != nil {
				if _, err := os.Stat(filepath.Join(node.Path, ".git")); err == nil {
					fmt.Printf("%s: creating linked worktree\n", projectName)
					if err := addWorktree(node.Path, targetPath); err != nil {
						fmt.Printf("    Error: failed to create worktree for %s: %v\n", projectName, err)
						failedMembers = append(failedMembers, projectName)
					}
					continue
				}
			}
		}

		// 3. Ecosystem-scoped localWorkspaces fallback (already filtered to the
		//    canonical root). Covers the unified-container anchor mapping and any
		//    member whose on-disk location differs from <root>/<name> but is still
		//    within the canonical ecosystem.
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
			return fmt.Errorf("requested repo %q not found at %s or in local workspaces", projectName, filepath.Join(rootEcosystemPath, projectPath))
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
