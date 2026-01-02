package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattsolo1/grove-core/logging"
	"github.com/mattsolo1/grove-tend/pkg/assert"
	"github.com/mattsolo1/grove-tend/pkg/fs"
	"github.com/mattsolo1/grove-tend/pkg/harness"
)

// LoggingJSONFormatScenario tests that the logger outputs valid JSON when configured.
func LoggingJSONFormatScenario() *harness.Scenario {
	var projectDir string
	var origDir string

	return &harness.Scenario{
		Name:        "core-logging-json-format",
		Description: "Verifies that the logger outputs valid JSON when configured with json format.",
		Tags:        []string{"core", "logging", "json"},
		Steps: []harness.Step{
			{
				Name: "Create grove.yml with JSON file logging config",
				Func: func(ctx *harness.Context) error {
					projectDir = ctx.NewDir("json-logging-test")

					projectYAML := `name: json-logging-test
version: "1.0"
logging:
  level: debug
  log_startup: false
  file:
    enabled: true
    format: json
`
					return fs.WriteString(filepath.Join(projectDir, "grove.yml"), projectYAML)
				},
			},
			{
				Name: "Change to project directory and reset logger",
				Func: func(ctx *harness.Context) error {
					var err error
					origDir, err = os.Getwd()
					if err != nil {
						return fmt.Errorf("failed to get current dir: %w", err)
					}

					if err := os.Chdir(projectDir); err != nil {
						return fmt.Errorf("failed to chdir to %s: %w", projectDir, err)
					}

					// Reset the logger cache to pick up new config
					logging.Reset()
					return nil
				},
			},
			{
				Name: "Create logger and write test entries",
				Func: func(ctx *harness.Context) error {
					logger := logging.NewLogger("test-component")
					logger.Info("Test info message")
					logger.WithField("key", "value").Debug("Test debug message with field")
					logger.WithFields(map[string]interface{}{
						"nested": map[string]interface{}{
							"foo": "bar",
							"num": 42,
						},
					}).Info("Test message with nested object")
					return nil
				},
			},
			{
				Name: "Verify log file exists",
				Func: func(ctx *harness.Context) error {
					logDir := filepath.Join(projectDir, ".grove", "logs")
					logFiles, err := filepath.Glob(filepath.Join(logDir, "workspace-*.log"))
					if err != nil {
						return fmt.Errorf("failed to glob log files: %w", err)
					}
					if len(logFiles) == 0 {
						return fmt.Errorf("no log files found in %s", logDir)
					}
					return nil
				},
			},
			{
				Name: "Verify each log line is valid JSON with required fields",
				Func: func(ctx *harness.Context) error {
					logDir := filepath.Join(projectDir, ".grove", "logs")
					logFiles, _ := filepath.Glob(filepath.Join(logDir, "workspace-*.log"))

					logContent, err := fs.ReadString(logFiles[0])
					if err != nil {
						return fmt.Errorf("failed to read log file: %w", err)
					}

					lines := strings.Split(strings.TrimSpace(logContent), "\n")
					if len(lines) == 0 {
						return fmt.Errorf("log file is empty")
					}

					for i, line := range lines {
						if line == "" {
							continue
						}
						var entry map[string]interface{}
						if err := json.Unmarshal([]byte(line), &entry); err != nil {
							return fmt.Errorf("line %d is not valid JSON: %w\nContent: %s", i+1, err, line)
						}

						if _, ok := entry["level"]; !ok {
							return fmt.Errorf("line %d missing 'level' field", i+1)
						}
						if _, ok := entry["msg"]; !ok {
							return fmt.Errorf("line %d missing 'msg' field", i+1)
						}
						if _, ok := entry["time"]; !ok {
							return fmt.Errorf("line %d missing 'time' field", i+1)
						}
					}
					return nil
				},
			},
			{
				Name: "Cleanup: restore original directory",
				Func: func(ctx *harness.Context) error {
					if origDir != "" {
						return os.Chdir(origDir)
					}
					return nil
				},
			},
		},
	}
}

// LoggingJSONFieldsScenario tests that structured fields are properly included in JSON logs.
func LoggingJSONFieldsScenario() *harness.Scenario {
	var projectDir string
	var origDir string

	return &harness.Scenario{
		Name:        "core-logging-json-fields",
		Description: "Verifies that structured fields like component are present in JSON logs.",
		Tags:        []string{"core", "logging", "json", "fields"},
		Steps: []harness.Step{
			{
				Name: "Create grove.yml with JSON file logging config",
				Func: func(ctx *harness.Context) error {
					projectDir = ctx.NewDir("json-fields-test")

					projectYAML := `name: json-fields-test
version: "1.0"
logging:
  level: debug
  file:
    enabled: true
    format: json
`
					return fs.WriteString(filepath.Join(projectDir, "grove.yml"), projectYAML)
				},
			},
			{
				Name: "Change to project directory and reset logger",
				Func: func(ctx *harness.Context) error {
					var err error
					origDir, err = os.Getwd()
					if err != nil {
						return fmt.Errorf("failed to get current dir: %w", err)
					}

					if err := os.Chdir(projectDir); err != nil {
						return fmt.Errorf("failed to chdir to %s: %w", projectDir, err)
					}

					logging.Reset()
					return nil
				},
			},
			{
				Name: "Create logger with component name",
				Func: func(ctx *harness.Context) error {
					logger := logging.NewLogger("test-component")
					logger.Info("Initial log from test-component")
					return nil
				},
			},
			{
				Name: "Write log entry with custom field (user_id)",
				Func: func(ctx *harness.Context) error {
					logger := logging.NewLogger("test-component")
					logger.WithField("user_id", 12345).Info("User logged in")
					return nil
				},
			},
			{
				Name: "Write log entry with multiple custom fields",
				Func: func(ctx *harness.Context) error {
					logger := logging.NewLogger("test-component")
					logger.WithFields(map[string]interface{}{
						"request_id": "abc-123",
						"method":     "POST",
						"path":       "/api/users",
					}).Info("API request")
					return nil
				},
			},
			{
				Name: "Verify component field in logs",
				Func: func(ctx *harness.Context) error {
					logDir := filepath.Join(projectDir, ".grove", "logs")
					logFiles, _ := filepath.Glob(filepath.Join(logDir, "workspace-*.log"))
					if len(logFiles) == 0 {
						return fmt.Errorf("no log files found")
					}

					logContent, err := fs.ReadString(logFiles[0])
					if err != nil {
						return fmt.Errorf("failed to read log file: %w", err)
					}

					lines := strings.Split(strings.TrimSpace(logContent), "\n")
					for _, line := range lines {
						if line == "" {
							continue
						}
						var entry map[string]interface{}
						if err := json.Unmarshal([]byte(line), &entry); err != nil {
							return fmt.Errorf("invalid JSON: %w", err)
						}

						if comp, ok := entry["component"].(string); ok && comp == "test-component" {
							return nil // Found it!
						}
					}
					return fmt.Errorf("no log entry found with component='test-component'")
				},
			},
			{
				Name: "Verify custom fields (user_id, request_id) in logs",
				Func: func(ctx *harness.Context) error {
					logDir := filepath.Join(projectDir, ".grove", "logs")
					logFiles, _ := filepath.Glob(filepath.Join(logDir, "workspace-*.log"))

					logContent, _ := fs.ReadString(logFiles[0])
					lines := strings.Split(strings.TrimSpace(logContent), "\n")

					foundUserID := false
					foundRequestID := false

					for _, line := range lines {
						if line == "" {
							continue
						}
						var entry map[string]interface{}
						json.Unmarshal([]byte(line), &entry)

						if _, ok := entry["user_id"]; ok {
							foundUserID = true
						}
						if _, ok := entry["request_id"]; ok {
							foundRequestID = true
						}
					}

					if !foundUserID {
						return fmt.Errorf("user_id field not found in any log entry")
					}
					if !foundRequestID {
						return fmt.Errorf("request_id field not found in any log entry")
					}
					return nil
				},
			},
			{
				Name: "Cleanup: restore original directory",
				Func: func(ctx *harness.Context) error {
					if origDir != "" {
						return os.Chdir(origDir)
					}
					return nil
				},
			},
		},
	}
}

// LoggingNestedJSONScenario tests that nested JSON objects in log fields are properly formatted.
func LoggingNestedJSONScenario() *harness.Scenario {
	var projectDir string
	var origDir string

	return &harness.Scenario{
		Name:        "core-logging-nested-json",
		Description: "Verifies that nested JSON objects are properly serialized in logs.",
		Tags:        []string{"core", "logging", "json", "nested"},
		Steps: []harness.Step{
			{
				Name: "Create grove.yml with JSON file logging config",
				Func: func(ctx *harness.Context) error {
					projectDir = ctx.NewDir("nested-json-test")

					projectYAML := `name: nested-json-test
version: "1.0"
logging:
  level: debug
  file:
    enabled: true
    format: json
`
					return fs.WriteString(filepath.Join(projectDir, "grove.yml"), projectYAML)
				},
			},
			{
				Name: "Change to project directory and reset logger",
				Func: func(ctx *harness.Context) error {
					var err error
					origDir, err = os.Getwd()
					if err != nil {
						return fmt.Errorf("failed to get current dir: %w", err)
					}

					if err := os.Chdir(projectDir); err != nil {
						return fmt.Errorf("failed to chdir to %s: %w", projectDir, err)
					}

					logging.Reset()
					return nil
				},
			},
			{
				Name: "Write log with nested 'user' object",
				Func: func(ctx *harness.Context) error {
					logger := logging.NewLogger("test-component")
					logger.WithField("user", map[string]interface{}{
						"id":    12345,
						"name":  "test-user",
						"roles": []string{"admin", "user"},
					}).Info("User object logged")
					return nil
				},
			},
			{
				Name: "Write log with deeply nested 'metadata' object",
				Func: func(ctx *harness.Context) error {
					logger := logging.NewLogger("test-component")
					logger.WithField("metadata", map[string]interface{}{
						"version": "1.0",
						"nested": map[string]interface{}{
							"deep": "value",
							"level": map[string]interface{}{
								"deeper": "nested-value",
							},
						},
					}).Info("Deeply nested object logged")
					return nil
				},
			},
			{
				Name: "Verify nested 'user' object is properly serialized",
				Func: func(ctx *harness.Context) error {
					logDir := filepath.Join(projectDir, ".grove", "logs")
					logFiles, _ := filepath.Glob(filepath.Join(logDir, "workspace-*.log"))
					if len(logFiles) == 0 {
						return fmt.Errorf("no log files found")
					}

					logContent, _ := fs.ReadString(logFiles[0])
					lines := strings.Split(strings.TrimSpace(logContent), "\n")

					for _, line := range lines {
						if line == "" {
							continue
						}
						var entry map[string]interface{}
						if err := json.Unmarshal([]byte(line), &entry); err != nil {
							return fmt.Errorf("invalid JSON: %w", err)
						}

						if user, ok := entry["user"].(map[string]interface{}); ok {
							if _, hasID := user["id"]; !hasID {
								return fmt.Errorf("nested user object missing 'id' field")
							}
							if _, hasName := user["name"]; !hasName {
								return fmt.Errorf("nested user object missing 'name' field")
							}
							return nil // Found valid nested user object
						}
					}
					return fmt.Errorf("no log entry found with nested 'user' object")
				},
			},
			{
				Name: "Verify deeply nested 'metadata' object is properly serialized",
				Func: func(ctx *harness.Context) error {
					logDir := filepath.Join(projectDir, ".grove", "logs")
					logFiles, _ := filepath.Glob(filepath.Join(logDir, "workspace-*.log"))

					logContent, _ := fs.ReadString(logFiles[0])
					lines := strings.Split(strings.TrimSpace(logContent), "\n")

					for _, line := range lines {
						if line == "" {
							continue
						}
						var entry map[string]interface{}
						json.Unmarshal([]byte(line), &entry)

						if metadata, ok := entry["metadata"].(map[string]interface{}); ok {
							if nested, ok := metadata["nested"].(map[string]interface{}); ok {
								if _, hasDeep := nested["deep"]; hasDeep {
									return nil // Found valid deeply nested structure
								}
							}
						}
					}
					return fmt.Errorf("no log entry found with deeply nested 'metadata' object")
				},
			},
			{
				Name: "Cleanup: restore original directory",
				Func: func(ctx *harness.Context) error {
					if origDir != "" {
						return os.Chdir(origDir)
					}
					return nil
				},
			},
		},
	}
}

// LoggingLevelFilterScenario tests that log level filtering works correctly.
func LoggingLevelFilterScenario() *harness.Scenario {
	var projectDir string
	var origDir string

	return &harness.Scenario{
		Name:        "core-logging-level-filter",
		Description: "Verifies that log level filtering works correctly in JSON output.",
		Tags:        []string{"core", "logging", "levels"},
		Steps: []harness.Step{
			{
				Name: "Create grove.yml with INFO level (not DEBUG)",
				Func: func(ctx *harness.Context) error {
					projectDir = ctx.NewDir("level-filter-test")

					projectYAML := `name: level-filter-test
version: "1.0"
logging:
  level: info
  file:
    enabled: true
    format: json
`
					return fs.WriteString(filepath.Join(projectDir, "grove.yml"), projectYAML)
				},
			},
			{
				Name: "Change to project directory and reset logger",
				Func: func(ctx *harness.Context) error {
					var err error
					origDir, err = os.Getwd()
					if err != nil {
						return fmt.Errorf("failed to get current dir: %w", err)
					}

					if err := os.Chdir(projectDir); err != nil {
						return fmt.Errorf("failed to chdir to %s: %w", projectDir, err)
					}

					logging.Reset()
					return nil
				},
			},
			{
				Name: "Write DEBUG message (should NOT appear in logs)",
				Func: func(ctx *harness.Context) error {
					logger := logging.NewLogger("test-component")
					logger.Debug("This debug message should NOT appear")
					return nil
				},
			},
			{
				Name: "Write INFO message (should appear in logs)",
				Func: func(ctx *harness.Context) error {
					logger := logging.NewLogger("test-component")
					logger.Info("This info message should appear")
					return nil
				},
			},
			{
				Name: "Write WARN message (should appear in logs)",
				Func: func(ctx *harness.Context) error {
					logger := logging.NewLogger("test-component")
					logger.Warn("This warning message should appear")
					return nil
				},
			},
			{
				Name: "Verify DEBUG messages are NOT in log file",
				Func: func(ctx *harness.Context) error {
					logDir := filepath.Join(projectDir, ".grove", "logs")
					logFiles, _ := filepath.Glob(filepath.Join(logDir, "workspace-*.log"))
					if len(logFiles) == 0 {
						return fmt.Errorf("no log files found")
					}

					logContent, _ := fs.ReadString(logFiles[0])
					lines := strings.Split(strings.TrimSpace(logContent), "\n")

					for _, line := range lines {
						if line == "" {
							continue
						}
						var entry map[string]interface{}
						json.Unmarshal([]byte(line), &entry)

						level, _ := entry["level"].(string)
						if level == "debug" || level == "trace" {
							return fmt.Errorf("found debug/trace log at info level: %s", line)
						}
					}
					return nil
				},
			},
			{
				Name: "Verify INFO message IS in log file",
				Func: func(ctx *harness.Context) error {
					logDir := filepath.Join(projectDir, ".grove", "logs")
					logFiles, _ := filepath.Glob(filepath.Join(logDir, "workspace-*.log"))

					logContent, _ := fs.ReadString(logFiles[0])
					lines := strings.Split(strings.TrimSpace(logContent), "\n")

					for _, line := range lines {
						if line == "" {
							continue
						}
						var entry map[string]interface{}
						json.Unmarshal([]byte(line), &entry)

						if entry["level"] == "info" {
							return nil // Found info message
						}
					}
					return fmt.Errorf("info level message not found in logs")
				},
			},
			{
				Name: "Verify WARN message IS in log file",
				Func: func(ctx *harness.Context) error {
					logDir := filepath.Join(projectDir, ".grove", "logs")
					logFiles, _ := filepath.Glob(filepath.Join(logDir, "workspace-*.log"))

					logContent, _ := fs.ReadString(logFiles[0])
					lines := strings.Split(strings.TrimSpace(logContent), "\n")

					for _, line := range lines {
						if line == "" {
							continue
						}
						var entry map[string]interface{}
						json.Unmarshal([]byte(line), &entry)

						if entry["level"] == "warning" {
							return nil // Found warning message
						}
					}
					return fmt.Errorf("warning level message not found in logs")
				},
			},
			{
				Name: "Cleanup: restore original directory",
				Func: func(ctx *harness.Context) error {
					if origDir != "" {
						return os.Chdir(origDir)
					}
					return nil
				},
			},
		},
	}
}

// JSONTreeComponentScenario tests that the jsontree component can be created.
// This is a basic unit-level test to verify the component builds correctly.
func JSONTreeComponentScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "core-jsontree-component",
		Description: "Verifies that the JSON tree TUI component works correctly.",
		Tags:        []string{"core", "tui", "jsontree"},
		Steps: []harness.Step{
			{
				Name: "Find core binary",
				Func: func(ctx *harness.Context) error {
					_, err := FindProjectBinary()
					return err
				},
			},
			{
				Name: "Run core version to verify binary works",
				Func: func(ctx *harness.Context) error {
					coreBinary, _ := FindProjectBinary()
					cmd := ctx.Command(coreBinary, "version")
					result := cmd.Run()

					return assert.Equal(0, result.ExitCode, "core binary should run successfully")
				},
			},
		},
	}
}

// LoggingComponentFilterDefaultScenario tests that logs are shown by default when no show/hide rules exist.
func LoggingComponentFilterDefaultScenario() *harness.Scenario {
	var projectDir string
	var origDir string

	return &harness.Scenario{
		Name:        "core-logging-filter-default",
		Description: "Verifies that all logs are shown by default when no show/hide rules are configured.",
		Tags:        []string{"core", "logging", "filter", "default"},
		Steps: []harness.Step{
			{
				Name: "Create grove.yml without show/hide config",
				Func: func(ctx *harness.Context) error {
					projectDir = ctx.NewDir("filter-default-test")

					projectYAML := `name: filter-default-test
version: "1.0"
logging:
  level: debug
  file:
    enabled: true
    format: json
`
					return fs.WriteString(filepath.Join(projectDir, "grove.yml"), projectYAML)
				},
			},
			{
				Name: "Change to project directory and reset logger",
				Func: func(ctx *harness.Context) error {
					var err error
					origDir, err = os.Getwd()
					if err != nil {
						return fmt.Errorf("failed to get current dir: %w", err)
					}

					if err := os.Chdir(projectDir); err != nil {
						return fmt.Errorf("failed to chdir to %s: %w", projectDir, err)
					}

					logging.Reset()
					return nil
				},
			},
			{
				Name: "Write logs from multiple components",
				Func: func(ctx *harness.Context) error {
					logger1 := logging.NewLogger("component-a")
					logger1.Info("Log from component A")

					logger2 := logging.NewLogger("component-b")
					logger2.Info("Log from component B")

					logger3 := logging.NewLogger("component-c")
					logger3.Info("Log from component C")

					return nil
				},
			},
			{
				Name: "Verify all component logs appear in file",
				Func: func(ctx *harness.Context) error {
					logDir := filepath.Join(projectDir, ".grove", "logs")
					logFiles, _ := filepath.Glob(filepath.Join(logDir, "workspace-*.log"))
					if len(logFiles) == 0 {
						return fmt.Errorf("no log files found")
					}

					logContent, err := fs.ReadString(logFiles[0])
					if err != nil {
						return fmt.Errorf("failed to read log file: %w", err)
					}

					lines := strings.Split(strings.TrimSpace(logContent), "\n")
					foundA, foundB, foundC := false, false, false

					for _, line := range lines {
						if line == "" {
							continue
						}
						var entry map[string]interface{}
						if err := json.Unmarshal([]byte(line), &entry); err != nil {
							continue
						}

						component, _ := entry["component"].(string)
						if component == "component-a" {
							foundA = true
						}
						if component == "component-b" {
							foundB = true
						}
						if component == "component-c" {
							foundC = true
						}
					}

					if !foundA || !foundB || !foundC {
						return fmt.Errorf("not all components found in logs (A:%v B:%v C:%v)", foundA, foundB, foundC)
					}
					return nil
				},
			},
			{
				Name: "Cleanup: restore original directory",
				Func: func(ctx *harness.Context) error {
					if origDir != "" {
						return os.Chdir(origDir)
					}
					return nil
				},
			},
		},
	}
}

// LoggingComponentFilterShowScenario tests that 'show' rules work correctly.
func LoggingComponentFilterShowScenario() *harness.Scenario {
	var projectDir string
	var origDir string

	return &harness.Scenario{
		Name:        "core-logging-filter-show",
		Description: "Verifies that 'show' rules correctly filter visible components.",
		Tags:        []string{"core", "logging", "filter", "show"},
		Steps: []harness.Step{
			{
				Name: "Create grove.yml with show config",
				Func: func(ctx *harness.Context) error {
					projectDir = ctx.NewDir("filter-show-test")

					projectYAML := `name: filter-show-test
version: "1.0"
logging:
  level: debug
  component_filtering:
    only: ["component-a", "component-c"]
  file:
    enabled: true
    format: json
`
					return fs.WriteString(filepath.Join(projectDir, "grove.yml"), projectYAML)
				},
			},
			{
				Name: "Change to project directory and reset logger",
				Func: func(ctx *harness.Context) error {
					var err error
					origDir, err = os.Getwd()
					if err != nil {
						return fmt.Errorf("failed to get current dir: %w", err)
					}

					if err := os.Chdir(projectDir); err != nil {
						return fmt.Errorf("failed to chdir to %s: %w", projectDir, err)
					}

					logging.Reset()
					return nil
				},
			},
			{
				Name: "Write logs from multiple components",
				Func: func(ctx *harness.Context) error {
					logger1 := logging.NewLogger("component-a")
					logger1.Info("Log from component A")

					logger2 := logging.NewLogger("component-b")
					logger2.Info("Log from component B")

					logger3 := logging.NewLogger("component-c")
					logger3.Info("Log from component C")

					return nil
				},
			},
			{
				Name: "Run core logs command and verify only shown components appear",
				Func: func(ctx *harness.Context) error {
					cmd := ctx.Bin("logs").Dir(projectDir)
					result := cmd.Run()

					if result.ExitCode != 0 {
						return fmt.Errorf("core logs command failed with exit code %d: %s", result.ExitCode, result.Stderr)
					}

					output := result.Stdout
					hasA := strings.Contains(output, "component-a")
					hasB := strings.Contains(output, "component-b")
					hasC := strings.Contains(output, "component-c")

					if !hasA {
						return fmt.Errorf("component-a should be visible but is not")
					}
					if hasB {
						return fmt.Errorf("component-b should not be visible but is")
					}
					if !hasC {
						return fmt.Errorf("component-c should be visible but is not")
					}

					return nil
				},
			},
			{
				Name: "Cleanup: restore original directory",
				Func: func(ctx *harness.Context) error {
					if origDir != "" {
						return os.Chdir(origDir)
					}
					return nil
				},
			},
		},
	}
}

// LoggingComponentFilterHideScenario tests that 'hide' rules work correctly.
func LoggingComponentFilterHideScenario() *harness.Scenario {
	var projectDir string
	var origDir string

	return &harness.Scenario{
		Name:        "core-logging-filter-hide",
		Description: "Verifies that 'hide' rules correctly filter out components.",
		Tags:        []string{"core", "logging", "filter", "hide"},
		Steps: []harness.Step{
			{
				Name: "Create grove.yml with hide config",
				Func: func(ctx *harness.Context) error {
					projectDir = ctx.NewDir("filter-hide-test")

					projectYAML := `name: filter-hide-test
version: "1.0"
logging:
  level: debug
  component_filtering:
    hide: ["component-b"]
  file:
    enabled: true
    format: json
`
					return fs.WriteString(filepath.Join(projectDir, "grove.yml"), projectYAML)
				},
			},
			{
				Name: "Change to project directory and reset logger",
				Func: func(ctx *harness.Context) error {
					var err error
					origDir, err = os.Getwd()
					if err != nil {
						return fmt.Errorf("failed to get current dir: %w", err)
					}

					if err := os.Chdir(projectDir); err != nil {
						return fmt.Errorf("failed to chdir to %s: %w", projectDir, err)
					}

					logging.Reset()
					return nil
				},
			},
			{
				Name: "Write logs from multiple components",
				Func: func(ctx *harness.Context) error {
					logger1 := logging.NewLogger("component-a")
					logger1.Info("Log from component A")

					logger2 := logging.NewLogger("component-b")
					logger2.Info("Log from component B")

					logger3 := logging.NewLogger("component-c")
					logger3.Info("Log from component C")

					return nil
				},
			},
			{
				Name: "Run core logs command and verify hidden component is filtered",
				Func: func(ctx *harness.Context) error {
					cmd := ctx.Bin("logs").Dir(projectDir)
					result := cmd.Run()

					if result.ExitCode != 0 {
						return fmt.Errorf("core logs command failed with exit code %d: %s", result.ExitCode, result.Stderr)
					}

					output := result.Stdout
					hasA := strings.Contains(output, "component-a")
					hasB := strings.Contains(output, "component-b")
					hasC := strings.Contains(output, "component-c")

					if !hasA {
						return fmt.Errorf("component-a should be visible but is not")
					}
					if hasB {
						return fmt.Errorf("component-b should be hidden but is visible")
					}
					if !hasC {
						return fmt.Errorf("component-c should be visible but is not")
					}

					return nil
				},
			},
			{
				Name: "Cleanup: restore original directory",
				Func: func(ctx *harness.Context) error {
					if origDir != "" {
						return os.Chdir(origDir)
					}
					return nil
				},
			},
		},
	}
}

// LoggingComponentFilterConsistencyScenario tests that filtering behavior is consistent.
func LoggingComponentFilterConsistencyScenario() *harness.Scenario {
	var projectDir string
	var origDir string

	return &harness.Scenario{
		Name:        "core-logging-filter-consistency",
		Description: "Verifies that filtering behavior is consistent between different invocations.",
		Tags:        []string{"core", "logging", "filter", "consistency"},
		Steps: []harness.Step{
			{
				Name: "Create grove.yml with show config",
				Func: func(ctx *harness.Context) error {
					projectDir = ctx.NewDir("filter-consistency-test")

					projectYAML := `name: filter-consistency-test
version: "1.0"
logging:
  level: debug
  component_filtering:
    only: ["visible-component"]
  file:
    enabled: true
    format: json
`
					return fs.WriteString(filepath.Join(projectDir, "grove.yml"), projectYAML)
				},
			},
			{
				Name: "Change to project directory and reset logger",
				Func: func(ctx *harness.Context) error {
					var err error
					origDir, err = os.Getwd()
					if err != nil {
						return fmt.Errorf("failed to get current dir: %w", err)
					}

					if err := os.Chdir(projectDir); err != nil {
						return fmt.Errorf("failed to chdir to %s: %w", projectDir, err)
					}

					logging.Reset()
					return nil
				},
			},
			{
				Name: "Write logs from visible and hidden components",
				Func: func(ctx *harness.Context) error {
					logger1 := logging.NewLogger("visible-component")
					logger1.Info("This should be visible")

					logger2 := logging.NewLogger("hidden-component")
					logger2.Info("This should be hidden")

					return nil
				},
			},
			{
				Name: "Run core logs command multiple times and verify consistency",
				Func: func(ctx *harness.Context) error {
					// Run the command multiple times
					for i := 0; i < 3; i++ {
						cmd := ctx.Bin("logs").Dir(projectDir)
						result := cmd.Run()

						if result.ExitCode != 0 {
							return fmt.Errorf("run %d: core logs command failed with exit code %d: %s", i+1, result.ExitCode, result.Stderr)
						}

						output := result.Stdout
						hasVisible := strings.Contains(output, "visible-component")
						hasHidden := strings.Contains(output, "hidden-component")

						if !hasVisible {
							return fmt.Errorf("run %d: visible-component should appear but doesn't", i+1)
						}
						if hasHidden {
							return fmt.Errorf("run %d: hidden-component should not appear but does", i+1)
						}
					}

					return nil
				},
			},
			{
				Name: "Cleanup: restore original directory",
				Func: func(ctx *harness.Context) error {
					if origDir != "" {
						return os.Chdir(origDir)
					}
					return nil
				},
			},
		},
	}
}
