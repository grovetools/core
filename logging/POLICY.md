# Grove logging policy

Rules for what gets logged, at what level, and where it goes. Referenced from
the `core::concept-unified-logging` concept.

## Level semantics

- **debug** — per-iteration/per-item tracing: loop bodies, cache hits, no-op
  detections, "function called" breadcrumbs. Never emit per watched-path,
  per-node, or per-render lines at any higher level. Debug may be voluminous
  *per event burst* but must not be voluminous *at steady state*: anything a
  timer re-emits every tick must be state-change-gated or absent.
- **info** — one line per user-meaningful state transition: job
  created/launched/finished, plan finished, note mutated, daemon/channel/
  watcher started or stopped, a sync *that changed something*. An info line
  answers "what did the system do on my behalf?" — never "what is the system
  checking right now?". One transition = one line, at exactly one funnel point.
- **warn** — degraded but self-healing. A warn that fires on a timer forever
  is a bug in the emitter: repeated identical warns must be deduplicated,
  rate-limited, or escalated to a single error by a circuit breaker.
- **error** — failed and will not self-heal without user action. Always
  visible everywhere.

## Sink policy

- **File sink** — the audit trail. Receives every level the file-sink level
  (`logging.file.level`, defaulting to the console level) permits. Component
  visibility filtering stays *display-only*; the file never has write-time
  component blind spots.
- **Console / pretty** — human feedback for the command being run, gated at
  the console level (default info). Pure CLI feedback uses `.PrettyOnly()`;
  pure audit records use `.StructuredOnly()` (see `unified.go`).
- **User-visible default view** (treemux Logs panel, `core logs`) — the
  canonical default-event set below, plus all warn/error.

## Canonical default events

Structured log lines carrying an `event` field named `<domain>.<verb>`, at
info level, emitted at exactly one funnel point each:

| event | meaning |
|---|---|
| `job.created` | a flow job was added to a plan |
| `job.launched` | a job transitioned to running |
| `job.finished` | a job completed/failed/cancelled (with `status` field) |
| `plan.finished` | a plan was marked finished |
| `note.created` / `note.updated` / `note.deleted` / `note.moved` / `note.archived` | notebook mutations |
| `daemon.started` / `daemon.stopped` | groved lifecycle |
| `channel.up` / `channel.down` / `channel.disabled` | notification channel lifecycle |

`event` is machine-filterable; `component` remains the emitting subsystem.
Reuse the existing mechanisms before inventing new ones: the
`Success`/`Progress`/`Status` semantic constructors keep styling, `_verbosity`
field tags keep detail-folding, and daemon StateUpdate classification remains
the live "Daemon" scope in the logs TUI.
