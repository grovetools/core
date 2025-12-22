package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattsolo1/grove-core/logging"
	"github.com/mattsolo1/grove-tend/pkg/fs"
	"github.com/mattsolo1/grove-tend/pkg/git"
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

// WorkspaceRootLogPlacementScenario tests that logs are placed in the workspace root
// even when commands are executed from subdirectories, worktrees, or ecosystem sub-projects.
func WorkspaceRootLogPlacementScenario() *harness.Scenario {
	var projectDir string
	var subDir string
	var worktreeDir string
	var origDir string

	return &harness.Scenario{
		Name:        "workspace-root-log-placement",
		Description: "Verifies logs are always placed in workspace root regardless of execution directory",
		Tags:        []string{"core", "logging", "workspace"},
		Steps: []harness.Step{
			{
				Name: "Create project with subdirectories and worktree",
				Func: func(ctx *harness.Context) error {
					projectDir = ctx.NewDir("log-placement-proj")

					// Create grove.yml with file logging enabled
					groveYML := `name: log-placement-proj
version: "1.0"
logging:
  file:
    enabled: true
`
					if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), groveYML); err != nil {
						return err
					}

					// Initialize git repo
					repo, err := git.SetupTestRepo(projectDir)
					if err != nil {
						return fmt.Errorf("failed to setup git repo: %w", err)
					}
					if err := repo.AddCommit("initial commit"); err != nil {
						return err
					}

					// Create a subdirectory
					subDir = filepath.Join(projectDir, "src", "pkg", "deep")
					if err := fs.CreateDir(subDir); err != nil {
						return err
					}

					// Create a worktree
					worktreeDir = filepath.Join(projectDir, ".grove-worktrees", "feature-branch")
					if err := repo.CreateWorktree(worktreeDir, "feature-branch"); err != nil {
						return err
					}

					// Create grove.yml in worktree as well
					if err := fs.WriteString(filepath.Join(worktreeDir, "grove.yml"), groveYML); err != nil {
						return err
					}

					return nil
				},
			},
			{
				Name: "Save original directory",
				Func: func(ctx *harness.Context) error {
					var err error
					origDir, err = os.Getwd()
					return err
				},
			},
			{
				Name: "Execute from project root and verify log placement",
				Func: func(ctx *harness.Context) error {
					if err := os.Chdir(projectDir); err != nil {
						return err
					}
					logging.Reset()

					log := logging.NewLogger("root-test")
					log.Info("log from project root")

					logsDir := filepath.Join(projectDir, ".grove", "logs")
					if err := verifyLogFile(logsDir, "log from project root"); err != nil {
						return fmt.Errorf("root execution: %w", err)
					}
					return nil
				},
			},
			{
				Name: "Execute from deep subdirectory and verify logs go to root",
				Func: func(ctx *harness.Context) error {
					if err := os.Chdir(subDir); err != nil {
						return err
					}
					logging.Reset()

					log := logging.NewLogger("subdir-test")
					log.Info("log from subdirectory")

					// Logs should still go to project root, not subdirectory
					logsDir := filepath.Join(projectDir, ".grove", "logs")
					if err := verifyLogFile(logsDir, "log from subdirectory"); err != nil {
						return fmt.Errorf("subdirectory execution: %w", err)
					}

					// Verify no logs were created in subdirectory
					subdirLogsDir := filepath.Join(subDir, ".grove", "logs")
					if _, err := os.Stat(subdirLogsDir); !os.IsNotExist(err) {
						return fmt.Errorf("logs incorrectly created in subdirectory at %s", subdirLogsDir)
					}

					return nil
				},
			},
			{
				Name: "Execute from worktree and verify logs go to worktree root",
				Func: func(ctx *harness.Context) error {
					if err := os.Chdir(worktreeDir); err != nil {
						return err
					}
					logging.Reset()

					log := logging.NewLogger("worktree-test")
					log.Info("log from worktree")

					// Logs should go to the worktree's own .grove/logs directory
					worktreeLogsDir := filepath.Join(worktreeDir, ".grove", "logs")
					if err := verifyLogFile(worktreeLogsDir, "log from worktree"); err != nil {
						return fmt.Errorf("worktree execution: %w", err)
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

// EcosystemLogPlacementScenario tests log placement in ecosystem configurations
// with sub-projects and ecosystem worktrees.
func EcosystemLogPlacementScenario() *harness.Scenario {
	var ecoRootDir string
	var subProjDir string
	var ecoWorktreeDir string
	var linkedSubProjWorktreeDir string
	var origDir string

	return &harness.Scenario{
		Name:        "ecosystem-log-placement",
		Description: "Verifies logs are placed correctly in ecosystem roots and sub-projects",
		Tags:        []string{"core", "logging", "workspace", "ecosystem"},
		Steps: []harness.Step{
			{
				Name: "Create ecosystem with sub-project",
				Func: func(ctx *harness.Context) error {
					homeDir := ctx.HomeDir()

					// Create ecosystem root
					ecoRootDir = filepath.Join(homeDir, "my-ecosystem")
					groveYMLEco := `name: my-ecosystem
version: "1.0"
workspaces: ['my-subproject']
logging:
  file:
    enabled: true
`
					if err := fs.WriteString(filepath.Join(ecoRootDir, "grove.yml"), groveYMLEco); err != nil {
						return err
					}

					ecoRepo, err := git.SetupTestRepo(ecoRootDir)
					if err != nil {
						return err
					}
					if err := ecoRepo.AddCommit("initial ecosystem commit"); err != nil {
						return err
					}

					// Create sub-project
					subProjDir = filepath.Join(ecoRootDir, "my-subproject")
					groveYMLSubProj := `name: my-subproject
version: "1.0"
logging:
  file:
    enabled: true
`
					if err := fs.WriteString(filepath.Join(subProjDir, "grove.yml"), groveYMLSubProj); err != nil {
						return err
					}

					subProjRepo, err := git.SetupTestRepo(subProjDir)
					if err != nil {
						return err
					}
					if err := subProjRepo.AddCommit("initial sub-project commit"); err != nil {
						return err
					}

					// Create ecosystem worktree
					ecoWorktreeDir = filepath.Join(ecoRootDir, ".grove-worktrees", "eco-feature")
					if err := ecoRepo.CreateWorktree(ecoWorktreeDir, "eco-feature"); err != nil {
						return err
					}
					if err := fs.WriteString(filepath.Join(ecoWorktreeDir, "grove.yml"), groveYMLEco); err != nil {
						return err
					}

					// Create linked sub-project worktree inside ecosystem worktree
					linkedSubProjWorktreeDir = filepath.Join(ecoWorktreeDir, "my-subproject")
					if err := subProjRepo.CreateWorktree(linkedSubProjWorktreeDir, "linked-feature"); err != nil {
						return err
					}
					if err := fs.WriteString(filepath.Join(linkedSubProjWorktreeDir, "grove.yml"), groveYMLSubProj); err != nil {
						return err
					}

					return nil
				},
			},
			{
				Name: "Save original directory",
				Func: func(ctx *harness.Context) error {
					var err error
					origDir, err = os.Getwd()
					return err
				},
			},
			{
				Name: "Execute from ecosystem root and verify log placement",
				Func: func(ctx *harness.Context) error {
					if err := os.Chdir(ecoRootDir); err != nil {
						return err
					}
					logging.Reset()

					log := logging.NewLogger("eco-root-test")
					log.Info("log from ecosystem root")

					logsDir := filepath.Join(ecoRootDir, ".grove", "logs")
					if err := verifyLogFile(logsDir, "log from ecosystem root"); err != nil {
						return fmt.Errorf("ecosystem root execution: %w", err)
					}
					return nil
				},
			},
			{
				Name: "Execute from sub-project and verify logs go to sub-project root",
				Func: func(ctx *harness.Context) error {
					if err := os.Chdir(subProjDir); err != nil {
						return err
					}
					logging.Reset()

					log := logging.NewLogger("subproj-test")
					log.Info("log from sub-project")

					// Logs should go to the sub-project's own .grove/logs directory
					subProjLogsDir := filepath.Join(subProjDir, ".grove", "logs")
					if err := verifyLogFile(subProjLogsDir, "log from sub-project"); err != nil {
						return fmt.Errorf("sub-project execution: %w", err)
					}

					return nil
				},
			},
			{
				Name: "Execute from ecosystem worktree and verify logs go to ecosystem worktree",
				Func: func(ctx *harness.Context) error {
					if err := os.Chdir(ecoWorktreeDir); err != nil {
						return err
					}
					logging.Reset()

					log := logging.NewLogger("eco-worktree-test")
					log.Info("log from ecosystem worktree")

					ecoWorktreeLogsDir := filepath.Join(ecoWorktreeDir, ".grove", "logs")
					if err := verifyLogFile(ecoWorktreeLogsDir, "log from ecosystem worktree"); err != nil {
						return fmt.Errorf("ecosystem worktree execution: %w", err)
					}

					return nil
				},
			},
			{
				Name: "Execute from linked sub-project worktree and verify correct placement",
				Func: func(ctx *harness.Context) error {
					if err := os.Chdir(linkedSubProjWorktreeDir); err != nil {
						return err
					}
					logging.Reset()

					log := logging.NewLogger("linked-subproj-worktree-test")
					log.Info("log from linked sub-project worktree")

					// Logs should go to the linked sub-project worktree's own directory
					linkedLogsDir := filepath.Join(linkedSubProjWorktreeDir, ".grove", "logs")
					if err := verifyLogFile(linkedLogsDir, "log from linked sub-project worktree"); err != nil {
						return fmt.Errorf("linked sub-project worktree execution: %w", err)
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

// verifyLogFile is a helper function that checks if a log file exists in the given directory
// and contains the expected message.
func verifyLogFile(logsDir, expectedMessage string) error {
	// Check if logs directory exists
	if _, err := os.Stat(logsDir); os.IsNotExist(err) {
		return fmt.Errorf("logs directory does not exist at %s", logsDir)
	}

	// List all files in the logs directory
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		return fmt.Errorf("failed to read logs directory: %w", err)
	}

	// Find the most recently modified non-empty file
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

	// Verify the log contains the expected message
	if !strings.Contains(logContent, expectedMessage) {
		return fmt.Errorf("log file does not contain expected message '%s'", expectedMessage)
	}

	return nil
}
