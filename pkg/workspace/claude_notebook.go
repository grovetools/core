// IMPLEMENTATION PLAN — seed worktree .claude/settings.local.json with the
// UNION of every member repo's paired-notebook directory, for both no-prompt
// out-of-tree READS and OS-sandbox WRITES.
//
// PROBLEM. A grove ecosystem worktree (e.g.
//   /Users/.../grove/worktrees/<repo>-<hash>/<name>) holds MANY member repos as
// subdirs; EACH has its own paired notebook OUTSIDE the worktree (e.g.
//   ~/notebooks/grovetools/workspaces/<repo>/). Flow agents read briefings/
// plans/concepts and write .artifacts/ there. Two walls block this:
//   (1) out-of-tree reads prompt in default permission mode, and
//   (2) under /sandbox the writable boundary is (working dir + temp), so the
//       notebook is unwritable.
// We need the UNION of ALL member repos' notebook dirs in BOTH
//   permissions.additionalDirectories (reads) AND
//   sandbox.filesystem.allowWrite (writes), in the LOCAL gitignored scope,
//   merged non-destructively, recomputed when the worktree grows.
//
// SEAM / FILE:LINE ANCHORS (verified against this worktree).
//   - Worktree-creation hook point: workspace.Prepare, core/pkg/workspace/
//     prepare.go — the `if created { ... }` block (~66) already provisions the
//     marker, registry entry, and Claude trust pre-seed (~145-170). We add a
//     SeedNotebookDirsForWorktree call at the END of that block, right after
//     claudetrust.SeedTrust (~168), reusing the `provider` already discovered
//     at prepare.go ~80 and opts.SiblingWorkspaces (the member repos).
//   - add-worktrees path: flow/cmd/plan_add_worktrees.go runPlanAddWorktrees —
//     after workspace.UpdateWorktreeRepos(worktreePath, union) (~210), call the
//     same resolver with the FULL `union` and the `provider` built at ~141, so
//     newly-linked repos contribute their notebooks. Idempotent + additive, so
//     re-running is safe.
//   - NotebookLocator call: NewNotebookLocator(coreConfig) + GetNotesDir(node,
//     "inbox") -> filepath.Dir gives the repo's notebook root
//     (workspaces/<repo>), where .artifacts/plans/concepts live. core/pkg/
//     workspace/notebook_locator.go GetNotesDir ~203 / GetAllContentDirs ~282.
//   - Notebook mapping: per-member-repo subdir is resolved via
//     GetProjectByPath(<worktree>/<repo>), whose discovery assigns NotebookName
//     through the anchored-worktree / origin-grove rules (lookup.go
//     assignNotebookName ~50). Verified live: `nb context --json` from
//     <worktree>/nb resolves notebook_name=grovetools and paths under
//     ~/notebooks/grovetools/workspaces/nb/, exactly the union member we want.
//   - Settings-merge helper: claudenotebook.SeedNotebookDirs (core/pkg/
//     claudenotebook/seeder.go) — leaf pkg, atomic tmp+rename, additive merge
//     mirroring hooks/commands/install.go mergeHooks; this workspace file is the
//     ONLY caller (workspace already imports config+claudenotebook; the leaf
//     does NOT import workspace, breaking the cycle the same way claudetrust
//     does).
//
// MINIMAL CHANGE.
//   1. core/pkg/claudenotebook/seeder.go — leaf JSON merger (additive,
//      non-destructive, atomic) writing both keys. [new]
//   2. THIS file — SeedNotebookDirsForWorktree(worktreePath, repos, provider):
//      resolve each member repo's notebook root via NotebookLocator, dedupe,
//      hand to claudenotebook.SeedNotebookDirs. [new]
//   3. prepare.go — one call at the end of the created block. [1 line + warn]
//   4. flow/cmd/plan_add_worktrees.go — one call after UpdateWorktreeRepos.
//      [1 line + warn]
//
// EDGE CASES.
//   - Empty notebook / unresolvable repo: GetProjectByPath or GetNotesDir error
//     -> skip that repo (best-effort, never fail worktree creation).
//   - Duplicate roots: two repos could map to the same notebook; deduped in
//     claudenotebook.dedupeNonEmpty (+ sorted for deterministic output).
//   - Missing settings.local.json: created from {} by the seeder.
//   - Lightweight/empty submodule slots: an unchecked-out member subdir won't
//     exist on disk, so GetProjectByPath errors -> skipped. Only LINKED repos
//     (the ones actually passed in `repos`) contribute, which is correct.
//   - Legacy vs XDG layout: resolution is layout-independent — it keys off the
//     member subdir's resolved WorkspaceNode, not the container path shape.
//   - Local-mode notebooks (no centralized root_dir): GetNotesDir returns an
//     IN-TREE .notebook path under the worktree; that's already inside the
//     working dir, so seeding it is harmless (and still a valid read/write dir).
//
// CONCEPT-COVERAGE GAPS (flagged, not guessed).
//   - The six concept overviews describe NotebookLocator's ROLE and the
//     anchored-worktree NotebookName assignment, but DO NOT specify which
//     locator method yields the "paired notebook directory" as a single root.
//     There is no GetNotebookRootDir(); the overviews list GetPlansDir/
//     GetAllContentDirs only. RESOLUTION: derived empirically — the per-note-type
//     dirs share parent workspaces/<repo>, so filepath.Dir(GetNotesDir(node,
//     "inbox")) is the paired root. Verified against live `nb context --json`.
//   - cc-settings-model documents additionalDirectories + sandbox.filesystem in
//     the READ model (ComputeFilesystemBoundary) but the overview does NOT state
//     the exact WRITE-boundary key name; `sandbox.filesystem.allowWrite` is
//     taken from the briefing's explicit spec.
//
// TEST PLAN.
//   - claudenotebook (leaf): table tests for merge into (a) missing file, (b)
//     existing file with unrelated keys + user dirs (preserve + append), (c)
//     duplicate inputs (dedupe), (d) malformed JSON (error, file untouched),
//     (e) gate-off no-op, (f) both keys written, (g) atomic (no .tmp leak).
//   - workspace resolver: unit test that resolveNotebookDirsForRepos dedupes
//     and skips unresolvable repos (driven via an injectable locator/node
//     resolver to avoid real discovery in the unit test).

package workspace

import (
	"path/filepath"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/claudenotebook"
)

// SeedNotebookDirsForWorktree resolves the paired notebook directory of every
// member repo linked into the worktree and seeds them into the worktree's
// .claude/settings.local.json (both no-prompt reads and sandbox writes). It is
// best-effort: a repo that can't be resolved is skipped, and the whole call
// never returns a hard error from missing notebooks — only IO/parse failures of
// the settings file propagate (so the caller can warn).
//
// repos is the set of member-repo subdir names (e.g. ["core","nb"]) linked into
// worktreePath. provider may be nil; resolution falls back to per-path
// classification when it is.
func SeedNotebookDirsForWorktree(worktreePath string, repos []string, provider *Provider) error {
	dirs := resolveNotebookDirsForRepos(worktreePath, repos, provider)
	if len(dirs) == 0 {
		return nil
	}
	return claudenotebook.SeedNotebookDirs(worktreePath, dirs)
}

// resolveNotebookDirsForRepos maps each member repo subdir to its paired
// notebook root directory. The result is NOT deduped here (the seeder dedupes
// and sorts), but unresolvable repos are silently dropped.
func resolveNotebookDirsForRepos(worktreePath string, repos []string, provider *Provider) []string {
	// Load core config once so the locator can resolve centralized notebook
	// roots. A nil config degrades to the locator's built-in default, which is
	// still a valid (if non-ideal) directory.
	cfg, _ := config.LoadFrom(worktreePath)
	locator := NewNotebookLocator(cfg)

	var dirs []string
	for _, repo := range repos {
		if repo == "" {
			continue
		}
		repoPath := filepath.Join(worktreePath, repo)
		node := resolveRepoNode(repoPath, provider)
		if node == nil {
			continue
		}
		root := notebookRootForNode(locator, node)
		if root != "" {
			dirs = append(dirs, root)
		}
	}
	return dirs
}

// resolveRepoNode returns the WorkspaceNode for a member-repo subdir, preferring
// the provider's in-memory index (already enriched with NotebookName via the
// anchored-worktree rules) and falling back to per-path classification.
func resolveRepoNode(repoPath string, provider *Provider) *WorkspaceNode {
	if provider != nil {
		if node := provider.FindByPath(repoPath); node != nil {
			return node
		}
	}
	node, err := GetProjectByPath(repoPath)
	if err != nil {
		return nil
	}
	return node
}

// notebookRootForNode resolves the paired-notebook ROOT directory for a node:
// the parent of the per-note-type directories (workspaces/<repo>), where
// .artifacts/, plans/, and concepts/ live. It derives this from the notes dir
// because the locator exposes no single "root" accessor (see the concept-
// coverage gap note at the top of this file).
func notebookRootForNode(locator *NotebookLocator, node *WorkspaceNode) string {
	notesInbox, err := locator.GetNotesDir(node, "inbox")
	if err != nil || notesInbox == "" {
		return ""
	}
	// GetNotesDir(node,"inbox") = <root>/workspaces/<repo>/inbox (centralized)
	// or <repo>/.notebook/notes/inbox (local). The paired notebook root is the
	// grandparent for centralized (drop notes/inbox -> workspaces/<repo>) — but
	// since local mode nests one extra level (.notebook/notes), we normalize by
	// taking the directory that directly contains the note-type subdirs and
	// going up to its workspace root.
	//
	// For both modes filepath.Dir twice from the inbox dir yields the workspace
	// root:
	//   centralized: <root>/workspaces/<repo>/inbox -> Dir -> .../<repo>  (the
	//                notes dir is flat: GetNotesDir returns workspaces/<repo>/inbox,
	//                so a single Dir already gives workspaces/<repo>).
	// Empirically (verified via `nb context --json`) the per-type dirs are
	// siblings directly under workspaces/<repo>, so ONE Dir is correct for
	// centralized mode. Local mode returns <repo>/.notebook/notes/inbox; one
	// Dir gives .../notes which is still a valid in-tree dir to grant. We keep a
	// single Dir to match the dominant centralized layout.
	return filepath.Dir(notesInbox)
}
