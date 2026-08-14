package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/grovetools/tend/pkg/fs"
	"github.com/grovetools/tend/pkg/harness"

	"github.com/grovetools/core/pkg/paths"
)

// LogsCLIFilteringScenario tests the core logs CLI filtering flags.
//
// Baselined against the CLI surface `core logs` actually has today:
//   - `--also-show` / `--ignore-hide` were dropped in 99af0f7 when the flag set
//     was reorganized around --scope/--level/--component. `--component` is the
//     surviving way to surface a component the config hides: it is evaluated as
//     a strict whitelist BEFORE the hide rules (GetComponentVisibility step 2 vs
//     step 7), and it resolves group names through both the config's `groups`
//     and the built-in DefaultGroups.
//   - 6244afa made the CLI default to `--level info`, so debug entries need an
//     explicit `--level debug` no matter which component filter admits them.
//     `--show-all` lifts the component rules only; it is not a level override.
func LogsCLIFilteringScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "core-logs-cli-filtering",
		Description: "Tests CLI flags for log filtering (--show-all, --component, --level, etc.)",
		Tags:        []string{"core", "logging", "cli", "filtering"},
		Steps: []harness.Step{
			harness.NewStep("Setup test logs and config", func(ctx *harness.Context) error {
				projectDir := ctx.RootDir

				// Write the fixture logs where the `core logs` child will look:
				// Context.SandboxEnv() exports XDG_STATE_HOME=ctx.StateDir() to
				// every child command, so paths.StateDir() here has to resolve
				// to the same sandboxed tree.
				os.Setenv("XDG_STATE_HOME", ctx.StateDir())

				groveYML := `name: log-filtering-test
version: "1.0"
logging:
  file:
    enabled: true
    format: json
  groups:
    backend: [api, db]
  component_filtering:
    hide:
      - cache
      - grove-ecosystem
  show_current_project: false
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
				logsDir := filepath.Join(paths.StateDir(), "logs", "workspaces", "log-filtering-test")
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
				// Run logs command - should hide 'cache' and 'grove-mcp'
				cmd := ctx.Bin("logs", "--json").Dir(ctx.RootDir)
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
				cmd := ctx.Bin("logs", "--show-all", "--json").Dir(ctx.RootDir)
				result := cmd.Run()

				if result.ExitCode != 0 {
					return fmt.Errorf("logs --show-all failed with exit code %d: %s", result.ExitCode, result.Stderr)
				}

				output := result.Stdout

				// Every component rule is lifted...
				if !strings.Contains(output, `"component":"cache"`) {
					return fmt.Errorf("cache logs should be visible with --show-all")
				}
				// ...but the level threshold is not: grove-mcp only logs at
				// debug, and the CLI still defaults to info.
				if strings.Contains(output, `"component":"grove-mcp"`) {
					return fmt.Errorf("grove-mcp is a debug entry and should stay below the default info level")
				}

				return nil
			}),
			harness.NewStep("Test --show-all with --level debug", func(ctx *harness.Context) error {
				cmd := ctx.Bin("logs", "--show-all", "--level", "debug", "--json").Dir(ctx.RootDir)
				result := cmd.Run()

				if result.ExitCode != 0 {
					return fmt.Errorf("logs --show-all --level debug failed with exit code %d: %s", result.ExitCode, result.Stderr)
				}

				output := result.Stdout

				// With both the component rules and the level threshold
				// lowered, every fixture entry is visible.
				for _, component := range []string{"api", "db", "cache", "frontend", "grove-mcp"} {
					if !strings.Contains(output, fmt.Sprintf(`"component":%q`, component)) {
						return fmt.Errorf("%s logs should be visible with --show-all --level debug", component)
					}
				}

				return nil
			}),
			harness.NewStep("Test --component flag", func(ctx *harness.Context) error {
				cmd := ctx.Bin("logs", "--component", "db,frontend", "--json").Dir(ctx.RootDir)
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
			harness.NewStep("Test --component overrides a config hide rule", func(ctx *harness.Context) error {
				// The successor to the removed --also-show: naming a hidden
				// component explicitly wins over the config's hide list.
				cmd := ctx.Bin("logs", "--component", "cache", "--json").Dir(ctx.RootDir)
				result := cmd.Run()

				if result.ExitCode != 0 {
					return fmt.Errorf("logs --component=cache failed with exit code %d: %s", result.ExitCode, result.Stderr)
				}

				output := result.Stdout

				// cache is hidden by config but whitelisted here
				if !strings.Contains(output, `"component":"cache"`) {
					return fmt.Errorf("cache logs should be visible with --component=cache")
				}
				// the whitelist is strict: everything else drops out
				if strings.Contains(output, `"component":"api"`) {
					return fmt.Errorf("api logs should be hidden with --component=cache")
				}
				if strings.Contains(output, `"component":"grove-mcp"`) {
					return fmt.Errorf("grove-mcp should still be hidden")
				}

				return nil
			}),
			harness.NewStep("Test --component with a group name", func(ctx *harness.Context) error {
				// grove-ecosystem is a built-in group (DefaultGroups) containing
				// grove-mcp; --level debug is needed because its only entry is
				// below the default info threshold.
				cmd := ctx.Bin("logs", "--component", "grove-ecosystem", "--level", "debug", "--json").Dir(ctx.RootDir)
				result := cmd.Run()

				if result.ExitCode != 0 {
					return fmt.Errorf("logs --component=grove-ecosystem failed with exit code %d: %s", result.ExitCode, result.Stderr)
				}

				output := result.Stdout

				// grove-mcp should now be visible
				if !strings.Contains(output, `"component":"grove-mcp"`) {
					return fmt.Errorf("grove-mcp logs should be visible with --component=grove-ecosystem --level=debug")
				}
				if strings.Contains(output, `"component":"api"`) {
					return fmt.Errorf("api logs should be hidden with --component=grove-ecosystem")
				}

				return nil
			}),
			harness.NewStep("Test 'only' config rule", func(ctx *harness.Context) error {
				projectDir := ctx.RootDir

				// Update grove.yml to use 'only' rule
				groveYML := `name: log-filtering-test
version: "1.0"
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

				cmd := ctx.Bin("logs", "--json").Dir(ctx.RootDir)
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
				cmd := ctx.Bin("logs", "--component", "frontend", "--json").Dir(ctx.RootDir)
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
		Teardown: []harness.Step{
			harness.NewStep("Cleanup XDG state", func(ctx *harness.Context) error {
				os.Unsetenv("XDG_STATE_HOME")
				return nil
			}),
		},
	}
}
