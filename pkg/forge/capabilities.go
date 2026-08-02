package forge

import "sort"

// Support is the explicit tri-state answer to "can this provider do X?".
//
// The third state exists because "we never probed" and "the forge told us no"
// are different facts, and collapsing them makes a capability matrix lie in
// whichever direction the collapse chose.
type Support string

const (
	// SupportUnknown means the capability was not probed. This is the value
	// for any capability a provider does not explicitly declare, and the zero
	// value of Support once normalized (see Capabilities.Get).
	SupportUnknown Support = "unknown"
	// SupportSupported means the provider answers this call against a live forge.
	SupportSupported Support = "supported"
	// SupportUnsupported means the provider will return a ClassUnsupported
	// error for this call — proven absent, not merely unprobed.
	SupportUnsupported Support = "unsupported"
)

// Normalized maps the zero value onto SupportUnknown.
func (s Support) Normalized() Support {
	if s == "" {
		return SupportUnknown
	}
	return s
}

// Capability names one thing a caller may want to do with a forge.
type Capability string

const (
	// CapResolveRepo — parse a remote URL into a Repo identity.
	CapResolveRepo Capability = "resolve_repo"
	// CapListPRs — list pull requests for a repo.
	CapListPRs Capability = "list_prs"
	// CapGetPR — fetch a single pull request by number.
	CapGetPR Capability = "get_pr"
	// CapChecks — fetch the CI rollup for a ref.
	CapChecks Capability = "checks"
	// CapPagination — the provider pages through results rather than
	// silently returning only the first page.
	CapPagination Capability = "pagination"
	// CapDraftPRs — the provider distinguishes draft pull requests.
	CapDraftPRs Capability = "draft_prs"
	// CapPRReviews — read review threads and approvals. Unprobed in this
	// wave; providers report SupportUnknown, not SupportUnsupported.
	CapPRReviews Capability = "pr_reviews"
	// CapIssues — read issues as distinct from pull requests. Unprobed in
	// this wave.
	CapIssues Capability = "issues"
	// CapMutations — create/merge/close/comment. This wave's providers are
	// structurally read-only, so both report SupportUnsupported.
	CapMutations Capability = "mutations"
)

// Capabilities is a provider's capability matrix. A missing key is not an
// error and not a "no": Get reports SupportUnknown for it.
type Capabilities map[Capability]Support

// Get returns the support level for cap. Absence — and an explicitly stored
// empty value — is SupportUnknown, never SupportUnsupported.
func (c Capabilities) Get(cap Capability) Support {
	if c == nil {
		return SupportUnknown
	}
	return c[cap].Normalized()
}

// Supports reports whether cap is explicitly supported. Unknown is not
// supported: a caller gating behavior on this must treat "we don't know" as
// "don't".
func (c Capabilities) Supports(cap Capability) bool {
	return c.Get(cap) == SupportSupported
}

// Keys returns the explicitly declared capabilities, sorted, for rendering a
// matrix deterministically.
func (c Capabilities) Keys() []Capability {
	out := make([]Capability, 0, len(c))
	for k := range c {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Clone returns a copy, so a provider can hand out its matrix without a caller
// mutating the provider's state.
func (c Capabilities) Clone() Capabilities {
	out := make(Capabilities, len(c))
	for k, v := range c {
		out[k] = v
	}
	return out
}
