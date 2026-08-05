package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/grovetools/core/pkg/exectrust"
)

// Trust inheritance for worktree checkouts.
//
// The exec-provenance gate in execgate.go keys trust on (config file PATH,
// digest of the exec values that file carried when the user reviewed it). The
// path half is what makes a worktree expensive: `grove worktree` produces a
// second checkout of a repo the user already reviewed, at a new path, and the
// gate treats it as an unreviewed stranger. An ecosystem with 24 repos and 20
// plan worktrees owes ~500 approvals for what is really ONE decision per repo.
//
// This file closes that gap without weakening the gate, by relocating an
// existing decision rather than minting a new one:
//
//	trust <worktree>/<repo>/grove.toml at digest D
//	  IFF <owner>/<repo>/grove.toml is already trusted at exactly D
//
// The digest is the whole point. If the worktree's branch edited a hook, added
// a [tui.plugins] entry, or touched [claude], its digest differs from the
// owner's and NOTHING is inherited — the gate stays shut and the user is asked
// about the new content, which is exactly the behavior execgate.go's digest
// binding exists to produce. So inheritance never causes a command to run that
// the user has not read; it only stops asking twice about bytes they already
// read once.
//
// It is deliberately NOT content-addressed trust ("this digest is trusted
// anywhere"). Identical exec values are not identical risk: `make fmt` runs a
// different Makefile in a different tree, so a digest match between UNRELATED
// repos means nothing. Inheritance is anchored to a specific owner checkout of
// the same repo, and the caller supplies that provenance — from the worktree
// registry, or from Prepare, which just created the worktree and knows.

// InheritCandidate is one (source -> dest) trust relocation to evaluate.
// Source is the owner checkout's config file whose trust decision would be
// relocated; Dest is the worktree checkout's config file that would receive it.
type InheritCandidate struct {
	// Source is the owner checkout's config file path.
	Source string
	// Dest is the worktree checkout's config file path.
	Dest string
	// Repo names the member workspace, for reporting. Optional.
	Repo string
}

// InheritSkipReason explains why a candidate was not granted. Empty when it
// was.
type InheritSkipReason string

const (
	// InheritAlreadyTrusted: dest is already trusted at its current digest.
	InheritAlreadyTrusted InheritSkipReason = "already trusted"
	// InheritNoExecConfig: dest carries no exec-bearing values, so there is
	// nothing for the gate to withhold and nothing to trust.
	InheritNoExecConfig InheritSkipReason = "no exec-bearing config"
	// InheritDestUnreadable: dest is missing or unparseable.
	InheritDestUnreadable InheritSkipReason = "worktree config unreadable"
	// InheritSourceUnreadable: the owner checkout's file is missing or
	// unparseable, so there is no decision to relocate.
	InheritSourceUnreadable InheritSkipReason = "owner config unreadable"
	// InheritSourceUntrusted: the owner checkout has not been trusted, so
	// there is no decision to relocate. Trust the owner first.
	InheritSourceUntrusted InheritSkipReason = "owner config not trusted"
	// InheritDigestMismatch: the worktree's config carries different exec
	// values than the owner's — the branch changed them. The gate stays shut
	// by design; review the worktree directly.
	InheritDigestMismatch InheritSkipReason = "worktree config differs from owner"
)

// InheritOutcome is the per-candidate result.
type InheritOutcome struct {
	// Source and Dest echo the candidate.
	Source string `json:"source"`
	Dest   string `json:"dest"`
	// Repo names the member workspace, when the caller supplied it.
	Repo string `json:"repo,omitempty"`
	// Digest is dest's current exec digest ("" when it carries none).
	Digest string `json:"digest,omitempty"`
	// Granted reports whether trust was (or, in report mode, would be)
	// relocated onto Dest.
	Granted bool `json:"granted"`
	// Reason explains a non-grant. Empty when Granted.
	Reason InheritSkipReason `json:"reason,omitempty"`
}

// ExecDigestForFile computes the exec-value digest of ONE config file, along
// the same read -> expand -> unmarshal path the cascade uses, so the result is
// directly comparable with the digests the gate records. An empty digest with
// a nil error means the file carries no exec-bearing values at all.
func ExecDigestForFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	cfg, err := unmarshalConfig(path, []byte(expandEnvVars(string(data))))
	if err != nil {
		return "", err
	}
	return ExecDigest(cfg), nil
}

// WorktreeInheritCandidates builds the candidate set for one worktree: each
// member repo's config file in the worktree, paired with the same repo's file
// in the owner checkout.
//
// Owner resolution handles both worktree shapes. When the owner IS the
// ecosystem root, member repos sit directly beneath it (<owner>/<repo>). When
// the worktree is ANCHORED, the registry's owner is a sub-repo
// (~/Code/grovetools/grove) and the members are its siblings, one level up
// (<dir(owner)>/<repo>). The sibling form also covers a standalone repo's own
// worktree, where repo == filepath.Base(owner) makes the two forms identical.
//
// The container's own synthetic grove.toml is not a candidate: it holds only
// `workspaces = ["*"]`, so its digest is empty and there is nothing to trust.
func WorktreeInheritCandidates(ownerPath, worktreePath string, repos []string) []InheritCandidate {
	if ownerPath == "" || worktreePath == "" {
		return nil
	}
	out := make([]InheritCandidate, 0, len(repos))
	for _, repo := range repos {
		if repo == "" {
			continue
		}
		dest := filepath.Join(worktreePath, repo, "grove.toml")
		// Prefer the form that exists on disk; fall back to the nested form so
		// a missing source is reported as such rather than silently dropped.
		nested := filepath.Join(ownerPath, repo, "grove.toml")
		sibling := filepath.Join(filepath.Dir(ownerPath), repo, "grove.toml")
		source := nested
		if _, err := os.Stat(nested); err != nil {
			if _, sibErr := os.Stat(sibling); sibErr == nil {
				source = sibling
			}
		}
		out = append(out, InheritCandidate{Source: source, Dest: dest, Repo: repo})
	}
	return out
}

// InheritExecTrust evaluates candidates against the trust store and, when
// apply is true, records trust for every dest whose digest exactly matches a
// trusted source. The store is loaded once and saved once, so a backfill over
// hundreds of worktrees is a single write.
//
// With apply false nothing is written — the outcomes describe what WOULD be
// granted, which is what the report-only CLI path renders.
func InheritExecTrust(candidates []InheritCandidate, apply bool) ([]InheritOutcome, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	store := exectrust.Load()
	now := time.Now()
	outcomes := make([]InheritOutcome, 0, len(candidates))
	granted := 0

	for _, c := range candidates {
		outcome := InheritOutcome{Source: c.Source, Dest: c.Dest, Repo: c.Repo}

		destDigest, err := ExecDigestForFile(c.Dest)
		switch {
		case err != nil:
			outcome.Reason = InheritDestUnreadable
			outcomes = append(outcomes, outcome)
			continue
		case destDigest == "":
			outcome.Reason = InheritNoExecConfig
			outcomes = append(outcomes, outcome)
			continue
		}
		outcome.Digest = destDigest

		if store.IsTrusted(c.Dest, destDigest) {
			outcome.Reason = InheritAlreadyTrusted
			outcomes = append(outcomes, outcome)
			continue
		}

		sourceDigest, err := ExecDigestForFile(c.Source)
		if err != nil {
			outcome.Reason = InheritSourceUnreadable
			outcomes = append(outcomes, outcome)
			continue
		}
		// Distinguish "you never trusted the owner" from "the branch changed
		// the commands": both leave the gate shut, but only the first is fixed
		// by trusting the owner.
		if !store.IsTrusted(c.Source, sourceDigest) {
			outcome.Reason = InheritSourceUntrusted
			outcomes = append(outcomes, outcome)
			continue
		}
		if sourceDigest != destDigest {
			outcome.Reason = InheritDigestMismatch
			outcomes = append(outcomes, outcome)
			continue
		}

		outcome.Granted = true
		granted++
		if apply {
			store.Trust(c.Dest, destDigest, now)
		}
		outcomes = append(outcomes, outcome)
	}

	if apply && granted > 0 {
		if err := store.Save(); err != nil {
			return outcomes, fmt.Errorf("record inherited trust: %w", err)
		}
	}
	return outcomes, nil
}

// InheritGrantedCount counts the granted outcomes, for callers that only need
// the tally.
func InheritGrantedCount(outcomes []InheritOutcome) int {
	n := 0
	for _, o := range outcomes {
		if o.Granted {
			n++
		}
	}
	return n
}

// InheritWorktreeTrustEnabled reports whether grove may relocate trust onto a
// newly created worktree, resolved from the USER-controlled layers only — the
// same restriction execTrustMode applies, and for the same reason: a workspace
// grove.toml must not be able to turn on the mechanism that would propagate
// its own exec config.
//
// Unset means enabled. Inheritance grants nothing the user has not already
// reviewed for that exact repo at that exact digest, and defaulting it off
// would leave the per-worktree approval tax in place for everyone by default.
// Set [security] inherit_worktree_trust = false to require a fresh review in
// every worktree.
//
// Reports false when the gate is off entirely: there is nothing to withhold,
// so there is nothing worth recording.
func InheritWorktreeTrustEnabled(dir string) bool {
	layered, err := LoadLayered(dir)
	if err != nil {
		// Fail toward the default rather than toward "never inherit": a config
		// that will not load is already reported loudly by the caller's own
		// load, and silently disabling inheritance here would look like the
		// feature is broken rather than the config.
		return true
	}
	userCfg := userLayerConfig(layered)
	if execTrustMode(userCfg) == ExecTrustModeOff {
		return false
	}
	if userCfg != nil && userCfg.Security != nil && userCfg.Security.InheritWorktreeTrust != nil {
		return *userCfg.Security.InheritWorktreeTrust
	}
	return true
}
