package models

import "time"

// Assistant supervisor states. The supervisor is an ensure-running loop, not a
// hot restart loop, so its state describes the standing assistant chain rather
// than a single process.
const (
	// AssistantStateDisabled means no [assistant] block enabled it for this
	// ecosystem, so nothing is being supervised.
	AssistantStateDisabled = "disabled"
	// AssistantStateLive means a live session heads the assistant chain.
	AssistantStateLive = "live"
	// AssistantStateStarting means a continuation (resume/retry/chain reset)
	// has been launched and the head session has not appeared yet.
	AssistantStateStarting = "starting"
	// AssistantStateBackoff means the last continuation failed and the next
	// attempt is waiting out the backoff window.
	AssistantStateBackoff = "backoff"
	// AssistantStateStopped means the circuit breaker tripped: repeated
	// fast-failing continuations, which restarting can never fix. The
	// breaker re-arms on daemon restart or an explicit ensure request that
	// carries Force.
	AssistantStateStopped = "stopped"
)

// AssistantStatus is the daemon-side assistant supervisor's public state
// (assistant-pane spec §3.3). It rides the daemon state stream so the rail
// pane can render "assistant stopped: <error>" instead of spinning on
// "starting…", and it is what `groved health` prints.
type AssistantStatus struct {
	// Enabled reports whether an [assistant] block turned the supervisor on.
	Enabled bool `json:"enabled"`

	// State is one of the AssistantState* constants above.
	State string `json:"state"`

	// Plan is the assistant's home plan name, and PlanDir its absolute
	// directory. PlanDir is the address the supervisor actually uses: `--at`
	// resolves through the worktree registry, and the assistant plan is
	// deliberately worktree-less.
	Plan    string `json:"plan,omitempty"`
	PlanDir string `json:"plan_dir,omitempty"`

	// Scope is the ECOSYSTEM ROOT whose [assistant] block configured this
	// supervisor — not the daemon's own scope. The two coincide on a scoped
	// daemon (development in an ecosystem worktree) and differ on the global
	// daemon (production), whose scope is empty and which resolves the
	// ecosystem it supervises by discovery. It is what makes `groved health`
	// and `groved claws` answer "whose assistant is this?" truthfully in both
	// deployments.
	Scope string `json:"scope,omitempty"`

	// Candidates lists every ecosystem root that opted in, and is populated
	// ONLY when more than one did. A daemon supervises exactly one assistant
	// today (one status endpoint, one default claw), so a second opted-in
	// ecosystem is a configuration ambiguity the operator has to see rather
	// than a silent choice: Scope names the one that won (lowest path, chosen
	// deterministically) and this names all of them.
	Candidates []string `json:"candidates,omitempty"`

	// HeadJobID is the job at the head of the assistant chain, when one is
	// live. HeadJobFile is its filename within PlanDir.
	HeadJobID   string `json:"head_job_id,omitempty"`
	HeadJobFile string `json:"head_job_file,omitempty"`

	// LastAction names the continuation the supervisor last took: "resume",
	// "retry", "chain_reset", "reclaw", or "none" when a live head made all
	// of them unnecessary.
	LastAction   string     `json:"last_action,omitempty"`
	LastActionAt *time.Time `json:"last_action_at,omitempty"`

	// LastError is the most recent continuation failure, cleared on success.
	LastError string `json:"last_error,omitempty"`

	// RestartCount counts continuations launched since daemon start;
	// ChainResets counts the subset that created a fresh root job.
	RestartCount int `json:"restart_count"`
	ChainResets  int `json:"chain_resets"`

	// ConsecutiveFailures is the fast-exit counter that arms the breaker.
	ConsecutiveFailures int `json:"consecutive_failures,omitempty"`

	// NextAttemptAt is when the current backoff window expires. Nil unless
	// State is AssistantStateBackoff.
	NextAttemptAt *time.Time `json:"next_attempt_at,omitempty"`
}
