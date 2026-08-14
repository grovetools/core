package registry

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/coderoot"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/util/pathutil"
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
	binding, err := ResolvePlannedBinding(cfg, name)
	if err != nil {
		return "", err
	}
	return binding.Root, nil
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
	// Identity rung, ahead of the default: a notes-plane notespace with no
	// compiled binding is still locatable BY IDENTITY — its stamp id is the
	// recorded primary for its subject. Dropping straight to
	// notebooks.rules.default is what sent a notespace bound to a non-default
	// notebook to <default>/notespaces/<name>. Mirrors the daemon's
	// SyncHandler.recordedNotebookRoot, which the doc above calls the rung
	// shared with this one; it stopped being shared when only that copy grew
	// this rung.
	if notebook, root, ok := stampedNotebookRoot(cfg, name); ok {
		return notebook, root, nil
	}
	if cfg.Notebooks == nil || cfg.Notebooks.Rules == nil || cfg.Notebooks.Rules.Default == "" {
		return "", "", fmt.Errorf("workspace %q has no recorded code-root/notebook binding or default notebook", name)
	}
	notebook := cfg.Notebooks.Rules.Default
	definition := cfg.Notebooks.Definitions[notebook]
	if definition == nil || definition.RootDir == "" {
		return "", "", fmt.Errorf("workspace %q routes to default notebook %q without a recorded root", name, notebook)
	}
	// The compiled Groves branch above returns an already-resolved root; this
	// branch must match it. A recorded definition can still carry a declared
	// spelling in the legacy shape, and ResolveWorkspaceRoot joins whatever it
	// gets straight onto "notespaces/<name>" — a tilde survives that join
	// intact and every stat below it then answers about nothing.
	return notebook, coderoot.ExpandPath(definition.RootDir), nil
}

// stampedNotebookRoot answers which recorded notebook holds the stamped
// notespace a display name identifies, via the recorded-primary resolver
// (stamp id + machine.toml [primaries]) rather than a name-to-directory guess.
//
// ok is false whenever that chain cannot answer EXACTLY — an unstamped tree,
// or a name matching no single recorded primary — leaving the caller's default
// rung to decide as before.
func stampedNotebookRoot(cfg *config.Config, name string) (string, string, bool) {
	if cfg == nil || cfg.Notebooks == nil || len(cfg.Notebooks.Definitions) == 0 {
		return "", "", false
	}
	machineCfg, err := config.LoadMachineConfig()
	if err != nil || machineCfg == nil {
		return "", "", false
	}
	resolution, err := workspace.ResolveNotespaceName(name, cfg, machineCfg)
	if err != nil || resolution.Root == "" {
		return "", "", false
	}
	// The contract is a NOTEBOOK root that ResolveWorkspaceRoot re-joins with
	// "notespaces/<name>", so only a resolution that round-trips through that
	// join can be reported here.
	if filepath.Base(resolution.Root) != name {
		return "", "", false
	}
	notespacesDir := filepath.Dir(resolution.Root)
	if filepath.Base(notespacesDir) != workspace.NotespaceDirectory {
		return "", "", false
	}
	notebookRoot := filepath.Dir(notespacesDir)
	// Both sides are normalized first: RootDir is a RECORDED value and
	// resolution.Root a resolved one, and comparing those by raw string
	// equality is the mistake this rung exists to correct one layer down.
	for _, notebook := range slices.Sorted(maps.Keys(cfg.Notebooks.Definitions)) {
		definition := cfg.Notebooks.Definitions[notebook]
		if definition == nil || definition.RootDir == "" {
			continue
		}
		declared := coderoot.ExpandPath(definition.RootDir)
		if same, err := pathutil.ComparePaths(declared, notebookRoot); err == nil && same {
			return notebook, notebookRoot, true
		}
	}
	return "", "", false
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

// PlannedBinding is where routing places a workspace: the recorded notebook
// that holds it, that notebook's resolved root, and the notespace root itself.
type PlannedBinding struct {
	Notebook     string
	NotebookRoot string
	Root         string
}

// ResolvePlannedBinding is ResolvePlannedRoot's naming form, for a caller that
// must RECORD which notebook a workspace landed in.
//
// The notebook name comes from the same resolution that chose the root. A
// caller that re-derives it from notebooks.rules.default instead records the
// wrong name for every workspace the identity rung (or a compiled binding)
// routed somewhere other than the default.
func ResolvePlannedBinding(cfg *config.Config, name string) (PlannedBinding, error) {
	notebook, notebookRoot, err := recordedNotebookRoot(cfg, name)
	if err != nil {
		return PlannedBinding{}, err
	}
	return PlannedBinding{
		Notebook:     notebook,
		NotebookRoot: notebookRoot,
		Root:         filepath.Join(notebookRoot, workspace.NotespaceDirectory, name),
	}, nil
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
