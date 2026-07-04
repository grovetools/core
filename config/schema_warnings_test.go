package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

// newWarnCaptureLogger returns a logger whose output lands in the returned
// buffer. A *bytes.Buffer is not an *os.File, so writerIsInteractive treats
// it as safe and the fallback path emits into it.
func newWarnCaptureLogger() (*logrus.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&buf)
	return logger, &buf
}

// TestReportSchemaWarningDedupes locks in the per-process (source, error)
// dedupe: the same warning repeated across fragments/loads must emit once,
// while a different source or error still gets through.
func TestReportSchemaWarningDedupes(t *testing.T) {
	resetSchemaWarningsForTest()
	t.Cleanup(resetSchemaWarningsForTest)

	logger, buf := newWarnCaptureLogger()
	violation := errors.New("tui: additional properties not allowed")

	reportSchemaWarning(logger, "config TOML bytes", violation)
	reportSchemaWarning(logger, "config TOML bytes", violation)
	if got := strings.Count(buf.String(), "does not fully conform"); got != 1 {
		t.Fatalf("identical warning emitted %d times, want 1:\n%s", got, buf.String())
	}

	reportSchemaWarning(logger, "merged config", violation)
	reportSchemaWarning(logger, "config TOML bytes", errors.New("other violation"))
	if got := strings.Count(buf.String(), "does not fully conform"); got != 3 {
		t.Fatalf("distinct warnings emitted %d times, want 3:\n%s", got, buf.String())
	}
}

// TestReportSchemaWarningInteractiveGate covers the stderr-gating decision:
// a logger writing to an interactive terminal must stay silent (buffering
// for a future sink instead), and GROVE_DEBUG=1 overrides the suppression —
// mirroring logging's StructuredToStderr "auto" behavior.
func TestReportSchemaWarningInteractiveGate(t *testing.T) {
	resetSchemaWarningsForTest()
	t.Cleanup(resetSchemaWarningsForTest)

	origIsTerminal := isTerminalFd
	isTerminalFd = func(uintptr) bool { return true }
	t.Cleanup(func() { isTerminalFd = origIsTerminal })

	// An *os.File output with the TTY probe forced true simulates the
	// pre-redirect stderr of a TUI process.
	tty, err := os.Create(filepath.Join(t.TempDir(), "fake-tty"))
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	t.Cleanup(func() { tty.Close() })
	logger := logrus.New()
	logger.SetOutput(tty)

	readOutput := func() string {
		data, rerr := os.ReadFile(tty.Name())
		if rerr != nil {
			t.Fatalf("reading output: %v", rerr)
		}
		return string(data)
	}

	violation := errors.New("interactive-gate violation")
	reportSchemaWarning(logger, "config bytes", violation)

	if data := readOutput(); strings.Contains(data, "does not fully conform") {
		t.Fatalf("warning must not reach an interactive terminal, got:\n%s", data)
	}

	// The suppressed warning must still be observable: registering a sink
	// flushes it.
	var flushed []string
	SetSchemaWarningSink(func(source string, err error) {
		flushed = append(flushed, source+": "+err.Error())
	})
	if len(flushed) != 1 || !strings.Contains(flushed[0], "interactive-gate violation") {
		t.Fatalf("expected the suppressed warning to flush to the sink, got %v", flushed)
	}

	// GROVE_DEBUG=1 keeps the raw emission for debugging sessions.
	resetSchemaWarningsForTest()
	t.Setenv("GROVE_DEBUG", "1")
	reportSchemaWarning(logger, "config bytes", violation)
	if data := readOutput(); !strings.Contains(data, "does not fully conform") {
		t.Fatalf("GROVE_DEBUG=1 must bypass the interactive gate, got:\n%s", data)
	}
}

// TestSchemaWarningSinkTakesOver verifies that once a sink is registered,
// warnings go to it exclusively — the fallback logger sees nothing.
func TestSchemaWarningSinkTakesOver(t *testing.T) {
	resetSchemaWarningsForTest()
	t.Cleanup(resetSchemaWarningsForTest)

	var sunk []string
	SetSchemaWarningSink(func(source string, err error) {
		sunk = append(sunk, source)
	})

	logger, buf := newWarnCaptureLogger()
	reportSchemaWarning(logger, "layered config", errors.New("sink-takeover violation"))

	if len(sunk) != 1 || sunk[0] != "layered config" {
		t.Fatalf("expected sink to receive the warning, got %v", sunk)
	}
	if buf.Len() != 0 {
		t.Fatalf("fallback logger must stay silent once a sink exists, got:\n%s", buf.String())
	}
}
