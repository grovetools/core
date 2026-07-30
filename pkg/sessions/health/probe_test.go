package health

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/sessions"
)

// writeRegistryEntry lays down one on-disk session registry directory,
// the way the hooks lifecycle writes it.
func writeRegistryEntry(t *testing.T, stateDir, dir string, pid int, meta *sessions.SessionMetadata) {
	t.Helper()
	path := filepath.Join(stateDir, "hooks", "sessions", dir)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if pid > 0 {
		if err := os.WriteFile(filepath.Join(path, "pid.lock"), []byte(fmt.Sprintf("%d\n", pid)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if meta != nil {
		raw, err := json.Marshal(meta)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "metadata.json"), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestProbeGathersRegistryEvidence covers the lifecycle perspective end
// to end: a pid.lock naming a dead PID plus metadata.json is enough to
// convict a long-quiet session, and the evidence carries the detail the
// inspector renders.
func TestProbeGathersRegistryEvidence(t *testing.T) {
	stateDir := t.TempDir()
	// PID 1 is always alive; a PID this large is reliably absent.
	const deadPID = 4194303

	writeRegistryEntry(t, stateDir, "native-session-id", deadPID, &sessions.SessionMetadata{
		SessionID: "job-1",
		Status:    "running",
		Scope:     "/some/ecosystem",
	})

	p := &Prober{StateDir: stateDir}
	s := &models.Session{
		ID:              "job-1",
		ClaudeSessionID: "native-session-id",
		Status:          "running",
		Type:            "headless_agent",
		LastActivity:    time.Now().Add(-time.Hour),
	}

	probe := p.ProbeOne(context.Background(), s)
	if probe == nil {
		t.Fatal("ProbeOne returned nil")
	}
	ev := probe.Evidence
	if !ev.RegistryFound || !ev.HasPIDLock || !ev.HasMetadata {
		t.Fatalf("registry evidence not gathered: %+v", ev)
	}
	if ev.RegistryPID != deadPID {
		t.Errorf("RegistryPID = %d, want %d", ev.RegistryPID, deadPID)
	}
	if ev.RegistryPIDAlive {
		t.Error("RegistryPIDAlive = true for an absent PID")
	}
	if ev.MetaScope != "/some/ecosystem" {
		t.Errorf("MetaScope = %q, want the scope from metadata.json", ev.MetaScope)
	}
	if probe.Verdict.State != Stale {
		t.Errorf("verdict = %v (%s), want STALE", probe.Verdict.State, probe.Verdict.Reason)
	}
}

// TestProbeLiveRegistryPIDIsAlive is the other half: our own PID is
// unambiguously alive, so the session must not be convicted no matter
// how quiet it has been.
func TestProbeLiveRegistryPIDIsAlive(t *testing.T) {
	stateDir := t.TempDir()
	writeRegistryEntry(t, stateDir, "job-2", os.Getpid(), nil)

	p := &Prober{StateDir: stateDir}
	probe := p.ProbeOne(context.Background(), &models.Session{
		ID:           "job-2",
		Status:       "running",
		Type:         "interactive_agent",
		LastActivity: time.Now().Add(-24 * time.Hour),
	})

	if probe.Verdict.State != Alive {
		t.Errorf("verdict = %v (%s), want ALIVE for a live registry pid",
			probe.Verdict.State, probe.Verdict.Reason)
	}
}

// TestProbeJobFileReaderIsInjected pins the core-must-not-import-flow
// boundary: the frontmatter perspective arrives entirely through the
// injected reader.
func TestProbeJobFileReaderIsInjected(t *testing.T) {
	called := ""
	p := &Prober{
		StateDir: t.TempDir(),
		JobFile: func(path string) (string, bool, error) {
			called = path
			return "running", true, nil
		},
	}
	probe := p.ProbeOne(context.Background(), &models.Session{
		ID:           "job-3",
		Status:       "running",
		Type:         "chat",
		JobFilePath:  "/plans/p/03-job.md",
		LastActivity: time.Now(),
	})

	if called != "/plans/p/03-job.md" {
		t.Errorf("reader called with %q, want the session's job file path", called)
	}
	if !probe.Evidence.JobFileExists || probe.Evidence.JobFileStatus != "running" {
		t.Errorf("job file evidence = %+v, want exists/running", probe.Evidence)
	}
}

// TestProbeNoJobFileReaderSkipsFlowPerspective: a caller that supplies
// no reader still gets a usable probe, just without the flow evidence.
func TestProbeNoJobFileReaderSkipsFlowPerspective(t *testing.T) {
	p := &Prober{StateDir: t.TempDir()}
	probe := p.ProbeOne(context.Background(), &models.Session{
		ID:           "job-4",
		Status:       "running",
		Type:         "chat",
		JobFilePath:  "/plans/p/04-job.md",
		LastActivity: time.Now(),
	})
	if probe.Evidence.JobFileExists {
		t.Error("JobFileExists set without an injected reader")
	}
}

// stubTmux is a TmuxProber that records the sessions it was asked about.
type stubTmux struct {
	alive bool
	err   error
	calls []string
}

func (s *stubTmux) WindowAlive(_ context.Context, sess *models.Session) (bool, error) {
	s.calls = append(s.calls, sess.ID)
	return s.alive, s.err
}

// TestProbeOnlyAsksTmuxWhenItMatters: the tmux shell-out must stay off
// the hot path — it runs for the sessions that would otherwise be
// UNKNOWN and for no others.
func TestProbeOnlyAsksTmuxWhenItMatters(t *testing.T) {
	stateDir := t.TempDir()
	writeRegistryEntry(t, stateDir, "has-live-pid", os.Getpid(), nil)

	tm := &stubTmux{alive: false}
	p := &Prober{StateDir: stateDir, Tmux: tm}

	sessionsIn := []*models.Session{
		// Needs the probe: tmux-hosted, nothing else says it's alive.
		{
			ID: "tmux-hosted", Status: "running", Type: "interactive_agent",
			TmuxTarget: "proj:job-x", LastActivity: time.Now().Add(-time.Hour),
		},
		// Alive by a stronger signal — no probe.
		{
			ID: "has-live-pid", Status: "running", Type: "interactive_agent",
			TmuxTarget: "proj:job-y", LastActivity: time.Now().Add(-time.Hour),
		},
		// Not tmux-hosted at all — no probe.
		{
			ID: "no-tmux", Status: "running", Type: "headless_agent",
			LastActivity: time.Now().Add(-time.Hour),
		},
	}

	probes := p.ProbeAt(context.Background(), sessionsIn, time.Now())
	if len(tm.calls) != 1 || tm.calls[0] != "tmux-hosted" {
		t.Fatalf("tmux probed %v, want exactly [tmux-hosted]", tm.calls)
	}
	if probes[0].Verdict.State != Stale {
		t.Errorf("tmux-hosted verdict = %v (%s), want STALE once the window is gone",
			probes[0].Verdict.State, probes[0].Verdict.Reason)
	}
}

// TestProbeTmuxErrorStaysUnknown: a tmux we could not reach must not be
// read as "the window is gone".
func TestProbeTmuxErrorStaysUnknown(t *testing.T) {
	tm := &stubTmux{err: fmt.Errorf("no server running")}
	p := &Prober{StateDir: t.TempDir(), Tmux: tm}

	probe := p.ProbeOne(context.Background(), &models.Session{
		ID: "tmux-hosted", Status: "running", Type: "interactive_agent",
		TmuxTarget: "proj:job-x", LastActivity: time.Now().Add(-time.Hour),
	})
	if probe.Verdict.State != Unknown {
		t.Errorf("verdict = %v (%s), want UNKNOWN when tmux could not be reached",
			probe.Verdict.State, probe.Verdict.Reason)
	}
}

// TestStaleOf is the cleanup gate: only STALE is ever a candidate.
func TestStaleOf(t *testing.T) {
	probes := []*Probe{
		{Verdict: Verdict{State: Stale}},
		{Verdict: Verdict{State: Unknown}},
		{Verdict: Verdict{State: Federated}},
		{Verdict: Verdict{State: Grace}},
		{Verdict: Verdict{State: Alive}},
		{Verdict: Verdict{State: Stale}},
	}
	if got := StaleOf(probes); len(got) != 2 {
		t.Errorf("StaleOf returned %d probes, want 2 — only STALE is actionable", len(got))
	}
}
