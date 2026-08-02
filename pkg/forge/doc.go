// Package forge is the shared, read-only forge provider seam: one
// transport/domain-primitive interface over GitHub (via the `gh` CLI) and
// Forgejo/Gitea (via its REST API), consumed by the daemon's forge poller,
// read-only surfaces, and later mutation work.
//
// # Read-only by design
//
// This package declares no mutation methods. Creating, merging, closing or
// commenting on pull requests and issues is deliberately absent: declaring API
// that nothing implements is worse than adding it in the wave that needs it.
// Every operation here is a read; no implementation in this package or its
// sub-packages may write to a remote system.
//
// # Unknown is a first-class result
//
// The recurring failure mode of a status surface is rendering "I could not
// find out" as "everything is fine". This package refuses to do that:
//
//   - [CheckState] has an explicit [CheckStateUnknown] distinct from
//     [CheckStateNone] ("the forge reports no checks at all") and from
//     [CheckStateSuccess]. Only [CheckStateSuccess] is green; see
//     [CheckState.IsGreen].
//   - [RollupState] merges per-check states with failure > unknown > pending >
//     success precedence, so a single unknown check can never roll up green.
//   - [PRState] has an explicit [PRStateUnknown] for states a provider reports
//     that this package does not recognize — never silently mapped to closed
//     or merged.
//   - [Capabilities] returns [SupportUnknown] for any capability that was not
//     explicitly probed (see [Capabilities.Get]); capability absence is
//     unknown, not unsupported.
//
// # Error classes
//
// Every error a provider returns is wrapped in an [Error] carrying an
// [ErrorClass] so callers can decide policy without string matching:
//
//   - [ClassRetryable] — transient; the same call may succeed later
//     (rate limits, 5xx, timeouts).
//   - [ClassPermanent] — will not succeed by retrying (404, malformed input).
//   - [ClassUnavailable] — the transport itself is not usable right now
//     (`gh` not installed or not authenticated, forge unreachable, no token).
//     Callers should degrade to "unknown", never to "absent" or "green".
//   - [ClassUnsupported] — the provider cannot answer this question at all.
//
// # No implicit credentials
//
// No code path in this package or its sub-packages reads a token from the
// environment. The GitHub implementation delegates all credential handling to
// the `gh` CLI (which owns its own auth). The Forgejo implementation accepts a
// token only through an injected [forgejo.TokenFunc]; it never executes a
// `token_command` itself — that resolution belongs to the daemon.
//
// # Capability matrix
//
// The read-only surface as of this wave. "supported" means the implementation
// answers the call against a live forge; "unsupported" means it will return a
// [ClassUnsupported] error; "unknown" means it was never probed.
//
//	capability          github (gh CLI)   forgejo (REST)
//	------------------  ----------------  --------------
//	CapResolveRepo      supported         supported
//	CapListPRs          supported         supported
//	CapGetPR            supported         supported
//	CapChecks           supported         supported
//	CapPagination       supported         supported
//	CapDraftPRs         supported         supported
//	CapPRReviews        unknown           unknown
//	CapIssues           unknown           unknown
//	CapMutations        unsupported       unsupported
//
// CapPRReviews and CapIssues are "unknown" rather than "unsupported" on
// purpose: they are unprobed, not proven absent. CapMutations is
// "unsupported" because this wave's providers structurally cannot mutate.
package forge
