package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// UpdateWorktreeRepos rewrites the `repos:` block in a worktree's
// .grove/workspace marker to the given repo set, leaving every other key
// (branch/plan/created_at/owner/ecosystem) untouched.
//
// The marker is a line-based frozen format written at worktree creation by
// Prepare:
//
//	branch: <name>
//	plan: <name>
//	created_at: <rfc3339>
//	owner: <abs path>
//	ecosystem: <bool>
//	repos:
//	  - <repo>
//	  - <repo>
//
// This helper exists so `flow plan add-worktrees` can grow an existing
// ecosystem worktree's repo set on disk without re-deriving (and possibly
// reordering or dropping) the other keys. The repos: block is regenerated
// verbatim in the same indentation Prepare uses ("  - <repo>").
func UpdateWorktreeRepos(worktreePath string, repos []string) error {
	markerPath := filepath.Join(worktreePath, ".grove", "workspace")
	content, err := os.ReadFile(markerPath)
	if err != nil {
		return fmt.Errorf("failed to read workspace marker at %s: %w", markerPath, err)
	}

	// Keep every line that is NOT part of the existing repos: block. The
	// block is the `repos:` key line plus the indented `  - ` items that
	// immediately follow it; a non-list, non-blank line ends the block.
	var kept []string
	inReposBlock := false
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if inReposBlock {
			// Continuation of the repos list, or a trailing blank line
			// inside it: drop it.
			if trimmed == "" || strings.HasPrefix(trimmed, "- ") {
				continue
			}
			// A new key begins; the repos block is over.
			inReposBlock = false
		}
		if trimmed == "repos:" {
			inReposBlock = true
			continue
		}
		kept = append(kept, line)
	}

	// Drop any trailing blank lines so the rebuilt block sits flush at the end.
	for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
		kept = kept[:len(kept)-1]
	}

	var builder strings.Builder
	for _, line := range kept {
		builder.WriteString(line)
		builder.WriteString("\n")
	}
	if len(repos) > 0 {
		builder.WriteString("repos:\n")
		for _, repo := range repos {
			builder.WriteString(fmt.Sprintf("  - %s\n", repo))
		}
	}

	if err := os.WriteFile(markerPath, []byte(builder.String()), 0o644); err != nil { //nolint:gosec // workspace marker is not sensitive
		return fmt.Errorf("failed to write workspace marker at %s: %w", markerPath, err)
	}
	return nil
}
