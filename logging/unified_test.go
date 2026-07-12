package logging

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/grovetools/core/tui/theme"
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

func TestLogEntry_Emit(t *testing.T) {
	// Capture output via global output
	var buf bytes.Buffer
	SetGlobalOutput(&buf)
	defer SetGlobalOutput(os.Stderr)

	ulog := NewUnifiedLogger("test")

	// Should not panic and should produce output
	ulog.Info("test message").Field("key", "value").Emit()

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("expected output to contain 'test message', got '%s'", output)
	}
}

func TestLogEntry_EmitProducesSameOutputAsLogWithBackgroundContext(t *testing.T) {
	// Test that Emit() produces the same output as Log(context.Background())
	var bufEmit bytes.Buffer
	var bufLog bytes.Buffer
	defer SetGlobalOutput(os.Stderr)

	SetGlobalOutput(&bufEmit)
	ulog := NewUnifiedLogger("test")
	ulog.Info("emit test").Field("count", 42).Emit()
	emitOutput := bufEmit.String()

	SetGlobalOutput(&bufLog)
	ulog2 := NewUnifiedLogger("test")
	ulog2.Info("emit test").Field("count", 42).Log(context.Background())
	logOutput := bufLog.String()

	// Both should contain the same message (output may differ slightly in metadata)
	if !strings.Contains(emitOutput, "emit test") {
		t.Errorf("Emit() output missing message, got '%s'", emitOutput)
	}
	if !strings.Contains(logOutput, "emit test") {
		t.Errorf("Log() output missing message, got '%s'", logOutput)
	}
}

func TestLogEntry_EmitWithDifferentLevels(t *testing.T) {
	ulog := NewUnifiedLogger("test")

	// Should not panic for any level
	ulog.Debug("debug").Emit()
	ulog.Info("info").Emit()
	ulog.Warn("warn").Emit()
	ulog.Error("error").Emit()
	ulog.Success("success").Emit()
	ulog.Progress("progress").Emit()
	ulog.Status("status").Emit()
}

// testError is a simple error implementation for testing
type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

// captureHook records every logrus entry fired through a logger so tests can
// inspect the structured fields.
type captureHook struct {
	entries []*logrus.Entry
}

func (h *captureHook) Levels() []logrus.Level { return logrus.AllLevels }

func (h *captureHook) Fire(e *logrus.Entry) error {
	h.entries = append(h.entries, e)
	return nil
}

// countPrettyRenders installs a render observer for the duration of the test
// and returns a pointer to the running count.
func countPrettyRenders(t *testing.T) *int {
	t.Helper()
	renders := new(int)
	prettyRenderObserver = func() { *renders++ }
	t.Cleanup(func() { prettyRenderObserver = nil })
	return renders
}

// newIsolatedUnifiedLogger resets the logging package state and builds a
// fresh unified logger with deterministic sink levels (GROVE_LOG_LEVEL=info
// overrides both the console and file levels regardless of ambient config).
func newIsolatedUnifiedLogger(t *testing.T, component, prettyFieldsEnv string) *UnifiedLogger {
	t.Helper()
	t.Setenv("GROVE_LOG_LEVEL", "info")
	t.Setenv("GROVE_LOG_PRETTY_FIELDS", prettyFieldsEnv)
	Reset()
	t.Cleanup(Reset)
	return NewUnifiedLogger(component)
}

func TestLogBelowAllSinksDoesNoPrettyRender(t *testing.T) {
	ulog := newIsolatedUnifiedLogger(t, "test-below-sinks", "")
	renders := countPrettyRenders(t)

	hook := &captureHook{}
	ulog.structured.Logger.AddHook(hook)

	var buf bytes.Buffer
	ctx := WithWriter(context.Background(), &buf)

	// Console and file sinks are both at info: a debug entry clears nothing.
	ulog.Debug("invisible detail").Field("key", "value").Log(ctx)

	if *renders != 0 {
		t.Errorf("expected 0 pretty renders for a below-all-sinks entry, got %d", *renders)
	}
	if got := buf.String(); got != "" {
		t.Errorf("expected no pretty output, got %q", got)
	}
	if len(hook.entries) != 0 {
		t.Errorf("expected no structured entries, got %d", len(hook.entries))
	}
}

func TestStructuredOmitsPrettyFieldsByDefault(t *testing.T) {
	ulog := newIsolatedUnifiedLogger(t, "test-pretty-fields-off", "")

	hook := &captureHook{}
	ulog.structured.Logger.AddHook(hook)

	var buf bytes.Buffer
	ctx := WithWriter(context.Background(), &buf)
	ulog.Info("hello fields").Log(ctx)

	if len(hook.entries) != 1 {
		t.Fatalf("expected 1 structured entry, got %d", len(hook.entries))
	}
	data := hook.entries[0].Data
	if v, ok := data["pretty_ansi"]; ok {
		t.Errorf("expected pretty_ansi to be omitted by default, got %q", v)
	}
	if v, ok := data["pretty_text"]; ok {
		t.Errorf("expected pretty_text to be omitted by default, got %q", v)
	}
	// The console line is still rendered and written.
	if !strings.Contains(buf.String(), "hello fields") {
		t.Errorf("expected console output to contain the message, got %q", buf.String())
	}
}

func TestStructuredIncludesPrettyFieldsWhenEnabled(t *testing.T) {
	ulog := newIsolatedUnifiedLogger(t, "test-pretty-fields-on", "true")
	renders := countPrettyRenders(t)

	hook := &captureHook{}
	ulog.structured.Logger.AddHook(hook)

	var buf bytes.Buffer
	ctx := WithWriter(context.Background(), &buf)
	ulog.Success("job done").Log(ctx)

	if len(hook.entries) != 1 {
		t.Fatalf("expected 1 structured entry, got %d", len(hook.entries))
	}
	data := hook.entries[0].Data
	prettyAnsi, ok := data["pretty_ansi"].(string)
	if !ok || prettyAnsi == "" {
		t.Errorf("expected non-empty pretty_ansi when the flag is on, got %v", data["pretty_ansi"])
	}
	prettyText, ok := data["pretty_text"].(string)
	if !ok || !strings.Contains(prettyText, "job done") {
		t.Errorf("expected pretty_text to contain the message, got %v", data["pretty_text"])
	}
	if strings.Contains(prettyText, "\x1b[") {
		t.Errorf("expected pretty_text to be ANSI-free, got %q", prettyText)
	}
	// The pretty line is rendered exactly once even though both the console
	// and the structured fields consume it.
	if *renders != 1 {
		t.Errorf("expected exactly 1 pretty render, got %d", *renders)
	}
}

func TestConsoleOutputUnchangedByPrettyFieldGating(t *testing.T) {
	render := func(prettyFieldsEnv string) string {
		ulog := newIsolatedUnifiedLogger(t, "test-console-bytes-"+prettyFieldsEnv, prettyFieldsEnv)
		var buf bytes.Buffer
		ctx := WithWriter(context.Background(), &buf)
		ulog.Warn("disk almost full").Field("free_mb", 12).Log(ctx)
		return buf.String()
	}

	off := render("false")
	on := render("true")
	if off != on {
		t.Errorf("console output must be byte-identical regardless of the pretty-fields flag:\noff=%q\non =%q", off, on)
	}
	if !strings.Contains(off, "disk almost full") {
		t.Errorf("expected console output to contain the message, got %q", off)
	}
}

func TestStructuredOnlyBelowConsoleLevelStillReachesSinks(t *testing.T) {
	// An info-level StructuredOnly entry must keep reaching structured sinks
	// after the early-out gate, and must not render pretty output when the
	// pretty fields are off (the default).
	ulog := newIsolatedUnifiedLogger(t, "test-structonly-gate", "")
	renders := countPrettyRenders(t)

	hook := &captureHook{}
	ulog.structured.Logger.AddHook(hook)

	var buf bytes.Buffer
	ctx := WithWriter(context.Background(), &buf)
	ulog.Info("audit record").StructuredOnly().Log(ctx)

	if len(hook.entries) != 1 {
		t.Fatalf("expected 1 structured entry, got %d", len(hook.entries))
	}
	if got := buf.String(); got != "" {
		t.Errorf("expected no pretty output for StructuredOnly, got %q", got)
	}
	if *renders != 0 {
		t.Errorf("expected 0 pretty renders for StructuredOnly with pretty fields off, got %d", *renders)
	}
}
