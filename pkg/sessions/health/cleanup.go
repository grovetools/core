package health

import (
	"context"
	"fmt"
	"syscall"
	"time"

	"github.com/grovetools/core/pkg/daemon"
	"github.com/grovetools/core/pkg/sessions"
)

const (
	killTimeout = 5 * time.Second
	endTimeout  = 2 * time.Second
)

// Outcome records what a cleanup actually did, so callers can report it
// per session rather than just "ok / not ok".
type Outcome struct {
	DaemonKilled     bool   `json:"daemon_killed"`
	SignalledPID     int    `json:"signalled_pid,omitempty"`
	RemovedRecovery  bool   `json:"removed_recovery"`
	JobFileFrom      string `json:"job_file_from,omitempty"`
	JobFileTo        string `json:"job_file_to,omitempty"`
	JobFileUnchanged bool   `json:"job_file_unchanged"`
}

// String renders the outcome as the one-line summary the doctor CLI and
// the TUI status line print.
func (o Outcome) String() string {
	var parts []string
	if o.DaemonKilled {
		parts = append(parts, "daemon-killed")
	} else {
		if o.SignalledPID > 0 {
			parts = append(parts, fmt.Sprintf("SIGTERM %d", o.SignalledPID))
		}
		if o.RemovedRecovery {
			parts = append(parts, "recovery files cleared")
		}
		if len(parts) == 0 {
			parts = append(parts, "no live process to signal")
		}
	}
	if o.JobFileTo != "" {
		parts = append(parts, fmt.Sprintf("job file %s→%s", o.JobFileFrom, o.JobFileTo))
	}
	return joinNonEmpty(parts, ", ")
}

func joinNonEmpty(parts []string, sep string) string {
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out != "" {
			out += sep
		}
		out += p
	}
	return out
}

// Cleaner tears down sessions the classifier convicted.
//
// It is deliberately separate from the Prober: classification is a
// judgement, cleanup is an action, and every caller that acts should
// have had to look at a verdict first.
type Cleaner struct {
	// Client routes the preferred daemon-mediated kill. Nil (or a
	// stopped daemon) falls back to a local SIGTERM.
	Client daemon.Client

	// Reconcile flips the job file's frontmatter after the process is
	// gone. Nil skips the flow perspective — the process is still
	// cleaned up, the file just keeps claiming it runs.
	Reconcile JobFileReconciler

	// StateDir overrides paths.StateDir() for the registry. Tests set it.
	StateDir string
}

// Clean tears down one session: daemon-mediated kill first (SIGTERM +
// KillPty + recovery-file cleanup + store update in one atomic call),
// local fallback second, then job-file frontmatter reconciliation so
// flow tooling stops seeing a phantom "running" job.
//
// The local fallback clears pid.lock via RemoveRecoveryFiles and never
// purges the session directory: "this PID is dead" must not become
// "this session never happened" — metadata.json is history, not
// liveness. That distinction is the registry doctrine, and it is why
// `hooks sessions kill` no longer does os.RemoveAll.
func (c *Cleaner) Clean(ctx context.Context, p *Probe) (Outcome, error) {
	var out Outcome
	if p == nil || p.Session == nil {
		return out, fmt.Errorf("nil probe")
	}
	s := p.Session
	if s.Origin != "" {
		return out, fmt.Errorf("federated session (origin %q) — clean up on the satellite", s.Origin)
	}

	if c.Client != nil && c.Client.IsRunning() {
		kctx, cancel := context.WithTimeout(ctx, killTimeout)
		err := c.Client.KillSession(kctx, s.ID)
		cancel()
		out.DaemonKilled = err == nil
	}

	if !out.DaemonKilled {
		// Local fallback: signal the best-evidence PID, clear the
		// recovery files, and tell the daemon the session ended if it is
		// reachable at all.
		pid := 0
		switch {
		case p.Evidence.RegistryPIDAlive:
			pid = p.Evidence.RegistryPID
		case p.Evidence.SessionPIDAlive:
			pid = s.PID
		}
		if pid > 0 {
			if err := syscall.Kill(pid, syscall.SIGTERM); err == nil {
				out.SignalledPID = pid
			}
		}
		if reg, err := c.registry(); err == nil {
			dir := p.Evidence.RegistryDir
			if dir == "" {
				dir = s.ClaudeSessionID
			}
			if dir == "" {
				dir = s.ID
			}
			if err := reg.RemoveRecoveryFiles(dir); err == nil {
				out.RemovedRecovery = true
			}
		}
		if c.Client != nil && c.Client.IsRunning() {
			ectx, cancel := context.WithTimeout(ctx, endTimeout)
			_ = c.Client.EndSession(ectx, s.ID, "interrupted")
			cancel()
		}
	}

	if s.JobFilePath != "" && c.Reconcile != nil {
		want := ReconciledStatusFor(s.Type)
		if err := c.Reconcile(s.JobFilePath, want); err != nil {
			return out, fmt.Errorf("process cleaned but job file not reconciled: %w", err)
		}
		out.JobFileFrom = p.Evidence.JobFileStatus
		if IsJobFileStatusActive(p.Evidence.JobFileStatus) {
			out.JobFileTo = want
		} else {
			out.JobFileUnchanged = true
		}
	}
	return out, nil
}

func (c *Cleaner) registry() (*sessions.FileSystemRegistry, error) {
	if c.StateDir != "" {
		return sessions.NewFileSystemRegistryAt(c.StateDir)
	}
	return sessions.NewFileSystemRegistry()
}

// StaleOf filters a probe batch down to the sessions cleanup may act
// on. Nothing else is ever a cleanup candidate: UNKNOWN means we could
// not tell, FEDERATED belongs to a satellite, GRACE is too recent, and
// ALIVE/WAITING/INACTIVE are self-explanatory.
func StaleOf(probes []*Probe) []*Probe {
	var out []*Probe
	for _, p := range probes {
		if p != nil && p.Verdict.State == Stale {
			out = append(out, p)
		}
	}
	return out
}
