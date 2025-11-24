package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mattsolo1/grove-tend/pkg/fs"
	"github.com/mattsolo1/grove-tend/pkg/harness"
	"github.com/mattsolo1/grove-tend/pkg/tui"
)

// LoggingTUITestScenario tests the interactive logs TUI.
func LoggingTUITestScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "core-logs-tui-interactive",
		Description: "Tests the interactive logs TUI functionality including navigation, filtering, and follow mode.",
		Tags:        []string{"core", "logging", "tui", "interactive"},
		LocalOnly:   true, // TUI tests require tmux
		Steps: []harness.Step{
			harness.NewStep("Setup log files and config", func(ctx *harness.Context) error {
				// Use RootDir directly since StartTUI sets working directory to RootDir
				projectDir := ctx.RootDir

				// Create grove.yml
				groveYAML := `name: tui-log-test
version: "1.0"
logging:
  level: debug
  file:
    enabled: true
    format: json
`
				if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), groveYAML); err != nil {
					return fmt.Errorf("failed to write grove.yml: %w", err)
				}

				// Create logs directory
				logsDir := filepath.Join(projectDir, ".grove", "logs")
				if err := os.MkdirAll(logsDir, 0755); err != nil {
					return fmt.Errorf("failed to create logs directory: %w", err)
				}

				// Create log file with structured JSON logs
				logContent := `{"level":"info","component":"test-component","msg":"This is the first test message","time":"2024-01-01T10:00:00Z"}
{"level":"error","component":"another-component","msg":"This is an error message","time":"2024-01-01T10:00:01Z"}
{"level":"debug","component":"test-component","msg":"A third debug message","time":"2024-01-01T10:00:02Z"}
{"level":"info","component":"api-server","msg":"Request completed","time":"2024-01-01T10:00:03Z","data":{"method":"POST","path":"/api/users","status":201,"user":{"id":"usr_123","name":"Alice"}}}
{"level":"info","component":"orchestrator","msg":"Complex workflow completed","time":"2024-01-01T10:00:04Z","workflow":{"id":"wf_abc123","name":"data-pipeline","status":"completed","duration_ms":45230,"stages":[{"name":"extract","status":"success","duration_ms":12000,"records_processed":150000,"source":{"type":"postgres","host":"db.example.com","database":"analytics","table":"events"}},{"name":"transform","status":"success","duration_ms":28000,"transformations":["deduplicate","normalize","enrich"],"errors":[]},{"name":"load","status":"success","duration_ms":5230,"destination":{"type":"bigquery","project":"my-project","dataset":"warehouse","table":"processed_events"},"rows_inserted":148500}],"metadata":{"triggered_by":"scheduler","environment":"production","version":"2.1.0","tags":["etl","daily","critical"],"config":{"parallelism":8,"batch_size":10000,"retry_attempts":3,"timeout_seconds":3600}},"metrics":{"cpu_percent":72.5,"memory_mb":4096,"network_bytes_in":1073741824,"network_bytes_out":536870912}}}
`
				logFile := filepath.Join(logsDir, "workspace-2024-01-01.log")
				if err := fs.WriteString(logFile, logContent); err != nil {
					return fmt.Errorf("failed to write log file: %w", err)
				}

				return nil
			}),
			harness.NewStep("Launch logs TUI", func(ctx *harness.Context) error {
				coreBinary, err := findCoreBinary()
				if err != nil {
					return fmt.Errorf("failed to find core binary: %w", err)
				}

				// StartTUI runs in ctx.RootDir where we created grove.yml
				session, err := ctx.StartTUI(coreBinary, []string{"logs", "-i"})
				if err != nil {
					return fmt.Errorf("failed to start TUI: %w", err)
				}
				ctx.Set("tui_session", session)
				return nil
			}),
			harness.NewStep("Verify initial content", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// Wait for status line to appear - indicates TUI is loaded
				if err := session.WaitForText("Logs:", 10*time.Second); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("TUI did not load within timeout: %w\nContent: %s", err, content)
				}

				// Wait for UI to stabilize
				if err := session.WaitStable(); err != nil {
					return fmt.Errorf("UI did not stabilize: %w", err)
				}

				// Verify log messages are visible
				if err := session.AssertContains("first test message"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("expected 'first test message' not found: %w\nContent: %s", err, content)
				}

				if err := session.AssertContains("error message"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("expected 'error message' not found: %w\nContent: %s", err, content)
				}

				return nil
			}),
			harness.NewStep("Verify ANSI color codes are present", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)
				// Capture raw output to check for ANSI escape codes
				content, err := session.Capture(tui.WithRawOutput())
				if err != nil {
					return fmt.Errorf("failed to capture raw output: %w", err)
				}
				// Check for the common ANSI escape sequence prefix
				if !strings.Contains(content, "\x1b[") {
					return fmt.Errorf("no ANSI escape codes found in TUI output; styles are not being applied")
				}
				return nil
			}),
			harness.NewStep("Verify list navigation", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// Press down to move to next item
				if err := session.SendKeys("Down"); err != nil {
					return fmt.Errorf("failed to send down key: %w", err)
				}

				// Wait for UI to update
				if err := session.WaitStable(); err != nil {
					return fmt.Errorf("UI did not stabilize after navigation: %w", err)
				}

				// Verify we can still see logs
				if err := session.AssertContains("error message"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("navigation test failed, log entry not visible: %w\nContent: %s", err, content)
				}

				return nil
			}),
			harness.NewStep("Navigate to JSON log entry", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// Go to end to select the log entry with nested JSON data
				if err := session.SendKeys("G"); err != nil {
					return fmt.Errorf("failed to send G key: %w", err)
				}

				if err := session.WaitStable(); err != nil {
					return fmt.Errorf("UI did not stabilize after G: %w", err)
				}

				// Verify we're on the complex JSON log entry (5th entry with orchestrator component)
				if err := session.AssertContains("orchestrator"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("expected orchestrator log entry: %w\nContent: %s", err, content)
				}

				return nil
			}),
			harness.NewStep("View JSON tree", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// Press 'J' to view JSON tree
				if err := session.SendKeys("J"); err != nil {
					return fmt.Errorf("failed to send J key: %w", err)
				}

				if err := session.WaitStable(); err != nil {
					return fmt.Errorf("UI did not stabilize after J: %w", err)
				}

				// Wait for JSON tree view indicator
				if err := session.WaitForText("JSON VIEW", 2*time.Second); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("JSON tree view did not appear: %w\nContent: %s", err, content)
				}

				// Verify complex nested JSON data fields are visible
				if err := session.AssertContains("data-pipeline"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("JSON data-pipeline value not visible: %w\nContent: %s", err, content)
				}

				// Verify collapsed nested objects/arrays are shown
				if err := session.AssertContains("stages"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("JSON stages array not visible: %w\nContent: %s", err, content)
				}

				if err := session.AssertContains("metadata"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("JSON metadata field not visible: %w\nContent: %s", err, content)
				}

				return nil
			}),
			harness.NewStep("Close JSON tree view", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// Press Escape to close JSON view
				if err := session.SendKeys("Escape"); err != nil {
					return fmt.Errorf("failed to send Escape key: %w", err)
				}

				if err := session.WaitStable(); err != nil {
					return fmt.Errorf("UI did not stabilize after closing JSON view: %w", err)
				}

				// Wait for logs list to reappear
				if err := session.WaitForText("Logs:", 2*time.Second); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("logs list did not reappear: %w\nContent: %s", err, content)
				}

				return nil
			}),
			harness.NewStep("Verify filtering", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// Press '/' to start filtering
				if err := session.SendKeys("/"); err != nil {
					return fmt.Errorf("failed to send / key: %w", err)
				}

				// Wait for the searching prompt
				if err := session.WaitForText("[SEARCHING:", 2*time.Second); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("filter input prompt did not appear: %w\nContent: %s", err, content)
				}

				// Type component name to filter by
				if err := session.SendKeys("another-component"); err != nil {
					return fmt.Errorf("failed to type filter term: %w", err)
				}

				// Press Enter to apply filter
				if err := session.SendKeys("Enter"); err != nil {
					return fmt.Errorf("failed to press enter: %w", err)
				}

				// Wait for UI to update
				if err := session.WaitStable(); err != nil {
					return fmt.Errorf("UI did not stabilize after filtering: %w", err)
				}

				// Verify filtering worked - status bar should show "1/1 (of 5)" indicating filter reduced results
				if err := session.AssertContains("1/1 (of 5)"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("filter did not reduce results to 1/1 (of 5): %w\nContent: %s", err, content)
				}

				// Verify filtered results - should see the error message from another-component
				if err := session.AssertContains("error message"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("expected filtered content not found: %w\nContent: %s", err, content)
				}

				// Clear filter with Escape
				if err := session.SendKeys("Esc"); err != nil {
					return fmt.Errorf("failed to clear filter: %w", err)
				}

				if err := session.WaitStable(); err != nil {
					return fmt.Errorf("UI did not stabilize after clearing filter: %w", err)
				}

				return nil
			}),
			harness.NewStep("Verify follow mode toggle", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// Press 'f' to toggle follow mode on
				if err := session.SendKeys("f"); err != nil {
					return fmt.Errorf("failed to send f key: %w", err)
				}

				// Wait for the follow indicator
				if err := session.WaitForText("[FOLLOWING]", 2*time.Second); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("follow mode indicator did not appear: %w\nContent: %s", err, content)
				}

				// Press 'f' again to toggle it off
				if err := session.SendKeys("f"); err != nil {
					return fmt.Errorf("failed to send f key again: %w", err)
				}

				// Wait for UI to stabilize
				if err := session.WaitStable(); err != nil {
					return fmt.Errorf("UI did not stabilize after toggling follow off: %w", err)
				}

				// Verify follow indicator is gone
				if err := session.AssertNotContains("[FOLLOWING]"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("follow mode indicator should be gone: %w\nContent: %s", err, content)
				}

				return nil
			}),
			harness.NewStep("Open help menu", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// Press '?' to open help
				if err := session.SendKeys("?"); err != nil {
					return fmt.Errorf("failed to send ? key: %w", err)
				}

				// Wait for UI to stabilize after opening help
				if err := session.WaitStable(); err != nil {
					return fmt.Errorf("UI did not stabilize after opening help: %w", err)
				}

				// Wait for help menu title - "Help" is the default title for the help view
				if err := session.WaitForText("Help", 2*time.Second); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("help menu did not appear: %w\nContent: %s", err, content)
				}

				// Verify help is showing by checking for key binding descriptions
				if err := session.AssertContains("toggle follow"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("help menu content missing: %w\nContent: %s", err, content)
				}

				return nil
			}),
			harness.NewStep("Close help menu", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// Press Escape to close help
				if err := session.SendKeys("Escape"); err != nil {
					return fmt.Errorf("failed to send Escape key: %w", err)
				}

				// Wait for UI to stabilize after closing help
				if err := session.WaitStable(); err != nil {
					return fmt.Errorf("UI did not stabilize after closing help: %w", err)
				}

				// Wait for logs list to reappear
				if err := session.WaitForText("Logs:", 2*time.Second); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("help did not close: %w\nContent: %s", err, content)
				}

				return nil
			}),
			harness.NewStep("Switch to full-screen details view", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// Press Tab to switch focus to viewport (full-screen details)
				if err := session.SendKeys("Tab"); err != nil {
					return fmt.Errorf("failed to send Tab key: %w", err)
				}

				// Wait for UI to stabilize after switching focus
				if err := session.WaitStable(); err != nil {
					return fmt.Errorf("UI did not stabilize after Tab: %w", err)
				}

				// Wait for scrolling mode indicator
				if err := session.WaitForText("[SCROLLING - tab to return]", 2*time.Second); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("viewport focus indicator did not appear: %w\nContent: %s", err, content)
				}

				// Verify full-screen mode shows details panel with "Log Entry Details" header
				if err := session.AssertContains("Log Entry Details"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("full-screen details view should show 'Log Entry Details': %w\nContent: %s", err, content)
				}

				return nil
			}),
			harness.NewStep("Switch back to list view", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// Press Tab again to switch back to list
				if err := session.SendKeys("Tab"); err != nil {
					return fmt.Errorf("failed to send Tab key: %w", err)
				}

				// Wait for UI to stabilize after switching back
				if err := session.WaitStable(); err != nil {
					return fmt.Errorf("UI did not stabilize after Tab back: %w", err)
				}

				// Wait for log list to reappear
				if err := session.WaitForText("first test message", 2*time.Second); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("log list did not reappear after tab: %w\nContent: %s", err, content)
				}

				return nil
			}),
			harness.NewStep("Quit TUI", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// Press 'q' to quit - session cleanup is handled by the harness
				return session.SendKeys("q")
			}),
		},
	}
}

// LoggingTUIVimNavigationScenario tests vim-style navigation in the logs TUI.
func LoggingTUIVimNavigationScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "core-logs-tui-vim-navigation",
		Description: "Tests vim-style navigation keys (j, k, G, gg) in the logs TUI.",
		Tags:        []string{"core", "logging", "tui", "vim"},
		LocalOnly:   true,
		Steps: []harness.Step{
			harness.NewStep("Setup log files", func(ctx *harness.Context) error {
				// Use RootDir directly since StartTUI sets working directory to RootDir
				projectDir := ctx.RootDir

				groveYAML := `name: vim-nav-test
version: "1.0"
logging:
  level: debug
  file:
    enabled: true
    format: json
`
				if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), groveYAML); err != nil {
					return err
				}

				logsDir := filepath.Join(projectDir, ".grove", "logs")
				if err := os.MkdirAll(logsDir, 0755); err != nil {
					return err
				}

				// Create multiple log entries for navigation testing
				var logLines []string
				for i := 1; i <= 10; i++ {
					logLines = append(logLines, fmt.Sprintf(`{"level":"info","component":"nav-test","msg":"Log entry %d","time":"2024-01-01T10:00:%02dZ"}`, i, i))
				}
				logContent := strings.Join(logLines, "\n") + "\n"

				logFile := filepath.Join(logsDir, "workspace-2024-01-01.log")
				if err := fs.WriteString(logFile, logContent); err != nil {
					return err
				}

				return nil
			}),
			harness.NewStep("Launch logs TUI", func(ctx *harness.Context) error {
				coreBinary, err := findCoreBinary()
				if err != nil {
					return err
				}

				// StartTUI runs in ctx.RootDir where we created grove.yml
				session, err := ctx.StartTUI(coreBinary, []string{"logs", "-i"})
				if err != nil {
					return err
				}
				ctx.Set("tui_session", session)
				return nil
			}),
			harness.NewStep("Wait for TUI to load", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)
				if err := session.WaitForText("Logs:", 10*time.Second); err != nil {
					return err
				}
				if err := session.WaitStable(); err != nil {
					return err
				}

				// Verify first log entry is visible on initial load
				if err := session.AssertContains("Log entry 1"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("Log entry 1 not visible on initial load: %w\nContent: %s", err, content)
				}
				return nil
			}),
			harness.NewStep("Test j/k navigation", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// Verify we start at position 1
				if err := session.AssertContains("1/10"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("expected to start at position 1/10: %w\nContent: %s", err, content)
				}

				// Press 'j' to move down
				if err := session.SendKeys("j"); err != nil {
					return err
				}
				if err := session.WaitStable(); err != nil {
					return err
				}

				// Verify position changed to 2/10
				if err := session.AssertContains("2/10"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("j key did not move cursor to position 2/10: %w\nContent: %s", err, content)
				}

				// Press 'k' to move back up
				if err := session.SendKeys("k"); err != nil {
					return err
				}
				if err := session.WaitStable(); err != nil {
					return err
				}

				// Verify position is back to 1/10
				if err := session.AssertContains("1/10"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("k key did not move cursor back to position 1/10: %w\nContent: %s", err, content)
				}

				return nil
			}),
			harness.NewStep("Test G to go to end", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// Press 'G' to go to end
				if err := session.SendKeys("G"); err != nil {
					return err
				}
				if err := session.WaitStable(); err != nil {
					return err
				}

				// Should see the last log entry
				if err := session.AssertContains("Log entry 10"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("expected to see last log entry: %w\nContent: %s", err, content)
				}
				return nil
			}),
			harness.NewStep("Test gg to go to top", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// Press 'g' twice quickly to go to top
				if err := session.SendKeys("g"); err != nil {
					return err
				}
				time.Sleep(100 * time.Millisecond) // Small delay between keypresses
				if err := session.SendKeys("g"); err != nil {
					return err
				}
				if err := session.WaitStable(); err != nil {
					return err
				}

				// Should see the first log entry - this catches a bug where row 1 disappears
				if err := session.AssertContains("Log entry 1"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("Log entry 1 not visible after gg navigation (potential TUI bug): %w\nContent: %s", err, content)
				}
				return nil
			}),
			harness.NewStep("Quit TUI", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)
				return session.SendKeys("q")
			}),
		},
	}
}

// LoggingTUIJsonSearchScenario tests the JSON viewer search functionality.
func LoggingTUIJsonSearchScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "core-logs-tui-json-search",
		Description: "Tests the JSON viewer search functionality with / key, n/N navigation.",
		Tags:        []string{"core", "logging", "tui", "json", "search"},
		LocalOnly:   true,
		Steps: []harness.Step{
			harness.NewStep("Setup log files with JSON data", func(ctx *harness.Context) error {
				projectDir := ctx.RootDir

				groveYAML := `name: json-search-test
version: "1.0"
logging:
  level: debug
  file:
    enabled: true
    format: json
`
				if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), groveYAML); err != nil {
					return err
				}

				logsDir := filepath.Join(projectDir, ".grove", "logs")
				if err := os.MkdirAll(logsDir, 0755); err != nil {
					return err
				}

				// Create a log entry with deeply nested JSON for search testing
				logContent := `{"level":"info","component":"search-test","msg":"Test entry with nested data","time":"2024-01-01T10:00:00Z","data":{"searchable":"findme","nested":{"deep_value":"needleinstack","array":["alpha","beta","gamma"]},"numbers":{"count":42,"total":100}}}
`
				logFile := filepath.Join(logsDir, "workspace-2024-01-01.log")
				if err := fs.WriteString(logFile, logContent); err != nil {
					return err
				}

				return nil
			}),
			harness.NewStep("Launch logs TUI", func(ctx *harness.Context) error {
				coreBinary, err := findCoreBinary()
				if err != nil {
					return err
				}
				session, err := ctx.StartTUI(coreBinary, []string{"logs", "-i"})
				if err != nil {
					return err
				}
				ctx.Set("tui_session", session)
				return session.WaitForText("Logs:", 10*time.Second)
			}),
			harness.NewStep("Open JSON viewer", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// Press 'J' to view JSON tree
				if err := session.SendKeys("J"); err != nil {
					return fmt.Errorf("failed to send J key: %w", err)
				}

				// Wait for JSON view indicator
				if err := session.WaitForText("JSON VIEW", 5*time.Second); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("JSON tree view did not appear: %w\nContent: %s", err, content)
				}

				return nil
			}),
			harness.NewStep("Search for value in JSON", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// Press '/' to start search
				if err := session.SendKeys("/"); err != nil {
					return fmt.Errorf("failed to send / key: %w", err)
				}

				// Type search term
				if err := session.SendKeys("needle"); err != nil {
					return fmt.Errorf("failed to type search term: %w", err)
				}

				// Press Enter to execute search
				if err := session.SendKeys("Enter"); err != nil {
					return fmt.Errorf("failed to press Enter: %w", err)
				}

				if err := session.WaitStable(); err != nil {
					return fmt.Errorf("UI did not stabilize after search: %w", err)
				}

				// Verify search results indicator
				if err := session.AssertContains("[1/1]"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("search result indicator not found: %w\nContent: %s", err, content)
				}

				return nil
			}),
			harness.NewStep("Close JSON viewer and quit", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// Press Escape to close JSON view
				if err := session.SendKeys("Escape"); err != nil {
					return err
				}

				// Wait for logs list to reappear
				if err := session.WaitForText("Logs:", 2*time.Second); err != nil {
					return err
				}

				return session.SendKeys("q")
			}),
		},
	}
}

// LoggingTUIVisualModeYankScenario tests visual mode and yank-to-clipboard functionality.
func LoggingTUIVisualModeYankScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "core-logs-tui-visual-yank",
		Description: "Tests visual line mode (V) and yank to clipboard (y) in the logs TUI.",
		Tags:        []string{"core", "logging", "tui", "visual", "yank"},
		LocalOnly:   true,
		Steps: []harness.Step{
			harness.NewStep("Setup log files", func(ctx *harness.Context) error {
				projectDir := ctx.RootDir

				groveYAML := `name: visual-yank-test
version: "1.0"
logging:
  level: debug
  file:
    enabled: true
    format: json
`
				if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), groveYAML); err != nil {
					return err
				}

				logsDir := filepath.Join(projectDir, ".grove", "logs")
				if err := os.MkdirAll(logsDir, 0755); err != nil {
					return err
				}

				// Create multiple log entries for visual selection
				logContent := `{"level":"info","component":"yank-test","msg":"First entry to copy","time":"2024-01-01T10:00:00Z"}
{"level":"info","component":"yank-test","msg":"Second entry to copy","time":"2024-01-01T10:00:01Z"}
{"level":"info","component":"yank-test","msg":"Third entry to copy","time":"2024-01-01T10:00:02Z"}
`
				logFile := filepath.Join(logsDir, "workspace-2024-01-01.log")
				if err := fs.WriteString(logFile, logContent); err != nil {
					return err
				}

				return nil
			}),
			harness.NewStep("Launch logs TUI", func(ctx *harness.Context) error {
				coreBinary, err := findCoreBinary()
				if err != nil {
					return err
				}
				session, err := ctx.StartTUI(coreBinary, []string{"logs", "-i"})
				if err != nil {
					return err
				}
				ctx.Set("tui_session", session)
				return session.WaitForText("Logs:", 10*time.Second)
			}),
			harness.NewStep("Enter visual mode and verify status", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// Verify we're NOT in visual mode initially
				if err := session.AssertNotContains("[VISUAL]"); err == nil {
					// Good - no VISUAL indicator yet
				}

				// Press 'V' to enter visual mode
				if err := session.SendKeys("V"); err != nil {
					return fmt.Errorf("failed to send V key: %w", err)
				}

				// Wait for visual mode indicator in status bar
				if err := session.WaitForText("[VISUAL]", 2*time.Second); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("visual mode indicator [VISUAL] not found in status bar: %w\nContent: %s", err, content)
				}

				return nil
			}),
			harness.NewStep("Extend selection and verify", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// Press 'j' to extend selection to next line
				if err := session.SendKeys("j"); err != nil {
					return fmt.Errorf("failed to send j key: %w", err)
				}

				if err := session.WaitStable(); err != nil {
					return fmt.Errorf("UI did not stabilize after extending selection: %w", err)
				}

				// Verify still in visual mode after navigation
				if err := session.AssertContains("[VISUAL]"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("visual mode should persist after j navigation: %w\nContent: %s", err, content)
				}

				return nil
			}),
			harness.NewStep("Yank selection and verify exit", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// Press 'y' to yank selection
				if err := session.SendKeys("y"); err != nil {
					return fmt.Errorf("failed to send y key: %w", err)
				}

				// Wait for copy confirmation message
				if err := session.WaitForText("Copied 2 log entries", 2*time.Second); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("copy confirmation not found: %w\nContent: %s", err, content)
				}

				// Verify visual mode exited after yank
				if err := session.WaitStable(); err != nil {
					return err
				}
				if err := session.AssertNotContains("[VISUAL]"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("visual mode should exit after yank: %w\nContent: %s", err, content)
				}

				return nil
			}),
			harness.NewStep("Quit TUI", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)
				return session.SendKeys("q")
			}),
		},
	}
}

// LoggingTUIExistingLogsScenario tests that the TUI loads all existing logs on startup.
func LoggingTUIExistingLogsScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "core-logs-tui-existing-logs",
		Description: "Tests that the logs TUI correctly loads multiple pre-existing log files on startup.",
		Tags:        []string{"core", "logging", "tui", "regression"},
		LocalOnly:   true,
		Steps: []harness.Step{
			harness.NewStep("Setup with multiple existing log files", func(ctx *harness.Context) error {
				projectDir := ctx.RootDir
				groveYAML := `name: existing-logs-test
logging:
  file: { enabled: true, format: json }`
				if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), groveYAML); err != nil {
					return err
				}

				logsDir := filepath.Join(projectDir, ".grove", "logs")
				if err := os.MkdirAll(logsDir, 0755); err != nil {
					return err
				}

				// Create first log file
				logContent1 := `{"level":"info","component":"first-log","msg":"Message from first log file","time":"2024-01-01T10:00:00Z"}` + "\n"
				if err := fs.WriteString(filepath.Join(logsDir, "workspace-2024-01-01.log"), logContent1); err != nil {
					return err
				}

				// Create second log file
				logContent2 := `{"level":"warn","component":"second-log","msg":"Message from second log file","time":"2024-01-02T11:00:00Z"}` + "\n"
				if err := fs.WriteString(filepath.Join(logsDir, "workspace-2024-01-02.log"), logContent2); err != nil {
					return err
				}

				return nil
			}),
			harness.NewStep("Launch logs TUI", func(ctx *harness.Context) error {
				coreBinary, err := findCoreBinary()
				if err != nil {
					return err
				}
				session, err := ctx.StartTUI(coreBinary, []string{"logs", "-i"})
				if err != nil {
					return err
				}
				ctx.Set("tui_session", session)
				return nil
			}),
			harness.NewStep("Verify content from both logs is loaded", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				if err := session.WaitForText("Logs:", 10*time.Second); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("TUI did not load: %w\nContent: %s", err, content)
				}

				if err := session.AssertContains("Message from first log file"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("missing content from first log file: %w\nContent: %s", err, content)
				}

				if err := session.AssertContains("Message from second log file"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("missing content from second log file: %w\nContent: %s", err, content)
				}

				// Verify the status bar shows the correct total count of log entries.
				if err := session.AssertContains("2/2"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("log count in status bar is incorrect, should be 2/2: %w\nContent: %s", err, content)
				}
				return nil
			}),
			harness.NewStep("Quit TUI", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)
				return session.SendKeys("q")
			}),
		},
	}
}

// LoggingTUINewFilesScenario tests that the TUI picks up new log files after launch.
func LoggingTUINewFilesScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "core-logs-tui-new-files",
		Description: "Tests that the logs TUI detects and displays logs from files created after the TUI launches.",
		Tags:        []string{"core", "logging", "tui", "watch"},
		LocalOnly:   true,
		Steps: []harness.Step{
			harness.NewStep("Setup workspace without logs", func(ctx *harness.Context) error {
				projectDir := ctx.RootDir

				// Create grove.yml
				groveYAML := `name: new-files-test
version: "1.0"
logging:
  level: debug
  file:
    enabled: true
    format: json
`
				if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), groveYAML); err != nil {
					return fmt.Errorf("failed to write grove.yml: %w", err)
				}

				// Create empty logs directory (no log files yet)
				logsDir := filepath.Join(projectDir, ".grove", "logs")
				if err := os.MkdirAll(logsDir, 0755); err != nil {
					return fmt.Errorf("failed to create logs directory: %w", err)
				}

				ctx.Set("logs_dir", logsDir)
				return nil
			}),
			harness.NewStep("Launch logs TUI before logs exist", func(ctx *harness.Context) error {
				coreBinary, err := findCoreBinary()
				if err != nil {
					return fmt.Errorf("failed to find core binary: %w", err)
				}

				session, err := ctx.StartTUI(coreBinary, []string{"logs", "-i"})
				if err != nil {
					return fmt.Errorf("failed to start TUI: %w", err)
				}
				ctx.Set("tui_session", session)

				// Wait for TUI to load (shows "No items" when empty)
				if err := session.WaitForText("Logs:", 10*time.Second); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("TUI did not load: %w\nContent: %s", err, content)
				}

				return nil
			}),
			harness.NewStep("Create log file after TUI launch", func(ctx *harness.Context) error {
				logsDir := ctx.Get("logs_dir").(string)

				// Create a log file after the TUI has started
				logContent := `{"level":"info","component":"new-file-test","msg":"Log from new file","time":"2024-01-01T10:00:00Z"}
{"level":"error","component":"new-file-test","msg":"Error from new file","time":"2024-01-01T10:00:01Z"}
`
				logFile := filepath.Join(logsDir, "workspace-2024-01-01.log")
				if err := fs.WriteString(logFile, logContent); err != nil {
					return fmt.Errorf("failed to write log file: %w", err)
				}

				return nil
			}),
			harness.NewStep("Verify TUI picks up new logs", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// Wait for the new log content to appear (with timeout)
				if err := session.WaitForText("Log from new file", 5*time.Second); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("new log content did not appear: %w\nContent: %s", err, content)
				}

				// Also verify the error log appeared
				if err := session.AssertContains("Error from new file"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("error log not found: %w\nContent: %s", err, content)
				}

				return nil
			}),
			harness.NewStep("Append more logs and verify", func(ctx *harness.Context) error {
				logsDir := ctx.Get("logs_dir").(string)
				session := ctx.Get("tui_session").(*tui.Session)

				// Append additional logs to the file
				logFile := filepath.Join(logsDir, "workspace-2024-01-01.log")
				f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0644)
				if err != nil {
					return fmt.Errorf("failed to open log file for append: %w", err)
				}
				_, err = f.WriteString(`{"level":"info","component":"new-file-test","msg":"Appended log entry","time":"2024-01-01T10:00:02Z"}
`)
				f.Close()
				if err != nil {
					return fmt.Errorf("failed to append to log file: %w", err)
				}

				// Wait for appended content to appear
				if err := session.WaitForText("Appended log entry", 5*time.Second); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("appended log did not appear: %w\nContent: %s", err, content)
				}

				return nil
			}),
			harness.NewStep("Quit TUI", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)
				return session.SendKeys("q")
			}),
		},
	}
}
