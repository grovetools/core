package procsample

import (
	"context"
	"sync"
	"syscall"
	"testing"
	"time"
)

// sigCall records one killFn invocation.
type sigCall struct {
	pid int
	sig syscall.Signal
}

// killHarness stubs killFn/aliveFn/snapshotFn/getpidFn with a fake proc
// world so no real process is ever signalled.
type killHarness struct {
	mu         sync.Mutex
	calls      []sigCall
	dead       map[int]bool
	diesOnTerm map[int]bool
	diesOnKill map[int]bool
	resnapshot map[int]Proc // returned by snapshotFn after TERM phase
}

func newKillHarness(t *testing.T, selfPID int) *killHarness {
	t.Helper()
	h := &killHarness{
		dead:       map[int]bool{},
		diesOnTerm: map[int]bool{},
		diesOnKill: map[int]bool{},
	}
	origKill, origAlive, origSnap, origPid := killFn, aliveFn, snapshotFn, getpidFn
	origPoll, origKillWait := pollInterval, killWait
	killFn = func(pid int, sig syscall.Signal) error {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.calls = append(h.calls, sigCall{pid: pid, sig: sig})
		if pid < 0 {
			return nil // process-group signal: fake world tracks pids only
		}
		if (sig == syscall.SIGTERM && h.diesOnTerm[pid]) ||
			(sig == syscall.SIGKILL && h.diesOnKill[pid]) {
			h.dead[pid] = true
		}
		return nil
	}
	aliveFn = func(pid int) bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		return !h.dead[pid]
	}
	snapshotFn = func() (map[int]Proc, error) {
		return h.resnapshot, nil
	}
	getpidFn = func() int { return selfPID }
	pollInterval = 2 * time.Millisecond
	killWait = 10 * time.Millisecond
	t.Cleanup(func() {
		killFn, aliveFn, snapshotFn, getpidFn = origKill, origAlive, origSnap, origPid
		pollInterval, killWait = origPoll, origKillWait
	})
	return h
}

func (h *killHarness) sent(pid int, sig syscall.Signal) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, c := range h.calls {
		if c.pid == pid && c.sig == sig {
			return true
		}
	}
	return false
}

// killTable: root 100 with children 101-104; self 900 under 1.
func killTable() map[int]Proc {
	return table(
		proc(1, 0, "init", 0, 0, 0),
		proc(100, 1, "shell", 0, 0, 0),
		proc(101, 100, "git", 0, 0, 0),
		proc(102, 100, "nvim", 0, 0, 0),
		proc(103, 100, "gopls", 0, 0, 0),
		proc(104, 100, "stuck", 0, 0, 0),
		proc(900, 1, "test-self", 0, 0, 0),
	)
}

func outcomes(results []KillResult) map[int]string {
	m := make(map[int]string, len(results))
	for _, r := range results {
		m[r.PID] = r.Outcome
	}
	return m
}

func TestKillSubtreeEscalation(t *testing.T) {
	h := newKillHarness(t, 900)
	// 100, 101 die on TERM; 102 needs KILL; 103 escapes the subtree and
	// needs KILL; 104 survives everything.
	h.diesOnTerm[100] = true
	h.diesOnTerm[101] = true
	h.diesOnKill[102] = true
	h.diesOnKill[103] = true
	// Post-TERM world: 103 reparented to 1, root 100 gone but 102/104 remain
	// under it in the snapshot's tree.
	h.resnapshot = table(
		proc(1, 0, "init", 0, 0, 0),
		proc(100, 1, "shell", 0, 0, 0),
		proc(102, 100, "nvim", 0, 0, 0),
		proc(103, 1, "gopls", 0, 0, 0),
		proc(104, 100, "stuck", 0, 0, 0),
	)

	sample := NewSampler().sample(killTable(), time.Now())
	results, err := KillSubtree(context.Background(), sample, 100, 30*time.Millisecond)
	if err != nil {
		t.Fatalf("KillSubtree: %v", err)
	}

	got := outcomes(results)
	want := map[int]string{
		100: "term",
		101: "term",
		102: "kill",
		103: "escaped-kill",
		104: "survived",
	}
	for pid, o := range want {
		if got[pid] != o {
			t.Errorf("pid %d outcome = %q, want %q", pid, got[pid], o)
		}
	}
	if len(results) != len(want) {
		t.Errorf("got %d results, want %d: %+v", len(results), len(want), results)
	}

	// Process group TERMed first.
	h.mu.Lock()
	calls := append([]sigCall(nil), h.calls...)
	h.mu.Unlock()
	if len(calls) == 0 || calls[0] != (sigCall{pid: -100, sig: syscall.SIGTERM}) {
		t.Errorf("first signal = %+v, want SIGTERM to pgroup -100", calls)
	}
	// Every TERM precedes every KILL.
	lastTerm, firstKill := -1, len(calls)
	for i, c := range calls {
		switch c.sig {
		case syscall.SIGTERM:
			lastTerm = i
		case syscall.SIGKILL:
			if i < firstKill {
				firstKill = i
			}
		default:
			t.Errorf("unexpected signal %v to pid %d", c.sig, c.pid)
		}
	}
	if lastTerm > firstKill {
		t.Errorf("TERM at index %d after first KILL at %d: %+v", lastTerm, firstKill, calls)
	}
	// TERM-dead pids never receive KILL.
	for _, pid := range []int{100, 101} {
		if h.sent(pid, syscall.SIGKILL) {
			t.Errorf("pid %d died on TERM but was sent SIGKILL", pid)
		}
	}
	// Escapee and survivor got KILLed.
	for _, pid := range []int{102, 103, 104} {
		if !h.sent(pid, syscall.SIGKILL) {
			t.Errorf("pid %d was not sent SIGKILL", pid)
		}
	}
}

func TestKillSubtreeAllDieOnTerm(t *testing.T) {
	h := newKillHarness(t, 900)
	for _, pid := range []int{100, 101, 102, 103, 104} {
		h.diesOnTerm[pid] = true
	}
	sample := NewSampler().sample(killTable(), time.Now())
	results, err := KillSubtree(context.Background(), sample, 100, 30*time.Millisecond)
	if err != nil {
		t.Fatalf("KillSubtree: %v", err)
	}
	for _, r := range results {
		if r.Outcome != "term" {
			t.Errorf("pid %d outcome = %q, want term", r.PID, r.Outcome)
		}
	}
	for _, c := range func() []sigCall { h.mu.Lock(); defer h.mu.Unlock(); return append([]sigCall(nil), h.calls...) }() {
		if c.sig == syscall.SIGKILL {
			t.Errorf("SIGKILL sent to %d although everything died on TERM", c.pid)
		}
	}
}

func TestKillSubtreeGuards(t *testing.T) {
	h := newKillHarness(t, 900)
	sample := NewSampler().sample(killTable(), time.Now())

	// pid <= 1 refused outright.
	for _, root := range []int{-5, 0, 1} {
		if _, err := KillSubtree(context.Background(), sample, root, time.Millisecond); err == nil {
			t.Errorf("KillSubtree(root=%d) succeeded, want error", root)
		}
	}
	// Our own pid and our ancestors are refused as roots.
	if _, err := KillSubtree(context.Background(), sample, 900, time.Millisecond); err == nil {
		t.Error("KillSubtree(self) succeeded, want error")
	}
	// Unknown root errors.
	if _, err := KillSubtree(context.Background(), sample, 4242, time.Millisecond); err == nil {
		t.Error("KillSubtree(unknown pid) succeeded, want error")
	}
	if len(h.calls) != 0 {
		t.Errorf("guard-refused calls still signalled: %+v", h.calls)
	}
}

func TestKillSubtreeRefusesWhenSelfInsideSubtree(t *testing.T) {
	// Self (900) lives INSIDE the subtree being killed, which makes root 100
	// one of our ancestors: the whole call must be refused, nothing signalled.
	h := newKillHarness(t, 900)
	tbl := table(
		proc(1, 0, "init", 0, 0, 0),
		proc(100, 1, "shell", 0, 0, 0),
		proc(101, 100, "git", 0, 0, 0),
		proc(900, 100, "test-self", 0, 0, 0),
	)
	sample := NewSampler().sample(tbl, time.Now())
	if _, err := KillSubtree(context.Background(), sample, 100, 30*time.Millisecond); err == nil {
		t.Fatal("KillSubtree with self inside subtree succeeded, want error")
	}
	if len(h.calls) != 0 {
		t.Errorf("refused call still signalled: %+v", h.calls)
	}
}

func TestKillSubtreeSkipsProtectedPidsInsideSubtree(t *testing.T) {
	// The per-pid skip guard only matters when the sample is internally
	// inconsistent (self's ancestry chain does not reach root even though
	// the tree lists self under it). Build such a sample by hand: 900's
	// PPID points at an unknown pid, but the tree places it under 100.
	h := newKillHarness(t, 900)
	h.diesOnTerm[100] = true
	h.diesOnTerm[101] = true
	sample := &Sample{
		At: time.Now(),
		Procs: table(
			proc(100, 1, "shell", 0, 0, 0),
			proc(101, 100, "git", 0, 0, 0),
			proc(900, 5000, "test-self", 0, 0, 0),
		),
		CPU:  map[int]float64{},
		tree: map[int][]int{100: {101, 900}},
	}
	results, err := KillSubtree(context.Background(), sample, 100, 30*time.Millisecond)
	if err != nil {
		t.Fatalf("KillSubtree: %v", err)
	}
	got := outcomes(results)
	if got[900] != "skipped" {
		t.Errorf("self outcome = %q, want skipped", got[900])
	}
	if h.sent(900, syscall.SIGTERM) || h.sent(900, syscall.SIGKILL) {
		t.Errorf("protected pid 900 was signalled directly: %+v", h.calls)
	}
	if got[100] != "term" || got[101] != "term" {
		t.Errorf("outcomes = %v", got)
	}
}

func TestKillSubtreeNilSample(t *testing.T) {
	if _, err := KillSubtree(context.Background(), nil, 100, time.Second); err == nil {
		t.Error("KillSubtree(nil sample) succeeded, want error")
	}
}
