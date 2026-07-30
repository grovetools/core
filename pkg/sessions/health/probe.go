package health

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/grovetools/core/pkg/daemon"
	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/process"
	"github.com/grovetools/core/pkg/sessions"
)

// ptyListTimeout bounds the one PTY-list RPC a batch makes. Short: an
// unreachable tuimux must degrade a probe to UNKNOWN promptly, not hang
// a TUI's background scan.
const ptyListTimeout = 3 * time.Second

// Prober gathers evidence. One Prober is reusable across batches and is
// safe to keep on a long-lived model — it holds no per-batch state.
//
// Everything except Client is optional: a zero-value dependency simply
// removes that perspective from the evidence, and the classifier is
// written so a missing perspective yields caution (UNKNOWN / GRACE),
// never a false STALE.
type Prober struct {
	// Client is the daemon client used for the PTY list (and resource
	// rollup). A nil or not-running client leaves PTY.Queried false.
	Client daemon.Client

	// JobFile reads a job file's frontmatter status. Nil skips the flow
	// perspective entirely.
	JobFile JobFileStatusReader

	// Tmux answers "does this session's tmux window still exist". Nil
	// leaves tmux-hosted agents on UNKNOWN, which is the pre-existing
	// behavior.
	Tmux TmuxProber

	// StateDir overrides paths.StateDir() for the registry scan. Empty
	// uses the real one; tests set it.
	StateDir string

	// WithResources additionally fetches the per-PTY resource rollup so
	// callers can render blast-radius confirms. Costs one extra RPC per
	// batch; off by default because the badge scan doesn't need it.
	WithResources bool
}

// Probe gathers evidence for each session and classifies it. The batch
// shares one PTY-list RPC, one optional resource RPC and one registry
// directory scan, so a scan over the whole session list stays cheap
// enough to run on a TUI's refresh tick.
//
// tmux is the exception: it is probed per session, and only for the
// sessions that would otherwise be UNKNOWN (see NeedsTmuxProbe).
func (p *Prober) Probe(ctx context.Context, sessions []*models.Session) []*Probe {
	return p.ProbeAt(ctx, sessions, time.Now())
}

// ProbeAt is Probe with an explicit clock, for tests.
func (p *Prober) ProbeAt(ctx context.Context, sess []*models.Session, now time.Time) []*Probe {
	ptys, ptyErr := p.listPTYs(ctx)
	resources := p.listResources(ctx)
	reg := p.loadRegistrySnapshot()

	probes := make([]*Probe, 0, len(sess))
	for _, s := range sess {
		if s == nil {
			continue
		}
		ev := p.gather(ctx, s, ptys, ptyErr, resources, reg)
		probes = append(probes, &Probe{
			Session:  s,
			ProbedAt: now,
			Evidence: ev,
			Verdict:  Classify(s, ev, now),
		})
	}
	return probes
}

// ProbeOne is the single-session convenience wrapper. Returns nil when
// the session is nil.
func (p *Prober) ProbeOne(ctx context.Context, s *models.Session) *Probe {
	got := p.Probe(ctx, []*models.Session{s})
	if len(got) == 0 {
		return nil
	}
	return got[0]
}

func (p *Prober) listPTYs(ctx context.Context) ([]daemon.PTYSessionInfo, error) {
	if p.Client == nil || !p.Client.IsRunning() {
		return nil, fmt.Errorf("daemon unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, ptyListTimeout)
	defer cancel()
	return p.Client.ListPTYs(ctx)
}

// listResources fetches the per-PTY resource rollup, keyed by PTY ID.
// Best-effort: a failure just means no Resources on the evidence.
func (p *Prober) listResources(ctx context.Context) map[string]*PTYResourceEvidence {
	if !p.WithResources || p.Client == nil || !p.Client.IsRunning() {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, ptyListTimeout)
	defer cancel()
	res, err := p.Client.GetPTYResources(ctx, daemon.PTYResourcesOptions{})
	if err != nil || res == nil {
		return nil
	}
	out := make(map[string]*PTYResourceEvidence, len(res.Sessions))
	for _, s := range res.Sessions {
		e := &PTYResourceEvidence{CPUPct: s.CPUPct, RSSKB: s.RSSKB, Procs: s.Procs}
		if s.Top != nil {
			e.TopComm = s.Top.Comm
			e.TopPID = s.Top.PID
		}
		out[s.PTYID] = e
	}
	return out
}

// regEntry is one directory of the on-disk session registry.
type regEntry struct {
	dir        string
	path       string
	hasPIDLock bool
	pid        int
	meta       *sessions.SessionMetadata
}

func (p *Prober) registryBase() string {
	base := p.StateDir
	if base == "" {
		base = paths.StateDir()
	}
	return filepath.Join(base, "hooks", "sessions")
}

// loadRegistrySnapshot reads every session dir under
// <state>/hooks/sessions once so per-session matching is slice work.
func (p *Prober) loadRegistrySnapshot() []regEntry {
	base := p.registryBase()
	dirents, err := os.ReadDir(base)
	if err != nil {
		return nil
	}
	entries := make([]regEntry, 0, len(dirents))
	for _, d := range dirents {
		if !d.IsDir() {
			continue
		}
		e := regEntry{dir: d.Name(), path: filepath.Join(base, d.Name())}
		if raw, err := os.ReadFile(filepath.Join(e.path, "pid.lock")); err == nil {
			e.hasPIDLock = true
			fmt.Sscanf(strings.TrimSpace(string(raw)), "%d", &e.pid)
		}
		if raw, err := os.ReadFile(filepath.Join(e.path, "metadata.json")); err == nil {
			var meta sessions.SessionMetadata
			if json.Unmarshal(raw, &meta) == nil {
				e.meta = &meta
			}
		}
		entries = append(entries, e)
	}
	return entries
}

// findRegistryEntry locates the registry dir for a session. Dir names are
// the agent's native session ID (ClaudeSessionID) when known, else the
// job ID; legacy/edge records are found by metadata JobID/SessionID.
func findRegistryEntry(s *models.Session, entries []regEntry) *regEntry {
	for i := range entries {
		if entries[i].dir == s.ClaudeSessionID && s.ClaudeSessionID != "" {
			return &entries[i]
		}
	}
	for i := range entries {
		if entries[i].dir == s.ID {
			return &entries[i]
		}
	}
	for i := range entries {
		m := entries[i].meta
		if m == nil {
			continue
		}
		if (m.JobID != "" && m.JobID == s.ID) || (m.SessionID != "" && m.SessionID == s.ID) {
			return &entries[i]
		}
	}
	return nil
}

// findPTY locates the live PTY carrying a session: by the session's
// recorded PtyID, by the PTY's back-reference session_id, or by the
// flow job_id label agents are tagged with at spawn.
func findPTY(s *models.Session, ptys []daemon.PTYSessionInfo) (daemon.PTYSessionInfo, string, bool) {
	for _, pty := range ptys {
		if s.PtyID != "" && pty.ID == s.PtyID {
			return pty, "pty_id", true
		}
	}
	for _, pty := range ptys {
		if pty.SessionID != "" && pty.SessionID == s.ID {
			return pty, "session_id", true
		}
	}
	for _, pty := range ptys {
		if pty.Labels["job_id"] != "" && pty.Labels["job_id"] == s.ID {
			return pty, "job_id label", true
		}
	}
	return daemon.PTYSessionInfo{}, "", false
}

func (p *Prober) gather(
	ctx context.Context,
	s *models.Session,
	ptys []daemon.PTYSessionInfo,
	ptyErr error,
	resources map[string]*PTYResourceEvidence,
	reg []regEntry,
) Evidence {
	var ev Evidence

	// lifecycle: on-disk registry
	if e := findRegistryEntry(s, reg); e != nil {
		ev.RegistryFound = true
		ev.RegistryDir = e.dir
		ev.RegistryPath = e.path
		ev.HasPIDLock = e.hasPIDLock
		ev.RegistryPID = e.pid
		if e.meta != nil {
			ev.HasMetadata = true
			ev.MetaStatus = e.meta.Status
			ev.MetaScope = e.meta.Scope
			ev.MetaPID = e.meta.PID
		}
	}

	// ps: liveness of the recorded PIDs
	if s.PID > 0 {
		ev.SessionPIDAlive = process.IsProcessAlive(s.PID)
	}
	if ev.RegistryPID > 0 {
		ev.RegistryPIDAlive = process.IsProcessAlive(ev.RegistryPID)
	}

	// tuimux / treemux: live PTY carrying this session
	ev.PTY.Queried = ptyErr == nil
	if ptyErr != nil {
		ev.PTY.QueryErr = ptyErr.Error()
	} else if pty, matchedBy, ok := findPTY(s, ptys); ok {
		ev.PTY.Found = true
		ev.PTY.ID = pty.ID
		ev.PTY.PID = pty.PID
		ev.PTY.AttachedClients = pty.AttachedClients
		ev.PTY.Foreground = pty.ForegroundProcess
		ev.PTY.LastDetached = pty.LastDetached
		ev.PTY.PanelID = pty.PanelID
		ev.PTY.Workspace = pty.Workspace
		ev.PTY.MatchedBy = matchedBy
		ev.PTY.Resources = resources[pty.ID]
	}

	// tmux: only when it could change the verdict (see NeedsTmuxProbe) —
	// this is a shell-out, unlike everything above.
	if p.Tmux != nil && NeedsTmuxProbe(s, ev) {
		ev.Tmux.Target = tmuxTargetOf(s)
		alive, err := p.Tmux.WindowAlive(ctx, s)
		if err != nil {
			ev.Tmux.QueryErr = err.Error()
		} else {
			ev.Tmux.Queried = true
			ev.Tmux.Found = alive
		}
	}

	// flow: job file frontmatter status
	if s.JobFilePath != "" && p.JobFile != nil {
		if status, exists, err := p.JobFile(s.JobFilePath); err == nil {
			ev.JobFileExists = exists
			ev.JobFileStatus = status
		}
	}

	return ev
}

// tmuxTargetOf names the tmux attach point for display: the explicit
// target when known, else the session key.
func tmuxTargetOf(s *models.Session) string {
	if s.TmuxTarget != "" {
		return s.TmuxTarget
	}
	return s.TmuxKey
}
