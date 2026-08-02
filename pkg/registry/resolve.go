package registry

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/workspace"
)

// Subscription returns the registry-role sync subscription, or nil when this
// machine has none.
//
// The role, not the workspace name, is what makes a workspace the registry.
// `registry` is only the working name: an operator who calls it something else
// still gets registry semantics, and a workspace literally named "registry"
// with no role declared is an ordinary push-only subscription.
func Subscription(syncCfg *config.SyncConfig) *config.SyncWorkspace {
	if syncCfg == nil {
		return nil
	}
	for i := range syncCfg.Workspaces {
		if syncCfg.Workspaces[i].Role == config.SyncRoleRegistry {
			return &syncCfg.Workspaces[i]
		}
	}
	return nil
}

// WorkspaceRoot resolves the on-disk root of a subscribed notebook workspace
// from config alone, with no existence requirement.
//
// It mirrors SyncHandler.syntheticNodeFor + nodeWorkspaceRoot in the daemon,
// which is what the writer uses (the daemon owns those and the contract names
// them). This copy exists so the READ surfaces — `grove machines`, the treemux
// panel — can find the registry without a running daemon, which is the whole
// point of reading the replicated files directly. The two must agree; the
// preference order below is the same one, in the same order, for that reason:
//
//  1. a notebook definition whose resolved root already exists on disk;
//  2. the notebook referenced by a configured grove, groves visited in sorted
//     order for determinism;
//  3. no notebook name at all — the locator then falls back to
//     notebooks.rules.default and the builtin default.
func WorkspaceRoot(cfg *config.Config, name string) string {
	if name == "" {
		return ""
	}
	if root := existingDefinitionRoot(cfg, name); root != "" {
		return root
	}
	if cfg != nil && cfg.Notebooks != nil && cfg.Notebooks.Definitions != nil {
		for _, groveName := range slices.Sorted(maps.Keys(cfg.Groves)) {
			nb := cfg.Groves[groveName].Notebook
			if nb == "" {
				continue
			}
			if _, ok := cfg.Notebooks.Definitions[nb]; !ok {
				continue
			}
			return rootForNode(cfg, &workspace.WorkspaceNode{Name: name, NotebookName: nb})
		}
	}
	return rootForNode(cfg, &workspace.WorkspaceNode{Name: name})
}

// rootForNode is the shared "node → workspace root" step: the notes content dir
// the locator resolves, walked back to the workspace root. Returns "" when the
// locator fails or resolves a non-absolute path (a local-mode notebook has no
// project path to anchor to).
func rootForNode(cfg *config.Config, node *workspace.WorkspaceNode) string {
	notesDir, err := workspace.NewNotebookLocator(cfg).GetNotesDir(node, "inbox")
	if err != nil || !filepath.IsAbs(notesDir) {
		return ""
	}
	return workspaceRootForDir(filepath.Dir(notesDir))
}

// existingDefinitionRoot is rung 1 on its own: the first DECLARED notebook, in
// sorted order, whose resolved root for this workspace is already a directory.
// Split out because PlannedRoot needs exactly this rung and must not be allowed
// to fall through to the locator's builtin default.
func existingDefinitionRoot(cfg *config.Config, name string) string {
	if cfg == nil || cfg.Notebooks == nil || len(cfg.Notebooks.Definitions) == 0 {
		return ""
	}
	for _, defName := range slices.Sorted(maps.Keys(cfg.Notebooks.Definitions)) {
		root := rootForNode(cfg, &workspace.WorkspaceNode{Name: name, NotebookName: defName})
		if root == "" {
			continue
		}
		if fi, err := os.Stat(root); err == nil && fi.IsDir() {
			return root
		}
	}
	return ""
}

// workspaceRootForDir derives the workspace root a content dir belongs to.
// Centralized notebook layouts follow <root>/workspaces/<name>/...; without
// that marker the content dir's parent is the best available root. Kept
// byte-identical to the daemon watcher's private copy of the same rule.
func workspaceRootForDir(dir string) string {
	marker := string(filepath.Separator) + "workspaces" + string(filepath.Separator)
	if idx := strings.LastIndex(dir, marker); idx >= 0 {
		rest := dir[idx+len(marker):]
		if slash := strings.IndexByte(rest, filepath.Separator); slash > 0 {
			return dir[:idx+len(marker)+slash]
		}
		return dir
	}
	return filepath.Dir(dir)
}

// PlannedRoot resolves where a workspace that does NOT exist yet should be
// created, and is what `grove join` needs before it can make the registry
// resolvable at all.
//
// WorkspaceRoot has a chicken-and-egg property that only bites the creating
// caller: its first and strongest rung prefers a notebook whose resolved root
// already exists on disk, and its last rung passes NO notebook name, which
// sends the locator to its hardcoded `~/.grove/notebooks/nb` default. A verb
// that used it to decide where to MkdirAll would therefore create the registry
// under that default rather than under the notebook the machine configured —
// on a machine with notebooks declared, in a directory nothing else reads.
//
// So this names a notebook explicitly:
//
//  1. a DECLARED notebook whose root for this workspace already exists — the
//     same rung WorkspaceRoot leads with, so re-running converges;
//  2. the configured default notebook (notebooks.rules.default) when it names a
//     real definition;
//  3. otherwise the first definition by sorted name — the same order rung 1
//     will scan in once the directory exists, so the two agree from then on.
//
// It returns "" when the machine declares no notebooks at all. That is a
// refusal, not a fallback: a caller must NOT quietly create a notebook tree in
// the locator's home-anchored default, which nothing else on a configured
// machine reads.
func PlannedRoot(cfg *config.Config, name string) string {
	if name == "" {
		return ""
	}
	if root := existingDefinitionRoot(cfg, name); root != "" {
		return root
	}
	if cfg == nil || cfg.Notebooks == nil || len(cfg.Notebooks.Definitions) == 0 {
		return ""
	}

	notebook := ""
	if cfg.Notebooks.Rules != nil && cfg.Notebooks.Rules.Default != "" {
		if _, ok := cfg.Notebooks.Definitions[cfg.Notebooks.Rules.Default]; ok {
			notebook = cfg.Notebooks.Rules.Default
		}
	}
	if notebook == "" {
		notebook = slices.Sorted(maps.Keys(cfg.Notebooks.Definitions))[0]
	}
	return rootForNode(cfg, &workspace.WorkspaceNode{Name: name, NotebookName: notebook})
}

// Locate is the one-call read-surface entry point: load the machine's config,
// find the registry subscription, and resolve its root.
//
// It returns a typed "no registry configured" error rather than an empty
// string, so a CLI can tell "you have not joined a registry" apart from "the
// registry is configured but empty".
func Locate() (name, root string, err error) {
	syncCfg, err := config.LoadSyncConfig()
	if err != nil {
		return "", "", fmt.Errorf("failed to load sync config: %w", err)
	}
	sub := Subscription(syncCfg)
	if sub == nil {
		return "", "", ErrNoRegistry
	}
	cfg, err := config.LoadDefault()
	if err != nil {
		return sub.Name, "", fmt.Errorf("failed to load grove config: %w", err)
	}
	root = WorkspaceRoot(cfg, sub.Name)
	if root == "" {
		return sub.Name, "", fmt.Errorf("cannot resolve a local root for registry workspace %q", sub.Name)
	}
	return sub.Name, root, nil
}

// ErrNoRegistry means no sync subscription declares role = "registry".
var ErrNoRegistry = errNoRegistry{}

type errNoRegistry struct{}

func (errNoRegistry) Error() string {
	return `no registry workspace is configured (no sync subscription has role = "registry")`
}
