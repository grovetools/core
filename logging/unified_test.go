package logging

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/mattsolo1/grove-core/tui/theme"
	"github.com/sirupsen/logrus"
)

func TestNewUnifiedLogger(t *testing.T) {
	ulog := NewUnifiedLogger("test-component")

	if ulog.Component() != "test-component" {
		t.Errorf("expected component 'test-component', got '%s'", ulog.Component())
	}

	if ulog.pretty == nil {
		t.Error("expected pretty logger to be initialized")
	}

	if ulog.structured == nil {
		t.Error("expected structured logger to be initialized")
	}
}

func TestLogEntryField(t *testing.T) {
	ulog := NewUnifiedLogger("test")
	entry := ulog.Info("test message").
		Field("key1", "value1").
		Field("key2", 42)

	if entry.fields["key1"] != "value1" {
		t.Errorf("expected field 'key1' to be 'value1', got '%v'", entry.fields["key1"])
	}

	if entry.fields["key2"] != 42 {
		t.Errorf("expected field 'key2' to be 42, got '%v'", entry.fields["key2"])
	}
}

func TestLogEntryFields(t *testing.T) {
	ulog := NewUnifiedLogger("test")
	entry := ulog.Info("test message").
		Fields(map[string]interface{}{
			"key1": "value1",
			"key2": 42,
		})

	if entry.fields["key1"] != "value1" {
		t.Errorf("expected field 'key1' to be 'value1', got '%v'", entry.fields["key1"])
	}

	if entry.fields["key2"] != 42 {
		t.Errorf("expected field 'key2' to be 42, got '%v'", entry.fields["key2"])
	}
}

func TestLogEntryErr(t *testing.T) {
	ulog := NewUnifiedLogger("test")
	testErr := &testError{msg: "test error"}
	entry := ulog.Error("operation failed").Err(testErr)

	if entry.err != testErr {
		t.Error("expected error to be set")
	}

	if entry.fields["error"] != "test error" {
		t.Errorf("expected error field to be 'test error', got '%v'", entry.fields["error"])
	}
}

func TestLogEntryErrNil(t *testing.T) {
	ulog := NewUnifiedLogger("test")
	entry := ulog.Error("operation failed").Err(nil)

	if entry.err != nil {
		t.Error("expected error to be nil when nil is passed")
	}

	if _, exists := entry.fields["error"]; exists {
		t.Error("expected error field not to be set when nil is passed")
	}
}

func TestLogEntryIcon(t *testing.T) {
	ulog := NewUnifiedLogger("test")
	entry := ulog.Info("custom icon").Icon(theme.IconSuccess)

	if entry.icon != theme.IconSuccess {
		t.Errorf("expected icon to be '%s', got '%s'", theme.IconSuccess, entry.icon)
	}
}

func TestLogEntryNoIcon(t *testing.T) {
	ulog := NewUnifiedLogger("test")
	entry := ulog.Warn("no icon").NoIcon()

	if !entry.noIcon {
		t.Error("expected noIcon to be true")
	}
}

func TestLogEntryPretty(t *testing.T) {
	ulog := NewUnifiedLogger("test")
	styledMsg := "styled message"
	entry := ulog.Info("plain message").Pretty(styledMsg)

	if entry.prettyMsg != styledMsg {
		t.Errorf("expected prettyMsg to be '%s', got '%s'", styledMsg, entry.prettyMsg)
	}
}

func TestLogEntryPrettyOnly(t *testing.T) {
	ulog := NewUnifiedLogger("test")
	entry := ulog.Info("pretty only").PrettyOnly()

	if !entry.prettyOnly {
		t.Error("expected prettyOnly to be true")
	}
}

func TestLogEntryStructuredOnly(t *testing.T) {
	ulog := NewUnifiedLogger("test")
	entry := ulog.Info("structured only").StructuredOnly()

	if !entry.structOnly {
		t.Error("expected structOnly to be true")
	}
}

func TestLevelMethods(t *testing.T) {
	ulog := NewUnifiedLogger("test")

	tests := []struct {
		name          string
		entry         *LogEntry
		expectedLevel logrus.Level
		expectedIcon  string
	}{
		{
			name:          "Debug",
			entry:         ulog.Debug("debug message"),
			expectedLevel: logrus.DebugLevel,
			expectedIcon:  "",
		},
		{
			name:          "Info",
			entry:         ulog.Info("info message"),
			expectedLevel: logrus.InfoLevel,
			expectedIcon:  "",
		},
		{
			name:          "Warn",
			entry:         ulog.Warn("warn message"),
			expectedLevel: logrus.WarnLevel,
			expectedIcon:  theme.IconWarning,
		},
		{
			name:          "Error",
			entry:         ulog.Error("error message"),
			expectedLevel: logrus.ErrorLevel,
			expectedIcon:  theme.IconError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.entry.level != tt.expectedLevel {
				t.Errorf("expected level %v, got %v", tt.expectedLevel, tt.entry.level)
			}
			if tt.entry.icon != tt.expectedIcon {
				t.Errorf("expected icon '%s', got '%s'", tt.expectedIcon, tt.entry.icon)
			}
		})
	}
}

func TestSemanticMethods(t *testing.T) {
	ulog := NewUnifiedLogger("test")

	tests := []struct {
		name           string
		entry          *LogEntry
		expectedIcon   string
		expectedStatus string
	}{
		{
			name:           "Success",
			entry:          ulog.Success("success message"),
			expectedIcon:   theme.IconSuccess,
			expectedStatus: "success",
		},
		{
			name:           "Progress",
			entry:          ulog.Progress("progress message"),
			expectedIcon:   theme.IconRunning,
			expectedStatus: "progress",
		},
		{
			name:           "Status",
			entry:          ulog.Status("status message"),
			expectedIcon:   theme.IconInfo,
			expectedStatus: "info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.entry.icon != tt.expectedIcon {
				t.Errorf("expected icon '%s', got '%s'", tt.expectedIcon, tt.entry.icon)
			}
			if tt.entry.fields["status"] != tt.expectedStatus {
				t.Errorf("expected status '%s', got '%v'", tt.expectedStatus, tt.entry.fields["status"])
			}
			if tt.entry.level != logrus.InfoLevel {
				t.Errorf("expected INFO level, got %v", tt.entry.level)
			}
		})
	}
}

func TestLogPrettyOutput(t *testing.T) {
	var buf bytes.Buffer
	ctx := WithWriter(context.Background(), &buf)

	ulog := NewUnifiedLogger("test")
	ulog.Info("test message").Log(ctx)

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("expected output to contain 'test message', got '%s'", output)
	}
}

func TestLogPrettyWithIcon(t *testing.T) {
	var buf bytes.Buffer
	ctx := WithWriter(context.Background(), &buf)

	ulog := NewUnifiedLogger("test")
	ulog.Success("completed").Log(ctx)

	output := buf.String()
	if !strings.Contains(output, "completed") {
		t.Errorf("expected output to contain 'completed', got '%s'", output)
	}
	// Icon should be present (checking for the success icon)
	if !strings.Contains(output, theme.IconSuccess) {
		t.Errorf("expected output to contain success icon, got '%s'", output)
	}
}

func TestLogPrettyNoIcon(t *testing.T) {
	var buf bytes.Buffer
	ctx := WithWriter(context.Background(), &buf)

	ulog := NewUnifiedLogger("test")
	ulog.Warn("warning").NoIcon().Log(ctx)

	output := buf.String()
	if !strings.Contains(output, "warning") {
		t.Errorf("expected output to contain 'warning', got '%s'", output)
	}
	// Icon should NOT be present
	if strings.Contains(output, theme.IconWarning) {
		t.Errorf("expected output to NOT contain warning icon when NoIcon() is set, got '%s'", output)
	}
}

func TestLogPrettyCustomMessage(t *testing.T) {
	var buf bytes.Buffer
	ctx := WithWriter(context.Background(), &buf)

	ulog := NewUnifiedLogger("test")
	ulog.Info("plain").Pretty("custom styled message").Log(ctx)

	output := buf.String()
	if !strings.Contains(output, "custom styled message") {
		t.Errorf("expected output to contain 'custom styled message', got '%s'", output)
	}
	// Should NOT contain the plain message
	if strings.Contains(output, "plain") && !strings.Contains(output, "custom styled message") {
		t.Errorf("expected output to use custom message instead of plain, got '%s'", output)
	}
}

func TestLogStructuredOnly(t *testing.T) {
	var buf bytes.Buffer
	ctx := WithWriter(context.Background(), &buf)

	ulog := NewUnifiedLogger("test")
	ulog.Info("structured only").StructuredOnly().Log(ctx)

	output := buf.String()
	if output != "" {
		t.Errorf("expected no pretty output when StructuredOnly() is set, got '%s'", output)
	}
}

func TestLogPrettyOnly(t *testing.T) {
	var buf bytes.Buffer
	ctx := WithWriter(context.Background(), &buf)

	ulog := NewUnifiedLogger("test")
	// PrettyOnly should still produce pretty output
	ulog.Info("pretty only").PrettyOnly().Log(ctx)

	output := buf.String()
	if !strings.Contains(output, "pretty only") {
		t.Errorf("expected pretty output when PrettyOnly() is set, got '%s'", output)
	}
}

func TestChainedCalls(t *testing.T) {
	var buf bytes.Buffer
	ctx := WithWriter(context.Background(), &buf)

	ulog := NewUnifiedLogger("test")
	ulog.Info("chained").
		Field("count", 5).
		Field("paths", []string{"/a", "/b"}).
		Icon(theme.IconSuccess).
		Log(ctx)

	output := buf.String()
	if !strings.Contains(output, "chained") {
		t.Errorf("expected output to contain message, got '%s'", output)
	}
}

func TestWithStructured(t *testing.T) {
	ulog := NewUnifiedLogger("test")
	entry := ulog.WithStructured()

	if entry == nil {
		t.Error("expected WithStructured to return a non-nil entry")
	}
}

func TestWithPretty(t *testing.T) {
	ulog := NewUnifiedLogger("test")
	pretty := ulog.WithPretty()

	if pretty == nil {
		t.Error("expected WithPretty to return a non-nil logger")
	}
}

// testError is a simple error implementation for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
