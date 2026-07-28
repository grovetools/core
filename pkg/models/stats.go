package models

import "time"

// SystemStats is the payload of GET /api/system/stats: a point-in-time view
// of one daemon's Go runtime plus its own OS process subtree. It complements
// SystemInfo (static version/commit identity) with live resource numbers so
// the CLI (`groved stats`), agents (curl on the unix socket), and the TUI all
// read the same data.
//
// Field names are a stable snake_case contract; treat renames/removals as
// breaking changes. Counters and Warnings are reserved in R2 (always present,
// always empty) and filled by R3.
type SystemStats struct {
	SampledAt time.Time    `json:"sampled_at"`
	Runtime   RuntimeStats `json:"runtime"`
	Self      SelfStats    `json:"self"`
	// Counters is a flat name -> value map (R3: event counters and gauges
	// such as sweep totals or watcher scan counts). A map of float64 was
	// chosen over json.RawMessage so R3 extends it purely additively — new
	// counter names appear without any struct or decoder change on either
	// side, and both integral counts and rates fit. Always non-nil ({}
	// in JSON) so consumers can index without a nil check.
	Counters map[string]float64 `json:"counters"`
	// Warnings lists active health-rule violations (R3, doc 50). Always
	// non-nil ([] in JSON); empty until R3 wires the rules engine.
	Warnings []HealthWarning `json:"warnings"`
	// Budgets lists every evaluated resource-class budget (R3, doc 06),
	// exceeded or not — the UI needs the headroom, not only the breach, and
	// the CLI needs to show "3 budgets, 0 exceeded". Evaluation is
	// SERVER-side so `groved stats --json`, an agent curling the socket and
	// the inspector all read one verdict instead of three reimplementations
	// of the thresholds. Always non-nil ([] in JSON).
	Budgets []Budget `json:"budgets"`
}

// RuntimeStats reports the daemon's Go runtime state.
type RuntimeStats struct {
	Goroutines int    `json:"goroutines"`
	HeapAlloc  uint64 `json:"heap_alloc"` // bytes currently allocated (runtime.MemStats.HeapAlloc)
	HeapSys    uint64 `json:"heap_sys"`   // bytes obtained from the OS for the heap
	NumGC      uint32 `json:"num_gc"`     // completed GC cycles
	// GCPauseTotalMS is the cumulative stop-the-world pause time in
	// milliseconds (MemStats.PauseTotalNs / 1e6).
	GCPauseTotalMS float64 `json:"gc_pause_total_ms"`
	// GoMemLimit is the runtime soft memory limit in bytes as read via
	// debug.SetMemoryLimit(-1). math.MaxInt64 means "no limit set".
	GoMemLimit int64 `json:"gomemlimit"`
	// UptimeMS is milliseconds since the daemon process started.
	UptimeMS int64 `json:"uptime_ms"`
}

// SelfStats is the two-sample process-tree rollup of the daemon's own pid
// (see core/pkg/procsample). CPU% is interval-true once the server-side
// sampler has history. RSSKB counts shared pages once per process, so
// subtree sums overstate the memory a kill would actually reclaim.
type SelfStats struct {
	PID    int     `json:"pid"`
	CPUPct float64 `json:"cpu_pct"` // subtree CPU% sum
	RSSKB  int64   `json:"rss_kb"`  // subtree RSS sum
	Procs  int     `json:"procs"`   // subtree process count (root inclusive)
	// Top is the hottest descendant by CPU (ties by RSS); nil when the
	// daemon pid was absent from the sample.
	Top *ProcStat `json:"top,omitempty"`
	// Children lists the subtree's descendant processes (root excluded),
	// hottest first, capped at 20 by the server. Always non-nil.
	Children []ProcStat `json:"children"`
}

// ProcStat is one process row inside SelfStats.
type ProcStat struct {
	PID    int     `json:"pid"`
	Comm   string  `json:"comm"`
	CPUPct float64 `json:"cpu_pct"`
	RSSKB  int64   `json:"rss_kb"`
}

// Budget is one evaluated resource-class budget (doc 06's "resource classes
// and budgets for spawned tools"): an observed Value against a Limit in a
// named Unit. Over-budget rows are rendered in the warn style by the
// inspector and summarized by its warning strip ("2 budgets exceeded").
type Budget struct {
	// Name is the stable budget id, e.g. "daemon.heap_vs_gomemlimit".
	Name string `json:"name"`
	// Class is the doc 06 resource class: "daemon", "agent", "editor", "pty".
	Class string `json:"class"`
	// Value and Limit are in Unit: "count", "kb", "bytes" or "pct".
	Value float64 `json:"value"`
	Limit float64 `json:"limit"`
	Unit  string  `json:"unit"`
	// Exceeded is the server's verdict — clients must render this rather
	// than recomputing Value > Limit, so a future non-linear rule (hysteresis,
	// sustained-for-N-seconds) changes behaviour in ONE place.
	Exceeded bool `json:"exceeded"`
	// Offender attributes the consumption when a single item dominates
	// (the largest agent subtree, the hottest child). May be empty.
	Offender string `json:"offender,omitempty"`
}

// HealthWarning is one active health-rule violation (doc 50): a watched
// path/subsystem whose condition currently holds, attributed to an offender.
// Defined in R2 so the SystemStats shape is final; populated by R3.
type HealthWarning struct {
	Path      string    `json:"path"`      // watched subsystem/path the rule applies to
	Condition string    `json:"condition"` // human-readable rule that fired
	Offender  string    `json:"offender"`  // attributed cause (comm, scope, workspace, ...)
	Since     time.Time `json:"since"`     // when the condition started holding
}
