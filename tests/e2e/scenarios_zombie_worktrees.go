package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/mattsolo1/grove-tend/pkg/fs"
	"github.com/mattsolo1/grove-tend/pkg/git"
	"github.com/mattsolo1/grove-tend/pkg/harness"
)

// ZombieWorktreeLogRecreationScenario reproduces the issue where deleted worktree directories
// are recreated by long-running processes attempting to log.
func ZombieWorktreeLogRecreationScenario() *harness.Scenario {
	var projectDir string
	var worktreeDir string
	var bgProcess *exec.Cmd
	var cancel context.CancelFunc

	return &harness.Scenario{
		Name:        "zombie-worktree-log-recreation",
		Description: "Verifies that deleted worktree directories are not recreated by logging.",
		Tags:        []string{"core", "logging", "worktree", "regression"},
		Steps: []harness.Step{
			{
				Name: "Setup project and worktree",
				Func: func(ctx *harness.Context) error {
					projectDir = ctx.NewDir("zombie-test-proj")
					worktreeDir = filepath.Join(projectDir, ".grove-worktrees", "zombie-feature")

					// 1. Create grove.yml with file logging
					groveYML := `name: zombie-test-proj
version: "1.0"
logging:
  file:
    enabled: true
`
					if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), groveYML); err != nil {
						return err
					}

					// 2. Init git repo
					repo, err := git.SetupTestRepo(projectDir)
					if err != nil {
						return err
					}
					if err := repo.AddCommit("initial commit"); err != nil {
						return err
					}

					// 3. Create worktree
					return repo.CreateWorktree(worktreeDir, "zombie-feature")
				},
			},
			{
				Name: "Start background logging process",
				Func: func(ctx *harness.Context) error {
					// This Go program will run in the background, simulating a long-running process
					// that holds onto a single logger instance.
					program := `
package main

import (
	"fmt"
	"os"
	"time"
	"github.com/mattsolo1/grove-core/logging"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: go run main.go <workdir>")
		os.Exit(1)
	}
	workDir := os.Args[1]
	if err := os.Chdir(workDir); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to chdir to %s: %v\n", workDir, err)
		os.Exit(1)
	}

	// Initialize logger ONCE to simulate a long-running process
	log := logging.NewLogger("zombie-logger")

	for {
		log.Info("Background logger is still active.")
		time.Sleep(500 * time.Millisecond)
	}
}
`
					// Write the program to a temporary file
					tmpDir := ctx.NewDir("bg-process")
					programPath := filepath.Join(tmpDir, "main.go")
					if err := fs.WriteString(programPath, program); err != nil {
						return fmt.Errorf("failed to write background program: %w", err)
					}

					// Create a cancellable context for the background process
					var processCtx context.Context
					processCtx, cancel = context.WithCancel(context.Background())

					// Build and run the program
					bgProcess = exec.CommandContext(processCtx, "go", "run", programPath, worktreeDir)
					// Capture output for debugging
					bgProcess.Stdout = os.Stdout
					bgProcess.Stderr = os.Stderr

					if err := bgProcess.Start(); err != nil {
						return fmt.Errorf("failed to start background process: %w", err)
					}

					// Give it a moment to start logging
					time.Sleep(2 * time.Second)

					// Verify initial log file was created in the worktree
					logFiles, err := filepath.Glob(filepath.Join(worktreeDir, ".grove", "logs", "*.log"))
					if err != nil || len(logFiles) == 0 {
						return fmt.Errorf("background logger did not create initial log file in worktree")
					}

					return nil
				},
			},
			{
				Name: "Delete the worktree directory",
				Func: func(ctx *harness.Context) error {
					return os.RemoveAll(worktreeDir)
				},
			},
			{
				Name: "Verify worktree directory is NOT recreated",
				Func: func(ctx *harness.Context) error {
					// Wait to see if the logger process recreates the directory
					time.Sleep(2 * time.Second)

					// This is the core assertion. With the fix, the directory should NOT exist.
					// Initially, this would fail, proving the bug.
					if _, err := os.Stat(worktreeDir); !os.IsNotExist(err) {
						// For debugging the initial failing test, let's see what's in there
						content, _ := os.ReadDir(worktreeDir)
						return fmt.Errorf("worktree directory should not be recreated by the logger. Contents: %v", content)
					}
					return nil
				},
			},
			{
				Name: "Verify logs are redirected to project root",
				Func: func(ctx *harness.Context) error {
					// Find the log file in the main project root
					logFiles, err := filepath.Glob(filepath.Join(projectDir, ".grove", "logs", "*.log"))
					if err != nil || len(logFiles) == 0 {
						return fmt.Errorf("log file was not created in the project root after redirection")
					}

					// Read the log file
					logContent, err := fs.ReadString(logFiles[0])
					if err != nil {
						return fmt.Errorf("failed to read redirected log file: %w", err)
					}

					// Assert that the logs from our background process are present
					// The logs are in text format, so we check for the component name in brackets
					if !contains(logContent, "[zombie-logger]") && !contains(logContent, `"component":"zombie-logger"`) {
						return fmt.Errorf("log content from background process should be redirected to the root log file. Got: %s", logContent)
					}
					return nil
				},
			},
		},
		Teardown: []harness.Step{
			{
				Name: "Stop background process",
				Func: func(ctx *harness.Context) error {
					if cancel != nil {
						cancel() // Ensure the background process is terminated
					}
					if bgProcess != nil {
						bgProcess.Wait() // Wait for it to clean up
					}
					return nil
				},
			},
		},
	}
}

// contains is a simple helper to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && hasSubstring(s, substr))
}

func hasSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
