package health

import (
	"strings"
	"testing"
	"time"

	"github.com/grovetools/core/pkg/models"
)

var now = time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)

// active returns a session that looks live to the daemon and has been
// quiet for d — the shape every ladder case starts from.
func active(quiet time.Duration) *models.Session {
	return &models.Session{
		ID:           "job-1",
		Status:       "running",
		Type:         "interactive_agent",
		LastActivity: now.Add(-quiet),
		StartedAt:    now.Add(-quiet),
	}
}

func TestClassifyGuards(t *testing.T) {
	tests := []struct {
		name string
		s    *models.Session
		ev   Evidence
		want State
	}{
		{
			name: "federated is hands-off even when provably dead",
			s: func() *models.Session {
				s := active(time.Hour)
				s.Origin = "satellite-1"
				s.PID = 4242
				return s
			}(),
			ev:   Evidence{PTY: PTYEvidence{Queried: true}},
			want: Federated,
		},
		{
			name: "terminal status is nothing to clean",
			s: func() *models.Session {
				s := active(time.Hour)
				s.Status = "completed"
				return s
			}(),
			want: Inactive,
		},
		{
			name: "a parked chat has no process between turns",
			s: func() *models.Session {
				s := active(time.Hour)
				s.Type = "chat"
				s.Status = "pending_user"
				return s
			}(),
			want: Waiting,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Classify(tc.s, tc.ev, now).State; got != tc.want {
				t.Errorf("state = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestClassifyLivenessLadder pins the precedence: a live PTY outranks a
// live tmux window outranks a live registry PID outranks a live session
// PID, and any one of them is enough.
func TestClassifyLivenessLadder(t *testing.T) {
	tests := []struct {
		name string
		ev   Evidence
	}{
		{"live pty", Evidence{PTY: PTYEvidence{Queried: true, Found: true, ID: "pty-abcdef01"}}},
		{"live tmux window", Evidence{Tmux: TmuxEvidence{Queried: true, Found: true, Target: "proj:job-x"}}},
		{"live registry pid", Evidence{RegistryPID: 99, RegistryPIDAlive: true}},
		{"live session pid", Evidence{SessionPIDAlive: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := active(time.Hour) // long past every grace window
			s.PID = 4242
			if got := Classify(s, tc.ev, now); got.State != Alive {
				t.Errorf("state = %v (%s), want ALIVE", got.State, got.Reason)
			}
		})
	}
}

// TestClassifyGraceWindow: dead signals inside the grace window never
// convict, so a freshly respawned agent can't be swept mid-handoff.
func TestClassifyGraceWindow(t *testing.T) {
	ev := Evidence{PTY: PTYEvidence{Queried: true}}

	s := active(GracePeriod - time.Second)
	s.PID = 4242
	if got := Classify(s, ev, now); got.State != Grace {
		t.Errorf("inside grace: state = %v (%s), want GRACE", got.State, got.Reason)
	}

	s = active(GracePeriod + time.Second)
	s.PID = 4242
	if got := Classify(s, ev, now); got.State != Stale {
		t.Errorf("past grace: state = %v (%s), want STALE", got.State, got.Reason)
	}
}

// TestClassifyUnreachableMultiplexerIsUnknown: we never convict on an
// answer we could not get. An unreachable tuimux (or tmux) means
// UNKNOWN, not STALE — that asymmetry is the whole safety property.
func TestClassifyUnreachableMultiplexerIsUnknown(t *testing.T) {
	s := active(time.Hour)
	s.PID = 4242
	s.PtyID = "pty-abcdef01"
	// PTY.Queried false == tuimux unreachable.
	if got := Classify(s, Evidence{}, now); got.State != Unknown {
		t.Errorf("tuimux unreachable: state = %v (%s), want UNKNOWN", got.State, got.Reason)
	}

	tmuxOnly := active(time.Hour)
	tmuxOnly.TmuxTarget = "proj:job-x"
	if got := Classify(tmuxOnly, Evidence{PTY: PTYEvidence{Queried: true}}, now); got.State != Unknown {
		t.Errorf("tmux not probed: state = %v (%s), want UNKNOWN", got.State, got.Reason)
	}
}

// TestClassifyTmuxProbeResolvesUnknown is item #6: once tmux IS probed
// and the window is gone, the same session becomes a real verdict.
func TestClassifyTmuxProbeResolvesUnknown(t *testing.T) {
	s := active(time.Hour)
	s.TmuxTarget = "proj:job-x"
	ev := Evidence{
		PTY:  PTYEvidence{Queried: true},
		Tmux: TmuxEvidence{Queried: true, Found: false, Target: "proj:job-x"},
	}
	got := Classify(s, ev, now)
	if got.State != Stale {
		t.Fatalf("state = %v (%s), want STALE once tmux was actually probed", got.State, got.Reason)
	}
	if !strings.Contains(got.Reason, "tmux window proj:job-x gone") {
		t.Errorf("reason = %q, want it to name the missing tmux window", got.Reason)
	}
}

// TestClassifyStaleReasonNamesEverySignal: the Reason is the artifact a
// wrong flip is diagnosed from, so it must enumerate every dead signal,
// not just the first one found.
func TestClassifyStaleReasonNamesEverySignal(t *testing.T) {
	s := active(5 * time.Minute)
	s.PID = 4242
	s.PtyID = "pty-gone"
	ev := Evidence{
		RegistryPID: 4243,
		PTY:         PTYEvidence{Queried: true, Found: false},
	}
	got := Classify(s, ev, now)
	if got.State != Stale {
		t.Fatalf("state = %v (%s), want STALE", got.State, got.Reason)
	}
	for _, want := range []string{"pid 4242 dead", "registry pid 4243 dead", "pty pty-gone gone", "quiet 5m"} {
		if !strings.Contains(got.Reason, want) {
			t.Errorf("reason %q missing %q", got.Reason, want)
		}
	}
}

// TestClassifyRunningChatWithDeadPidIsStale: a turn-based job gets the
// "no process between turns" pass only while it is parked. One that
// claims to be RUNNING with a dead pid.lock is a real ghost.
func TestClassifyRunningChatWithDeadPidIsStale(t *testing.T) {
	s := active(5 * time.Minute)
	s.Type = "chat"
	ev := Evidence{RegistryPID: 777, PTY: PTYEvidence{Queried: true}}
	if got := Classify(s, ev, now); got.State != Stale {
		t.Errorf("state = %v (%s), want STALE for a running chat with a dead pid", got.State, got.Reason)
	}
}

// TestClassifyNoEvidenceNeedsALongQuietSpell: absence of evidence only
// becomes evidence of absence after NoEvidenceStaleAfter.
func TestClassifyNoEvidenceNeedsALongQuietSpell(t *testing.T) {
	ev := Evidence{PTY: PTYEvidence{Queried: true}}

	s := active(NoEvidenceStaleAfter - time.Minute)
	s.Type = "headless_agent"
	if got := Classify(s, ev, now); got.State != Grace {
		t.Errorf("state = %v (%s), want GRACE before the no-evidence threshold", got.State, got.Reason)
	}

	s = active(NoEvidenceStaleAfter + time.Minute)
	s.Type = "headless_agent"
	if got := Classify(s, ev, now); got.State != Stale {
		t.Errorf("state = %v (%s), want STALE past the no-evidence threshold", got.State, got.Reason)
	}
}

// TestNeedsTmuxProbe keeps the shell-out off the hot path: it only runs
// where it could change the answer.
func TestNeedsTmuxProbe(t *testing.T) {
	tmuxHosted := func() *models.Session {
		s := active(time.Hour)
		s.TmuxTarget = "proj:job-x"
		return s
	}

	if !NeedsTmuxProbe(tmuxHosted(), Evidence{PTY: PTYEvidence{Queried: true}}) {
		t.Error("a tmux-hosted session with no other liveness signal should be probed")
	}
	if NeedsTmuxProbe(tmuxHosted(), Evidence{PTY: PTYEvidence{Queried: true, Found: true}}) {
		t.Error("a session with a live PTY needs no tmux probe")
	}
	if NeedsTmuxProbe(tmuxHosted(), Evidence{SessionPIDAlive: true}) {
		t.Error("a session with a live PID needs no tmux probe")
	}
	if NeedsTmuxProbe(active(time.Hour), Evidence{PTY: PTYEvidence{Queried: true}}) {
		t.Error("a session with no tmux attach point should not be probed")
	}
	federated := tmuxHosted()
	federated.Origin = "satellite-1"
	if NeedsTmuxProbe(federated, Evidence{}) {
		t.Error("federated sessions are never probed")
	}
}

// TestReconciledStatusFor pins the adoption philosophy: re-runnable
// turn-based jobs get the terminal "interrupted"; agent jobs get the
// non-terminal "orphaned" ("we lost it", not "it failed").
func TestReconciledStatusFor(t *testing.T) {
	for _, typ := range []string{"chat", "oneshot", "note", ""} {
		if got := ReconciledStatusFor(typ); got != "interrupted" {
			t.Errorf("ReconciledStatusFor(%q) = %q, want interrupted", typ, got)
		}
	}
	for _, typ := range []string{"interactive_agent", "isolated_agent", "headless_agent", "agent"} {
		if got := ReconciledStatusFor(typ); got != "orphaned" {
			t.Errorf("ReconciledStatusFor(%q) = %q, want orphaned", typ, got)
		}
	}
}

func TestSplitTmuxTarget(t *testing.T) {
	tests := []struct {
		target, key string
		wantSession string
		wantWindow  string
	}{
		{target: "proj_main:job-foo", wantSession: "proj_main", wantWindow: "job-foo"},
		{target: "main:0", wantSession: "main", wantWindow: "0"},
		{target: "bare-session", wantSession: "bare-session"},
		{key: "proj_main", wantSession: "proj_main"},
		{target: "proj:win", key: "ignored", wantSession: "proj", wantWindow: "win"},
	}
	for _, tc := range tests {
		s := &models.Session{TmuxTarget: tc.target, TmuxKey: tc.key}
		gotSession, gotWindow := splitTmuxTarget(s)
		if gotSession != tc.wantSession || gotWindow != tc.wantWindow {
			t.Errorf("splitTmuxTarget(%q,%q) = (%q,%q), want (%q,%q)",
				tc.target, tc.key, gotSession, gotWindow, tc.wantSession, tc.wantWindow)
		}
	}
}
