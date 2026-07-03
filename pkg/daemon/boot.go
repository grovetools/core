package daemon

// BootStatus reports the daemon's boot progress. It is the payload of the
// GET /api/system/boot endpoint and of the "boot_phase" SSE update type.
//
// The daemon only tracks live phases when it was started with
// --ready-at=bind (the early-bind reorder, used by treemux's cold-start
// splash). Under the default --ready-at=boot ordering the socket binds last,
// after every boot step, so by the time any client can reach the endpoint the
// daemon is already serving — the endpoint then reports Done=true immediately.
type BootStatus struct {
	// Phase is a short human-readable label for the boot step currently
	// executing (e.g. "environment", "watchers"). Empty once Done.
	Phase string `json:"phase,omitempty"`
	// PhaseIndex is the 1-based index of the current phase; 0 before the
	// first phase begins.
	PhaseIndex int `json:"phase_index"`
	// PhaseTotal is the total number of boot phases the daemon will run.
	PhaseTotal int `json:"phase_total"`
	// Done is true once every boot step has finished and the daemon is fully
	// serving.
	Done bool `json:"done"`
	// Err carries a non-fatal boot error string, if any step reported one.
	// Boot continues past recoverable failures, so a non-empty Err does not
	// imply the daemon is unusable.
	Err string `json:"err,omitempty"`
}
