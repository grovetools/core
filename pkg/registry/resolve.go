package registry

import (
	"fmt"
	"path/filepath"

	"github.com/grovetools/core/config"
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
	root, _ := ResolveWorkspaceRoot(cfg, name)
	return root
}

// ResolveWorkspaceRoot is the fail-loud form used by creation, daemon, and
// doctor surfaces. WorkspaceRoot remains as a compatibility read helper while
// downstream callers migrate to explicit errors.
func ResolveWorkspaceRoot(cfg *config.Config, name string) (string, error) {
	_, notebookRoot, err := recordedNotebookRoot(cfg, name)
	if err != nil {
		return "", err
	}
	return filepath.Join(notebookRoot, "notespaces", name), nil
}

// recordedNotebookRoot is the literal rung 0 shared with the daemon. A
// compiled root named for the workspace contributes both the notebook name and
// its already-resolved root. The root is never looked up again by name.
func recordedNotebookRoot(cfg *config.Config, name string) (string, string, error) {
	if cfg == nil || name == "" {
		return "", "", fmt.Errorf("workspace %q has no recorded code-root/notebook binding", name)
	}
	if grove, ok := cfg.Groves[name]; ok {
		if grove.Notebook != "" || grove.NotebookRoot != "" {
			if grove.Notebook == "" || grove.NotebookRoot == "" {
				return "", "", fmt.Errorf("workspace %q has an incomplete recorded code-root/notebook binding", name)
			}
			return grove.Notebook, grove.NotebookRoot, nil
		}
	}
	if cfg.Notebooks == nil || cfg.Notebooks.Rules == nil || cfg.Notebooks.Rules.Default == "" {
		return "", "", fmt.Errorf("workspace %q has no recorded code-root/notebook binding or default notebook", name)
	}
	notebook := cfg.Notebooks.Rules.Default
	definition := cfg.Notebooks.Definitions[notebook]
	if definition == nil || definition.RootDir == "" {
		return "", "", fmt.Errorf("workspace %q routes to default notebook %q without a recorded root", name, notebook)
	}
	return notebook, definition.RootDir, nil
}

// PlannedRoot is the recorded routed notebook root even before the workspace
// exists. It intentionally has the same resolution as WorkspaceRoot: creation
// must not choose a directory that later reads would resolve differently.
func PlannedRoot(cfg *config.Config, name string) string {
	root, _ := ResolvePlannedRoot(cfg, name)
	return root
}

// ResolvePlannedRoot is PlannedRoot's explicit-error form. New write paths
// must use it so an absent binding cannot become creation-by-omission.
func ResolvePlannedRoot(cfg *config.Config, name string) (string, error) {
	return ResolveWorkspaceRoot(cfg, name)
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
	root, err = ResolveWorkspaceRoot(cfg, sub.Name)
	if err != nil {
		return sub.Name, "", fmt.Errorf("cannot resolve a local root for registry workspace %q: %w", sub.Name, err)
	}
	return sub.Name, root, nil
}

// ErrNoRegistry means no sync subscription declares role = "registry".
var ErrNoRegistry = errNoRegistry{}

type errNoRegistry struct{}

func (errNoRegistry) Error() string {
	return `no registry workspace is configured (no sync subscription has role = "registry")`
}
