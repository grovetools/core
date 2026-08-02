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

// EntrySchemaVersion is the current shape of the persisted Entry. It is
// written by Tombstone (the first write path that introduced a shape older
// readers cannot fully interpret) and is ABSENT on every entry written before
// it existed. Absent must therefore be read as "the pre-versioned shape", which
// is byte-compatible with this one — never as an error.
const EntrySchemaVersion = 1

// Lifecycle status values for Entry.Status. The zero value ("") is the
// pre-versioned shape and means StatusActive: every registry file written
// before tombstones existed describes a live worktree.
const (
	// StatusActive is a live worktree — the default for an absent status.
	StatusActive = "active"
	// StatusFinished is a tombstone: the plan was finished and the worktree
	// retired, but the binding (plan, repos, labels, final SHAs) is kept as
	// history instead of being deleted.
	StatusFinished = "finished"
)

// RepoFinalState is one repository's last known position at the moment its
// worktree was tombstoned. Source records HOW the SHA was obtained so a reader
// can tell a landed-and-receipted repo apart from one whose branch head was
// simply sampled at teardown.
type RepoFinalState struct {
	// Repo is the workspace/repo name as it appears in Entry.Repos.
	Repo string `json:"repo"`
	// Branch is the plan branch the SHA was read from, when known.
	Branch string `json:"branch,omitempty"`
	// SHA is the repository's final commit at finish time.
	SHA string `json:"sha,omitempty"`
	// Source is the provenance of SHA: SHASourceReceipt when it came from a
	// landing receipt, SHASourceBranchHead when it was read from the live
	// checkout at finish. Empty when unknown.
	Source string `json:"source,omitempty"`
}

// Provenance values for RepoFinalState.Source.
const (
	// SHASourceReceipt: the SHA came from a durable landing receipt — the
	// repository actually landed and the movement is recorded elsewhere too.
	SHASourceReceipt = "receipt"
	// SHASourceBranchHead: the SHA was read from the live checkout at finish
	// time. There is no receipt, so nothing asserts this work landed.
	SHASourceBranchHead = "branch_head"
)

// Entry is the JSON payload persisted to StateDir()/worktrees/<id>.json.
type Entry struct {
	// SchemaVersion is the persisted shape version. Absent (0) means the
	// pre-versioned shape, which is compatible — readers must not reject it.
	SchemaVersion int `json:"schema_version,omitempty"`

	// Status is the entry's lifecycle state: StatusActive or StatusFinished.
	// Absent means StatusActive (see EffectiveStatus).
	Status string `json:"status,omitempty"`

	// FinishedAt records when the entry was tombstoned. Zero while active.
	FinishedAt time.Time `json:"finished_at,omitempty"`

	// FinalSHAs is the per-repo position captured at tombstone time. It is
	// the durable answer to "what did this worktree end up producing", and it
	// outlives the branches and checkouts the SHAs were read from.
	FinalSHAs []RepoFinalState `json:"final_shas,omitempty"`

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

// EffectiveStatus returns the entry's lifecycle status with the legacy default
// applied: an absent status is StatusActive, because every registry file
// written before tombstones existed describes a live worktree.
func (e *Entry) EffectiveStatus() string {
	if e == nil || e.Status == "" {
		return StatusActive
	}
	return e.Status
}

// IsFinished reports whether this entry is a tombstone. Finished entries are
// history: they are excluded from every default query path (ListAll, Resolve,
// PlanForPath, FindByRef) and Reconcile never prunes or heals them, because the
// worktree they describe is gone by design rather than missing by accident.
func (e *Entry) IsFinished() bool {
	return e != nil && e.Status == StatusFinished
}
