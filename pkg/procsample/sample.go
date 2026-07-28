package procsample

import (
	"sort"
	"strings"
	"time"
)

// DefaultInterest covers grove-shaped leak candidates for Orphans
// classification. Matching is a case-insensitive substring test against
// Proc.Comm.
var DefaultInterest = []string{"nvim", "gopls", "claude", "pi", "git", "hash-object"}

// Sampler holds the previous snapshot so CPU% is a true cputime delta over
// the sample interval. The first Sample() falls back to ps's decaying PctCPU.
// A Sampler is not safe for concurrent use.
type Sampler struct {
	prev   map[int]Proc
	prevAt time.Time
}

// NewSampler returns a Sampler with no history; its first Sample() reports
// ps pcpu= values as CPU%.
func NewSampler() *Sampler {
	return &Sampler{}
}

// Sample takes a snapshot and derives interval CPU% against the previous one.
func (s *Sampler) Sample() (*Sample, error) {
	procs, err := Snapshot()
	if err != nil {
		return nil, err
	}
	return s.sample(procs, time.Now()), nil
}

// sample builds a Sample from an already-parsed proc table. Split out from
// Sample so tests can feed synthetic tables and timestamps.
func (s *Sampler) sample(procs map[int]Proc, at time.Time) *Sample {
	wall := at.Sub(s.prevAt).Seconds()
	cpu := make(map[int]float64, len(procs))
	for pid, p := range procs {
		// A cputime delta is only meaningful when the pid existed in the
		// previous snapshot and its counter did not go backwards (pid reuse).
		if prev, ok := s.prev[pid]; ok && wall > 0 && p.CPUTime >= prev.CPUTime {
			cpu[pid] = (p.CPUTime - prev.CPUTime).Seconds() / wall * 100
		} else {
			cpu[pid] = p.PctCPU
		}
	}
	s.prev = procs
	s.prevAt = at
	return &Sample{
		At:    at,
		Procs: procs,
		CPU:   cpu,
		tree:  buildTree(procs),
	}
}

// Sample is one snapshot with derived per-pid CPU% and the process tree.
type Sample struct {
	At    time.Time
	Procs map[int]Proc
	// CPU maps pid to interval CPU% (cores*100 max). On the first sample of
	// a Sampler it holds ps pcpu= fallback values.
	CPU  map[int]float64
	tree map[int][]int // ppid -> child pids, sorted
}

// buildTree indexes procs by parent pid with sorted child slices.
func buildTree(procs map[int]Proc) map[int][]int {
	tree := make(map[int][]int)
	for pid, p := range procs {
		tree[p.PPID] = append(tree[p.PPID], pid)
	}
	for _, kids := range tree {
		sort.Ints(kids)
	}
	return tree
}

// Children returns the direct children of pid, sorted ascending.
func (s *Sample) Children(pid int) []int {
	return append([]int(nil), s.tree[pid]...)
}

// Subtree returns pid plus all transitive descendants present in the sample,
// sorted ascending. It returns nil when root is not in the sample.
func (s *Sample) Subtree(root int) []int {
	if _, ok := s.Procs[root]; !ok {
		return nil
	}
	return subtreePids(s.tree, root)
}

// subtreePids walks tree from root (inclusive), guarding against cycles.
func subtreePids(tree map[int][]int, root int) []int {
	var pids []int
	seen := map[int]bool{}
	queue := []int{root}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		if seen[pid] {
			continue
		}
		seen[pid] = true
		pids = append(pids, pid)
		queue = append(queue, tree[pid]...)
	}
	sort.Ints(pids)
	return pids
}

// Rollup aggregates a subtree.
type Rollup struct {
	Root int
	// CPU is the subtree CPU% sum, interval-true when the sample has history.
	CPU float64
	// RSSKB is the sum of every subtree process's resident set size. Shared
	// pages (copy-on-write after fork, shared libraries) are counted once per
	// process, so this overstates the memory actually reclaimed by killing
	// the subtree.
	RSSKB int64
	Procs int
	// Top is the hottest descendant by CPU, ties broken by RSS. When the
	// subtree is just the root, Top is the root process itself.
	Top    Proc
	TopCPU float64
	Pids   []int
}

// Rollup aggregates the subtree rooted at root. A root that is not in the
// sample yields a zero Rollup (Procs == 0).
func (s *Sample) Rollup(root int) Rollup {
	r := Rollup{Root: root}
	pids := s.Subtree(root)
	r.Pids = pids
	r.Procs = len(pids)
	haveTop := false
	for _, pid := range pids {
		p := s.Procs[pid]
		cpu := s.CPU[pid]
		r.CPU += cpu
		r.RSSKB += p.RSSKB
		if pid == root && len(pids) > 1 {
			continue // Top ranks descendants; root only stands in for leaves.
		}
		if !haveTop || cpu > r.TopCPU || (cpu == r.TopCPU && p.RSSKB > r.Top.RSSKB) {
			r.Top = p
			r.TopCPU = cpu
			haveTop = true
		}
	}
	return r
}

// Orphans returns processes whose comm matches interest (case-insensitive
// substring) but that live outside every tracked subtree: not in any subtree
// of trackedRoots, not an ancestor of a tracked root, not pid 1 or a kernel
// thread, and not the calling process or one of its ancestors. Results are
// sorted by pid.
func (s *Sample) Orphans(trackedRoots []int, interest []string) []Proc {
	excluded := make(map[int]bool)
	for _, root := range trackedRoots {
		for _, pid := range s.Subtree(root) {
			excluded[pid] = true
		}
		for pid := range s.ancestrySet(root) {
			excluded[pid] = true
		}
	}
	for pid := range s.ancestrySet(getpidFn()) {
		excluded[pid] = true
	}
	// Linux kernel threads all descend from kthreadd (pid 2).
	if p, ok := s.Procs[2]; ok && p.Comm == "kthreadd" {
		for _, pid := range s.Subtree(2) {
			excluded[pid] = true
		}
	}
	patterns := make([]string, 0, len(interest))
	for _, pat := range interest {
		if pat = strings.ToLower(strings.TrimSpace(pat)); pat != "" {
			patterns = append(patterns, pat)
		}
	}
	var out []Proc
	for pid, p := range s.Procs {
		if pid <= 1 || excluded[pid] {
			continue
		}
		comm := strings.ToLower(p.Comm)
		for _, pat := range patterns {
			if strings.Contains(comm, pat) {
				out = append(out, p)
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PID < out[j].PID })
	return out
}

// ancestrySet returns pid plus every ancestor reachable through the sample's
// PPID chain (cycle-safe). The pid itself is included even when absent from
// the sample.
func (s *Sample) ancestrySet(pid int) map[int]bool {
	set := map[int]bool{pid: true}
	cur := pid
	for {
		p, ok := s.Procs[cur]
		if !ok || p.PPID <= 0 || set[p.PPID] {
			return set
		}
		cur = p.PPID
		set[cur] = true
	}
}
