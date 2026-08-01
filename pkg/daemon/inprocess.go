package daemon

import "sync"

// ---------------------------------------------------------------------------
// In-process daemon marker
// ---------------------------------------------------------------------------
//
// groved runs a large amount of borrowed code in-process: flow's orchestration
// package (the JobRunner executes jobs by calling straight into it), the
// adoption poller, collectors, channel handlers. All of that code was written
// for CLIs and reaches for daemon.NewWithAutoStart, whose contract is "give me
// a daemon, starting one if needed".
//
// Inside a daemon that contract is wrong. A groved that spawns a sibling groved
// for some other scope creates a daemon with no clients, no UI, and no work —
// it just idles until --auto-shutdown reaps it 2 minutes later, and reappears
// on the next poll. During a fleet restart, when nothing is registered as a
// host yet, that turns one restart into one surplus daemon per active worktree.
//
// The marker below makes the rule structural rather than a call-site
// convention: a process that has declared itself a daemon never spawns a
// SCOPED daemon. It is deliberately not an Option — options are per-call and
// this is a property of the process. See newAutoStart for the enforcement and
// for the one spawn that stays allowed (a scoped daemon starting the global
// daemon, which owns the shared proxy and route table).

var (
	inProcessMu     sync.RWMutex
	inProcessSocket string
	inProcessSet    bool
)

// MarkInProcessDaemon declares that the calling process IS a groved serving
// socketPath. It changes the auto-start factories' spawn policy for the rest of
// the process's life:
//
//   - a scoped daemon is never spawned (see newAutoStart); the caller gets this
//     daemon's own client instead, or a LocalClient when even that cannot be
//     dialed yet;
//   - this daemon's own socket is never spawned either — a daemon can only be
//     a duplicate of itself;
//   - the GLOBAL daemon may still be auto-started by a scoped daemon, which is
//     how proxy-route registration and the env manager reach it.
//
// groved calls this once at startup, before any boot step can run borrowed
// code. Everything else — CLIs, TUIs, hooks, agents — must not call it.
func MarkInProcessDaemon(socketPath string) {
	inProcessMu.Lock()
	defer inProcessMu.Unlock()
	inProcessSocket = socketPath
	inProcessSet = true
}

// inProcessDaemon reports whether this process is a groved, and which socket it
// serves. The socket may be empty if the caller did not know it yet; the
// no-spawn rule still applies.
func inProcessDaemon() (socketPath string, isDaemon bool) {
	inProcessMu.RLock()
	defer inProcessMu.RUnlock()
	return inProcessSocket, inProcessSet
}

// inDaemonSpawnSuppressed decides whether a spawn for (scope, socketPath) must
// be refused because this process is itself a daemon, and returns the socket to
// fall back to when it is.
//
// Refused:
//   - any SCOPED daemon (scope != ""). A groved that forks a sibling for
//     another worktree creates a daemon with no clients, no UI and no work; it
//     idles until --auto-shutdown reaps it and reappears on the next poll.
//   - our OWN socket. A daemon can only ever be a duplicate of itself, and
//     during the boot window before Listen the socket is legitimately
//     undialable.
//
// Allowed — the one carve-out: a scoped daemon auto-starting the GLOBAL daemon
// (scope == "" and not our own socket). The global daemon owns the shared proxy
// and the host-wide route table, and scoped daemons legitimately start it
// (NewGlobalClient from the env manager and the channel manager). That spawn
// creates the machine's one singleton, never a per-worktree straggler.
//
// Deliberately not overridable by RequireScopedDaemon/EarlyReady/PairWith:
// those exist for UI hosts booting the daemon they are about to pair with,
// which a daemon never does.
func inDaemonSpawnSuppressed(scope, socketPath string) (selfSocket string, suppressed bool) {
	selfSocket, isDaemon := inProcessDaemon()
	if !isDaemon {
		return "", false
	}
	return selfSocket, scope != "" || socketPath == selfSocket
}

// resetInProcessDaemon clears the marker. Test-only.
func resetInProcessDaemon() {
	inProcessMu.Lock()
	defer inProcessMu.Unlock()
	inProcessSocket = ""
	inProcessSet = false
}
