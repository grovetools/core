package models

import "time"

// Orphan reasons reported in ResourceOrphan.Reason.
const (
	// OrphanReasonDeadPTY marks a PTY session that has sat with zero attached
	// clients past the daemon's leak TTL: its subtree is still burning
	// resources but nobody is watching it.
	OrphanReasonDeadPTY = "dead-pty"
	// OrphanReasonUnaccounted marks a grove-shaped process (nvim, gopls,
	// claude, git, ...) that lives outside every tracked PTY subtree.
	OrphanReasonUnaccounted = "unaccounted"
)

// PTYResources is the payload of GET /api/resources on the tuimux daemon,
// which groved re-exposes verbatim as GET /api/pty/resources through its
// existing /api/pty/ reverse proxy.
//
// tuimuxd owns every PTY process, so it is the only process that can attribute
// CPU/RSS to a PTY without racing the tree. It samples on a slow always-on
// cadence (CadenceMS) so per-session history predates the moment a human opens
// the inspector, and re-samples on request when the cached sample is stale.
//
// Field names are a stable snake_case contract; treat renames/removals as
// breaking changes.
type PTYResources struct {
	// SampledAt is when the process table underlying this response was read
	// (not when the response was written).
	SampledAt time.Time `json:"sampled_at"`
	// CadenceMS is the always-on background sampling period in milliseconds.
	// It is also the spacing of the History rings.
	CadenceMS int64 `json:"cadence_ms"`
	// Host is the tuimux daemon process itself.
	Host ResourceHost `json:"host"`
	// Sessions is one entry per live PTY session, hottest first. Always
	// non-nil ([] in JSON).
	Sessions []PTYResourceSession `json:"sessions"`
	// Orphans lists leak candidates, largest RSS first. Always non-nil.
	Orphans []ResourceOrphan `json:"orphans"`
}

// ResourceHost describes the tuimux daemon process. CPUPct/RSSKB are the
// daemon's OWN process only, not a subtree rollup: every PTY subtree is
// itemized in Sessions, so a subtree number here would double-count.
type ResourceHost struct {
	PID        int     `json:"pid"`
	CPUPct     float64 `json:"cpu_pct"`
	RSSKB      int64   `json:"rss_kb"`
	Goroutines int     `json:"goroutines"`
}

// PTYResourceSession is one PTY session's subtree rollup. RSSKB counts shared
// pages once per process, so it overstates what killing the subtree reclaims.
type PTYResourceSession struct {
	PTYID           string            `json:"pty_id"`
	RootPID         int               `json:"root_pid"`
	Workspace       string            `json:"workspace"`
	Label           string            `json:"label,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	AttachedClients int               `json:"attached_clients"`
	CPUPct          float64           `json:"cpu_pct"` // subtree CPU% sum
	RSSKB           int64             `json:"rss_kb"`  // subtree RSS sum
	Procs           int               `json:"procs"`   // subtree size, root inclusive
	// Top is the hottest descendant by CPU (ties by RSS); nil when the root
	// pid was absent from the sample.
	Top *ProcStat `json:"top,omitempty"`
	// ProcsDetail is the flat subtree in pid order, root included. Only sent
	// with ?detail=1.
	ProcsDetail []ProcStat `json:"procs_detail,omitempty"`
	// History is the daemon's per-session ring buffer, oldest first, one
	// entry per CadenceMS. Only sent with ?history=1.
	History *ResourceHistory `json:"history,omitempty"`
	// LastDetached is when the last client disconnected (zero/omitted while
	// attached or never detached).
	LastDetached *time.Time `json:"last_detached,omitempty"`
}

// ResourceHistory is a pair of equal-length rings sampled at the response's
// CadenceMS, oldest first.
type ResourceHistory struct {
	CPU   []float64 `json:"cpu"`
	RSSKB []int64   `json:"rss_kb"`
}

// ResourceOrphan is one leak candidate: either a detached-past-TTL PTY subtree
// (Reason OrphanReasonDeadPTY, PTYID set) or a grove-shaped process outside
// every tracked subtree (Reason OrphanReasonUnaccounted).
type ResourceOrphan struct {
	PID    int     `json:"pid"`
	Comm   string  `json:"comm"`
	CPUPct float64 `json:"cpu_pct"`
	RSSKB  int64   `json:"rss_kb"`
	// Procs is the subtree size for dead-pty rows; 1 for unaccounted rows.
	Procs  int    `json:"procs,omitempty"`
	Reason string `json:"reason"`
	// PTYID is set on dead-pty rows so a client kills through the daemon's
	// PTY verb rather than signalling the tree itself.
	PTYID string `json:"pty_id,omitempty"`
	// DetachedAt is when the dead PTY lost its last client.
	DetachedAt *time.Time `json:"detached_at,omitempty"`
}
