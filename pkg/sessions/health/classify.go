package health

import (
	"fmt"
	"strings"
	"time"

	"github.com/grovetools/core/pkg/models"
)

// IsActiveSessionStatus reports whether a status claims a live agent.
// Mirrors the daemon session collector's active set minus "pending"
// (a queued job legitimately has no process yet).
func IsActiveSessionStatus(status string) bool {
	switch status {
	case "running", "idle", "pending_user":
		return true
	}
	return false
}

// IsTurnBasedType reports session types executed one turn at a time:
// between turns there is legitimately no process, so only a "running"
// status can go stale for them.
func IsTurnBasedType(t string) bool {
	switch t {
	case "chat", "oneshot", "note", "file", "generate-recipe", "oneshot_job":
		return true
	}
	return false
}

// IsInteractiveType reports session types whose agent lives in a PTY or
// tmux window rather than in the process that launched it. For these,
// a dead launcher PID is not on its own proof of death.
func IsInteractiveType(t string) bool {
	switch t {
	case "interactive_agent", "isolated_agent":
		return true
	}
	return false
}

// LeaseFor returns this row's lease duration. A turn-based chat parked in
// pending_user is waiting on a human and is intentionally exempt; an
// interactive agent with the same status is not immortal.
func LeaseFor(s *models.Session, policy LeasePolicy) (time.Duration, bool) {
	if s == nil || s.Origin != "" || !IsActiveSessionStatus(s.Status) {
		return 0, false
	}
	if IsTurnBasedType(s.Type) {
		if s.Status == "pending_user" {
			return 0, false
		}
		return policy.TurnBased, policy.TurnBased > 0
	}
	if IsInteractiveType(s.Type) {
		return policy.Interactive, policy.Interactive > 0
	}
	return policy.Headless, policy.Headless > 0
}

// LeaseExpired is pure over the supplied clock and policy for deterministic
// collector tests.
func LeaseExpired(s *models.Session, now time.Time, policy LeasePolicy) bool {
	lease, expirable := LeaseFor(s, policy)
	return expirable && now.Sub(LatestActivity(s)) > lease
}

// IsJobFileStatusActive reports frontmatter statuses that claim the job
// is currently executing — the ones a cleanup reconciles away.
func IsJobFileStatusActive(status string) bool {
	switch status {
	case "running", "in_progress":
		return true
	}
	return false
}

// ReconciledStatusFor picks the frontmatter status a lost job should be
// flipped to, mirroring the jobrunner's adoption philosophy: turn-based
// jobs (chat/oneshot/note) are safely re-runnable so they get the
// terminal "interrupted", while agent jobs get the non-terminal
// "orphaned" — "we lost it", not "it failed".
func ReconciledStatusFor(sessionType string) string {
	if IsTurnBasedType(sessionType) || sessionType == "" {
		return "interrupted"
	}
	return "orphaned"
}

// LatestActivity is the most recent timestamp the daemon has for a
// session — the clock the grace windows are measured against.
func LatestActivity(s *models.Session) time.Time {
	t := s.StartedAt
	if s.LastActivity.After(t) {
		t = s.LastActivity
	}
	return t
}

// QuietFor reports how long a session has been silent as of now.
func QuietFor(s *models.Session, now time.Time) time.Duration {
	return now.Sub(LatestActivity(s))
}

// Classify folds probe evidence into a verdict. Pure over its inputs
// (now included) so the ladder is unit-testable and so callers holding
// richer evidence than a one-shot probe can reuse it.
//
// The ladder mirrors the daemon's own: positive evidence of life wins,
// strongest signal first (live PTY > live tmux window > registry PID >
// session PID); then the guards (federated sessions are hands-off, a
// recent LastActivity wins a grace period, an unreachable multiplexer
// means UNKNOWN rather than STALE); only then the negative signals.
func Classify(s *models.Session, ev Evidence, now time.Time) Verdict {
	// Federated sessions belong to a satellite daemon; never signal them.
	if s.Origin != "" {
		return Verdict{Federated, fmt.Sprintf("owned by satellite %q — clean up there", s.Origin)}
	}

	if !IsActiveSessionStatus(s.Status) {
		return Verdict{Inactive, fmt.Sprintf("status %q — nothing to clean", s.Status)}
	}

	// Turn-based jobs (chat/oneshot/...) have no process between turns;
	// idle / pending_user is their normal resting state, not a hang.
	if IsTurnBasedType(s.Type) && s.Status != "running" {
		return Verdict{Waiting, fmt.Sprintf("%s %s — no process expected between turns", s.Type, s.Status)}
	}

	// Positive evidence of life, strongest signal first.
	if ev.PTY.Found {
		detail := fmt.Sprintf("live tuimux pty %s", ShortID(ev.PTY.ID))
		if ev.PTY.AttachedClients > 0 {
			detail += fmt.Sprintf(", %d client(s) attached", ev.PTY.AttachedClients)
		}
		return Verdict{Alive, detail}
	}
	if ev.Tmux.Found {
		return Verdict{Alive, fmt.Sprintf("live tmux window %s", ev.Tmux.Target)}
	}
	if ev.RegistryPIDAlive {
		return Verdict{Alive, fmt.Sprintf("registry pid %d alive", ev.RegistryPID)}
	}
	if ev.SessionPIDAlive {
		return Verdict{Alive, fmt.Sprintf("pid %d alive", s.PID)}
	}

	quiet := QuietFor(s, now)
	interactive := IsInteractiveType(s.Type)

	// A PTY-hosted interactive agent whose launcher PID died is not
	// provably dead while tuimux is unreachable — the agent lives in the
	// PTY, and the launcher exiting is normal.
	if interactive && s.PtyID != "" && !ev.PTY.Queried {
		return Verdict{Unknown, "pty liveness unknown (tuimux unreachable) — not calling it"}
	}
	// Same caution for a tmux-hosted interactive agent we could not
	// reach tmux for. When the tmux probe DID run and came back empty,
	// that is real negative evidence and falls through to the ladder
	// below — which is how these stop being permanent UNKNOWNs.
	if interactive && s.PtyID == "" && isTmuxHosted(s) && ev.RegistryPID <= 0 && !ev.Tmux.Queried {
		reason := "tmux-hosted agent (not probed) — check the tmux window"
		if ev.Tmux.QueryErr != "" {
			reason = "tmux liveness unknown (" + ev.Tmux.QueryErr + ") — not calling it"
		}
		return Verdict{Unknown, reason}
	}

	// Collect the negative signals.
	var dead []string
	if s.PID > 0 {
		dead = append(dead, fmt.Sprintf("pid %d dead", s.PID))
	}
	if ev.RegistryPID > 0 && ev.RegistryPID != s.PID {
		dead = append(dead, fmt.Sprintf("registry pid %d dead", ev.RegistryPID))
	}
	if s.PtyID != "" && ev.PTY.Queried {
		dead = append(dead, fmt.Sprintf("pty %s gone", ShortID(s.PtyID)))
	}
	if ev.Tmux.Queried && !ev.Tmux.Found && ev.Tmux.Target != "" {
		dead = append(dead, fmt.Sprintf("tmux window %s gone", ev.Tmux.Target))
	}

	if len(dead) > 0 {
		if quiet < GracePeriod {
			return Verdict{Grace, fmt.Sprintf("dead process signals but active %s ago — inside grace window", RoundDur(quiet))}
		}
		return Verdict{Stale, strings.Join(dead, ", ") + fmt.Sprintf(" — quiet %s", RoundDur(quiet))}
	}

	// No PID, no pid.lock, no PTY tag: nothing ever registered. Absence
	// of evidence only convicts after a long quiet spell.
	if quiet >= NoEvidenceStaleAfter {
		return Verdict{Stale, fmt.Sprintf("no process evidence (no PID, no pid.lock, no pty) — quiet %s", RoundDur(quiet))}
	}
	return Verdict{Grace, fmt.Sprintf("no process evidence yet — quiet %s (< %s threshold)", RoundDur(quiet), NoEvidenceStaleAfter)}
}

// isTmuxHosted reports whether the session records a tmux attach point.
func isTmuxHosted(s *models.Session) bool {
	return s.TmuxTarget != "" || s.TmuxKey != ""
}

// NeedsTmuxProbe reports whether probing tmux could change this
// session's verdict. Used to keep the tmux shell-out off the hot path:
// it only runs for sessions that would otherwise land on UNKNOWN.
func NeedsTmuxProbe(s *models.Session, ev Evidence) bool {
	if s == nil || s.Origin != "" || !IsActiveSessionStatus(s.Status) {
		return false
	}
	if !isTmuxHosted(s) {
		return false
	}
	// Already provably alive by a stronger signal.
	if ev.PTY.Found || ev.RegistryPIDAlive || ev.SessionPIDAlive {
		return false
	}
	return true
}

// ShortID truncates an ID to the 8-character form used in prompts and
// log lines.
func ShortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// RoundDur rounds a duration to a readable precision: seconds under a
// minute, 10-second steps above.
func RoundDur(d time.Duration) time.Duration {
	if d > time.Minute {
		return d.Round(time.Second * 10)
	}
	return d.Round(time.Second)
}

// DisplayName is the label a session is referred to by in prompts, log
// lines and doctor output: its job file's basename when it has one,
// then its job title, then a short ID.
func DisplayName(s *models.Session) string {
	if s == nil {
		return ""
	}
	if s.JobFilePath != "" {
		return baseName(s.JobFilePath)
	}
	if s.JobTitle != "" {
		return s.JobTitle
	}
	return ShortID(s.ID)
}

func baseName(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}
