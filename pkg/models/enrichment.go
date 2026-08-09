// Package models provides shared types used across the grove ecosystem.
// This file contains enrichment types that define the API contract between
// the daemon and its consumers (nav, hooks, grove, etc.).
package models

import (
	"time"

	"github.com/grovetools/core/git"
	"github.com/grovetools/core/pkg/forge"
	"github.com/grovetools/core/pkg/workspace"
)

// TaskResult records the outcome of a developer hygiene or build task.
type TaskResult struct {
	ExitCode     int       `json:"exit_code"`
	CommitHash   string    `json:"commit_hash"`
	DurationMs   int64     `json:"duration_ms"`
	Timestamp    time.Time `json:"timestamp"`
	ErrorSummary string    `json:"error_summary,omitempty"`
}

type ScenarioResult struct {
	Name       string   `json:"name"`
	Status     string   `json:"status"` // pass, fail, skip
	DurationMs int64    `json:"duration_ms"`
	Steps      int      `json:"steps"`
	FailedStep string   `json:"failed_step,omitempty"`
	Error      string   `json:"error,omitempty"`
	Tags       []string `json:"tags,omitempty"`
}

type TestSummary struct {
	Total      int   `json:"total"`
	Passed     int   `json:"passed"`
	Failed     int   `json:"failed"`
	Skipped    int   `json:"skipped"`
	DurationMs int64 `json:"duration_ms"`
}

type TestReport struct {
	Verb      string           `json:"verb"`
	Scenarios []ScenarioResult `json:"scenarios"`
	Summary   TestSummary      `json:"summary"`
	Timestamp time.Time        `json:"timestamp"`
}

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
	TotalPlans        int    `json:"total_plans"`
	ActivePlan        string `json:"active_plan"`
	Running           int    `json:"running"`
	Pending           int    `json:"pending"`
	Completed         int    `json:"completed"`
	Failed            int    `json:"failed"`
	Todo              int    `json:"todo"`
	Hold              int    `json:"hold"`
	Abandoned         int    `json:"abandoned"`
	PlanStatus        string `json:"plan_status,omitempty"` // Status of the explicitly associated plan.
	AssociatedPlan    string `json:"associated_plan,omitempty"`
	AssociatedPlanDir string `json:"associated_plan_dir,omitempty"`
}

// ReviewStatsSchemaVersion is the current shape of ReviewStats. Readers must
// tolerate an older (or absent) version rather than assuming fields exist.
const ReviewStatsSchemaVersion = 1

// ReviewFreshness says how much of a ReviewStats may be believed.
//
// It exists because "the forge affirmatively reports no pull requests" and
// "nobody has managed to ask the forge" are opposite facts that an int-only
// shape renders identically, as zero. Every consumer must branch on this
// before reading a count.
type ReviewFreshness string

const (
	// ReviewFreshnessUnknown means no successful fetch has ever landed for this
	// repo: the poller is off, the provider is unavailable, or every attempt so
	// far has failed. Counts are absent, not zero.
	ReviewFreshnessUnknown ReviewFreshness = "unknown"
	// ReviewFreshnessFresh means the data came from a successful fetch inside
	// the poller's freshness window.
	ReviewFreshnessFresh ReviewFreshness = "fresh"
	// ReviewFreshnessStale means a fetch succeeded at some point but the latest
	// attempt failed or the entry aged past the freshness window. The counts
	// are the last known good values and must be rendered as dated — never as
	// current, and never promoted to green.
	ReviewFreshnessStale ReviewFreshness = "stale"
)

// Known reports whether this freshness permits reading the counts at all. The
// zero value ("" — what an older daemon's absent field decodes to) is not
// known, so an un-set ReviewStats can never be mistaken for an empty one.
func (f ReviewFreshness) Known() bool {
	return f == ReviewFreshnessFresh || f == ReviewFreshnessStale
}

// PRCounts buckets a repo's pull requests by lifecycle state.
//
// A PRCounts value only ever appears on a ReviewStats whose Freshness is
// Known, which is what makes an all-zero value meaningful: it says the forge
// reported no pull requests, not that we failed to ask.
type PRCounts struct {
	Open int `json:"open"`
	// Draft counts the subset of Open that the forge marks as a draft.
	Draft  int `json:"draft"`
	Merged int `json:"merged"`
	Closed int `json:"closed"`
	// Unknown counts pull requests whose state the provider could not
	// recognize. They are deliberately not folded into any other bucket.
	Unknown int `json:"unknown"`
	// Truncated is true when the poller's per-repo fetch bound was reached, so
	// these are counts over a recent window rather than a census. Open and
	// Draft are complete whenever it is false.
	Truncated bool `json:"truncated,omitempty"`
}

// ReviewStats is the daemon's projection of forge review state onto one
// workspace: what the forge says about the repo this workspace checks out, plus
// the local plan status it should be read alongside.
//
// It is computed daemon-side by the forge poller from its own cache and
// attached like NoteCounts/PlanStats. It is read-only in every sense: nothing
// here drives a mutation, and nothing here is authoritative about the notebook
// (which ticket a PR belongs to lives with the ticket↔PR join, not here).
type ReviewStats struct {
	SchemaVersion int `json:"schema_version"`

	// Freshness gates everything below it, and is the authoritative answer to
	// "how current is this". Read it first.
	Freshness ReviewFreshness `json:"freshness"`
	// FetchedAt is a LOWER BOUND on when the data below was last confirmed by a
	// successful fetch: the poller re-emits this value only when something
	// material changed, so a repo that has been quiet for a week carries a
	// week-old timestamp while still being polled — and reported — as fresh.
	// It dates the CONTENT; Freshness dates the ANSWER. It is the zero time
	// when Freshness is unknown, and it never advances on a failed poll.
	FetchedAt time.Time `json:"fetched_at,omitempty"`

	// Provider is the forge provider name ("github"), and Repo the fully
	// qualified "host/owner/name" identity the counts describe. Two repos with
	// the same slug on different hosts are different repos.
	Provider string `json:"provider,omitempty"`
	Repo     string `json:"repo,omitempty"`

	// PRs is nil when Freshness is not Known — nil means "don't know", an
	// all-zero PRCounts means "the forge reports no pull requests". This
	// distinction is the whole reason the field is a pointer.
	PRs *PRCounts `json:"prs,omitempty"`

	// Checks is the merged CI state across the repo's open pull requests,
	// using forge.RollupState precedence (failure > unknown > pending >
	// neutral > success) and forge.CheckStateNone for "no open PRs to check".
	// Only forge.CheckStateSuccess is green; the zero value normalizes to
	// unknown, so a rollup nobody filled in can never render as passing.
	Checks forge.CheckState `json:"checks,omitempty"`

	// PlanStatus mirrors PlanStats.PlanStatus at projection time so a review
	// surface can show forge state and local plan state together without a
	// second lookup. Empty means no associated plan.
	PlanStatus string `json:"plan_status,omitempty"`

	// LastError is the most recent poll failure, present whenever the last
	// attempt failed (so on every stale entry, and on an unknown entry that has
	// been tried). It explains the freshness; it never substitutes for it.
	LastError string `json:"last_error,omitempty"`
}

// MachineSyncSchemaVersion is the current wire shape of MachineSync.
const MachineSyncSchemaVersion = 1

// MachineSyncState is the bounded tier-0 relationship between this workspace's
// committed tip and one machine's replicated registry tip. Tier 0 deliberately
// carries no ahead/behind counts: unequal object IDs prove only divergence,
// not direction or distance.
type MachineSyncState string

const (
	MachineSyncEqual    MachineSyncState = "equal"
	MachineSyncDiverged MachineSyncState = "diverged_unknown"
	MachineSyncAbsent   MachineSyncState = "absent"
	MachineSyncExcluded MachineSyncState = "excluded"
	MachineSyncUnknown  MachineSyncState = "unknown"
)

// MachineSyncPeer is one registry machine projected onto a local workspace.
// LastSeen and AgeSeconds date the replicated note, not a live connection.
type MachineSyncPeer struct {
	MachineID string           `json:"machine_id"`
	Label     string           `json:"label"`
	State     MachineSyncState `json:"state"`
	Branch    string           `json:"branch,omitempty"`
	SHA       string           `json:"sha,omitempty"`
	// SameBranch is metadata only. Equality is committed-tip equality; two
	// branch names may legitimately point at the same commit.
	SameBranch bool   `json:"same_branch"`
	LastSeen   string `json:"last_seen,omitempty"`
	// AgeSeconds is nil when last_seen is absent or malformed. Consumers must
	// not turn nil into a fresh/green age.
	AgeSeconds *int64 `json:"age_seconds,omitempty"`
	Suspect    bool   `json:"suspect,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// MachineSync is the daemon's tier-0, per-workspace projection of replicated
// machine registry notes. It compares committed tips only: dirty worktree state
// is intentionally outside this contract. Error makes the whole projection
// unavailable; it must render unknown, never as an empty/equal fleet.
type MachineSync struct {
	SchemaVersion  int               `json:"schema_version"`
	LocalMachineID string            `json:"local_machine_id,omitempty"`
	RootID         string            `json:"root_id,omitempty"`
	RepoPath       string            `json:"repo_path,omitempty"`
	LocalBranch    string            `json:"local_branch,omitempty"`
	LocalSHA       string            `json:"local_sha,omitempty"`
	Peers          []MachineSyncPeer `json:"peers"`
	Error          string            `json:"error,omitempty"`
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
	TaskResults  map[string]*TaskResult `json:"task_results,omitempty"`
	TestReports  map[string]*TestReport `json:"test_reports,omitempty"`
	ChangedFiles []git.FileStatus       `json:"changed_files,omitempty"`
	BlobHashes   map[string]string      `json:"blob_hashes,omitempty"`
	// ChangedFilesComputed is true once the per-file scan has run, so a clean
	// repo (ChangedFiles nil, dropped by omitempty) is distinguishable from an
	// uncomputed one. Key cache-hit/backfill on this, not ChangedFiles != nil.
	// No omitempty: absent from an older daemon decodes false = safe fallback.
	ChangedFilesComputed bool `json:"changed_files_computed"`
	// GitLanding is the repo's rebase-preflight position — local-main
	// divergence, origin-branch presence, behind-origin, last-commit time — all
	// measured against the bases git.LandingState documents. It is what lets a
	// consumer render a landing verdict that AGREES with the Rebase page without
	// shelling out; GitStatus' AheadMainCount/BehindMainCount use a base that
	// varies with the checkout and must not be used for that. Nil (or
	// Computed=false, from an older daemon) means "not known yet" — render
	// pending, never a confident zero.
	GitLanding *git.LandingState `json:"git_landing,omitempty"`
	// ReviewStats is the forge poller's projection for this workspace. Nil
	// means the poller never spoke about this workspace at all (disabled, no
	// forge remote, or an older daemon) — which is "unknown", not "no PRs".
	ReviewStats *ReviewStats `json:"review_stats,omitempty"`
	// MachineSync is the daemon's committed-tip-only projection of replicated
	// machine registry notes. Nil means no projection has run (or an older
	// daemon), which is unknown rather than equal.
	MachineSync *MachineSync `json:"machine_sync,omitempty"`
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
	TaskResults  map[string]*TaskResult `json:"task_results,omitempty"`
	TestReports  map[string]*TestReport `json:"test_reports,omitempty"`
	ChangedFiles []git.FileStatus       `json:"changed_files,omitempty"`
	BlobHashes   map[string]string      `json:"blob_hashes,omitempty"`
	// *bool per the pointer convention above: nil = unchanged. Only the git
	// delta builders set it, so non-git deltas leave the stored flag intact.
	ChangedFilesComputed *bool `json:"changed_files_computed,omitempty"`
	// GitLanding follows the pointer convention: nil = unchanged, so only the
	// git delta builders ever replace the stored landing state.
	GitLanding *git.LandingState `json:"git_landing,omitempty"`
	// ReviewStats follows the pointer convention: nil = unchanged. Only the
	// forge poller sets it, and it always sends a complete value — the poller
	// degrades an entry to stale rather than dropping fields off it.
	ReviewStats *ReviewStats `json:"review_stats,omitempty"`
	// MachineSync follows the ReviewStats pointer convention: nil = unchanged;
	// a non-nil value always replaces the complete projection.
	MachineSync *MachineSync `json:"machine_sync,omitempty"`
}
