package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mattsolo1/grove-tend/pkg/fs"
	"github.com/mattsolo1/grove-tend/pkg/harness"
)

// LogsCLIFilteringScenario tests the core logs CLI filtering flags.
func LogsCLIFilteringScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "core-logs-cli-filtering",
		Description: "Tests CLI flags for log filtering (--show-all, --component, --also-show, etc.)",
		Tags:        []string{"core", "logging", "cli", "filtering"},
		Steps: []harness.Step{
			harness.NewStep("Setup test logs and config", func(ctx *harness.Context) error {
				projectDir := ctx.RootDir

				// Create grove.yml with component filtering
				groveYML := `name: log-filtering-test
version: "1.0"
extensions:
  logging:
    file:
      enabled: true
      format: json
    groups:
      backend: [api, db]
    component_filtering:
      hide:
        - cache
`
				if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), groveYML); err != nil {
					return fmt.Errorf("failed to write grove.yml: %w", err)
				}

				// Create log file with entries from various components
				logContent := `{"component":"api","level":"info","msg":"API server started","time":"2023-01-01T12:00:00Z"}
{"component":"db","level":"info","msg":"Database connected","time":"2023-01-01T12:00:01Z"}
{"component":"cache","level":"warn","msg":"Cache is cold","time":"2023-01-01T12:00:02Z"}
{"component":"frontend","level":"info","msg":"Component rendered","time":"2023-01-01T12:00:03Z"}
{"component":"grove-mcp","level":"debug","msg":"Internal ecosystem log","time":"2023-01-01T12:00:04Z"}
`
				logsDir := filepath.Join(projectDir, ".grove", "logs")
				if err := fs.EnsureDir(logsDir); err != nil {
					return fmt.Errorf("failed to create logs directory: %w", err)
				}

				logFile := filepath.Join(logsDir, "workspace-2023-01-01.log")
				if err := fs.WriteString(logFile, logContent); err != nil {
					return fmt.Errorf("failed to write log file: %w", err)
				}

				return nil
			}),
			harness.NewStep("Test default filtering", func(ctx *harness.Context) error {
				coreBinary, err := findCoreBinary()
				if err != nil {
					return err
				}

				// Run logs command - should hide 'cache' and 'grove-mcp'
				cmd := ctx.Command(coreBinary, "logs", "--json")
				result := cmd.Run()

				if result.ExitCode != 0 {
					return fmt.Errorf("logs command failed with exit code %d: %s", result.ExitCode, result.Stderr)
				}

				output := result.Stdout

				// Verify visible logs
				if !strings.Contains(output, `"component":"api"`) {
					return fmt.Errorf("api logs should be visible")
				}
				if !strings.Contains(output, `"component":"db"`) {
					return fmt.Errorf("db logs should be visible")
				}
				if !strings.Contains(output, `"component":"frontend"`) {
					return fmt.Errorf("frontend logs should be visible")
				}

				// Verify hidden logs
				if strings.Contains(output, `"component":"cache"`) {
					return fmt.Errorf("cache logs should be hidden by config")
				}
				if strings.Contains(output, `"component":"grove-mcp"`) {
					return fmt.Errorf("grove-mcp logs should be hidden by default")
				}

				return nil
			}),
			harness.NewStep("Test --show-all flag", func(ctx *harness.Context) error {
				coreBinary, err := findCoreBinary()
				if err != nil {
					return err
				}

				cmd := ctx.Command(coreBinary, "logs", "--show-all", "--json")
				result := cmd.Run()

				if result.ExitCode != 0 {
					return fmt.Errorf("logs --show-all failed with exit code %d: %s", result.ExitCode, result.Stderr)
				}

				output := result.Stdout

				// All logs should be visible
				if !strings.Contains(output, `"component":"cache"`) {
					return fmt.Errorf("cache logs should be visible with --show-all")
				}
				if !strings.Contains(output, `"component":"grove-mcp"`) {
					return fmt.Errorf("grove-mcp logs should be visible with --show-all")
				}

				return nil
			}),
			harness.NewStep("Test --component flag", func(ctx *harness.Context) error {
				coreBinary, err := findCoreBinary()
				if err != nil {
					return err
				}

				cmd := ctx.Command(coreBinary, "logs", "--component", "db,frontend", "--json")
				result := cmd.Run()

				if result.ExitCode != 0 {
					return fmt.Errorf("logs --component failed with exit code %d: %s", result.ExitCode, result.Stderr)
				}

				output := result.Stdout

				// Only db and frontend should be visible
				if strings.Contains(output, `"component":"api"`) {
					return fmt.Errorf("api logs should be hidden with --component=db,frontend")
				}
				if !strings.Contains(output, `"component":"db"`) {
					return fmt.Errorf("db logs should be visible")
				}
				if !strings.Contains(output, `"component":"frontend"`) {
					return fmt.Errorf("frontend logs should be visible")
				}
				if strings.Contains(output, `"component":"cache"`) {
					return fmt.Errorf("cache logs should be hidden")
				}

				return nil
			}),
			harness.NewStep("Test --also-show flag", func(ctx *harness.Context) error {
				coreBinary, err := findCoreBinary()
				if err != nil {
					return err
				}

				cmd := ctx.Command(coreBinary, "logs", "--also-show", "cache", "--json")
				result := cmd.Run()

				if result.ExitCode != 0 {
					return fmt.Errorf("logs --also-show failed with exit code %d: %s", result.ExitCode, result.Stderr)
				}

				output := result.Stdout

				// cache should now be visible
				if !strings.Contains(output, `"component":"cache"`) {
					return fmt.Errorf("cache logs should be visible with --also-show=cache")
				}
				// grove-mcp still hidden
				if strings.Contains(output, `"component":"grove-mcp"`) {
					return fmt.Errorf("grove-mcp should still be hidden")
				}

				return nil
			}),
			harness.NewStep("Test --also-show with group", func(ctx *harness.Context) error {
				coreBinary, err := findCoreBinary()
				if err != nil {
					return err
				}

				cmd := ctx.Command(coreBinary, "logs", "--also-show", "grove-ecosystem", "--json")
				result := cmd.Run()

				if result.ExitCode != 0 {
					return fmt.Errorf("logs --also-show=grove-ecosystem failed with exit code %d: %s", result.ExitCode, result.Stderr)
				}

				output := result.Stdout

				// grove-mcp should now be visible
				if !strings.Contains(output, `"component":"grove-mcp"`) {
					return fmt.Errorf("grove-mcp logs should be visible with --also-show=grove-ecosystem")
				}

				return nil
			}),
			harness.NewStep("Test 'only' config rule", func(ctx *harness.Context) error {
				projectDir := ctx.RootDir

				// Update grove.yml to use 'only' rule
				groveYML := `name: log-filtering-test
version: "1.0"
extensions:
  logging:
    file:
      enabled: true
      format: json
    groups:
      backend: [api, db]
    component_filtering:
      only:
        - backend
`
				if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), groveYML); err != nil {
					return fmt.Errorf("failed to write grove.yml: %w", err)
				}

				coreBinary, err := findCoreBinary()
				if err != nil {
					return err
				}

				cmd := ctx.Command(coreBinary, "logs", "--json")
				result := cmd.Run()

				if result.ExitCode != 0 {
					return fmt.Errorf("logs command failed with exit code %d: %s", result.ExitCode, result.Stderr)
				}

				output := result.Stdout

				// Only backend group (api, db) should be visible
				if !strings.Contains(output, `"component":"api"`) {
					return fmt.Errorf("api should be visible (in backend group)")
				}
				if !strings.Contains(output, `"component":"db"`) {
					return fmt.Errorf("db should be visible (in backend group)")
				}
				if strings.Contains(output, `"component":"frontend"`) {
					return fmt.Errorf("frontend should be hidden (not in backend group)")
				}

				return nil
			}),
			harness.NewStep("Test --component overrides config 'only'", func(ctx *harness.Context) error {
				coreBinary, err := findCoreBinary()
				if err != nil {
					return err
				}

				cmd := ctx.Command(coreBinary, "logs", "--component", "frontend", "--json")
				result := cmd.Run()

				if result.ExitCode != 0 {
					return fmt.Errorf("logs --component failed with exit code %d: %s", result.ExitCode, result.Stderr)
				}

				output := result.Stdout

				// Only frontend should be visible
				if strings.Contains(output, `"component":"api"`) {
					return fmt.Errorf("api should be hidden")
				}
				if !strings.Contains(output, `"component":"frontend"`) {
					return fmt.Errorf("frontend should be visible")
				}

				return nil
			}),
		},
	}
}
