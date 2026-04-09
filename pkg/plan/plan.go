// Package plan provides utilities for detecting and resolving active flow plans.
// This package is shared by cx, flow, and other grove tools that need
// to know which plan is currently active for a given project.
package plan

import (
	"os"
	"path/filepath"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/git"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/state"
)

const (
	// StateKey is the state key used to store the active plan name.
	StateKey = "flow.active_plan"

	// LegacyStateKey is the old state key, kept for migration.
	LegacyStateKey = "active_plan"

	// DefaultRulesFile is the filename for plan-scoped default rules.
	DefaultRulesFile = "default.rules"
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

	// 2. Fall back to state (for main-branch plans with explicit flow set)
	return activePlanFromState()
}

// activePlanFromState reads the active plan from state, handling legacy key migration.
func activePlanFromState() string {
	plan, _ := state.GetString(StateKey)
	if plan != "" {
		return plan
	}

	// Check legacy key and migrate
	oldPlan, _ := state.GetString(LegacyStateKey)
	if oldPlan != "" {
		// Migrate: set new key and delete old key
		_ = state.Set(StateKey, oldPlan)
		_ = state.Delete(LegacyStateKey)
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
