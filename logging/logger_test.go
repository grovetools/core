package logging

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestNewLogger(t *testing.T) {
	// Test creating a logger
	logger := NewLogger("test-component")
	if logger == nil {
		t.Fatal("Expected logger to be created")
	}

	// Verify it's a logrus.Entry with the component field
	if logger.Data["component"] != "test-component" {
		t.Errorf("Expected component to be 'test-component', got %v", logger.Data["component"])
	}
}

func TestLoggerOutput(t *testing.T) {
	// Create a buffer to capture output
	var buf bytes.Buffer
	
	// Create a new logger and redirect output to buffer
	logger := logrus.New()
	logger.SetOutput(&buf)
	logger.SetFormatter(&TextFormatter{Config: FormatConfig{}})
	
	entry := logger.WithField("component", "test")
	entry.Info("Test message")
	
	output := buf.String()
	
	// Check that output contains expected elements
	if !strings.Contains(output, "[INFO]") {
		t.Errorf("Expected output to contain [INFO], got: %s", output)
	}
	if !strings.Contains(output, "[test]") {
		t.Errorf("Expected output to contain [test], got: %s", output)
	}
	if !strings.Contains(output, "Test message") {
		t.Errorf("Expected output to contain 'Test message', got: %s", output)
	}
}

func TestTextFormatter(t *testing.T) {
	tests := []struct {
		name   string
		config FormatConfig
		entry  *logrus.Entry
		want   []string // Parts that should be in the output
		notWant []string // Parts that should NOT be in the output
	}{
		{
			name:   "default format",
			config: FormatConfig{},
			entry: &logrus.Entry{
				Level:   logrus.InfoLevel,
				Message: "test message",
				Data: logrus.Fields{
					"component": "test-component",
					"key1":      "value1",
				},
			},
			want:    []string{"[INFO]", "[test-component]", "test message", "key1=value1"},
			notWant: []string{},
		},
		{
			name: "simple format",
			config: FormatConfig{
				DisableTimestamp: true,
				DisableComponent: true,
			},
			entry: &logrus.Entry{
				Level:   logrus.WarnLevel,
				Message: "warning message",
				Data: logrus.Fields{
					"component": "test-component",
				},
			},
			want:    []string{"[WARN]", "warning message"},
			notWant: []string{"[test-component]"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := &TextFormatter{Config: tt.config}
			
			// Set a fixed time for consistent testing
			tt.entry.Time = tt.entry.Time.UTC()
			
			output, err := formatter.Format(tt.entry)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			
			outputStr := string(output)
			
			// Check for expected parts
			for _, want := range tt.want {
				if !strings.Contains(outputStr, want) {
					t.Errorf("Expected output to contain '%s', got: %s", want, outputStr)
				}
			}
			
			// Check for parts that should NOT be present
			for _, notWant := range tt.notWant {
				if strings.Contains(outputStr, notWant) {
					t.Errorf("Expected output NOT to contain '%s', got: %s", notWant, outputStr)
				}
			}
		})
	}
}

func TestLogLevels(t *testing.T) {
	// Test that log level filtering works
	var buf bytes.Buffer
	logger := logrus.New()
	logger.SetOutput(&buf)
	logger.SetLevel(logrus.WarnLevel)
	
	entry := logger.WithField("component", "test")
	
	// These should not appear
	entry.Debug("debug message")
	entry.Info("info message")
	
	// These should appear
	entry.Warn("warn message")
	entry.Error("error message")
	
	output := buf.String()
	
	if strings.Contains(output, "debug message") {
		t.Error("Debug message should not appear at Warn level")
	}
	if strings.Contains(output, "info message") {
		t.Error("Info message should not appear at Warn level")
	}
	if !strings.Contains(output, "warn message") {
		t.Error("Warn message should appear at Warn level")
	}
	if !strings.Contains(output, "error message") {
		t.Error("Error message should appear at Warn level")
	}
}

func TestEnvironmentVariables(t *testing.T) {
	// Save original env vars
	origLevel := os.Getenv("GROVE_LOG_LEVEL")
	origCaller := os.Getenv("GROVE_LOG_CALLER")
	
	// Clean up after test
	defer func() {
		os.Setenv("GROVE_LOG_LEVEL", origLevel)
		os.Setenv("GROVE_LOG_CALLER", origCaller)
		// Clear the loggers cache
		loggersMu.Lock()
		loggers = make(map[string]*logrus.Entry)
		loggersMu.Unlock()
	}()
	
	// Test log level from env
	os.Setenv("GROVE_LOG_LEVEL", "debug")
	os.Setenv("GROVE_LOG_CALLER", "true")
	
	logger := NewLogger("env-test")
	
	// The underlying logger should have debug level
	if logger.Logger.Level != logrus.DebugLevel {
		t.Errorf("Expected debug level from env var, got %v", logger.Logger.Level)
	}
	
	// Should have caller reporting enabled
	if !logger.Logger.ReportCaller {
		t.Error("Expected caller reporting to be enabled from env var")
	}
}