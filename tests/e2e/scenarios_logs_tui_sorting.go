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
	"github.com/mattsolo1/grove-tend/pkg/verify"
)

// LoggingTUIChronologicalSortingScenario tests that logs are displayed in chronological order
// when loaded from multiple sources with interleaved timestamps.
func LoggingTUIChronologicalSortingScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "core-logs-tui-chronological-sorting",
		Description: "Tests that the logs TUI displays entries in chronological order by timestamp when loading from multiple sources.",
		Tags:        []string{"core", "logging", "tui", "sorting"},
		LocalOnly:   true,
		Steps: []harness.Step{
			harness.NewStep("Setup logs with interleaved timestamps from multiple files", func(ctx *harness.Context) error {
				projectDir := ctx.RootDir

				// Create grove.yml
				groveYAML := `name: sorting-test
version: "1.0"
logging:
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
					return fmt.Errorf("failed to create logs dir: %w", err)
				}

				// Create multiple log files with intentionally out-of-order timestamps
				// File 1: Has entries at T+1s and T+5s
				file1Logs := `{"component":"alpha","level":"info","msg":"Message A1 (T+1s)","time":"2024-01-01T10:00:01Z"}
{"component":"alpha","level":"info","msg":"Message A2 (T+5s)","time":"2024-01-01T10:00:05Z"}
`
				// File 2: Has entries at T+2s and T+6s
				file2Logs := `{"component":"beta","level":"info","msg":"Message B1 (T+2s)","time":"2024-01-01T10:00:02Z"}
{"component":"beta","level":"info","msg":"Message B2 (T+6s)","time":"2024-01-01T10:00:06Z"}
`
				// File 3: Has entry at T+4s (should be inserted in the middle)
				file3Logs := `{"component":"beta","level":"info","msg":"Message B3 (T+4s)","time":"2024-01-01T10:00:04Z"}
`

				if err := fs.WriteString(filepath.Join(logsDir, "workspace-2024-01-01-1.log"), file1Logs); err != nil {
					return fmt.Errorf("failed to write log file 1: %w", err)
				}
				if err := fs.WriteString(filepath.Join(logsDir, "workspace-2024-01-01-2.log"), file2Logs); err != nil {
					return fmt.Errorf("failed to write log file 2: %w", err)
				}

				// Sleep briefly to ensure different file modification time
				time.Sleep(100 * time.Millisecond)

				if err := fs.WriteString(filepath.Join(logsDir, "workspace-2024-01-01-3.log"), file3Logs); err != nil {
					return fmt.Errorf("failed to write log file 3: %w", err)
				}

				return nil
			}),
			harness.NewStep("Launch logs TUI", func(ctx *harness.Context) error {
				coreBinary, err := FindProjectBinary()
				if err != nil {
					return fmt.Errorf("failed to find core binary: %w", err)
				}

				// Start TUI to read from log files
				session, err := ctx.StartTUI(coreBinary, []string{"logs", "-i"})
				if err != nil {
					return fmt.Errorf("failed to start TUI: %w", err)
				}
				ctx.Set("tui_session", session)
				return nil
			}),
			harness.NewStep("Verify logs are loaded", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// Wait for TUI to load all entries
				if err := session.WaitForText("Logs:", 10*time.Second); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("TUI did not load: %w\nContent: %s", err, content)
				}

				if err := session.WaitStable(); err != nil {
					return fmt.Errorf("UI did not stabilize: %w", err)
				}

				// Verify we have 5 log entries loaded
				if err := session.AssertContains("Logs: 1/5"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("expected 5 log entries: %w\nContent: %s", err, content)
				}

				return nil
			}),
			harness.NewStep("Verify chronological order by navigating list", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// We should start at position 1/5, verify the timestamp is 10:00:01 (T+1s)
				if err := session.AssertContains("10:00:01"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("position 1/5 should show 10:00:01 timestamp: %w\nContent: %s", err, content)
				}
				if err := session.AssertContains("Message A1"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("position 1/5 should show Message A1: %w\nContent: %s", err, content)
				}

				// Move to position 2/5 - should be T+2s (10:00:02)
				if err := session.SendKeys("j"); err != nil {
					return err
				}
				if err := session.WaitStable(); err != nil {
					return err
				}
				if err := session.AssertContains("10:00:02"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("position 2/5 should show 10:00:02 timestamp: %w\nContent: %s", err, content)
				}
				if err := session.AssertContains("Message B1"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("position 2/5 should show Message B1: %w\nContent: %s", err, content)
				}

				// Move to position 3/5 - should be T+4s (10:00:04)
				if err := session.SendKeys("j"); err != nil {
					return err
				}
				if err := session.WaitStable(); err != nil {
					return err
				}
				if err := session.AssertContains("10:00:04"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("position 3/5 should show 10:00:04 timestamp: %w\nContent: %s", err, content)
				}
				if err := session.AssertContains("Message B3"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("position 3/5 should show Message B3: %w\nContent: %s", err, content)
				}

				// Move to position 4/5 - should be T+5s (10:00:05)
				if err := session.SendKeys("j"); err != nil {
					return err
				}
				if err := session.WaitStable(); err != nil {
					return err
				}
				if err := session.AssertContains("10:00:05"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("position 4/5 should show 10:00:05 timestamp: %w\nContent: %s", err, content)
				}
				if err := session.AssertContains("Message A2"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("position 4/5 should show Message A2: %w\nContent: %s", err, content)
				}

				// Move to position 5/5 - should be T+6s (10:00:06)
				if err := session.SendKeys("j"); err != nil {
					return err
				}
				if err := session.WaitStable(); err != nil {
					return err
				}
				if err := session.AssertContains("10:00:06"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("position 5/5 should show 10:00:06 timestamp: %w\nContent: %s", err, content)
				}
				if err := session.AssertContains("Message B2"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("position 5/5 should show Message B2: %w\nContent: %s", err, content)
				}

				return nil
			}),
			harness.NewStep("Verify final position", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// Should be at position 5/5 after navigation
				if err := session.AssertContains("Logs: 5/5"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("expected status to show 5/5: %w\nContent: %s", err, content)
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

// LoggingTUILiveUpdateSortingScenario tests that live log updates are inserted
// in the correct chronological position, not just appended.
func LoggingTUILiveUpdateSortingScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "core-logs-tui-live-update-sorting",
		Description: "Tests that live log updates are inserted in chronological order, even when they have older timestamps.",
		Tags:        []string{"core", "logging", "tui", "sorting", "live"},
		LocalOnly:   true,
		Steps: []harness.Step{
			harness.NewStep("Setup initial logs", func(ctx *harness.Context) error {
				projectDir := ctx.RootDir

				groveYAML := `name: live-sorting-test
version: "1.0"
logging:
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

				// Create initial log entries
				initialLogs := `{"component":"test","level":"info","msg":"Message T+2s","time":"2024-01-01T10:00:02Z"}
{"component":"test","level":"info","msg":"Message T+4s","time":"2024-01-01T10:00:04Z"}
{"component":"test","level":"info","msg":"Message T+6s","time":"2024-01-01T10:00:06Z"}
`
				logFile := filepath.Join(logsDir, "workspace-2024-01-01.log")
				if err := fs.WriteString(logFile, initialLogs); err != nil {
					return err
				}

				ctx.Set("log_file", logFile)
				return nil
			}),
			harness.NewStep("Launch logs TUI", func(ctx *harness.Context) error {
				coreBinary, err := FindProjectBinary()
				if err != nil {
					return err
				}

				session, err := ctx.StartTUI(coreBinary, []string{"logs", "-i"})
				if err != nil {
					return err
				}
				ctx.Set("tui_session", session)

				// Wait for TUI to load
				if err := session.WaitForText("Logs:", 10*time.Second); err != nil {
					return err
				}
				return session.WaitStable()
			}),
			harness.NewStep("Verify initial state", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				return ctx.Verify(func(v *verify.Collector) {
					// Check for total count of 3 (shown as "X/3" in status bar)
					v.Equal("initial count is 3", nil, session.AssertContains("/3"))
					v.Equal("T+2s visible", nil, session.AssertContains("Message T+2s"))
					v.Equal("T+4s visible", nil, session.AssertContains("Message T+4s"))
					v.Equal("T+6s visible", nil, session.AssertContains("Message T+6s"))
				})
			}),
			harness.NewStep("Append log with older timestamp (T+1s)", func(ctx *harness.Context) error {
				logFile := ctx.Get("log_file").(string)

				// Append a log entry that is OLDER than all existing ones
				olderLog := `{"component":"test","level":"info","msg":"Message T+1s (older)","time":"2024-01-01T10:00:01Z"}
`
				f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0644)
				if err != nil {
					return err
				}
				defer f.Close()

				_, err = f.WriteString(olderLog)
				return err
			}),
			harness.NewStep("Verify older entry inserted at beginning", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// Wait for new entry to appear
				if err := session.WaitForText("Message T+1s (older)", 5*time.Second); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("older log entry did not appear: %w\nContent: %s", err, content)
				}

				content, err := session.Capture()
				if err != nil {
					return err
				}

				// Verify chronological order: T+1s should appear before T+2s
				return ctx.Verify(func(v *verify.Collector) {
					// Check total count is now 4 (shown as "X/4" in status bar)
					v.Equal("count is now 4", nil, session.AssertContains("/4"))

					idxT1 := strings.Index(content, "Message T+1s (older)")
					idxT2 := strings.Index(content, "Message T+2s")

					v.True("T+1s appears before T+2s", idxT1 < idxT2 && idxT1 >= 0 && idxT2 >= 0)
				})
			}),
			harness.NewStep("Append log with middle timestamp (T+3s)", func(ctx *harness.Context) error {
				logFile := ctx.Get("log_file").(string)

				// Append a log entry that should go in the MIDDLE
				middleLog := `{"component":"test","level":"info","msg":"Message T+3s (middle)","time":"2024-01-01T10:00:03Z"}
`
				f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0644)
				if err != nil {
					return err
				}
				defer f.Close()

				_, err = f.WriteString(middleLog)
				return err
			}),
			harness.NewStep("Verify middle entry inserted correctly", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// Wait for new entry to appear
				if err := session.WaitForText("Message T+3s (middle)", 5*time.Second); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("middle log entry did not appear: %w\nContent: %s", err, content)
				}

				content, err := session.Capture()
				if err != nil {
					return err
				}

				// Verify chronological order: T+2s < T+3s < T+4s
				return ctx.Verify(func(v *verify.Collector) {
					// Check total count is now 5 (shown as "X/5" in status bar)
					v.Equal("count is now 5", nil, session.AssertContains("/5"))

					idxT2 := strings.Index(content, "Message T+2s")
					idxT3 := strings.Index(content, "Message T+3s (middle)")
					idxT4 := strings.Index(content, "Message T+4s")

					v.True("T+2s before T+3s", idxT2 < idxT3 && idxT2 >= 0 && idxT3 >= 0)
					v.True("T+3s before T+4s", idxT3 < idxT4 && idxT3 >= 0 && idxT4 >= 0)
				})
			}),
			harness.NewStep("Quit TUI", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)
				return session.SendKeys("q")
			}),
		},
	}
}

// LoggingTUIFollowModeSortingScenario tests that follow mode tracks the newest
// timestamp even when older entries are inserted.
func LoggingTUIFollowModeSortingScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "core-logs-tui-follow-mode-sorting",
		Description: "Tests that follow mode correctly tracks the newest log by timestamp, not insertion order.",
		Tags:        []string{"core", "logging", "tui", "sorting", "follow"},
		LocalOnly:   true,
		Steps: []harness.Step{
			harness.NewStep("Setup initial logs", func(ctx *harness.Context) error {
				projectDir := ctx.RootDir

				groveYAML := `name: follow-sorting-test
version: "1.0"
logging:
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

				// Create initial log entries
				initialLogs := `{"component":"test","level":"info","msg":"Message T+1s","time":"2024-01-01T10:00:01Z"}
{"component":"test","level":"info","msg":"Message T+2s","time":"2024-01-01T10:00:02Z"}
`
				logFile := filepath.Join(logsDir, "workspace-2024-01-01.log")
				if err := fs.WriteString(logFile, initialLogs); err != nil {
					return err
				}

				ctx.Set("log_file", logFile)
				return nil
			}),
			harness.NewStep("Launch logs TUI with follow mode", func(ctx *harness.Context) error {
				coreBinary, err := FindProjectBinary()
				if err != nil {
					return err
				}

				// Start with -f flag for follow mode
				session, err := ctx.StartTUI(coreBinary, []string{"logs", "-i", "-f"})
				if err != nil {
					return err
				}
				ctx.Set("tui_session", session)

				// Wait for TUI to load and verify follow mode is on
				if err := session.WaitForText("Logs:", 10*time.Second); err != nil {
					return err
				}

				if err := session.WaitForText("[Follow:ON]", 3*time.Second); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("follow mode not enabled: %w\nContent: %s", err, content)
				}

				return session.WaitStable()
			}),
			harness.NewStep("Append new latest message", func(ctx *harness.Context) error {
				logFile := ctx.Get("log_file").(string)

				// Append a new LATEST message (T+10s)
				latestLog := `{"component":"test","level":"info","msg":"Message T+10s (latest)","time":"2024-01-01T10:00:10Z"}
`
				f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0644)
				if err != nil {
					return err
				}
				defer f.Close()

				_, err = f.WriteString(latestLog)
				return err
			}),
			harness.NewStep("Verify follow mode shows latest message", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// Wait for latest message to appear
				if err := session.WaitForText("Message T+10s (latest)", 5*time.Second); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("latest message did not appear: %w\nContent: %s", err, content)
				}

				// Verify we're at the latest entry (3/3 position)
				if err := session.AssertContains("3/3"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("should be at position 3/3: %w\nContent: %s", err, content)
				}

				return nil
			}),
			harness.NewStep("Append older message while in follow mode", func(ctx *harness.Context) error {
				logFile := ctx.Get("log_file").(string)

				// Append an OLDER message (T+5s) - this should NOT move the view
				olderLog := `{"component":"test","level":"info","msg":"Message T+5s (older)","time":"2024-01-01T10:00:05Z"}
`
				f, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY, 0644)
				if err != nil {
					return err
				}
				defer f.Close()

				_, err = f.WriteString(olderLog)
				return err
			}),
			harness.NewStep("Verify follow mode still shows latest message", func(ctx *harness.Context) error {
				session := ctx.Get("tui_session").(*tui.Session)

				// Wait a moment for the log to be processed
				time.Sleep(1 * time.Second)

				if err := session.WaitStable(); err != nil {
					return err
				}

				// Verify we now have 4 total entries
				if err := session.WaitForText("4/4", 3*time.Second); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("count should be 4/4: %w\nContent: %s", err, content)
				}

				// Crucially: verify we're still showing the latest message (T+10s)
				// Follow mode should keep us at the newest timestamp, not jump to the newly inserted older entry
				if err := session.AssertContains("Message T+10s (latest)"); err != nil {
					content, _ := session.Capture()
					return fmt.Errorf("follow mode should still show latest message: %w\nContent: %s", err, content)
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
