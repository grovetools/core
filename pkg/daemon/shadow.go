package daemon

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/process"
)

// This file is about daemons that are running and are not the daemon.
//
// `groved start` does not fork: the invoked process IS the daemon, and the
// pidfile is what makes exactly one of them THE daemon. Everything that
// censuses daemons — `groved status`, `groved health`, `groved stats`, the
// inspector's fleet tab — starts from those pidfiles, so a groved that is
// running without owning one is invisible to all of them while still running
// collectors, fsnotify watchers, log tailers and its own signal-cli.
//
// That is not hypothetical. On 2026-08-15 two full unscoped daemons ran side by
// side for 2h22m — one holding the pidfile at ~250% CPU, one holding nothing at
// ~120% CPU with a duplicate signal-cli receiver on the same account — and
// every daemon-side surface reported a single healthy daemon. `ps` was the only
// thing that knew. The pidfile lock (daemon's pidfile package) is what stops
// that pair from being elected in the first place; this census is what makes
// the state findable in minutes rather than hours if one ever appears anyway,
// from an older binary, a hand-run `groved start --socket`, or a bug.
//
// The identity rule is the same one FindFixtureDaemons uses: a process is
// judged by the socket and pidfile it was ASKED to serve, never by its name.

// ShadowDaemon is a running `groved start` process that does not own the
// pidfile it is serving under.
type ShadowDaemon struct {
	PID  int
	PPID int
	// Elapsed is how long the process has been running (0 = unknown).
	Elapsed time.Duration
	// PidPath is the pidfile this process should own — its --pidfile argument,
	// or the global pidfile when it passed neither --pidfile nor --scope.
	PidPath string
	// OwnerPID is who that pidfile actually names: another live daemon, or 0
	// when the file is absent, empty, or names a dead process.
	OwnerPID int
	// SockPath is the --socket argument, when it passed one.
	SockPath string
	// Cmdline is the full argv, so whoever reads the warning can see exactly
	// what is running before deciding what to do about it.
	Cmdline string
}

// Reason explains, in one line, why this process is reported.
func (s ShadowDaemon) Reason() string {
	if s.OwnerPID > 0 {
		return fmt.Sprintf("%s is owned by pid %d", filepath.Base(s.PidPath), s.OwnerPID)
	}
	return fmt.Sprintf("%s names no live daemon", filepath.Base(s.PidPath))
}

// FindShadowDaemons enumerates running groved processes that no pidfile in this
// machine's state directory accounts for.
//
// Deliberately NOT reported, because each is a legitimate daemon that this
// census simply cannot adjudicate:
//
//   - fixture daemons, whose socket or pidfile lives in a sandbox namespace.
//     They belong to a test run and have their own census (FindFixtureDaemons).
//   - daemons whose pidfile is outside our state directory — a different
//     GROVE_HOME or XDG_STATE_HOME, i.e. not our fleet at all.
//   - scoped daemons started with --scope but no --pidfile, whose pidfile path
//     depends on a scope resolution that needs their working directory. In
//     practice the auto-start factory always passes --pidfile, so this only
//     skips hand-run invocations.
//
// A `groved upgrade` handoff transiently qualifies: for the moment between the
// successor taking the pidfile and the predecessor finishing its teardown, the
// predecessor is a running groved that owns nothing. That window is
// sub-second, and this census is only ever run by a human asking a question.
func FindShadowDaemons() ([]ShadowDaemon, error) {
	procs, err := process.List()
	if err != nil {
		return nil, err
	}
	return findShadowDaemons(procs, paths.StateDir(), paths.PidFilePath(), pidfileState), nil
}

// findShadowDaemons is FindShadowDaemons against an explicit process list,
// state directory, global pidfile path and pidfile reader (the test seam).
func findShadowDaemons(
	procs []process.Entry,
	stateDir, globalPidPath string,
	owner func(pidPath string) (bool, int),
) []ShadowDaemon {
	// Every PID that ps says is a live groved, so a pidfile naming a process
	// that is gone (or naming something that is not a groved at all) does not
	// vouch for anybody.
	liveGroveds := make(map[int]bool, len(procs))
	for _, p := range procs {
		if isGrovedStart(p.Args) {
			liveGroveds[p.PID] = true
		}
	}

	var found []ShadowDaemon
	for _, p := range procs {
		if !isGrovedStart(p.Args) {
			continue
		}
		sock := flagValue(p.Args, "--socket")
		pidPath := flagValue(p.Args, "--pidfile")
		scope := flagValue(p.Args, "--scope")

		// A sandboxed test daemon is somebody else's business.
		if FixtureNamespace(sock) != "" || FixtureNamespace(pidPath) != "" {
			continue
		}

		if pidPath == "" {
			if scope != "" {
				// Unresolvable without the process's cwd — see the doc comment.
				continue
			}
			pidPath = globalPidPath
		}
		if !withinDir(stateDir, pidPath) {
			continue
		}

		running, ownerPID := owner(pidPath)
		if ownerPID == p.PID {
			continue
		}
		if !running || !liveGroveds[ownerPID] {
			ownerPID = 0
		}

		found = append(found, ShadowDaemon{
			PID:      p.PID,
			PPID:     p.PPID,
			Elapsed:  p.Elapsed,
			PidPath:  pidPath,
			OwnerPID: ownerPID,
			SockPath: sock,
			Cmdline:  p.Args,
		})
	}

	sort.Slice(found, func(i, j int) bool { return found[i].PID < found[j].PID })
	return found
}

// isGrovedStart reports whether a command line is a groved DAEMON, not one of
// the many groved client subcommands. "groved status" and "groved stop" are
// short-lived processes that must never be mistaken for one.
//
// The "start" token is matched as a whole argv word anywhere after the binary —
// the same test fixtureSocketArg applies — rather than positionally, because
// flags and their values may precede the subcommand.
func isGrovedStart(cmdline string) bool {
	fields := strings.Fields(cmdline)
	if len(fields) < 2 {
		return false
	}
	if filepath.Base(fields[0]) != "groved" {
		return false
	}
	return hasWord(fields[1:], "start")
}

// flagValue extracts "--name value" or "--name=value" from a command line.
func flagValue(cmdline, name string) string {
	fields := strings.Fields(cmdline)
	for i, f := range fields {
		if f == name && i+1 < len(fields) {
			return fields[i+1]
		}
		if v, ok := strings.CutPrefix(f, name+"="); ok {
			return v
		}
	}
	return ""
}

// FormatShadowDaemons renders a census for a human, one process per stanza.
func FormatShadowDaemons(shadows []ShadowDaemon) string {
	if len(shadows) == 0 {
		return "none\n"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d groved process(es) running without owning a pidfile\n", len(shadows))
	for _, s := range shadows {
		age := "unknown age"
		if s.Elapsed > 0 {
			age = s.Elapsed.Round(time.Second).String()
		}
		fmt.Fprintf(&b, "  pid %d (ppid %d, %s) — %s\n", s.PID, s.PPID, age, s.Reason())
		fmt.Fprintf(&b, "    %s\n", s.Cmdline)
	}
	b.WriteString("  → these serve no client and no census can see them; `kill` them after confirming with `ps`\n")
	return b.String()
}
