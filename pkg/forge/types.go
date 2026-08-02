package forge

import (
	"fmt"
	"strings"
	"time"
)

// Repo is a forge-agnostic repository identity: the parsed, validated result
// of resolving a git remote URL. Host is the lowercased hostname with no
// userinfo and no port — port is a transport detail, and the forge base URL
// comes from configuration, not from a remote URL.
type Repo struct {
	Host  string `json:"host"`
	Owner string `json:"owner"`
	Name  string `json:"name"`
}

// Slug returns the "owner/name" identity used by both forges' APIs.
func (r Repo) Slug() string { return r.Owner + "/" + r.Name }

// String returns the fully qualified "host/owner/name" identity. Two repos
// with the same slug on different hosts are different repos; callers that
// compare identity across forges must compare this, not Slug.
func (r Repo) String() string { return r.Host + "/" + r.Slug() }

// IsZero reports whether the repo carries no identity at all.
func (r Repo) IsZero() bool { return r.Host == "" && r.Owner == "" && r.Name == "" }

// PRState is a pull request's lifecycle state.
//
// PRStateUnknown is not a placeholder: it is what a provider must report when
// the remote returns a state this package does not recognize. Mapping an
// unrecognized state onto "closed" or "merged" would let a surface claim work
// finished that did not.
type PRState string

const (
	// PRStateUnknown means the remote's state could not be recognized.
	PRStateUnknown PRState = "unknown"
	// PRStateOpen means the PR is open (including drafts; see
	// PullRequest.IsDraft).
	PRStateOpen PRState = "open"
	// PRStateClosed means the PR was closed without merging.
	PRStateClosed PRState = "closed"
	// PRStateMerged means the PR was merged.
	PRStateMerged PRState = "merged"
)

// ParsePRState maps a provider's raw state string onto a PRState. Anything
// unrecognized — including the empty string — becomes PRStateUnknown.
func ParsePRState(s string) PRState {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "open":
		return PRStateOpen
	case "closed":
		return PRStateClosed
	case "merged":
		return PRStateMerged
	default:
		return PRStateUnknown
	}
}

// Normalized maps the zero value onto PRStateUnknown so a PRState that was
// never set can never be mistaken for a real state.
func (s PRState) Normalized() PRState {
	if s == "" {
		return PRStateUnknown
	}
	return s
}

// PullRequest is the read-only projection of a pull request. It carries
// transport facts only — no lifecycle or notebook policy (which ticket it
// belongs to, whether it satisfies a gate) lives here.
type PullRequest struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	State     PRState   `json:"state"`
	IsDraft   bool      `json:"is_draft"`
	Author    string    `json:"author,omitempty"`
	HeadRef   string    `json:"head_ref,omitempty"`
	HeadSHA   string    `json:"head_sha,omitempty"`
	BaseRef   string    `json:"base_ref,omitempty"`
	URL       string    `json:"url,omitempty"`
	Labels    []string  `json:"labels,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
	// MergedAt is nil unless the forge reported a merge timestamp. A nil
	// MergedAt with State == PRStateMerged is possible (the forge said merged
	// but gave no timestamp) and must not be read as "not merged".
	MergedAt *time.Time `json:"merged_at,omitempty"`
}

// CheckState is the state of a single CI check or of a rollup.
//
// The distinction between CheckStateUnknown and CheckStateNone is load-bearing:
// "I could not determine the checks" and "this forge reports zero checks" have
// opposite meanings for a gate, and neither is green.
type CheckState string

const (
	// CheckStateUnknown means the state could not be determined. This is the
	// value providers must use when the transport failed, when the forge
	// reported a status string this package does not recognize, or when the
	// data is stale beyond the caller's tolerance.
	CheckStateUnknown CheckState = "unknown"
	// CheckStateNone means the forge affirmatively reported no checks.
	CheckStateNone CheckState = "none"
	// CheckStatePending means at least one check is queued or running.
	CheckStatePending CheckState = "pending"
	// CheckStateSuccess means every check completed successfully. It is the
	// only green state.
	CheckStateSuccess CheckState = "success"
	// CheckStateFailure means at least one check failed, errored, or was
	// cancelled/timed out.
	CheckStateFailure CheckState = "failure"
	// CheckStateNeutral means the check completed without a pass/fail verdict
	// (GitHub "neutral"/"skipped"). Neutral is not failure and not green.
	CheckStateNeutral CheckState = "neutral"
)

// IsGreen reports whether this state may be rendered as passing. Only
// CheckStateSuccess is green — in particular the zero value is not, so a
// CheckState nobody filled in can never be mistaken for a pass.
func (c CheckState) IsGreen() bool { return c == CheckStateSuccess }

// Normalized maps the zero value onto CheckStateUnknown.
func (c CheckState) Normalized() CheckState {
	if c == "" {
		return CheckStateUnknown
	}
	return c
}

// Check is one CI check on a ref.
type Check struct {
	// Name is the check's display name (GitHub check-run name, Forgejo status
	// context).
	Name string `json:"name"`
	// State is the normalized state; never empty on a provider-returned Check.
	State CheckState `json:"state"`
	// RawState is the provider's own state string, preserved so a surface can
	// show what the forge actually said when State is unknown.
	RawState string `json:"raw_state,omitempty"`
	URL      string `json:"url,omitempty"`
	// CompletedAt is the zero time while the check is still running.
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

// CheckRollup is the CI rollup for a ref: the merged state plus the checks it
// was computed from.
type CheckRollup struct {
	// Ref is the ref the rollup was computed for, as passed to Checks.
	Ref string `json:"ref"`
	// State is the merged state (see RollupState).
	State CheckState `json:"state"`
	// Checks are the individual checks, in the order the forge returned them.
	Checks []Check `json:"checks,omitempty"`
	// Truncated is true when the provider hit its page bound before the forge
	// ran out of checks: State was computed from an incomplete set. A
	// truncated rollup is never green (RollupState is not consulted for this;
	// providers set State to CheckStateUnknown when truncating).
	Truncated bool `json:"truncated,omitempty"`
}

// UnknownRollup returns a rollup that asserts nothing. Providers use it when a
// transport error prevents them from answering at all.
func UnknownRollup(ref string) CheckRollup {
	return CheckRollup{Ref: ref, State: CheckStateUnknown}
}

// RollupState merges per-check states into a single state with the precedence
//
//	failure > unknown > pending > neutral > success
//
// and returns CheckStateNone for an empty set. Unknown deliberately outranks
// pending and success: a set containing one indeterminate check is
// indeterminate, never green.
func RollupState(checks []Check) CheckState {
	if len(checks) == 0 {
		return CheckStateNone
	}
	var sawUnknown, sawPending, sawNeutral bool
	for _, c := range checks {
		switch c.State.Normalized() {
		case CheckStateFailure:
			return CheckStateFailure
		case CheckStateUnknown:
			sawUnknown = true
		case CheckStatePending:
			sawPending = true
		case CheckStateNeutral:
			sawNeutral = true
		case CheckStateSuccess, CheckStateNone:
			// Success contributes nothing; a nested "none" is treated as a
			// check that reported nothing to fail on.
		default:
			// An unrecognized state is an unknown state, never an ignorable one.
			sawUnknown = true
		}
	}
	switch {
	case sawUnknown:
		return CheckStateUnknown
	case sawPending:
		return CheckStatePending
	case sawNeutral:
		return CheckStateNeutral
	default:
		return CheckStateSuccess
	}
}

// String renders a rollup compactly for logs and debugging.
func (r CheckRollup) String() string {
	s := fmt.Sprintf("%s(%d checks)", r.State.Normalized(), len(r.Checks))
	if r.Truncated {
		s += " [truncated]"
	}
	return s
}
