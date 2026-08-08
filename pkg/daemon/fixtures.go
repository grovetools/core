package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/process"
)

// This file is about daemons that tests spawn and then fail to collect.
//
// A tend scenario or a TUI-pilot run sandboxes its subject by pointing
// HOME/XDG_* at a throwaway namespace under the temp root. Anything the
// subject auto-starts — a groved, a tuimux daemon — binds its socket inside
// that namespace, which is exactly what makes the sandbox a sandbox. It also
// makes those daemons invisible: `groved status` enumerates the REAL state
// dir, so a fixture groved that outlives its run does not appear in any
// census, holds fsnotify watchers and log tailers forever, and is discovered
// only by someone reading `ps` output during a performance investigation.
//
// Orphaning is the normal case, not the exceptional one: any interrupted run
// (agent killed mid-verification, Bash timeout, crashed scenario) skips the
// harness's teardown defer. Pairing (see pkg/pairwatch) fixes that going
// forward by making the daemons self-reaping. This file is the backstop for
// what pairing cannot cover — daemons spawned by an older binary, or by a
// path that never learned to pair — plus the census that makes them findable.
//
// Everything here is built around one rule: a candidate is a fixture only if
// its own socket path lives inside a recognized fixture namespace. Never the
// process name, never the executable, never "it looks like a test". A real
// groved serving ~/.local/state/grove/groved.sock must survive every sweep,
// however aggressive, because getting that wrong reaps the user's live
// sessions (see the job-52 incident: a fixture daemon whose scope resolved
// global adopted REAL tuimuxd PTYs and killed them on shutdown).

// FixtureLeaseName is the file a fixture-spawning harness drops in its
// namespace to say "this namespace is in use, and here is how to tell when it
// stops being in use". It is a sidecar, not a lock: its absence never makes a
// namespace sweepable on its own, it only lets the sweep act sooner and with
// more confidence than an age threshold allows.
const FixtureLeaseName = ".grove-fixture-lease"

// fixtureNamespacePrefixes are the directory/socket name prefixes that mark a
// path as belonging to a disposable test fixture. Each corresponds to a real
// producer:
//
//	tend-<testID>-*   pkg/harness   XDG_RUNTIME_DIR for one scenario
//	tend-debug-*      tend run      --debug-session runtime dir
//	grove-tend-*      pkg/harness   sandbox home (TempDirManager base)
//	tendlab-*         pkg/lab       lab runtime dir
//	tuipilot-*        grove-tui-pilot skill  run-namespaced sockets in /tmp
//
// A path qualifies when the first component below a temp root carries one of
// these prefixes, so both a namespace DIRECTORY (/tmp/tend-foo-123/…) and a
// bare namespaced SOCKET (/tmp/tuipilot-20260805-gtd-tuimux.sock) match.
var fixtureNamespacePrefixes = []string{
	"tend-",
	"grove-tend-",
	"tendlab-",
	"tuipilot-",
}

// tempRoots returns the directories under which a fixture namespace may live.
//
// /tmp is listed explicitly because fs.CreateShortTempDir forces it on
// macOS/Linux (the default macOS TMPDIR is too long for the ~104-char Unix
// socket limit), while TempDirManager uses the ambient TMPDIR — so the runtime
// namespace and the sandbox home of the SAME run routinely live under
// different roots.
func tempRoots() []string {
	roots := []string{"/tmp", "/private/tmp"}
	if td := strings.TrimRight(os.TempDir(), "/"); td != "" {
		roots = append(roots, td)
	}
	if td := strings.TrimRight(os.Getenv("TMPDIR"), "/"); td != "" {
		roots = append(roots, td)
	}

	// Keep every spelling of every root. On macOS /tmp is a symlink to
	// /private/tmp and TMPDIR resolves under /private/var/folders, so a socket
	// path recorded as /tmp/... and the same path resolved to /private/tmp/...
	// must BOTH match — collapsing to the resolved form alone would silently
	// stop recognizing the literal /tmp paths that ps actually reports.
	seen := make(map[string]bool, len(roots)*2)
	var out []string
	add := func(r string) {
		if r != "" && !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	for _, r := range roots {
		add(r)
		if resolved, err := filepath.EvalSymlinks(r); err == nil {
			add(strings.TrimRight(resolved, "/"))
		}
	}
	return out
}

// FixtureNamespace returns the fixture namespace that vouches for path, or ""
// when path is not inside one.
//
// The returned namespace is the full path of the first component below a temp
// root — the directory a sweep may reason about the staleness of. For a bare
// namespaced socket file the namespace IS that file.
//
// This is the single identity gate for everything in this file. It is
// deliberately strict in both directions: a path outside every temp root can
// never be a fixture (that is what protects ~/.local/state), and a path under
// a temp root whose leading component carries no fixture prefix can never be
// one either (that is what protects unrelated temp files).
func FixtureNamespace(path string) string {
	if path == "" || !filepath.IsAbs(path) {
		return ""
	}
	clean := filepath.Clean(path)

	// Refuse anything resolving into the real Grove directories even when a
	// temp root somehow contains them (GROVE_HOME=/tmp/…, TMPDIR games). The
	// prefix check below would already reject these, but the cost of being
	// wrong here is the user's live daemon, so the guard is explicit.
	for _, real := range []string{paths.StateDir(), paths.RuntimeDir(), paths.DataDir(), paths.ConfigDir()} {
		if real != "" && withinDir(real, clean) {
			return ""
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" && withinDir(filepath.Join(home, ".local"), clean) {
		return ""
	}

	for _, root := range tempRoots() {
		rel, err := filepath.Rel(root, clean)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			continue
		}
		first := rel
		if idx := strings.IndexByte(rel, filepath.Separator); idx >= 0 {
			first = rel[:idx]
		}
		for _, prefix := range fixtureNamespacePrefixes {
			if strings.HasPrefix(first, prefix) {
				return filepath.Join(root, first)
			}
		}
	}
	return ""
}

// sameNamespace reports whether two namespace paths name the same directory.
//
// Literal comparison is not enough on macOS, where /tmp is a symlink to
// /private/tmp and TMPDIR resolves under /private/var/folders: a harness holds
// the path it created (/tmp/tend-x) while `ps` may report the daemon's socket
// resolved (/private/tmp/tend-x). Both spellings are compared, and a
// resolution failure falls back to the literal rather than matching — a scoped
// sweep that cannot prove the namespaces are the same must not sweep.
func sameNamespace(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if a == b {
		return true
	}
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}

// withinDir reports whether path is dir itself or lives underneath it.
func withinDir(dir, path string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// FixtureLease records who owns a fixture namespace and how to tell that they
// are gone. A sweep treats a namespace as abandoned when the lease says so —
// which is faster and far more certain than waiting out an age threshold.
type FixtureLease struct {
	// OwnerPID is the process that created the namespace and is expected to
	// tear it down. Dead PID means the run died without collecting.
	OwnerPID int `json:"owner_pid,omitempty"`
	// OwnerPath is a directory whose existence means the namespace is still
	// wanted. Long-lived fixtures (a `tend lab`) outlive the process that
	// created them, so they lease against their lab directory instead of a PID.
	OwnerPath string `json:"owner_path,omitempty"`
	// Owner names the producer, for humans reading a census.
	Owner     string    `json:"owner,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// Abandoned reports whether the lease's owner is provably gone, and why.
//
// "Provably" is the operative word. A dead OwnerPID is proof: PID reuse can
// only ever make a dead owner look ALIVE, which errs toward not sweeping. A
// missing OwnerPath is proof for the same reason — a directory does not come
// back. A lease that asserts neither is not proof of anything, so it returns
// false and leaves the decision to the age threshold.
func (l FixtureLease) Abandoned() (bool, string) {
	switch {
	case l.OwnerPID > 0 && !process.IsProcessAlive(l.OwnerPID):
		return true, fmt.Sprintf("lease owner pid %d is gone", l.OwnerPID)
	case l.OwnerPath != "":
		if _, err := os.Stat(l.OwnerPath); os.IsNotExist(err) {
			return true, fmt.Sprintf("lease owner dir %s no longer exists", l.OwnerPath)
		}
	}
	return false, ""
}

// WriteFixtureLease drops a lease file into a fixture namespace directory.
// Failure is returned but is never worth aborting a test run over: without a
// lease the namespace is still swept, just on the slower age threshold.
func WriteFixtureLease(namespaceDir string, lease FixtureLease) error {
	if lease.CreatedAt.IsZero() {
		lease.CreatedAt = time.Now().UTC()
	}
	data, err := json.Marshal(lease)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(namespaceDir, FixtureLeaseName), append(data, '\n'), 0o600)
}

// ReadFixtureLease loads the lease from a namespace directory. A missing or
// unreadable lease yields the zero value and ok=false.
func ReadFixtureLease(namespaceDir string) (FixtureLease, bool) {
	data, err := os.ReadFile(filepath.Join(namespaceDir, FixtureLeaseName)) //nolint:gosec // G304: caller-supplied namespace, gated by FixtureNamespace
	if err != nil {
		return FixtureLease{}, false
	}
	var lease FixtureLease
	if err := json.Unmarshal(data, &lease); err != nil {
		return FixtureLease{}, false
	}
	return lease, true
}

// FixtureDaemon is one running daemon whose socket lives inside a fixture
// namespace — i.e. one that belongs to a test run rather than to the user.
type FixtureDaemon struct {
	// Kind is "groved" or "tuimux", derived from the socket the process was
	// asked to serve, not from its executable name.
	Kind string
	PID  int
	// SockPath is the --socket argument the daemon was started with. It is
	// both the identity evidence and the thing that made this daemon a
	// fixture: FixtureNamespace(SockPath) != "".
	SockPath string
	// Namespace is the fixture namespace that vouches for SockPath.
	Namespace string
	// Elapsed is how long the process has been running (0 = unknown).
	Elapsed time.Duration
	// SocketLive reports whether the socket file still exists. A running
	// daemon with a vanished socket serves nobody and is pure waste — the
	// signature of both leaked tuipilot fixtures found on 2026-08-07.
	SocketLive bool
	// Lease is the namespace's lease, when it has one.
	Lease    FixtureLease
	HasLease bool
	// Cmdline is the full argv, so a human reviewing a census can see exactly
	// what is being proposed for the chop.
	Cmdline string
}

// Orphaned reports whether this daemon's owning run is provably gone.
func (f FixtureDaemon) Orphaned() (bool, string) {
	if f.HasLease {
		return f.Lease.Abandoned()
	}
	return false, ""
}

// FindFixtureDaemons enumerates every running groved or tuimux daemon whose
// socket lives inside a fixture namespace.
//
// The census reads `ps` and matches on the --socket ARGUMENT, never on the
// process name: "a process called groved" is not evidence of anything, while
// "a process serving /tmp/tend-…/grove/groved.sock" is proof of both what it
// is and whose it is. A process without a --socket argument is skipped
// entirely — the real unscoped groved takes its socket from paths.SocketPath()
// and passes no flag, so it can never be matched here even by accident.
func FindFixtureDaemons() ([]FixtureDaemon, error) {
	procs, err := process.List()
	if err != nil {
		return nil, err
	}

	var found []FixtureDaemon
	for _, p := range procs {
		kind, sock := fixtureSocketArg(p.Args)
		if kind == "" {
			continue
		}
		ns := FixtureNamespace(sock)
		if ns == "" {
			continue
		}

		lease, hasLease := ReadFixtureLease(ns)
		_, sockErr := os.Stat(sock)
		found = append(found, FixtureDaemon{
			Kind:       kind,
			PID:        p.PID,
			SockPath:   sock,
			Namespace:  ns,
			Elapsed:    p.Elapsed,
			SocketLive: sockErr == nil,
			Lease:      lease,
			HasLease:   hasLease,
			Cmdline:    p.Args,
		})
	}

	sort.Slice(found, func(i, j int) bool {
		if found[i].Namespace != found[j].Namespace {
			return found[i].Namespace < found[j].Namespace
		}
		return found[i].PID < found[j].PID
	})
	return found, nil
}

// fixtureSocketArg extracts (kind, socketPath) from a command line that
// belongs to a daemon this package understands, or ("", "") otherwise.
//
// It requires BOTH the daemon subcommand ("groved start", "tuimux daemon")
// and an explicit --socket, so a client invocation that merely mentions a
// socket path ("tuimux daemon stop --socket …", "groved status") is never
// mistaken for the daemon itself.
func fixtureSocketArg(cmdline string) (kind, sock string) {
	fields := strings.Fields(cmdline)
	if len(fields) < 2 {
		return "", ""
	}

	exe := filepath.Base(fields[0])
	switch {
	case exe == "groved" && hasWord(fields[1:], "start"):
		kind = "groved"
	case exe == "tuimux" && fields[1] == "daemon":
		// "tuimux daemon" runs the daemon; "tuimux daemon stop|status|restart"
		// are clients that also take --socket.
		if len(fields) > 2 && !strings.HasPrefix(fields[2], "-") {
			return "", ""
		}
		kind = "tuimux"
	default:
		return "", ""
	}

	for i, f := range fields {
		if f == "--socket" && i+1 < len(fields) {
			return kind, fields[i+1]
		}
		if v, ok := strings.CutPrefix(f, "--socket="); ok {
			return kind, v
		}
	}
	return "", ""
}

// hasWord reports whether fields contains an exact token.
func hasWord(fields []string, want string) bool {
	for _, f := range fields {
		if f == want {
			return true
		}
	}
	return false
}

// SweepOptions tunes which orphaned fixture daemons a sweep is willing to kill.
type SweepOptions struct {
	// MinAge is how long a daemon with no abandoned lease must have been
	// running before the sweep will touch it. A daemon whose lease proves its
	// owner is gone bypasses this entirely — the proof is better than the
	// heuristic. Zero means DefaultSweepMinAge.
	MinAge time.Duration
	// IgnoreAge sweeps every identified fixture regardless of age or lease.
	// Still bounded by the namespace identity gate: this makes the sweep
	// eager, never less careful about WHAT it is willing to kill.
	IgnoreAge bool
	// Namespace narrows the sweep to fixtures whose namespace is exactly this
	// path; empty considers every fixture namespace. It answers a different
	// question from the options above — not "how sure must I be?" but "whose
	// daemons am I entitled to collect?".
	//
	// A harness reaping its OWN sandbox at teardown must not reach into a
	// concurrent run's, and a test exercising the kill path must not collect
	// the developer's live fixtures as a side effect. Both set this; a
	// machine-wide sweep (`tend clean`) leaves it empty.
	Namespace string
	// DryRun reports what would be killed without signalling anything.
	DryRun bool
}

// DefaultSweepMinAge is the age at which a lease-less fixture daemon is
// presumed abandoned. It is generous on purpose: a lease-less fixture is one
// this code cannot prove anything about, and a scenario that legitimately runs
// for twenty minutes must not have its daemon pulled out from under it. Runs
// that DO write a lease are collected the moment their owner dies, so the
// threshold only governs the residue of older binaries and hand-rolled
// sandboxes.
const DefaultSweepMinAge = 30 * time.Minute

// SweepDecision records what the sweep concluded about one fixture daemon.
type SweepDecision struct {
	Daemon FixtureDaemon
	// Swept is true when the daemon was signalled (or would have been, under
	// DryRun).
	Swept bool
	// Reason explains the decision either way, in terms a human can audit.
	Reason string
	// Err is set when the signal itself failed.
	Err error
}

// SweepFixtureDaemons finds orphaned fixture daemons and SIGTERMs them.
//
// Returns a decision for EVERY fixture daemon found, swept or not, so a caller
// can report "left 3 alone because their runs are live" as readily as what it
// killed. A sweep that silently does nothing is indistinguishable from a sweep
// that silently does the wrong thing.
//
// SIGTERM, never SIGKILL: both daemons handle it as a graceful shutdown that
// releases pidfiles, closes PTYs and unlinks sockets. A SIGKILLed daemon
// leaves exactly the residue this function exists to clean up.
func SweepFixtureDaemons(opts SweepOptions) ([]SweepDecision, error) {
	daemons, err := FindFixtureDaemons()
	if err != nil {
		return nil, err
	}

	minAge := opts.MinAge
	if minAge <= 0 {
		minAge = DefaultSweepMinAge
	}
	self := os.Getpid()

	decisions := make([]SweepDecision, 0, len(daemons))
	for _, d := range daemons {
		// Out-of-scope fixtures produce no decision at all rather than a
		// "kept" one: the caller asked about one namespace, and reporting on
		// somebody else's daemons would invite acting on them.
		if opts.Namespace != "" && !sameNamespace(opts.Namespace, d.Namespace) {
			continue
		}
		decision := SweepDecision{Daemon: d}

		orphaned, why := d.Orphaned()
		switch {
		case d.PID == self:
			// Defensive: a sweeper is never its own fixture, but the identity
			// gate is argv-based and this costs one comparison.
			decision.Reason = "skipped: that is us"
		case opts.IgnoreAge:
			decision.Swept, decision.Reason = true, "swept: --all requested"
		case orphaned:
			decision.Swept, decision.Reason = true, "swept: "+why
		case d.HasLease:
			decision.Reason = "kept: lease owner is still alive"
		case d.Elapsed == 0:
			// process.List could not parse an age. Unknown is not "old".
			decision.Reason = "kept: age unknown and no lease to prove abandonment"
		case d.Elapsed < minAge:
			decision.Reason = fmt.Sprintf("kept: only %s old (threshold %s), no lease", d.Elapsed.Round(time.Second), minAge)
		default:
			decision.Swept, decision.Reason = true, fmt.Sprintf("swept: %s old with no owning run (threshold %s)", d.Elapsed.Round(time.Second), minAge)
		}

		if decision.Swept && !opts.DryRun {
			if err := signalTerm(d.PID); err != nil {
				decision.Err = err
			}
		}
		decisions = append(decisions, decision)
	}
	return decisions, nil
}

// signalTerm sends SIGTERM to a PID, treating an already-dead process as
// success: the sweep's goal is "this daemon is not running", and it already
// is not.
func signalTerm(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

// FormatFixtureDaemons renders a census as an aligned table, or a single line
// when there is nothing to report.
func FormatFixtureDaemons(daemons []FixtureDaemon) string {
	if len(daemons) == 0 {
		return "No fixture daemons running\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%-8s  %-8s  %-10s  %-8s  %s\n", "PID", "KIND", "AGE", "SOCKET", "NAMESPACE")
	for _, d := range daemons {
		age := "?"
		if d.Elapsed > 0 {
			age = d.Elapsed.Round(time.Second).String()
		}
		sockState := "live"
		if !d.SocketLive {
			sockState = "dead"
		}
		fmt.Fprintf(&b, "%-8d  %-8s  %-10s  %-8s  %s\n", d.PID, d.Kind, age, sockState, d.Namespace)
	}
	return b.String()
}
