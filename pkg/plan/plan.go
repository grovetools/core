// Package plan provides utilities for detecting and resolving active flow plans.
// This package is shared by cx, flow, and other grove tools that need
// to know which plan is currently active for a given project.
package plan

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/git"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/pkg/worktreeregistry"
	"github.com/grovetools/core/state"
)

const (
	// StateKey is the state key used to store the active plan name.
	StateKey = "flow.active_plan"

	// LegacyStateKey is the old state key, kept for migration.
	LegacyStateKey = "active_plan"

	// DefaultRulesFile is the filename for plan-scoped default rules.
	DefaultRulesFile = "default.rules"

	// RollingPlanName is the name of the auto-created "rolling" plan used when
	// no plan is specified — a shared home for quick tasks. Materialized lazily
	// (and self-healed) via EnsureRollingPlan.
	RollingPlanName = "rolling"

	// rollingPlanConfigBody is the contents of the rolling plan's
	// .grove-plan.yml marker. Kept identical to flow's historical inline body.
	rollingPlanConfigBody = "# Rolling plan - auto-created for quick tasks without a formal plan.\n"
)

// ActivePlan returns the name of the currently active flow plan.
// It checks in order:
//  1. Branch match — if a plan directory exists matching the current git branch
//     (preferred because it's always correct for the current worktree)
//  2. State key "flow.active_plan" (set explicitly by flow)
//  3. Legacy state key "active_plan" (auto-migrated to new key)
//
// Returns empty string if no active plan can be determined.
func ActivePlan(workDir string) string {
	// 1. Check branch match first — always correct for the current worktree
	if name := activePlanFromBranch(workDir); name != "" {
		return name
	}

	// 2. Fall back to state (for main-branch plans with explicit flow set).
	//    Scope the state read to workDir's ecosystem so a process whose CWD is
	//    elsewhere (e.g. $HOME) can't leak another ecosystem's active plan.
	return activePlanFromState(workDir)
}

// ActivePlanForPath resolves the active plan for an arbitrary path, preferring
// the XDG worktree registry (the canonical store) over branch/state heuristics.
// It walks path up to its owning worktree container and reads the registry
// Plan; if that yields nothing it falls back to the existing ActivePlan logic.
func ActivePlanForPath(path string) string {
	if root, ok := workspace.WorktreeRootForPath(path); ok {
		if plan, ok := worktreeregistry.PlanForPath(root); ok {
			return plan
		}
	}
	return ActivePlan(path)
}

// activePlanFromState reads the active plan from state for workDir's ecosystem,
// handling legacy key migration. A workDir outside any ecosystem (e.g. $HOME)
// reads empty state and returns "" (no error, no leak).
func activePlanFromState(workDir string) string {
	plan, _ := state.GetString(workDir, StateKey)
	if plan != "" {
		return plan
	}

	// Check legacy key and migrate
	oldPlan, _ := state.GetString(workDir, LegacyStateKey)
	if oldPlan != "" {
		// Migrate: set new key and delete old key
		_ = state.Set(workDir, StateKey, oldPlan)
		_ = state.Delete(workDir, LegacyStateKey)
		return oldPlan
	}

	return ""
}

// activePlanFromBranch checks if a plan directory exists matching the current git branch.
func activePlanFromBranch(workDir string) string {
	_, branch, err := git.GetRepoInfo(workDir)
	if err != nil || branch == "" || branch == "HEAD" {
		return ""
	}

	plansDir := ResolvePlansDir(workDir)
	if plansDir == "" {
		return ""
	}

	planPath := filepath.Join(plansDir, branch)
	if info, err := os.Stat(planPath); err == nil && info.IsDir() {
		return branch
	}
	return ""
}

// ResolvePlanDir returns the absolute path to a plan's directory,
// or empty string if it cannot be resolved.
func ResolvePlanDir(workDir, planName string) string {
	plansDir := ResolvePlansDir(workDir)
	if plansDir == "" {
		return ""
	}
	return filepath.Join(plansDir, planName)
}

// DefaultRulesPath returns the path to plans/<plan>/rules/default.rules
// for the given plan, or empty string if paths cannot be resolved.
func DefaultRulesPath(workDir, planName string) string {
	plansDir := ResolvePlansDir(workDir)
	if plansDir == "" {
		return ""
	}
	return filepath.Join(plansDir, planName, "rules", DefaultRulesFile)
}

// ContextPath returns the path to plans/<plan>/context/generated/context
// for the given plan, or empty string if paths cannot be resolved.
func ContextPath(workDir, planName string) string {
	planDir := ResolvePlanDir(workDir, planName)
	if planDir == "" {
		return ""
	}
	return filepath.Join(planDir, "context", "generated", "context")
}

// CachedContextPath returns the path to plans/<plan>/context/cache/cached-context
// for the given plan, or empty string if paths cannot be resolved.
func CachedContextPath(workDir, planName string) string {
	planDir := ResolvePlanDir(workDir, planName)
	if planDir == "" {
		return ""
	}
	return filepath.Join(planDir, "context", "cache", "cached-context")
}

// CachedContextFilesListPath returns the path to plans/<plan>/context/cache/cached-context-files
// for the given plan, or empty string if paths cannot be resolved.
func CachedContextFilesListPath(workDir, planName string) string {
	planDir := ResolvePlanDir(workDir, planName)
	if planDir == "" {
		return ""
	}
	return filepath.Join(planDir, "context", "cache", "cached-context-files")
}

// ResolvePlansDir returns the plans directory for the workspace containing
// workDir, honoring the centralized notebook layout via NotebookLocator.
// Returns "" if the workspace can't be resolved or has no plans dir configured.
// Callers (e.g. the terminal plan panel) should fall back to a reasonable
// default when this returns empty.
func ResolvePlansDir(workDir string) string {
	node, err := workspace.GetProjectByPath(workDir)
	if err != nil {
		return ""
	}

	cfg, err := config.LoadFrom(workDir)
	if err != nil {
		cfg, _ = config.LoadDefault()
		if cfg == nil {
			cfg = &config.Config{}
		}
	}

	locator := workspace.NewNotebookLocator(cfg)
	plansDir, err := locator.GetPlansDir(node)
	if err != nil {
		return ""
	}
	return plansDir
}

// RollingPlanDir returns the absolute path to the rolling plan directory for
// the workspace containing workDir, or "" if the plans dir cannot be resolved.
func RollingPlanDir(workDir string) string {
	return ResolvePlanDir(workDir, RollingPlanName)
}

// EnsureRollingPlan materializes the rolling plan directory and its
// .grove-plan.yml marker for the workspace containing workDir, creating them if
// missing. It is the single shared materializer for the rolling plan, used both
// by the write path (flow add/chat with no plan) and by the read paths that
// self-heal a missing rolling dir.
//
// It returns the resolved directory, whether THIS call wrote the marker
// (created), and any error. created is true only when this process performed the
// write — concurrent first-touch losers see created=false.
//
// Because ResolvePlansDir returns "" (not an error) when workDir is outside any
// resolvable workspace, EnsureRollingPlan converts that to an explicit error so
// callers never create a stray "rolling/" in an unrelated directory. It does
// NOT print to stderr — callers decide whether/how to notify the user.
func EnsureRollingPlan(workDir string) (dir string, created bool, err error) {
	plansDir := ResolvePlansDir(workDir)
	if plansDir == "" {
		return "", false, fmt.Errorf("no workspace found for %q: cannot resolve rolling plan directory", workDir)
	}

	dir = filepath.Join(plansDir, RollingPlanName)
	if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
		return "", false, fmt.Errorf("creating rolling plan directory: %w", mkErr)
	}

	// Heal on the marker file specifically, not just the directory: a dir that
	// exists but lost its .grove-plan.yml still needs the marker written.
	configPath := filepath.Join(dir, ".grove-plan.yml")
	if _, statErr := os.Stat(configPath); statErr == nil {
		return dir, false, nil // marker already present — nothing to do
	} else if !os.IsNotExist(statErr) {
		return "", false, fmt.Errorf("checking rolling plan marker: %w", statErr)
	}

	// Write the marker with O_CREATE|O_EXCL so concurrent first-touch has a
	// single winner; losers observe IsExist and treat it as already created.
	f, openErr := os.OpenFile(configPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if openErr != nil {
		if os.IsExist(openErr) {
			return dir, false, nil
		}
		return "", false, fmt.Errorf("creating rolling plan .grove-plan.yml: %w", openErr)
	}
	defer f.Close()

	if _, writeErr := f.WriteString(rollingPlanConfigBody); writeErr != nil {
		return "", false, fmt.Errorf("writing rolling plan .grove-plan.yml: %w", writeErr)
	}

	return dir, true, nil
}
