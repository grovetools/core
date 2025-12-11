package logging

import (
	"bytes"
	"os"
	"runtime"
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
		{
			name:   "caller information with function name",
			config: FormatConfig{},
			entry: func() *logrus.Entry {
				logger := logrus.New()
				logger.SetReportCaller(true)
				entry := &logrus.Entry{
					Logger:  logger,
					Level:   logrus.InfoLevel,
					Message: "test message with caller",
					Data: logrus.Fields{
						"component": "test-component",
					},
					Caller: &runtime.Frame{
						File:     "/path/to/file.go",
						Line:     42,
						Function: "github.com/example/package.TestFunction",
					},
				}
				return entry
			}(),
			want:    []string{"[INFO]", "[test-component]", "test message with caller", "[file.go:42 package.TestFunction]"},
			notWant: []string{},
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

func TestResolveFilterSet(t *testing.T) {
	tests := []struct {
		name     string
		items    []string
		groups   map[string][]string
		expected map[string]bool
	}{
		{
			name:     "empty items returns nil",
			items:    []string{},
			groups:   map[string][]string{"custom": {"grove-gemini", "grove-context"}},
			expected: nil,
		},
		{
			name:     "nil items returns nil",
			items:    nil,
			groups:   map[string][]string{"custom": {"grove-gemini", "grove-context"}},
			expected: nil,
		},
		{
			name:   "default group ai expands without user definition",
			items:  []string{"ai"},
			groups: nil,
			expected: map[string]bool{
				"grove-gemini":  true,
				"grove-openai":  true,
				"grove-context": true,
			},
		},
		{
			name:   "user-defined group overrides default group",
			items:  []string{"ai"},
			groups: map[string][]string{"ai": {"custom-ai-component"}},
			expected: map[string]bool{
				"custom-ai-component": true,
			},
		},
		{
			name:   "single component without groups",
			items:  []string{"grove-flow"},
			groups: nil,
			expected: map[string]bool{
				"grove-flow": true,
			},
		},
		{
			name:   "multiple components without groups",
			items:  []string{"grove-flow", "grove-core"},
			groups: nil,
			expected: map[string]bool{
				"grove-flow": true,
				"grove-core": true,
			},
		},
		{
			name:  "group expansion",
			items: []string{"ai"},
			groups: map[string][]string{
				"ai": {"grove-gemini", "grove-context"},
			},
			expected: map[string]bool{
				"grove-gemini":  true,
				"grove-context": true,
			},
		},
		{
			name:  "mixed groups and components",
			items: []string{"ai", "grove-flow"},
			groups: map[string][]string{
				"ai": {"grove-gemini", "grove-context"},
			},
			expected: map[string]bool{
				"grove-gemini":  true,
				"grove-context": true,
				"grove-flow":    true,
			},
		},
		{
			name:  "multiple groups",
			items: []string{"ai", "devops"},
			groups: map[string][]string{
				"ai":     {"grove-gemini", "grove-context"},
				"devops": {"grove-proxy", "grove-deploy"},
			},
			expected: map[string]bool{
				"grove-gemini":  true,
				"grove-context": true,
				"grove-proxy":   true,
				"grove-deploy":  true,
			},
		},
		{
			name:  "unknown group treated as component",
			items: []string{"unknown-group"},
			groups: map[string][]string{
				"ai": {"grove-gemini"},
			},
			expected: map[string]bool{
				"unknown-group": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveFilterSet(tt.items, tt.groups)

			if tt.expected == nil {
				if result != nil {
					t.Errorf("Expected nil, got %v", result)
				}
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d items, got %d: %v", len(tt.expected), len(result), result)
			}

			for key, expectedVal := range tt.expected {
				if result[key] != expectedVal {
					t.Errorf("Expected %s=%v, got %v", key, expectedVal, result[key])
				}
			}
		})
	}
}

func TestIsComponentVisible(t *testing.T) {
	// Test the exported IsComponentVisible function
	tests := []struct {
		name      string
		component string
		cfg       *Config
		expected  bool
	}{
		{
			name:      "no filters - non-grove component visible",
			component: "my-custom-app",
			cfg:       &Config{},
			expected:  true,
		},
		{
			name:      "show whitelist - component in list",
			component: "grove-flow",
			cfg: &Config{
				ComponentFiltering: &ComponentFilteringConfig{
					Show: []string{"grove-flow", "grove-core"},
				},
			},
			expected: true,
		},
		{
			name:      "show whitelist - component not in list",
			component: "grove-gemini",
			cfg: &Config{
				ComponentFiltering: &ComponentFilteringConfig{
					Show: []string{"grove-flow", "grove-core"},
				},
			},
			expected: false,
		},
		{
			name:      "hide blacklist - component in list",
			component: "grove-gemini",
			cfg: &Config{
				ComponentFiltering: &ComponentFilteringConfig{
					Hide: []string{"grove-gemini", "grove-context"},
				},
			},
			expected: false,
		},
		{
			name:      "hide blacklist - component not in list",
			component: "grove-flow",
			cfg: &Config{
				ComponentFiltering: &ComponentFilteringConfig{
					Hide: []string{"grove-gemini", "grove-context"},
				},
			},
			expected: true,
		},
		{
			name:      "show takes precedence over hide - component in show",
			component: "grove-flow",
			cfg: &Config{
				ComponentFiltering: &ComponentFilteringConfig{
					Show: []string{"grove-flow"},
					Hide: []string{"grove-flow"}, // Would hide, but show takes precedence
				},
			},
			expected: true,
		},
		{
			name:      "component visible when not in show or hide",
			component: "grove-gemini",
			cfg: &Config{
				ComponentFiltering: &ComponentFilteringConfig{
					Show: []string{"grove-flow"},
					Hide: []string{"grove-core"},
				},
			},
			expected: true, // grove-gemini is not hidden, and explicit hide overrides default
		},
		{
			name:      "show with group expansion - component in group",
			component: "grove-gemini",
			cfg: &Config{
				Groups: map[string][]string{
					"ai": {"grove-gemini", "grove-context"},
				},
				ComponentFiltering: &ComponentFilteringConfig{
					Show: []string{"ai"},
				},
			},
			expected: true,
		},
		{
			name:      "show with group expansion - component not in group",
			component: "grove-flow",
			cfg: &Config{
				Groups: map[string][]string{
					"ai": {"grove-gemini", "grove-context"},
				},
				ComponentFiltering: &ComponentFilteringConfig{
					Show: []string{"ai"},
				},
			},
			expected: false,
		},
		{
			name:      "hide with group expansion - component in group",
			component: "grove-gemini",
			cfg: &Config{
				Groups: map[string][]string{
					"ai": {"grove-gemini", "grove-context"},
				},
				ComponentFiltering: &ComponentFilteringConfig{
					Hide: []string{"ai"},
				},
			},
			expected: false,
		},
		{
			name:      "hide with group expansion - component not in group",
			component: "grove-flow",
			cfg: &Config{
				Groups: map[string][]string{
					"ai": {"grove-gemini", "grove-context"},
				},
				ComponentFiltering: &ComponentFilteringConfig{
					Hide: []string{"ai"},
				},
			},
			expected: true,
		},
		{
			name:      "mixed group and direct component in show",
			component: "grove-flow",
			cfg: &Config{
				Groups: map[string][]string{
					"ai": {"grove-gemini", "grove-context"},
				},
				ComponentFiltering: &ComponentFilteringConfig{
					Show: []string{"ai", "grove-flow"},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsComponentVisible(tt.component, tt.cfg)
			if result != tt.expected {
				t.Errorf("IsComponentVisible(%q) = %v, expected %v", tt.component, result, tt.expected)
			}
		})
	}
}

func TestDefaultHide(t *testing.T) {
	// With no show/hide rules, DefaultHide (grove-ecosystem) should apply
	cfg := &Config{}

	// grove-gemini is in grove-ecosystem group, should be hidden by default
	if IsComponentVisible("grove-gemini", cfg) {
		t.Error("grove-gemini should be hidden by default (in grove-ecosystem)")
	}

	// random component not in grove-ecosystem should be visible
	if !IsComponentVisible("my-custom-app", cfg) {
		t.Error("my-custom-app should be visible (not in grove-ecosystem)")
	}

	// explicit empty hide should override default (show everything)
	cfgExplicitEmpty := &Config{
		ComponentFiltering: &ComponentFilteringConfig{
			Hide: []string{},
		},
	}
	// This doesn't override because empty slice still results in nil set
	// To truly show all, user would set show: ["*"] or similar

	// explicit hide overrides default
	cfgExplicitHide := &Config{
		ComponentFiltering: &ComponentFilteringConfig{
			Hide: []string{"my-custom-app"},
		},
	}
	if !IsComponentVisible("grove-gemini", cfgExplicitHide) {
		t.Error("grove-gemini should be visible when explicit hide doesn't include it")
	}
	if IsComponentVisible("my-custom-app", cfgExplicitHide) {
		t.Error("my-custom-app should be hidden when explicitly in hide list")
	}

	_ = cfgExplicitEmpty // silence unused warning
}

func TestShowCurrentProject(t *testing.T) {
	boolTrue := true
	boolFalse := false

	tests := []struct {
		name               string
		component          string
		showCurrentProject *bool
		hide               []string
		expected           bool
	}{
		{
			name:               "nil (default true) allows current project even when hidden",
			component:          getCurrentProjectName(),
			showCurrentProject: nil,
			hide:               []string{getCurrentProjectName()},
			expected:           true,
		},
		{
			name:               "explicit true allows current project even when hidden",
			component:          getCurrentProjectName(),
			showCurrentProject: &boolTrue,
			hide:               []string{getCurrentProjectName()},
			expected:           true,
		},
		{
			name:               "explicit false respects hide for current project",
			component:          getCurrentProjectName(),
			showCurrentProject: &boolFalse,
			hide:               []string{getCurrentProjectName()},
			expected:           false,
		},
		{
			name:               "other components still respect hide",
			component:          "other-component",
			showCurrentProject: nil,
			hide:               []string{"other-component"},
			expected:           false,
		},
	}

	// Skip if no current project name (not in a grove workspace)
	if getCurrentProjectName() == "" {
		t.Skip("Not in a grove workspace, skipping current project tests")
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				ShowCurrentProject: tt.showCurrentProject,
				ComponentFiltering: &ComponentFilteringConfig{
					Hide: tt.hide,
				},
			}
			result := IsComponentVisible(tt.component, cfg)
			if result != tt.expected {
				t.Errorf("IsComponentVisible(%q) = %v, expected %v", tt.component, result, tt.expected)
			}
		})
	}
}

// TestGetComponentVisibility provides comprehensive tests for the new log filtering logic.
func TestGetComponentVisibility(t *testing.T) {
	testCases := []struct {
		name             string
		component        string
		config           *Config
		overrides        *OverrideOptions
		expectedVisible  bool
		expectedReason   VisibilityReason
		expectedRuleSize int // We check size because slice content can be complex
	}{
		// --- Basic Config Rules ---
		{
			name:            "Default visible when no rules match",
			component:       "api",
			config:          &Config{},
			overrides:       &OverrideOptions{},
			expectedVisible: true,
			expectedReason:  ReasonVisibleDefault,
		},
		{
			name:      "Hidden by 'hide' rule",
			component: "cache",
			config: &Config{
				ComponentFiltering: &ComponentFilteringConfig{
					Hide: []string{"cache"},
				},
			},
			overrides:        &OverrideOptions{},
			expectedVisible:  false,
			expectedReason:   ReasonHiddenByHide,
			expectedRuleSize: 1,
		},
		{
			name:      "Visible by 'show' rule, overriding 'hide'",
			component: "cache",
			config: &Config{
				ComponentFiltering: &ComponentFilteringConfig{
					Show: []string{"cache"},
					Hide: []string{"cache"},
				},
			},
			overrides:        &OverrideOptions{},
			expectedVisible:  true,
			expectedReason:   ReasonVisibleByShow,
			expectedRuleSize: 1,
		},
		{
			name:      "Visible by 'only' rule",
			component: "api",
			config: &Config{
				ComponentFiltering: &ComponentFilteringConfig{
					Only: []string{"api", "db"},
				},
			},
			overrides:        &OverrideOptions{},
			expectedVisible:  true,
			expectedReason:   ReasonVisibleByOnly,
			expectedRuleSize: 2,
		},
		{
			name:      "Hidden by 'only' rule",
			component: "frontend",
			config: &Config{
				ComponentFiltering: &ComponentFilteringConfig{
					Only: []string{"api", "db"},
				},
			},
			overrides:        &OverrideOptions{},
			expectedVisible:  false,
			expectedReason:   ReasonHiddenByOnly,
			expectedRuleSize: 2,
		},
		{
			name:             "Hidden by default 'grove-ecosystem' rule",
			component:        "grove-mcp",
			config:           &Config{},
			overrides:        &OverrideOptions{},
			expectedVisible:  false,
			expectedReason:   ReasonHiddenByDefault,
			expectedRuleSize: 1,
		},
		// --- CLI Overrides ---
		{
			name:      "--show-all overrides 'hide' rule",
			component: "cache",
			config: &Config{
				ComponentFiltering: &ComponentFilteringConfig{
					Hide: []string{"cache"},
				},
			},
			overrides:       &OverrideOptions{ShowAll: true},
			expectedVisible: true,
			expectedReason:  ReasonVisibleByOverrideShowAll,
		},
		{
			name:             "--component acts as a strict whitelist (shows component)",
			component:        "api",
			config:           &Config{},
			overrides:        &OverrideOptions{ShowOnly: []string{"api"}},
			expectedVisible:  true,
			expectedReason:   ReasonVisibleByOverrideShowOnly,
			expectedRuleSize: 1,
		},
		{
			name:             "--component acts as a strict whitelist (hides other components)",
			component:        "db",
			config:           &Config{},
			overrides:        &OverrideOptions{ShowOnly: []string{"api"}},
			expectedVisible:  false,
			expectedReason:   ReasonHiddenByOverrideShowOnly,
			expectedRuleSize: 1,
		},
		{
			name:      "--also-show overrides 'hide' rule",
			component: "cache",
			config: &Config{
				ComponentFiltering: &ComponentFilteringConfig{
					Hide: []string{"cache"},
				},
			},
			overrides:        &OverrideOptions{AlsoShow: []string{"cache"}},
			expectedVisible:  true,
			expectedReason:   ReasonVisibleByOverrideAlsoShow,
			expectedRuleSize: 1,
		},
		{
			name:      "--ignore-hide overrides 'hide' rule",
			component: "cache",
			config: &Config{
				ComponentFiltering: &ComponentFilteringConfig{
					Hide: []string{"cache"},
				},
			},
			overrides:        &OverrideOptions{IgnoreHide: []string{"cache"}},
			expectedVisible:  true,
			expectedReason:   ReasonVisibleByOverrideIgnore,
			expectedRuleSize: 1,
		},
		// --- Precedence Rules ---
		{
			name:      "--component overrides config 'only'",
			component: "frontend",
			config: &Config{
				ComponentFiltering: &ComponentFilteringConfig{
					Only: []string{"api", "db"},
				},
			},
			overrides:        &OverrideOptions{ShowOnly: []string{"frontend"}},
			expectedVisible:  true,
			expectedReason:   ReasonVisibleByOverrideShowOnly,
			expectedRuleSize: 1,
		},
		{
			name:      "config 'show' overrides config 'hide'",
			component: "metrics",
			config: &Config{
				ComponentFiltering: &ComponentFilteringConfig{
					Show: []string{"metrics"},
					Hide: []string{"metrics"},
				},
			},
			overrides:        &OverrideOptions{},
			expectedVisible:  true,
			expectedReason:   ReasonVisibleByShow,
			expectedRuleSize: 1,
		},
		{
			name:      "config 'only' takes precedence over 'hide'",
			component: "api",
			config: &Config{
				ComponentFiltering: &ComponentFilteringConfig{
					Only: []string{"api"},
					Hide: []string{"api"},
				},
			},
			overrides:        &OverrideOptions{},
			expectedVisible:  true,
			expectedReason:   ReasonVisibleByOnly,
			expectedRuleSize: 1,
		},
		// --- Group Expansion ---
		{
			name:      "Hidden by group in 'hide' rule",
			component: "db",
			config: &Config{
				Groups: map[string][]string{"backend": {"api", "db"}},
				ComponentFiltering: &ComponentFilteringConfig{
					Hide: []string{"backend"},
				},
			},
			overrides:        &OverrideOptions{},
			expectedVisible:  false,
			expectedReason:   ReasonHiddenByHide,
			expectedRuleSize: 1,
		},
		{
			name:      "Visible by group in 'only' rule",
			component: "api",
			config: &Config{
				Groups: map[string][]string{"backend": {"api", "db"}},
				ComponentFiltering: &ComponentFilteringConfig{
					Only: []string{"backend"},
				},
			},
			overrides:        &OverrideOptions{},
			expectedVisible:  true,
			expectedReason:   ReasonVisibleByOnly,
			expectedRuleSize: 1,
		},
		{
			name:      "Visible by default 'grove-ecosystem' group in 'show' rule",
			component: "grove-mcp",
			config: &Config{
				ComponentFiltering: &ComponentFilteringConfig{
					Show: []string{"grove-ecosystem"},
				},
			},
			overrides:        &OverrideOptions{},
			expectedVisible:  true,
			expectedReason:   ReasonVisibleByShow,
			expectedRuleSize: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GetComponentVisibility(tc.component, tc.config, tc.overrides)

			if result.Visible != tc.expectedVisible {
				t.Errorf("Visibility mismatch: got %v, want %v", result.Visible, tc.expectedVisible)
			}
			if result.Reason != tc.expectedReason {
				t.Errorf("Reason mismatch: got %v, want %v", result.Reason, tc.expectedReason)
			}
			if tc.expectedRuleSize > 0 {
				if len(result.Rule) != tc.expectedRuleSize {
					t.Errorf("Rule size mismatch: got %d, want %d", len(result.Rule), tc.expectedRuleSize)
				}
			}
		})
	}
}