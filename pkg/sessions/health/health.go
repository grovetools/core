// Package health is the one place that answers "is this agent session
// still alive, and if not, how do we clean it up?".
//
// It exists because that question used to have three different answers:
// the daemon's session reaper, the jobrunner's adoption logic, and the
// hooks TUI's inspector each carried their own liveness ladder, and some
// legacy CLI paths actively disagreed with all three. Everything now
// routes through the contract here so "cleanup" means one thing across
// the ecosystem.
//
// The contract is a deliberate three-way split:
//
//	Gather   — I/O. Talks to groved, ps, the on-disk session registry,
//	           tuimux, tmux and the job file. Produces a pure Evidence
//	           struct and nothing else.
//	Classify — a pure function over Evidence. No I/O, no clock reads
//	           (now is a parameter), so the whole ladder is table-testable
//	           and callers with richer evidence than a one-shot probe
//	           (the daemon's pidLiveness strike history, say) can feed it
//	           without adopting the probe.
//	Cleanup  — a separate step that acts on a classified probe.
//
// The evidence perspectives, strongest first:
//
//   - tuimux:    does a live daemon PTY carry this session (which is also
//     the treemux perspective — treemux panes attach to daemon PTYs)
//   - tmux:      does the tmux window named by TmuxTarget/TmuxKey exist
//   - lifecycle: the on-disk session registry at
//     <state>/hooks/sessions/<claude-session-id>/ (pid.lock, metadata.json)
//   - ps:        is the recorded PID (and the registry PID) actually alive
//   - groved:    the daemon session record itself (status, PID, PtyID, …)
//   - flow:      the job file's frontmatter status, which can say
//     "running" long after every process died
//
// Two dependencies are injected rather than imported. The job file's
// frontmatter lives behind flow's parser and core must not import flow,
// so callers supply a JobFileStatusReader / JobFileReconciler pair. tmux
// probing is injected too, because the socket a job's tmux server lives
// on is flow's naming convention, not core's.
package health

import (
	"context"
	"time"

	"github.com/grovetools/core/pkg/models"
)

const (
	// GracePeriod mirrors the daemon session reaper: a session with
	// activity inside this window is never called stale, even with dead
	// PIDs, so freshly (re)spawned agents aren't swept mid-handoff.
	GracePeriod = 45 * time.Second

	// NoEvidenceStaleAfter guards sessions that never recorded any PID or
	// PTY (e.g. a chat turn whose runner doesn't register a pid.lock).
	// Absence of evidence only becomes evidence of absence after the
	// session has been quiet this long. Mirrors tuimux's orphan-PTY TTL.
	NoEvidenceStaleAfter = 10 * time.Minute
)

// State is the folded verdict about one session.
type State int

const (
	// Unknown means the probe could not reach a perspective it needed —
	// never a licence to clean up.
	Unknown State = iota
	// Alive means positive evidence of a live process or PTY.
	Alive
	// Grace means dead-process signals exist but the session was active
	// too recently to convict.
	Grace
	// Waiting means a turn-based job legitimately has no process right
	// now (a chat between turns).
	Waiting
	// Inactive means the session's own status is already terminal.
	Inactive
	// Federated means a satellite daemon owns it; hands off.
	Federated
	// Stale means provably dead and past the grace window — the only
	// state cleanup acts on.
	Stale
)

// String renders the state as the short uppercase label used in the
// inspector, the doctor CLI and log lines.
func (s State) String() string {
	switch s {
	case Alive:
		return "ALIVE"
	case Grace:
		return "GRACE"
	case Waiting:
		return "WAITING"
	case Inactive:
		return "INACTIVE"
	case Federated:
		return "FEDERATED"
	case Stale:
		return "STALE"
	default:
		return "UNKNOWN"
	}
}

// MarshalText makes State render as its label in JSON (the doctor CLI's
// --json output is a public interface for coordinator agents).
func (s State) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// Verdict is a State plus the human-readable evidence line that
// justifies it. The Reason is not decoration: it is what a log line, a
// confirm prompt, or a "wrong flip" post-mortem is read from.
type Verdict struct {
	State  State  `json:"state"`
	Reason string `json:"reason"`
}

// Short renders "STALE — pid 4242 dead, pty gone — quiet 23m".
func (v Verdict) Short() string {
	return v.State.String() + " — " + v.Reason
}

// PTYEvidence is the tuimux/treemux perspective for one session: whether
// the PTY list could be queried at all, and the matching PTY if any.
type PTYEvidence struct {
	Queried  bool   `json:"queried"`
	QueryErr string `json:"query_err,omitempty"`
	Found    bool   `json:"found"`

	ID              string    `json:"id,omitempty"`
	PID             int       `json:"pid,omitempty"`
	AttachedClients int       `json:"attached_clients,omitempty"`
	Foreground      string    `json:"foreground,omitempty"`
	LastDetached    time.Time `json:"last_detached,omitempty"`
	PanelID         string    `json:"panel_id,omitempty"`
	Workspace       string    `json:"workspace,omitempty"`
	MatchedBy       string    `json:"matched_by,omitempty"` // "pty_id" | "session_id" | "job_id label"

	// Resource rollup for the matched PTY, when the caller asked for it
	// (Prober.WithResources). Nil otherwise. Used for blast-radius
	// confirms: "kill: 14 procs, reclaim ~412M".
	Resources *PTYResourceEvidence `json:"resources,omitempty"`
}

// PTYResourceEvidence is the subtree rollup for the PTY carrying a
// session. RSSKB counts shared pages once per process, so it overstates
// what killing the subtree actually reclaims — present it as "~".
type PTYResourceEvidence struct {
	CPUPct float64 `json:"cpu_pct"`
	RSSKB  int64   `json:"rss_kb"`
	Procs  int     `json:"procs"`
	// TopComm/TopPID name the hottest descendant by CPU, which is what
	// tells you whether the PTY is busy or just parked.
	TopComm string `json:"top_comm,omitempty"`
	TopPID  int    `json:"top_pid,omitempty"`
}

// TmuxEvidence is the tmux perspective: whether the window named by
// TmuxTarget / TmuxKey still exists. Queried is false when the session
// is not tmux-hosted or no prober was supplied — which is why a missing
// tmux answer yields UNKNOWN rather than STALE.
type TmuxEvidence struct {
	Queried  bool   `json:"queried"`
	QueryErr string `json:"query_err,omitempty"`
	Found    bool   `json:"found"`
	Target   string `json:"target,omitempty"`
}

// Evidence is everything gathered about one session. Pure data so
// Classify stays a testable function over it.
type Evidence struct {
	// ps perspective
	SessionPIDAlive  bool `json:"session_pid_alive"`  // meaningful only when Session.PID > 0
	RegistryPID      int  `json:"registry_pid"`       // from pid.lock; 0 when absent
	RegistryPIDAlive bool `json:"registry_pid_alive"` //

	// lifecycle perspective (on-disk session registry)
	RegistryFound bool   `json:"registry_found"`
	RegistryDir   string `json:"registry_dir,omitempty"` // dir basename (== ClaudeSessionID or job ID)
	RegistryPath  string `json:"registry_path,omitempty"`
	HasPIDLock    bool   `json:"has_pid_lock"`
	HasMetadata   bool   `json:"has_metadata"`
	MetaStatus    string `json:"meta_status,omitempty"`
	MetaScope     string `json:"meta_scope,omitempty"`
	MetaPID       int    `json:"meta_pid,omitempty"`

	// tuimux / treemux perspective
	PTY PTYEvidence `json:"pty"`

	// tmux perspective
	Tmux TmuxEvidence `json:"tmux"`

	// flow perspective (job file frontmatter)
	JobFileExists bool   `json:"job_file_exists"`
	JobFileStatus string `json:"job_file_status,omitempty"`
}

// Probe pairs a session with its gathered evidence and folded verdict.
type Probe struct {
	Session  *models.Session `json:"session"`
	ProbedAt time.Time       `json:"probed_at"`
	Evidence Evidence        `json:"evidence"`
	Verdict  Verdict         `json:"verdict"`
}

// JobFileStatusReader reads a job file's frontmatter status. Injected
// because frontmatter parsing lives in flow and core must not import it.
// exists is false when the file is simply absent (not an error).
type JobFileStatusReader func(path string) (status string, exists bool, err error)

// JobFileReconciler rewrites a job file's frontmatter status, preserving
// the body. Injected for the same reason. Implementations must be
// no-ops for terminal statuses — see IsJobFileStatusActive.
type JobFileReconciler func(path, status string) error

// TmuxProber answers "does the tmux window hosting this session still
// exist?". Injected because a job's tmux server socket is flow's naming
// convention, not core's. Returning an error means "could not tell",
// which keeps the verdict UNKNOWN rather than convicting.
type TmuxProber interface {
	WindowAlive(ctx context.Context, s *models.Session) (bool, error)
}
