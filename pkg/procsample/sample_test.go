package procsample

import (
	"reflect"
	"testing"
	"time"
)

// proc is a test helper for building synthetic tables.
func proc(pid, ppid int, comm string, rss int64, cputime time.Duration, pct float64) Proc {
	return Proc{PID: pid, PPID: ppid, Comm: comm, RSSKB: rss, CPUTime: cputime, PctCPU: pct}
}

func table(procs ...Proc) map[int]Proc {
	m := make(map[int]Proc, len(procs))
	for _, p := range procs {
		m[p.PID] = p
	}
	return m
}

func withGetpid(t *testing.T, pid int) {
	t.Helper()
	orig := getpidFn
	getpidFn = func() int { return pid }
	t.Cleanup(func() { getpidFn = orig })
}

// synthetic tree:
//
//	1 ── 10 ── 20 ── 30
//	│     └── 21
//	└─ 11
func treeTable() map[int]Proc {
	return table(
		proc(1, 0, "init", 1000, 0, 0),
		proc(10, 1, "shell", 2000, 0, 1.0),
		proc(11, 1, "other", 3000, 0, 2.0),
		proc(20, 10, "nvim", 4000, 0, 3.0),
		proc(21, 10, "gopls", 9000, 0, 3.0),
		proc(30, 20, "git", 500, 0, 0.5),
	)
}

func TestChildrenAndSubtree(t *testing.T) {
	s := NewSampler().sample(treeTable(), time.Now())

	if got, want := s.Children(1), []int{10, 11}; !reflect.DeepEqual(got, want) {
		t.Errorf("Children(1) = %v, want %v", got, want)
	}
	if got, want := s.Children(10), []int{20, 21}; !reflect.DeepEqual(got, want) {
		t.Errorf("Children(10) = %v, want %v", got, want)
	}
	if got := s.Children(30); len(got) != 0 {
		t.Errorf("Children(30) = %v, want empty", got)
	}

	if got, want := s.Subtree(10), []int{10, 20, 21, 30}; !reflect.DeepEqual(got, want) {
		t.Errorf("Subtree(10) = %v, want %v", got, want)
	}
	if got, want := s.Subtree(30), []int{30}; !reflect.DeepEqual(got, want) {
		t.Errorf("Subtree(30) = %v, want %v", got, want)
	}
	if got := s.Subtree(999); got != nil {
		t.Errorf("Subtree(999) = %v, want nil", got)
	}
}

func TestSubtreeCycleSafe(t *testing.T) {
	// Degenerate table with a ppid cycle must not hang.
	s := NewSampler().sample(table(
		proc(10, 20, "a", 0, 0, 0),
		proc(20, 10, "b", 0, 0, 0),
	), time.Now())
	if got, want := s.Subtree(10), []int{10, 20}; !reflect.DeepEqual(got, want) {
		t.Errorf("Subtree(10) = %v, want %v", got, want)
	}
}

func TestSamplerCPUDelta(t *testing.T) {
	sampler := NewSampler()
	t0 := time.Now()

	first := sampler.sample(table(
		proc(100, 1, "busy", 100, 10*time.Second, 7.5),
		proc(101, 1, "idle", 100, 1*time.Second, 0.3),
	), t0)
	// First sample has no history: falls back to ps pcpu.
	if got := first.CPU[100]; got != 7.5 {
		t.Errorf("first sample CPU[100] = %v, want pcpu fallback 7.5", got)
	}

	second := sampler.sample(table(
		proc(100, 1, "busy", 100, 11*time.Second, 7.5),  // +1s cputime
		proc(101, 1, "idle", 100, 1*time.Second, 0.3),   // no change
		proc(102, 1, "fresh", 100, 5*time.Second, 4.2),  // new pid
		proc(103, 1, "reused", 100, 2*time.Second, 1.1), // not in prev
	), t0.Add(2*time.Second))

	// 1s of cputime over a 2s window = 50%.
	if got := second.CPU[100]; got != 50.0 {
		t.Errorf("CPU[100] = %v, want 50.0", got)
	}
	if got := second.CPU[101]; got != 0.0 {
		t.Errorf("CPU[101] = %v, want 0.0", got)
	}
	// New pid: pcpu fallback.
	if got := second.CPU[102]; got != 4.2 {
		t.Errorf("CPU[102] = %v, want pcpu fallback 4.2", got)
	}

	// Third sample: pid 100's cputime went backwards (pid reuse) -> fallback.
	third := sampler.sample(table(
		proc(100, 1, "busy", 100, 3*time.Second, 2.2),
	), t0.Add(4*time.Second))
	if got := third.CPU[100]; got != 2.2 {
		t.Errorf("CPU[100] after counter reset = %v, want pcpu fallback 2.2", got)
	}
}

func TestRollup(t *testing.T) {
	sampler := NewSampler()
	t0 := time.Now()
	sampler.sample(table(
		proc(1, 0, "init", 1000, 0, 0),
		proc(10, 1, "shell", 2000, 0, 0),
		proc(20, 10, "nvim", 4000, 0, 0),
		proc(21, 10, "gopls", 9000, 0, 0),
		proc(30, 20, "git", 500, 0, 0),
	), t0)
	// Second sample 1s later: shell +0.5s, nvim +0.2s, gopls +0.2s, git +0.1s.
	s := sampler.sample(table(
		proc(1, 0, "init", 1000, 0, 0),
		proc(10, 1, "shell", 2000, 500*time.Millisecond, 0),
		proc(20, 10, "nvim", 4000, 200*time.Millisecond, 0),
		proc(21, 10, "gopls", 9000, 200*time.Millisecond, 0),
		proc(30, 20, "git", 500, 100*time.Millisecond, 0),
	), t0.Add(1*time.Second))

	r := s.Rollup(10)
	if r.Root != 10 || r.Procs != 4 {
		t.Fatalf("Rollup = %+v", r)
	}
	if want := []int{10, 20, 21, 30}; !reflect.DeepEqual(r.Pids, want) {
		t.Errorf("Pids = %v, want %v", r.Pids, want)
	}
	if r.RSSKB != 2000+4000+9000+500 {
		t.Errorf("RSSKB = %d, want %d", r.RSSKB, 15500)
	}
	if want := 100.0; !closeTo(r.CPU, want) {
		t.Errorf("CPU = %v, want %v", r.CPU, want)
	}
	// Top ranks descendants only (root shell is hottest but excluded);
	// nvim and gopls tie at 20%, gopls wins on RSS.
	if r.Top.PID != 21 || !closeTo(r.TopCPU, 20.0) {
		t.Errorf("Top = %+v TopCPU = %v, want pid 21 at 20%%", r.Top, r.TopCPU)
	}

	// Leaf rollup: Top falls back to the root itself.
	leaf := s.Rollup(30)
	if leaf.Procs != 1 || leaf.Top.PID != 30 || !closeTo(leaf.TopCPU, 10.0) {
		t.Errorf("leaf rollup = %+v", leaf)
	}

	// Missing root: zero rollup.
	if r := s.Rollup(999); r.Procs != 0 || r.Pids != nil {
		t.Errorf("Rollup(999) = %+v, want zero", r)
	}
}

func closeTo(got, want float64) bool {
	d := got - want
	return d < 1e-9 && d > -1e-9
}

func TestOrphans(t *testing.T) {
	// self = 50, parented by 40, parented by 1.
	withGetpid(t, 50)
	s := NewSampler().sample(table(
		proc(1, 0, "init", 0, 0, 0),
		proc(2, 0, "kthreadd", 0, 0, 0),
		proc(3, 2, "kworker-git", 0, 0, 0),   // kernel thread, matches "git", excluded
		proc(10, 1, "tuimuxd", 0, 0, 0),      // tracked root
		proc(20, 10, "git", 0, 0, 0),         // inside tracked subtree
		proc(40, 1, "nvim", 0, 0, 0),         // our ancestor, excluded
		proc(50, 40, "claude", 0, 0, 0),      // ourselves, excluded
		proc(60, 1, "nvim", 0, 0, 0),         // orphan
		proc(61, 1, "Git", 0, 0, 0),          // orphan, case-insensitive match
		proc(62, 1, "gopls", 0, 0, 0),        // no matching interest
		proc(63, 60, "hash-object", 0, 0, 0), // orphan (child of orphan)
	), time.Now())

	got := s.Orphans([]int{10}, []string{"nvim", "git", "claude", "hash-object"})
	var pids []int
	for _, p := range got {
		pids = append(pids, p.PID)
	}
	if want := []int{60, 61, 63}; !reflect.DeepEqual(pids, want) {
		t.Errorf("Orphans = %v, want %v", pids, want)
	}
}

func TestOrphansExcludesTrackedRootAncestors(t *testing.T) {
	withGetpid(t, 99999) // not in table
	s := NewSampler().sample(table(
		proc(1, 0, "init", 0, 0, 0),
		proc(5, 1, "nvim", 0, 0, 0),   // ancestor of tracked root 10
		proc(10, 5, "shell", 0, 0, 0), // tracked root
		proc(30, 1, "nvim", 0, 0, 0),  // true orphan
	), time.Now())
	got := s.Orphans([]int{10}, DefaultInterest)
	if len(got) != 1 || got[0].PID != 30 {
		t.Errorf("Orphans = %+v, want just pid 30", got)
	}
}
