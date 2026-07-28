package procsample

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/grovetools/core/pkg/process"
)

// KillResult reports the outcome for one pid touched by KillSubtree:
//
//	"term"         — exited after SIGTERM within the grace period
//	"kill"         — survived SIGTERM, exited after SIGKILL
//	"escaped-kill" — reparented out of the subtree after the pre-TERM
//	                 capture, SIGKILLed via the captured pid list
//	"survived"     — still alive after SIGKILL
//	"skipped"      — protected pid (pid<=1, our own pid, or an ancestor)
type KillResult struct {
	PID     int
	Comm    string
	Outcome string
}

// Injection points so tests never signal real processes.
var (
	killFn = func(pid int, sig syscall.Signal) error {
		return syscall.Kill(pid, sig)
	}
	aliveFn      = process.IsProcessAlive
	snapshotFn   = Snapshot
	getpidFn     = os.Getpid
	pollInterval = 100 * time.Millisecond
	killWait     = 1 * time.Second
)

// defaultGrace is used when KillSubtree is called with grace <= 0.
const defaultGrace = 3 * time.Second

// KillSubtree escalates through the subtree rooted at root as captured in
// sample: SIGTERM to root's process group (kill(-root)) and to every subtree
// pid, then a grace wait (default 3s, polling liveness), then SIGKILL for
// survivors. Because the pid list is captured before the TERM, processes that
// reparent out of the subtree ("escapees") are still SIGKILLed and reported
// as "escaped-kill". Returns one KillResult per subtree pid.
//
// Guard rails: it never signals pid <= 1, the calling process, or any of the
// caller's ancestors — such pids are reported as "skipped", and the call
// errors outright when root itself is protected. Signalling is same-user by
// construction: kill(2) fails with EPERM for other users' processes.
func KillSubtree(ctx context.Context, sample *Sample, root int, grace time.Duration) ([]KillResult, error) {
	if sample == nil {
		return nil, errors.New("procsample: nil sample")
	}
	if root <= 1 {
		return nil, fmt.Errorf("procsample: refusing to signal pid %d", root)
	}
	protected := sample.ancestrySet(getpidFn())
	if protected[root] {
		return nil, fmt.Errorf("procsample: refusing to kill own process or ancestor (pid %d)", root)
	}
	if grace <= 0 {
		grace = defaultGrace
	}

	pids := sample.Subtree(root)
	if len(pids) == 0 {
		return nil, fmt.Errorf("procsample: pid %d not in sample", root)
	}
	outcome := make(map[int]string, len(pids))
	eligible := make([]int, 0, len(pids))
	for _, pid := range pids {
		if pid <= 1 || protected[pid] {
			outcome[pid] = "skipped"
			continue
		}
		eligible = append(eligible, pid)
	}

	// Phase 1: SIGTERM the process group, then each captured pid (catches
	// subtree members that already left the group).
	_ = killFn(-root, syscall.SIGTERM)
	for _, pid := range eligible {
		_ = killFn(pid, syscall.SIGTERM)
	}
	waitUntilDead(ctx, eligible, grace)

	var survivors []int
	for _, pid := range eligible {
		if aliveFn(pid) {
			survivors = append(survivors, pid)
		} else {
			outcome[pid] = "term"
		}
	}

	// Phase 2: SIGKILL survivors. A fresh snapshot distinguishes pids still
	// under root ("kill") from ones that reparented away ("escaped-kill").
	if len(survivors) > 0 && ctx.Err() == nil {
		inTree := make(map[int]bool, len(survivors))
		if procs, err := snapshotFn(); err == nil {
			if _, ok := procs[root]; ok {
				for _, pid := range subtreePids(buildTree(procs), root) {
					inTree[pid] = true
				}
			}
		} else {
			// Can't tell; classify everything as an in-tree kill.
			for _, pid := range survivors {
				inTree[pid] = true
			}
		}
		for _, pid := range survivors {
			_ = killFn(pid, syscall.SIGKILL)
			if inTree[pid] {
				outcome[pid] = "kill"
			} else {
				outcome[pid] = "escaped-kill"
			}
		}
		waitUntilDead(ctx, survivors, killWait)
		for _, pid := range survivors {
			if aliveFn(pid) {
				outcome[pid] = "survived"
			}
		}
	}

	results := make([]KillResult, 0, len(pids))
	for _, pid := range pids {
		o := outcome[pid]
		if o == "" {
			o = "survived" // ctx cancelled before escalation finished
		}
		results = append(results, KillResult{PID: pid, Comm: sample.Procs[pid].Comm, Outcome: o})
	}
	return results, ctx.Err()
}

// waitUntilDead polls aliveFn for the given pids until all are dead, the
// timeout elapses, or ctx is cancelled.
func waitUntilDead(ctx context.Context, pids []int, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for {
		anyAlive := false
		for _, pid := range pids {
			if aliveFn(pid) {
				anyAlive = true
				break
			}
		}
		if !anyAlive || !time.Now().Before(deadline) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(pollInterval):
		}
	}
}
