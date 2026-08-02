package forge

import "context"

// Bounds every implementation must respect. Pagination is handled inside the
// implementations — callers never see cursors — but it is bounded, so a
// runaway remote cannot make a poller loop forever.
const (
	// DefaultPageSize is the per-request page size implementations request.
	DefaultPageSize = 50
	// MaxPages caps how many pages any single call will fetch.
	MaxPages = 10
	// MaxItems is the hard ceiling on items returned by one list call.
	MaxItems = DefaultPageSize * MaxPages
)

// ListPROptions filters a ListPRs call. The zero value lists open pull
// requests up to MaxItems.
type ListPROptions struct {
	// State filters by lifecycle state. The zero value means open only; use
	// StateAll to list every state. PRStateUnknown is not a valid filter.
	State PRState
	// Limit caps the number of pull requests returned. Zero means MaxItems;
	// values above MaxItems are clamped to it.
	Limit int
}

// StateAll is the ListPROptions.State value meaning "do not filter by state".
const StateAll PRState = "all"

// EffectiveLimit resolves Limit against the package bounds.
func (o ListPROptions) EffectiveLimit() int {
	if o.Limit <= 0 || o.Limit > MaxItems {
		return MaxItems
	}
	return o.Limit
}

// EffectiveState resolves State, defaulting to open.
func (o ListPROptions) EffectiveState() PRState {
	if o.State == "" {
		return PRStateOpen
	}
	return o.State
}

// Provider is the read-only forge seam.
//
// It is transport- and domain-primitive on purpose: it answers "what does the
// forge say" and nothing else. Notebook lifecycle rules — which ticket a PR
// belongs to, whether a rollup satisfies a gate, when a note moves directories
// — live in the consumers, not here.
//
// Implementations must:
//
//   - return every error wrapped in *Error with a class (see ClassOf);
//   - never mutate a remote system;
//   - never read credentials from the environment implicitly;
//   - bound their own pagination (see MaxPages/MaxItems);
//   - report unknown as unknown, never as green, merged, or empty.
type Provider interface {
	// Name is the provider's stable identifier ("github", "forgejo"). It is
	// what gets persisted alongside a PR reference.
	Name() string

	// Capabilities returns this provider's capability matrix. Capabilities not
	// present are SupportUnknown (see Capabilities.Get).
	Capabilities() Capabilities

	// ResolveRepo turns a git remote URL into a Repo identity. It performs no
	// I/O — it is pure parsing and validation — so it works offline and never
	// returns ClassUnavailable.
	ResolveRepo(remoteURL string) (Repo, error)

	// ListPRs lists pull requests for a repo, bounded by opts.
	ListPRs(ctx context.Context, repo Repo, opts ListPROptions) ([]PullRequest, error)

	// GetPR fetches a single pull request by number.
	GetPR(ctx context.Context, repo Repo, number int) (PullRequest, error)

	// Checks returns the CI rollup for a ref (branch name or SHA).
	//
	// A transport failure must surface as an error, not as an empty rollup:
	// callers distinguish "no checks" (CheckStateNone) from "could not tell"
	// (an error, or CheckStateUnknown) and must never render either as green.
	Checks(ctx context.Context, repo Repo, ref string) (CheckRollup, error)
}
