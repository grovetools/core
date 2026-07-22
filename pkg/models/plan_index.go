package models

import "time"

// PlanSummary is the daemon's lightweight, repository-neutral projection of a
// Flow plan. PlanDir is the stable identity; plan names are not globally unique.
type PlanSummary struct {
	PlanDir         string         `json:"plan_dir"`
	PlanName        string         `json:"plan_name"`
	WorkspaceRoot   string         `json:"workspace_root,omitempty"`
	PlansDir        string         `json:"plans_dir,omitempty"`
	Lifecycle       string         `json:"lifecycle"`
	Selected        bool           `json:"selected"`
	Worktree        string         `json:"worktree,omitempty"`
	Anchor          string         `json:"anchor,omitempty"`
	Repositories    []string       `json:"repositories,omitempty"`
	JobCounts       map[string]int `json:"job_counts,omitempty"`
	RunningSessions int            `json:"running_sessions"`
	UpdatedAt       time.Time      `json:"updated_at"`
	ScannedAt       time.Time      `json:"scanned_at"`
}

// PlanIndexSnapshot is the reconciliation primitive. Consumers replace local
// state from it on connect/reconnect and after any revision gap.
type PlanIndexSnapshot struct {
	Revision  uint64        `json:"revision"`
	ScannedAt time.Time     `json:"scanned_at"`
	Plans     []PlanSummary `json:"plans"`
}

// PlanIndexDelta is the lossy SSE projection of one materialized-index update.
// Removed contains PlanDir identities. Revision is the post-apply revision.
type PlanIndexDelta struct {
	Revision  uint64        `json:"revision"`
	ScannedAt time.Time     `json:"scanned_at"`
	Upserts   []PlanSummary `json:"upserts,omitempty"`
	Removed   []string      `json:"removed,omitempty"`
}
