package registry

import (
	"fmt"
	"path/filepath"
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

// WorkspaceRoot resolves a subscribed workspace from recorded routing only.
// An exact compiled code-root binding wins; otherwise the explicitly recorded
// default notebook is used. Directory existence and map order never influence
// the result.
func WorkspaceRoot(cfg *config.Config, name string) string {
	notebook, ok := recordedNotebook(cfg, name)
	if !ok {
		return ""
	}
	return rootForNode(cfg, &workspace.WorkspaceNode{Name: name, NotebookName: notebook})
}

// recordedNotebook is the literal rung 0 shared with the daemon. A compiled
// root named for the workspace is an explicit per-root route. The only fallback
// is notebooks.toml's recorded default pointer; it is configuration, not an
// inferred first/sorted/existing choice.
func recordedNotebook(cfg *config.Config, name string) (string, bool) {
	if cfg == nil || name == "" {
		return "", false
	}
	if grove, ok := cfg.Groves[name]; ok && grove.Notebook != "" && grove.NotebookRoot != "" {
		return grove.Notebook, true
	}
	if cfg.Notebooks == nil || cfg.Notebooks.Rules == nil || cfg.Notebooks.Rules.Default == "" {
		return "", false
	}
	notebook := cfg.Notebooks.Rules.Default
	definition, ok := cfg.Notebooks.Definitions[notebook]
	return notebook, ok && definition != nil && definition.RootDir != ""
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

// PlannedRoot is the recorded routed notebook root even before the workspace
// exists. It intentionally has the same resolution as WorkspaceRoot: creation
// must not choose a directory that later reads would resolve differently.
func PlannedRoot(cfg *config.Config, name string) string {
	return WorkspaceRoot(cfg, name)
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
