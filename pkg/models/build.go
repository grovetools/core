package models

// --- Build Queue API Types ---
// Shared between the daemon's machine-wide build scheduler
// (daemon/internal/daemon/buildqueue) and grove's orchestrator, which
// submits jobs through the daemon Client's SubmitBuild/StreamBuildEvents.

// BuildJobRequest submits a single build job to the daemon's machine-wide
// build queue (POST /api/build/submit).
type BuildJobRequest struct {
	// Workspace is the workspace name (directory basename), used for
	// lifecycle event attribution and logging.
	Workspace string `json:"workspace"`
	// Dir is the absolute working directory the command runs in.
	Dir string `json:"dir"`
	// Command is the explicit command to run. Empty means `make <verb>`.
	Command []string `json:"command,omitempty"`
	// Env is the caller's fully resolved environment (PATH including
	// ExtraPathDirs, etc.) — the daemon's own environment lacks the
	// caller's toolchain entries, so the submitter must ship its own.
	Env []string `json:"env,omitempty"`
	// GroupID groups every job of one CLI invocation so they can be
	// cancelled together (Ctrl+C / fail-fast).
	GroupID string `json:"group_id"`
	// Verb is the make target used when Command is empty.
	Verb string `json:"verb"`
}

// BuildSubmitResponse is the response to a build job submission.
type BuildSubmitResponse struct {
	JobID string `json:"job_id"`
}

// BuildCancelRequest cancels every queued and running build job in a group.
type BuildCancelRequest struct {
	GroupID string `json:"group_id"`
}

// Build job event names carried on the per-job SSE stream
// (GET /api/build/jobs/{id}/stream).
const (
	BuildEventQueued   = "queued"
	BuildEventStarted  = "started"
	BuildEventOutput   = "output"
	BuildEventFinished = "finished"
)

// BuildJobEvent is a single event on a build job's SSE stream. Lifecycle
// events (queued/started/finished) additionally go through the daemon's
// store broadcast; output lines only ever travel on the per-job stream.
type BuildJobEvent struct {
	Event      string `json:"event"` // "queued", "started", "output", "finished"
	JobID      string `json:"job_id"`
	Line       string `json:"line,omitempty"`        // Event == "output"
	ExitCode   int    `json:"exit_code,omitempty"`   // Event == "finished"
	Error      string `json:"error,omitempty"`       // Event == "finished": failure or cancellation detail
	Cancelled  bool   `json:"cancelled,omitempty"`   // Event == "finished": job was cancelled
	DurationMs int64  `json:"duration_ms,omitempty"` // Event == "finished"
}
