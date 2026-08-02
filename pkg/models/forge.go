package models

import (
	"time"

	"github.com/grovetools/core/pkg/forge"
)

// This file mirrors the daemon's GET /api/forge/state payload the way SyncStatus
// mirrors GET /api/sync/status: the authoritative shape is the daemon's own
// forge cache entry, and this is the client-side view of the same JSON. It lives
// in models rather than in the daemon because the daemon's store package is
// internal, and every consumer of the read surface (git-viewer's PRs page and
// its headless twin) is outside the daemon module.
//
// Keep the json tags in lockstep with daemon/internal/daemon/store/forge.go —
// they ARE the contract between the two.

// Forge cache entry states. They are the same three words the daemon's SSE
// payload and models.ReviewFreshness use, deliberately: a surface must never
// have to reconcile two vocabularies for "does the daemon know this".
const (
	// ForgeStateUnknown means no successful fetch has ever landed for this repo.
	// PRs is nil, not empty — nothing about this repo may be rendered as a fact.
	ForgeStateUnknown = "unknown"
	// ForgeStateFresh means the last fetch succeeded inside the freshness window.
	ForgeStateFresh = "fresh"
	// ForgeStateStale means a fetch succeeded once but the latest attempt failed,
	// or the data aged past the freshness window. The payload carries the last
	// known good data, dated by FetchedAt.
	ForgeStateStale = "stale"
)

// ForgeRepoState is the daemon forge poller's cached knowledge of one repository.
//
// The load-bearing rule (STATE.md D4), and the reason PRs is a slice that may be
// nil: a poll failure degrades an entry to ForgeStateStale and KEEPS the data it
// already had. It never evicts the entry and never replaces the data with an
// empty slice — "we could not ask" must never render as "there are no pull
// requests", and an unknown entry must never render green.
type ForgeRepoState struct {
	// Provider is the forge provider name ("github").
	Provider string `json:"provider"`
	// Repo is the fully qualified "host/owner/name" identity. A slug alone is
	// ambiguous across forges.
	Repo string `json:"repo"`
	// State is one of the ForgeState* constants above.
	State string `json:"state"`
	// FetchedAt is when the carried data was last fetched SUCCESSFULLY. Zero
	// while State is unknown; it does NOT advance on a failed attempt.
	FetchedAt time.Time `json:"fetched_at,omitempty"`
	// LastAttemptAt is when the poller last tried, successfully or not. The gap
	// between this and FetchedAt is how long the entry has been failing.
	LastAttemptAt time.Time `json:"last_attempt_at,omitempty"`
	// PRs is nil when State is unknown, and a (possibly empty) slice otherwise.
	// An empty non-nil slice is the forge affirmatively reporting no PRs.
	PRs []forge.PullRequest `json:"prs,omitempty"`
	// Checks maps a pull request number to the rollup fetched for its head ref.
	// A PR absent from this map has an UNKNOWN rollup, never a green one.
	Checks map[int]forge.CheckRollup `json:"checks,omitempty"`
	// LastError is the most recent failure text, present whenever the last
	// attempt failed.
	LastError string `json:"last_error,omitempty"`
	// ConsecutiveFailures is how many sweeps in a row have failed for this repo.
	// Zero after any success.
	ConsecutiveFailures int `json:"consecutive_failures,omitempty"`
	// NextAttemptAt is when the poller will try this repo again — the honest
	// name for the quiet period after an outage. The poller backs a failing repo
	// off exponentially (2^n poll intervals, capped), so connectivity coming
	// back does NOT mean data comes back on the next sweep. Zero means "at the
	// next sweep". Render it: a surface that shows only "stale" turns a designed
	// wait into what reads like a hang.
	NextAttemptAt time.Time `json:"next_attempt_at,omitempty"`
}

// ForgeStatePayload is the wire payload of a "forge_state" SSE frame: the repo
// entries whose state MATERIALLY changed on one poller sweep, never the whole
// cache. It mirrors daemon/internal/daemon/store.ForgeStatePayload the way
// ForgeRepoState mirrors that package's cache entry.
//
// The stream is lossy by design (the poller emits only on change, and a late
// subscriber missed every earlier frame), so a consumer must never treat "I
// have seen no forge_state frame" as "there is nothing to review". Reconcile
// from GET /api/forge/state (ForgeStateSnapshot) or from the ReviewStats that
// rides workspaces_delta; use these frames as the freshening signal.
type ForgeStatePayload struct {
	// Repos are the changed entries, sorted by Repo for a deterministic frame.
	Repos []ForgeRepoState `json:"repos,omitempty"`
}

// ForgeStateSnapshot is the whole read surface: every repo the poller watches,
// plus whether the poller is running at all.
//
// Enabled is not decoration. An empty Repos list means two completely different
// things depending on it — "the poller is off, so nothing is known" versus "the
// poller is on and watches no repository" — and a surface that cannot tell them
// apart renders an empty list that reads as "no pull requests". Every consumer
// must check Enabled before interpreting Repos.
type ForgeStateSnapshot struct {
	// Enabled reports whether the daemon has a forge poller running. False means
	// [forge.poll] is off, or the provider's transport (the `gh` CLI) is absent.
	Enabled bool `json:"enabled"`
	// Provider names the running provider ("github"), empty when disabled.
	Provider string `json:"provider,omitempty"`
	// Repos is the cache, sorted by Repo. Nil/empty when disabled.
	Repos []ForgeRepoState `json:"repos,omitempty"`
}
