// Package worktreeregistry is a per-worktree metadata store located under
// paths.StateDir()/worktrees/<id>.json. It is the single source of truth for
// metadata that the filesystem cannot encode: AnchorOverride, Plan, Labels,
// CreatedAt/LastActive, SessionState, and a cached per-repo git status.
//
// Structural facts (existence, owner via git, repos via grove.toml) are always
// resolved from the live filesystem; the registry caches them and serves them
// when the FS becomes unavailable (zombie worktrees).
//
// This package must NOT import core/pkg/workspace (import cycle: workspace
// imports worktreeregistry for the owner-chain lookup).
package worktreeregistry

import "time"

// Entry is the JSON payload persisted to StateDir()/worktrees/<id>.json.
type Entry struct {
	// AbsPath is the absolute path to the worktree container directory.
	// It is authoritative — the registry ID is derived from this field.
	AbsPath string `json:"abs_path"`

	// Owner is the absolute path to the git root that owns this worktree.
	Owner string `json:"owner,omitempty"`

	// Repos is the list of workspace/repo names present inside this worktree.
	Repos []string `json:"repos,omitempty"`

	// AnchorOverride allows callers to reassign the "anchor" repo for this
	// worktree. When non-empty and matching a member of Repos, Resolve uses
	// it as the anchor; otherwise the anchor defaults to Owner.
	AnchorOverride string `json:"anchor_override,omitempty"`

	// Plan is the grove-flow plan name this worktree was created for.
	Plan string `json:"plan,omitempty"`

	// Labels is an arbitrary string→string tag bag for tooling.
	Labels map[string]string `json:"labels,omitempty"`

	// CreatedAt records when the worktree was created.
	CreatedAt time.Time `json:"created_at,omitempty"`

	// LastActive records the last time any grove tool wrote to this entry.
	LastActive time.Time `json:"last_active,omitempty"`

	// SessionState mirrors the key-value pairs stored in .grove/state.yml.
	// The registry is PRIMARY during the dual-write window; .grove/state.yml
	// is a fallback read / deprecation-window write.
	SessionState map[string]interface{} `json:"session_state,omitempty"`

	// GitCache is an opaque per-repo git status cache (repo name → status string).
	GitCache map[string]string `json:"git_cache,omitempty"`

	// ArchivedAt records when the worktree was archived (moved under
	// paths.WorktreeArchiveDir()). Zero for live worktrees.
	ArchivedAt time.Time `json:"archived_at,omitempty"`

	// OriginalPath is the AbsPath the worktree had before it was archived.
	// Empty for live worktrees.
	OriginalPath string `json:"original_path,omitempty"`
}

// IsArchived reports whether this entry describes an archived worktree.
// Archived entries keep their history (Plan, Labels, timestamps) but must be
// skipped by name/plan resolution and by Reconcile's anchor-heal — the
// worktree no longer lives under any active base.
func (e *Entry) IsArchived() bool {
	return !e.ArchivedAt.IsZero()
}
