// Package models provides shared types used across the grove ecosystem.
// This file defines the WorkspaceHUD payload streamed from the daemon to the
// terminal (and any other HUD consumers).
package models

// HUDPlanJobCounts holds job counts for the active plan.
type HUDPlanJobCounts struct {
	Running int `json:"running"`
	Pending int `json:"pending"`
	Done    int `json:"done"`
}

// WorkspaceHUD is a per-workspace HUD snapshot emitted by the daemon.
// It contains just enough information to render a single-line heads-up
// display without additional RPCs.
type WorkspaceHUD struct {
	WorkspacePath string `json:"workspace_path"`
	WorkspaceName string `json:"workspace_name"`
	ShortPath     string `json:"short_path"`

	// Hierarchy fields for rich workspace display. Populated by the
	// daemon from the WorkspaceNode's parent paths. When set, the HUD
	// renders "ecosystem > project > worktree" instead of just the name.
	EcosystemName string `json:"ecosystem_name,omitempty"` // e.g. "grovetools"
	WorktreeName  string `json:"worktree_name,omitempty"`  // e.g. "groveterm-pt3" (branch name if in a worktree)

	GitBranch string `json:"git_branch"`
	GitDirty  bool   `json:"git_dirty"`
	GitAhead  int    `json:"git_ahead"`
	GitBehind int    `json:"git_behind"`

	ActivePlan    string           `json:"active_plan"`
	PlanStatus    string           `json:"plan_status"` // running|paused|done|""
	PlanJobCounts HUDPlanJobCounts `json:"plan_job_counts"`

	CxFiles  int `json:"cx_files"`
	CxTokens int `json:"cx_tokens"`

	HooksActive   int `json:"hooks_active"`
	NotebookCount int `json:"notebook_count"`
}
