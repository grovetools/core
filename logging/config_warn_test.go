package logging

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/grovetools/core/config"
)

// TestRegisterConfigWarningSinkRoutesConfigWarnings verifies the seam that
// keeps config-load schema warnings off raw stderr: once the logging package
// registers its sink, a schema violation on the byte-load path (which
// historically logged via logrus.StandardLogger straight to stderr) is
// delivered through the configured logger instead, tagged with the config
// component and source.
func TestRegisterConfigWarningSinkRoutesConfigWarnings(t *testing.T) {
	t.Cleanup(func() { config.SetWarningSink(nil) })

	var buf bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&buf)
	logger.SetFormatter(&logrus.JSONFormatter{})
	registerConfigWarningSink(logger)

	// An out-of-enum logging.level survives the struct round-trip (it rides
	// in Extensions and serializes inline) and violates the composed schema,
	// but must never fail the load.
	data := []byte("version = \"1.0\"\n[logging]\nlevel = \"zz-not-a-level\"\n")
	if _, err := config.LoadFromTOMLBytes(data); err != nil {
		t.Fatalf("schema violation must not fail LoadFromTOMLBytes: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "does not fully conform") {
		t.Fatalf("expected schema warning through the registered sink, got:\n%s", out)
	}
	if !strings.Contains(out, `"component":"config"`) {
		t.Fatalf("expected component=config on the sink entry, got:\n%s", out)
	}
	if !strings.Contains(out, "config TOML bytes") {
		t.Fatalf("expected the load-path source field, got:\n%s", out)
	}
}

// A warning carries its own component, so subsystem-specific config warnings
// (the legacy machines/ migration nag) land under their own component in the
// logs rather than being flattened into "config".
func TestConfigWarningEmitterHonorsPerWarningComponent(t *testing.T) {
	var buf bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&buf)
	logger.SetFormatter(&logrus.JSONFormatter{})

	configWarningEmitter(logger)(config.Warning{
		Component: "config.machine",
		Message:   "/cfg/machines is ignored; migrate with `grove machine migrate`",
		Fields:    map[string]any{"path": "/cfg/machines"},
	})

	out := buf.String()
	if !strings.Contains(out, `"component":"config.machine"`) {
		t.Fatalf("expected the warning's own component, got:\n%s", out)
	}
	if !strings.Contains(out, "grove machine migrate") {
		t.Fatalf("expected the migration message, got:\n%s", out)
	}
	if !strings.Contains(out, `"path":"/cfg/machines"`) {
		t.Fatalf("expected the path field, got:\n%s", out)
	}
}

// A StructuredOnly warning is a standing nag: the file sink (hooks) must
// record it, the console must not print it. This is the case that used to
// spray `WARN[0000] ... machines is ignored` over every run and every TUI.
func TestConfigWarningEmitterKeepsStructuredOnlyOffTheConsole(t *testing.T) {
	var buf bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&buf)
	// The console formatter installed in "auto" mode on a non-interactive
	// stderr — the only mode in which structured entries reach the console.
	logger.SetFormatter(&dualEmitSuppressingFormatter{inner: &logrus.JSONFormatter{}})
	hook := &captureHook{}
	logger.AddHook(hook)

	emit := configWarningEmitter(logger)
	emit(config.Warning{
		Component:      "config.machine",
		Message:        "/cfg/machines is ignored; migrate with `grove machine migrate`",
		StructuredOnly: true,
	})
	emit(config.Warning{
		Component: "config",
		Message:   "an ordinary config warning",
	})

	if out := buf.String(); strings.Contains(out, "grove machine migrate") {
		t.Fatalf("structured-only warning reached the console:\n%s", out)
	}
	if out := buf.String(); !strings.Contains(out, "an ordinary config warning") {
		t.Fatalf("ordinary warning must still reach the console, got:\n%s", out)
	}
	// captureHook stands in for the FileHook: it sees every entry regardless
	// of what the console formatter does with it.
	if len(hook.entries) != 2 {
		t.Fatalf("file sink saw %d entries, want both", len(hook.entries))
	}
	if !strings.Contains(hook.entries[0].Message, "grove machine migrate") {
		t.Fatalf("file sink missed the structured-only warning: %q", hook.entries[0].Message)
	}
}
