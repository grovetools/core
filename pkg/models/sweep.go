package models

// GitSweepProgress is the payload of the daemon's sweep_started /
// sweep_progress / sweep_completed events: the live position of a tier-ordered
// git status sweep.
//
// It exists because the sweep is no longer a single burst whose only
// observable is "it finished". A boot sweep now runs hot workspaces at full
// concurrency and trickles the cold tail over minutes, so "is the fleet swept
// yet" is a question with an answer that changes for the whole duration —
// which is exactly what a progress bar in `groved monitor` or a treemux
// Inspector page needs, and exactly what polling /api/system/stats renders
// badly.
//
// Counts are cumulative within one sweep. TierDone/TierTotal describe the tier
// named by Tier; Done/Total describe the whole sweep. A sweep_completed event
// carries Done == Total (or fewer, when the sweep was cut short by shutdown —
// compare the two rather than assuming).
type GitSweepProgress struct {
	// Reason is what triggered the sweep: "boot", "refresh" (a bodyless
	// /api/refresh, e.g. after a rebuild or host registration), or
	// "reconcile" (the hourly correctness pass).
	Reason string `json:"reason"`
	// Scope is the owning daemon's scope ("" == global). Only the global
	// daemon sweeps, so this is empty in practice; it is carried so a consumer
	// watching several daemons can attribute a frame.
	Scope string `json:"scope,omitempty"`
	// Tier names the tier the sweep is working on: "hot", "active", "warm" or
	// "cold". Empty on sweep_started (no tier has begun) and on
	// sweep_completed.
	Tier string `json:"tier,omitempty"`
	// TierDone/TierTotal are that tier's progress. Tier membership is
	// re-evaluated as the sweep runs — a workspace that becomes focused mid
	// sweep is promoted to hot and swept next — so TierTotal may grow.
	TierDone  int `json:"tier_done,omitempty"`
	TierTotal int `json:"tier_total,omitempty"`
	// Done/Total are the whole sweep's progress in workspaces.
	Done  int `json:"done"`
	Total int `json:"total"`
	// TierTotals is the initial per-tier plan, keyed by tier name. Present on
	// sweep_started so a consumer can render the shape of the work before any
	// of it happens; omitted afterwards.
	TierTotals map[string]int `json:"tier_totals,omitempty"`
	// ElapsedMS is wall time since the sweep started, pacing sleeps included.
	ElapsedMS int64 `json:"elapsed_ms"`
	// WorkMS is scan time only, pacing sleeps excluded. The gap between this
	// and ElapsedMS is the deliberate slowness: a cold tail that is 90% sleep
	// is the design working, not a stall.
	WorkMS int64 `json:"work_ms"`
	// Emitted is how many workspaces produced a state change so far. A sweep
	// that scans hundreds and emits none is the healthy steady state.
	Emitted int `json:"emitted,omitempty"`
}
