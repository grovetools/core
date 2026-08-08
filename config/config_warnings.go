package config

import (
	"io"
	"os"
	"sync"

	"github.com/mattn/go-isatty"
	"github.com/sirupsen/logrus"
)

// Config-load warnings fire during early config loads — often at package-init
// time in TUI binaries, before any logging setup — so they must not go
// straight to a default logrus stderr logger: raw bytes on an interactive
// stderr corrupt full-screen (bubbletea alt-screen) TUIs, and a bare
// logrus.New() carries neither grove's formatter nor its file sink, so the
// line lands on the console and NOWHERE else. config also cannot import
// core/logging (logging imports config, and NewLogger loads config while
// holding its own lock), so the pipeline is injected instead: warnings are
// deduped per process, buffered until core/logging registers a sink (its
// first NewLogger call), and mirrored to the fallback logrus logger only when
// its destination cannot be an interactive terminal — the same TTY test
// logging's StructuredToStderr "auto" mode applies.
//
// Every warning on this channel reports a DEFECT in the operator's own config
// (a schema violation), which is why the console fallback is unconditional. A
// standing condition — "this dead directory still exists" — must NOT be
// reported here: it is true on every load of every binary forever, so even
// deduped per process it accumulates one log line per grove invocation. Those
// belong to `grove doctor` and the command that owns the subject (see
// LegacyMachinesDir).

// Warning is one deferred config-load warning as the sink sees it. Only the
// exported fields are meaningful outside this package; construction stays
// internal so every warning's dedupe key and console policy are decided next
// to the condition that raises it.
type Warning struct {
	// Component is the logging component the warning is attributed to, e.g.
	// "config" or "config.machine".
	Component string
	// Message is the human-readable warning text.
	Message string
	// Fields carries structured context (paths, sources) for the sink.
	Fields map[string]any
	// Err is the underlying error, when the warning has one.
	Err error

	// dedupeKey collapses repeats within one process. Config is loaded many
	// times per run (per-directory, per-layer, cache misses), so without it a
	// single condition warns on every load.
	dedupeKey string
}

// warnBufferCap bounds the pre-sink buffer. Dedupe keeps the set of distinct
// warnings tiny in practice; the cap only guards runaway drift.
const warnBufferCap = 64

var (
	warnMu     sync.Mutex
	warnSeen   = map[string]struct{}{}
	warnBuffer []Warning
	warnSink   func(Warning)
)

// isTerminalFd is swappable so tests can exercise the interactive gate
// without a real TTY.
var isTerminalFd = func(fd uintptr) bool {
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// SetWarningSink installs the process-wide destination for config-load
// warnings and flushes any warnings buffered before logging was ready.
// core/logging registers a component-logger sink on first logger
// construction. fn runs inside arbitrary config.Load* callers, so it must
// not call back into config.Load* or logging.NewLogger.
func SetWarningSink(fn func(Warning)) {
	warnMu.Lock()
	warnSink = fn
	buffered := warnBuffer
	warnBuffer = nil
	warnMu.Unlock()
	if fn == nil {
		return
	}
	for _, w := range buffered {
		fn(w)
	}
}

// reportWarning emits one warning per dedupe key per process: to the
// registered sink when logging is up, otherwise buffered for the future sink
// and mirrored to logger when that cannot corrupt an interactive terminal.
func reportWarning(logger *logrus.Logger, w Warning) {
	warnMu.Lock()
	if _, dup := warnSeen[w.dedupeKey]; dup {
		warnMu.Unlock()
		return
	}
	warnSeen[w.dedupeKey] = struct{}{}
	sink := warnSink
	if sink == nil && len(warnBuffer) < warnBufferCap {
		warnBuffer = append(warnBuffer, w)
	}
	warnMu.Unlock()

	if sink != nil {
		sink(w)
		return
	}
	if logger == nil {
		return
	}
	if writerIsInteractive(logger.Out) && os.Getenv("GROVE_DEBUG") != "1" {
		return
	}
	entry := logger.WithFields(logrus.Fields(w.Fields))
	if w.Err != nil {
		entry = entry.WithError(w.Err)
	}
	entry.Warn(w.Message)
}

// schemaWarningMessage is the single text every schema violation reports
// under; the specifics live in the source field and the attached error.
const schemaWarningMessage = "configuration does not fully conform to the schema (continuing; validation is advisory)"

// reportSchemaWarning reports a schema violation for one (source, error)
// pair. Schema warnings describe a defect in the user's own config, so they
// keep the console fallback: a piped or redirected run should still say the
// config is off-schema even when no logging pipeline ever comes up.
func reportSchemaWarning(logger *logrus.Logger, source string, err error) {
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	reportWarning(logger, Warning{
		Component: "config",
		Message:   schemaWarningMessage,
		Fields:    map[string]any{"source": source},
		Err:       err,
		dedupeKey: "schema\x00" + source + "\x00" + errText,
	})
}

// writerIsInteractive reports whether w is a terminal. Non-file writers
// (buffers, pipes wrapped by callers) are never terminals and always safe.
func writerIsInteractive(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return isTerminalFd(f.Fd())
}

// resetWarningsForTest clears dedupe, buffer, and sink state so tests can
// observe fresh emissions; production code has no reason to touch it.
func resetWarningsForTest() {
	warnMu.Lock()
	warnSeen = map[string]struct{}{}
	warnBuffer = nil
	warnSink = nil
	warnMu.Unlock()
}
