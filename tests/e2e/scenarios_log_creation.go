package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattsolo1/grove-core/logging"
	"github.com/mattsolo1/grove-tend/pkg/fs"
	"github.com/mattsolo1/grove-tend/pkg/harness"
)

// WorkspaceLogCreationScenario tests that logs can be created in a workspace project's .grove/logs dir.
func WorkspaceLogCreationScenario() *harness.Scenario {
	var projectDir string
	var origDir string

	return &harness.Scenario{
		Name:        "workspace-log-creation",
		Description: "Verifies logs can be created in a workspace's .grove/logs directory",
		Tags:        []string{"core", "logging", "workspace"},
		Steps: []harness.Step{
			{
				Name: "Create grove.yml with file logging enabled",
				Func: func(ctx *harness.Context) error {
					projectDir = ctx.NewDir("log-test-proj")

					groveYML := `name: log-test-proj
version: "1.0"
logging:
  file:
    enabled: true
`
					return fs.WriteString(filepath.Join(projectDir, "grove.yml"), groveYML)
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
				Name: "Create logger and write test message",
				Func: func(ctx *harness.Context) error {
					log := logging.NewLogger("log-test-app")
					log.Info("verifying log file creation")
					return nil
				},
			},
			{
				Name: "Verify log directory was created",
				Func: func(ctx *harness.Context) error {
					logsDir := filepath.Join(projectDir, ".grove", "logs")
					if _, err := os.Stat(logsDir); os.IsNotExist(err) {
						return fmt.Errorf("logs directory was not created at %s", logsDir)
					}
					return nil
				},
			},
			{
				Name: "Find and verify log file content",
				Func: func(ctx *harness.Context) error {
					logsDir := filepath.Join(projectDir, ".grove", "logs")

					// List all files in the logs directory
					entries, err := os.ReadDir(logsDir)
					if err != nil {
						return fmt.Errorf("failed to read logs directory: %w", err)
					}

					// Find the most recently modified file with content
					var latestFile string
					var latestModTime int64

					for _, entry := range entries {
						if entry.IsDir() {
							continue
						}

						info, err := entry.Info()
						if err != nil {
							continue
						}

						// Only consider non-empty files
						if info.Size() == 0 {
							continue
						}

						if info.ModTime().Unix() > latestModTime {
							latestModTime = info.ModTime().Unix()
							latestFile = filepath.Join(logsDir, entry.Name())
						}
					}

					if latestFile == "" {
						return fmt.Errorf("no log files found in %s", logsDir)
					}

					// Read the log file content
					content, err := os.ReadFile(latestFile)
					if err != nil {
						return fmt.Errorf("failed to read log file %s: %w", latestFile, err)
					}

					logContent := string(content)

					// Verify the log contains the expected component and message
					if !strings.Contains(logContent, "log-test-app") {
						return fmt.Errorf("log file does not contain expected component 'log-test-app'")
					}

					if !strings.Contains(logContent, "verifying log file creation") {
						return fmt.Errorf("log file does not contain expected message 'verifying log file creation'")
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
