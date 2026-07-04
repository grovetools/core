package logging

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/grovetools/core/config"
)

// TestRegisterSchemaWarningSinkRoutesConfigWarnings verifies the seam that
// keeps config-load schema warnings off raw stderr: once the logging package
// registers its sink, a schema violation on the byte-load path (which
// historically logged via logrus.StandardLogger straight to stderr) is
// delivered through the configured logger instead, tagged with the config
// component and source.
func TestRegisterSchemaWarningSinkRoutesConfigWarnings(t *testing.T) {
	t.Cleanup(func() { config.SetSchemaWarningSink(nil) })

	var buf bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&buf)
	logger.SetFormatter(&logrus.JSONFormatter{})
	registerSchemaWarningSink(logger)

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
