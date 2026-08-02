package git

// Landing state: the CANONICAL divergence contract behind every "can this repo
// land, and what's in the way" surface.
//
// The problem this type exists to close: ExtendedGitStatus' AheadMainCount /
// BehindMainCount are measured against a base that VARIES with the checkout —
// local main for a feature branch, origin/main for a main/master checkout with
// no upstream, and nothing at all (a confident zero) for a main/master checkout
// that HAS an upstream. The rebase preflight, by contrast, always measures
// against the repo's own local main/master. Feeding the former into a surface
// that renders the latter's verdict makes a mini view contradict the real one,
// which is worse than a slow view.
//
// LandingState is therefore defined by the preflight, not by git status: its
// bases are fixed, spelled out below, and identical to what
// viewer.Preflight computes live. A producer (the daemon's git enrichment) fills
// it once per repo per scan; a consumer (treemux's drawer state pane) renders
// from it with zero subprocesses. The one preflight input NOT here is the
// merge-tree conflict look-ahead: it is a function of the merge itself, far more
// expensive than a ref read, and consumers run it on their own slower cadence.

import (
	"errors"
	"path/filepath"
	"sync"
	"time"
)

// LandingState is a repository's ref-and-divergence position, measured against
// exactly the bases the rebase preflight uses:
//
//   - Onto is the repo's OWN local main/master (git.LocalMainBranch) — never
//     origin/main, never an upstream. "" means neither branch exists locally,
//     which is the preflight's "no main/master" refusal.
//   - Ahead/Behind are HEAD vs Onto (`rev-list --left-right --count
//     <onto>...HEAD`), computed for every checkout including a main/master one
//     (where they are 0/0 because HEAD is that ref).
//   - HasRemote/BehindOrigin are the push distance against the DEFAULT remote
//     (git.DefaultRemoteName): whether origin/<branch> exists and how far HEAD
//     trails it. Surfaces render the distance ONLY when HasRemote, leaving
//     un-pushed branches blank. Divergence against any OTHER remote is not part
//     of this shape — it is the Remotes surface's question, answered live by
//     git.ListRemoteBranchStates rather than cached here.
//   - LastCommitAt is HEAD's author date, so an age column costs no extra fork
//     at the consumer.
//
// Computed carries no omitempty on purpose (the house wire-compat rule, same as
// EnrichedWorkspace.ChangedFilesComputed): a payload from an older producer that
// knows nothing about landing state decodes to Computed=false, which consumers
// must treat as "not known yet" — a pending row — rather than as a confident
// zero divergence.
type LandingState struct {
	Onto   string `json:"onto"`
	Ahead  int    `json:"ahead"`
	Behind int    `json:"behind"`

	HasRemote    bool `json:"has_remote"`
	BehindOrigin int  `json:"behind_origin"`

	LastCommitAt time.Time `json:"last_commit_at,omitempty"`

	Computed bool `json:"computed"`
}

// GetLandingState computes the landing position of repoPath, whose current
// branch is branch (as reported by a status probe — pass the porcelain v2
// spelling; a detached HEAD in any of its spellings suppresses the origin
// probes via IsDetachedHead).
//
// Cost: up to two show-ref probes for local main/master, one for
// origin/<branch>, two rev-lists, and one `log -1` — but zero forks when the
// memo below is warm and no ref has moved, which is the steady state for a
// daemon sweeping hundreds of repos on a schedule.
//
// It is deliberately unconditional: unlike the preflight ladder it does NOT
// skip the divergence work for a dirty or detached checkout. The producer runs
// off the hot path and a consumer's ladder short-circuits on those states
// anyway, so computing them costs the producer little and saves the consumer a
// second round trip the moment the tree goes clean.
func GetLandingState(repoPath, branch string) *LandingState {
	cleanPath := filepath.Clean(repoPath)

	fp, resolved := resolveLandingFingerprint(cleanPath, branch)
	if resolved {
		if cached, ok := lookupLanding(cleanPath, fp); ok {
			return &cached
		}
	}

	state := LandingState{Computed: true}
	state.Onto = LocalMainBranch(cleanPath)

	if exists, behind := RemoteBranchDistance(cleanPath, DefaultRemoteName, branch); exists {
		state.HasRemote = true
		state.BehindOrigin = behind
	}

	if state.Onto != "" {
		if ahead, behind, err := GetCommitsDivergence(cleanPath, state.Onto, "HEAD"); err == nil {
			state.Ahead, state.Behind = ahead, behind
		}
	}

	if entries, err := GetLog(cleanPath, 1); err == nil && len(entries) > 0 {
		state.LastCommitAt = entries[0].Date
	}

	// Only cache when the endpoints are unchanged from the pre-fork snapshot:
	// a ref that moved while rev-list ran would pin a result to a fingerprint it
	// was not computed from. Mirrors storeDivergenceIfCurrent's discipline.
	if resolved {
		if after, ok := resolveLandingFingerprint(cleanPath, branch); ok && after == fp {
			storeLanding(cleanPath, fp, state)
		}
	}
	return &state
}

// landingFingerprint pins a LandingState to the refs it was computed from.
// Every field of the state is a pure function of these SHAs (plus HEAD's own
// commit, which supplies LastCommitAt), so an unchanged fingerprint means an
// unchanged answer.
type landingFingerprint struct {
	headSHA   string
	ontoRef   string
	ontoSHA   string
	originRef string
	originSHA string
}

type landingEntry struct {
	landingFingerprint
	state LandingState
}

const maxLandingCacheEntries = 8192

var (
	landingCacheMu sync.Mutex
	landingCache   = make(map[string]landingEntry) // keyed by cleaned repo path
)

func lookupLanding(cleanPath string, fp landingFingerprint) (LandingState, bool) {
	landingCacheMu.Lock()
	defer landingCacheMu.Unlock()
	e, ok := landingCache[cleanPath]
	if !ok || e.landingFingerprint != fp {
		return LandingState{}, false
	}
	return e.state, true
}

func storeLanding(cleanPath string, fp landingFingerprint, state LandingState) {
	landingCacheMu.Lock()
	defer landingCacheMu.Unlock()
	if len(landingCache) >= maxLandingCacheEntries {
		landingCache = make(map[string]landingEntry)
	}
	landingCache[cleanPath] = landingEntry{landingFingerprint: fp, state: state}
}

// resolveLandingFingerprint reads the three endpoints from git's own files,
// with zero forks, reusing the divergence cache's resolver (see
// divergence_cache.go for the layouts it handles and the ones it punts on).
//
// It is strictly a cache-VALIDITY probe, never an authority: ok=false means
// "recompute and do not cache", so correctness never depends on the resolver
// keeping up with git's on-disk formats. In particular a local main/master that
// does not cleanly resolve — including a repo that has neither, and any layout
// the resolver cannot read (reftable) — refuses the cache outright rather than
// fingerprinting a repo as "nothing here" and then never noticing it move.
func resolveLandingFingerprint(cleanPath, branch string) (landingFingerprint, bool) {
	gitDir, commonDir, err := resolveGitDirs(cleanPath)
	if err != nil {
		return landingFingerprint{}, false
	}
	headSHA, err := resolveHeadSHA(gitDir, commonDir)
	if err != nil {
		return landingFingerprint{}, false
	}

	fp := landingFingerprint{headSHA: headSHA}
	for _, ref := range localMainRefCandidates {
		sha, err := resolveRefSHA(commonDir, ref)
		switch {
		case err == nil:
			fp.ontoRef, fp.ontoSHA = ref, sha
		case errors.Is(err, errRefNotFound):
			continue
		default:
			return landingFingerprint{}, false
		}
		break
	}
	if fp.ontoRef == "" {
		return landingFingerprint{}, false
	}

	// A cleanly-absent origin/<branch> is a real answer ("not pushed"), so it
	// fingerprints as the empty SHA rather than refusing the cache.
	if !IsDetachedHead(branch) {
		ref := RemoteTrackingRef(DefaultRemoteName, branch)
		switch sha, err := resolveRefSHA(commonDir, ref); {
		case err == nil:
			fp.originRef, fp.originSHA = ref, sha
		case errors.Is(err, errRefNotFound):
		default:
			return landingFingerprint{}, false
		}
	}
	return fp, true
}
