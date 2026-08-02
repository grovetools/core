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
	locator := workspace.NewNotebookLocator(cfg)
	rootFor := func(node *workspace.WorkspaceNode) string {
		notesDir, err := locator.GetNotesDir(node, "inbox")
		if err != nil || !filepath.IsAbs(notesDir) {
			return ""
		}
		return workspaceRootForDir(filepath.Dir(notesDir))
	}

	if cfg != nil && cfg.Notebooks != nil && len(cfg.Notebooks.Definitions) > 0 {
		for _, defName := range slices.Sorted(maps.Keys(cfg.Notebooks.Definitions)) {
			node := &workspace.WorkspaceNode{Name: name, NotebookName: defName}
			if root := rootFor(node); root != "" {
				if fi, err := os.Stat(root); err == nil && fi.IsDir() {
					return root
				}
			}
		}
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
			return rootFor(&workspace.WorkspaceNode{Name: name, NotebookName: nb})
		}
	}
	return rootFor(&workspace.WorkspaceNode{Name: name})
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
