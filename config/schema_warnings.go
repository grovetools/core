package config

import (
	"io"
	"os"
	"sync"

	"github.com/mattn/go-isatty"
	"github.com/sirupsen/logrus"
)

// Schema warnings fire during early config loads — often at package-init time
// in TUI binaries, before any logging setup — so they must not go straight to
// a default logrus stderr logger: raw bytes on an interactive stderr corrupt
// full-screen (bubbletea alt-screen) TUIs. config also cannot import
// core/logging (logging imports config), so the pipeline is injected instead:
// warnings are deduped per process, buffered until core/logging registers a
// sink (its first NewLogger call), and mirrored to the fallback logger only
// when its destination cannot be an interactive terminal — the same TTY test
// logging's StructuredToStderr "auto" mode applies.

type schemaWarning struct {
	source string
	err    error
}

// schemaWarnBufferCap bounds the pre-sink buffer. Dedupe keeps the set of
// distinct warnings tiny in practice; the cap only guards runaway drift.
const schemaWarnBufferCap = 64

var (
	schemaWarnMu     sync.Mutex
	schemaWarnSeen   = map[string]struct{}{}
	schemaWarnBuffer []schemaWarning
	schemaWarnSink   func(source string, err error)
)

// isTerminalFd is swappable so tests can exercise the interactive gate
// without a real TTY.
var isTerminalFd = func(fd uintptr) bool {
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// SetSchemaWarningSink installs the process-wide destination for schema
// warnings and flushes any warnings buffered before logging was ready.
// core/logging registers a component-logger sink on first logger
// construction. fn runs inside arbitrary config.Load* callers, so it must
// not call back into config.Load* or logging.NewLogger.
func SetSchemaWarningSink(fn func(source string, err error)) {
	schemaWarnMu.Lock()
	schemaWarnSink = fn
	buffered := schemaWarnBuffer
	schemaWarnBuffer = nil
	schemaWarnMu.Unlock()
	if fn == nil {
		return
	}
	for _, w := range buffered {
		fn(w.source, w.err)
	}
}

// reportSchemaWarning emits one (source, error) warning per process: to the
// registered sink when logging is up, otherwise buffered for the future sink
// and mirrored to logger only when that cannot corrupt an interactive
// terminal.
func reportSchemaWarning(logger *logrus.Logger, source string, err error) {
	key := source + "\x00" + err.Error()
	schemaWarnMu.Lock()
	if _, dup := schemaWarnSeen[key]; dup {
		schemaWarnMu.Unlock()
		return
	}
	schemaWarnSeen[key] = struct{}{}
	sink := schemaWarnSink
	if sink == nil && len(schemaWarnBuffer) < schemaWarnBufferCap {
		schemaWarnBuffer = append(schemaWarnBuffer, schemaWarning{source: source, err: err})
	}
	schemaWarnMu.Unlock()

	if sink != nil {
		sink(source, err)
		return
	}
	if writerIsInteractive(logger.Out) && os.Getenv("GROVE_DEBUG") != "1" {
		return
	}
	logger.WithError(err).WithField("source", source).
		Warn("configuration does not fully conform to the schema (continuing; validation is advisory)")
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

// resetSchemaWarningsForTest clears dedupe, buffer, and sink state so tests
// can observe fresh emissions; production code has no reason to touch it.
func resetSchemaWarningsForTest() {
	schemaWarnMu.Lock()
	schemaWarnSeen = map[string]struct{}{}
	schemaWarnBuffer = nil
	schemaWarnSink = nil
	schemaWarnMu.Unlock()
}
