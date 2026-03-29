// Package models provides shared types used across the grove ecosystem.
// This file contains enrichment types that define the API contract between
// the daemon and its consumers (nav, hooks, grove, etc.).
package models

import (
	"github.com/grovetools/core/git"
	"github.com/grovetools/core/pkg/workspace"
)

// EnrichmentOptions controls which data to fetch and for which projects.
type EnrichmentOptions struct {
	FetchNoteCounts   bool
	FetchGitStatus    bool
	FetchPlanStats    bool
	FetchReleaseInfo  bool
	FetchBinaryStatus bool
	FetchCxStats      bool
	FetchRemoteURL    bool
	GitStatusPaths    map[string]bool // nil means all projects
}

// NoteCounts holds counts of notes by type.
type NoteCounts struct {
	Current    int `json:"current"`
	Issues     int `json:"issues"`
	Inbox      int `json:"inbox"`
	Docs       int `json:"docs"`
	Completed  int `json:"completed"`
	Review     int `json:"review"`
	InProgress int `json:"in_progress"`
	Other      int `json:"other"`
}

// PlanStats holds statistics about grove-flow plans.
type PlanStats struct {
	TotalPlans int    `json:"total_plans"`
	ActivePlan string `json:"active_plan"`
	Running    int    `json:"running"`
	Pending    int    `json:"pending"`
	Completed  int    `json:"completed"`
	Failed     int    `json:"failed"`
	Todo       int    `json:"todo"`
	Hold       int    `json:"hold"`
	Abandoned  int    `json:"abandoned"`
	PlanStatus string `json:"plan_status,omitempty"` // Status of the plan itself (e.g., "hold", "finished")
}

// ReleaseInfo holds release tag and commit information.
type ReleaseInfo struct {
	LatestTag    string `json:"latest_tag"`
	CommitsAhead int    `json:"commits_ahead"`
}

// BinaryStatus holds the active status of a project's binary.
type BinaryStatus struct {
	ToolName       string `json:"tool_name"`
	IsDevActive    bool   `json:"is_dev_active"`
	LinkName       string `json:"link_name"`
	CurrentVersion string `json:"current_version"`
}

// CxStats holds token counts from grove-context.
type CxStats struct {
	Files  int   `json:"total_files"`
	Tokens int   `json:"total_tokens"`
	Size   int64 `json:"total_size"`
}

// EnrichedWorkspace wraps a WorkspaceNode with enrichment data.
type EnrichedWorkspace struct {
	*workspace.WorkspaceNode
	GitStatus    *git.ExtendedGitStatus `json:"git_status,omitempty"`
	NoteCounts   *NoteCounts            `json:"note_counts,omitempty"`
	PlanStats    *PlanStats             `json:"plan_stats,omitempty"`
	ReleaseInfo  *ReleaseInfo           `json:"release_info,omitempty"`
	ActiveBinary *BinaryStatus          `json:"active_binary,omitempty"`
	CxStats      *CxStats               `json:"cx_stats,omitempty"`
	GitRemoteURL string                 `json:"git_remote_url,omitempty"`
}

// WorkspaceDelta carries only the fields that changed for a specific workspace.
// Pointers distinguish between an unchanged field (nil) and a zero value.
type WorkspaceDelta struct {
	Path         string                 `json:"path"`
	GitStatus    *git.ExtendedGitStatus `json:"git_status,omitempty"`
	NoteCounts   *NoteCounts            `json:"note_counts,omitempty"`
	PlanStats    *PlanStats             `json:"plan_stats,omitempty"`
	ReleaseInfo  *ReleaseInfo           `json:"release_info,omitempty"`
	ActiveBinary *BinaryStatus          `json:"active_binary,omitempty"`
	CxStats      *CxStats               `json:"cx_stats,omitempty"`
	GitRemoteURL *string                `json:"git_remote_url,omitempty"`
}
