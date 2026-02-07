package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/internal/daemon/collector"
	"github.com/grovetools/core/internal/daemon/engine"
	"github.com/grovetools/core/internal/daemon/pidfile"
	"github.com/grovetools/core/internal/daemon/server"
	"github.com/grovetools/core/internal/daemon/store"
	"github.com/grovetools/core/logging"
	"github.com/grovetools/core/pkg/daemon"
	"github.com/grovetools/core/pkg/paths"
	"github.com/spf13/cobra"
)

// NewGrovedCmd returns the groved daemon command with subcommands.
func NewGrovedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "groved",
		Short: "Grove ecosystem daemon",
		Long:  "Centralized state management daemon for the grove ecosystem.",
	}

	cmd.AddCommand(newGrovedStartCmd())
	cmd.AddCommand(newGrovedStopCmd())
	cmd.AddCommand(newGrovedStatusCmd())
	cmd.AddCommand(newGrovedConfigCmd())
	cmd.AddCommand(newGrovedMonitorCmd())

	return cmd
}

func newGrovedStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the daemon",
		Long:  "Start the grove daemon in foreground mode.",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := logging.NewLogger("groved")
			pidPath := paths.PidFilePath()
			sockPath := paths.SocketPath()

			// 1. Acquire Lock
			if err := pidfile.Acquire(pidPath); err != nil {
				return fmt.Errorf("failed to start: %w", err)
			}
			defer func() {
				if err := pidfile.Release(pidPath); err != nil {
					logger.Errorf("Failed to release pidfile: %v", err)
				}
			}()

			// 2. Load config for daemon settings
			cfg, err := config.LoadDefault()
			if err != nil {
				logger.WithError(err).Warn("Failed to load config, using defaults")
				cfg = &config.Config{}
			}

			// Parse intervals from config with defaults
			// Defaults match the collector defaults: git=10s, session=2s, workspace=30s, plan=30s, note=60s
			gitInterval := 10 * time.Second
			sessionInterval := 2 * time.Second
			workspaceInterval := 30 * time.Second
			planInterval := 30 * time.Second
			noteInterval := 60 * time.Second

			if cfg.Daemon != nil {
				if cfg.Daemon.GitInterval != "" {
					if d, err := time.ParseDuration(cfg.Daemon.GitInterval); err == nil {
						gitInterval = d
					}
				}
				if cfg.Daemon.SessionInterval != "" {
					if d, err := time.ParseDuration(cfg.Daemon.SessionInterval); err == nil {
						sessionInterval = d
					}
				}
				if cfg.Daemon.WorkspaceInterval != "" {
					if d, err := time.ParseDuration(cfg.Daemon.WorkspaceInterval); err == nil {
						workspaceInterval = d
					}
				}
				if cfg.Daemon.PlanInterval != "" {
					if d, err := time.ParseDuration(cfg.Daemon.PlanInterval); err == nil {
						planInterval = d
					}
				}
				if cfg.Daemon.NoteInterval != "" {
					if d, err := time.ParseDuration(cfg.Daemon.NoteInterval); err == nil {
						noteInterval = d
					}
				}
			}

			// 3. Setup Store and Engine
			st := store.New()
			eng := engine.New(st, logger)

			// Register collectors with configured intervals
			eng.Register(collector.NewWorkspaceCollector(workspaceInterval))
			eng.Register(collector.NewGitStatusCollector(gitInterval))
			eng.Register(collector.NewSessionCollector(sessionInterval))
			eng.Register(collector.NewPlanCollector(planInterval))
			eng.Register(collector.NewNoteCollector(noteInterval))

			// 4. Setup Server with engine
			srv := server.New(logger)
			srv.SetEngine(eng)

			// Set running config for introspection
			srv.SetRunningConfig(&server.RunningConfig{
				GitInterval:       gitInterval,
				SessionInterval:   sessionInterval,
				WorkspaceInterval: workspaceInterval,
				PlanInterval:      planInterval,
				NoteInterval:      noteInterval,
				StartedAt:         time.Now(),
			})

			// 5. Handle Signals
			ctx, cancel := context.WithCancel(context.Background())
			stop := make(chan os.Signal, 1)
			signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

			go func() {
				<-stop
				logger.Info("Received stop signal")
				cancel() // Stop the engine

				// Create shutdown context with timeout
				shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer shutdownCancel()

				if err := srv.Shutdown(shutdownCtx); err != nil {
					logger.Errorf("Server shutdown error: %v", err)
				}

				// Explicitly release pidfile before exit in signal handler
				_ = pidfile.Release(pidPath)
				os.Exit(0)
			}()

			// 6. Start Engine in background
			go eng.Start(ctx)

			// 7. Start Server (Blocking)
			logger.WithField("pid", os.Getpid()).Info("Starting daemon")
			if err := srv.ListenAndServe(sockPath); err != nil {
				return fmt.Errorf("server error: %w", err)
			}
			return nil
		},
	}
}

func newGrovedStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the running daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			pidPath := paths.PidFilePath()

			running, pid, err := pidfile.IsRunning(pidPath)
			if err != nil {
				return fmt.Errorf("error checking status: %w", err)
			}

			if !running {
				fmt.Println("Daemon is not running")
				return nil
			}

			// Send SIGTERM
			process, err := os.FindProcess(pid)
			if err != nil {
				return fmt.Errorf("failed to find process %d: %w", pid, err)
			}

			if err := process.Signal(syscall.SIGTERM); err != nil {
				return fmt.Errorf("failed to send stop signal: %w", err)
			}

			fmt.Printf("Sent SIGTERM to process %d\n", pid)
			return nil
		},
	}
}

func newGrovedStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Check daemon status",
		RunE: func(cmd *cobra.Command, args []string) error {
			pidPath := paths.PidFilePath()
			running, pid, err := pidfile.IsRunning(pidPath)

			if err != nil {
				return fmt.Errorf("error: %w", err)
			}

			if running {
				fmt.Printf("Running (PID: %d)\nSocket: %s\n", pid, paths.SocketPath())
			} else {
				fmt.Println("Stopped")
				os.Exit(1) // Return non-zero for stopped state (useful for scripts)
			}
			return nil
		},
	}
}

func newGrovedConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Show running daemon configuration",
		Long:  "Query the running daemon to show its active configuration intervals.",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := daemon.New()
			defer client.Close()

			if !client.IsRunning() {
				fmt.Println("Daemon is not running")
				os.Exit(1)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			cfg, err := client.GetConfig(ctx)
			if err != nil {
				return fmt.Errorf("failed to get config: %w", err)
			}

			fmt.Println("Running Daemon Configuration")
			fmt.Println("============================")
			fmt.Printf("Started At:         %s\n", cfg.StartedAt.Format(time.RFC3339))
			fmt.Printf("Uptime:             %s\n", time.Since(cfg.StartedAt).Round(time.Second))
			fmt.Println()
			fmt.Println("Collector Intervals:")
			fmt.Printf("  Git Status:       %s\n", cfg.GitInterval)
			fmt.Printf("  Session:          %s\n", cfg.SessionInterval)
			fmt.Printf("  Workspace:        %s\n", cfg.WorkspaceInterval)
			fmt.Printf("  Plan Stats:       %s\n", cfg.PlanInterval)
			fmt.Printf("  Note Counts:      %s\n", cfg.NoteInterval)

			return nil
		},
	}
}

func newGrovedMonitorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "monitor",
		Short: "Monitor daemon activity in real-time",
		Long:  "Subscribe to the daemon event stream and print activity logs.",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := daemon.New()
			defer client.Close()

			if !client.IsRunning() {
				fmt.Println("Daemon is not running")
				os.Exit(1)
			}

			ctx, cancel := context.WithCancel(context.Background())

			// Handle Ctrl+C gracefully
			stop := make(chan os.Signal, 1)
			signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
			go func() {
				<-stop
				fmt.Println("\nDisconnecting...")
				cancel()
			}()

			stream, err := client.StreamState(ctx)
			if err != nil {
				return fmt.Errorf("failed to connect to stream: %w", err)
			}

			fmt.Println("Monitoring daemon activity (Ctrl+C to stop)...")
			fmt.Println("==============================================")

			for update := range stream {
				timestamp := time.Now().Format("15:04:05")
				switch update.UpdateType {
				case "initial":
					fmt.Printf("[%s] Connected: %d workspaces loaded\n", timestamp, len(update.Workspaces))
				case "workspaces":
					source := update.Source
					if source == "" {
						source = "unknown"
					}
					if update.Scanned > 0 && update.Scanned != len(update.Workspaces) {
						fmt.Printf("[%s] %s: scanned %d/%d\n", timestamp, formatSource(source), update.Scanned, len(update.Workspaces))
					} else {
						fmt.Printf("[%s] %s: %d workspaces\n", timestamp, formatSource(source), len(update.Workspaces))
					}
				case "sessions":
					fmt.Printf("[%s] Session: %d active\n", timestamp, len(update.Sessions))
				case "focus":
					fmt.Printf("[%s] Focus: %d workspaces\n", timestamp, update.Scanned)
				default:
					fmt.Printf("[%s] Update: %s\n", timestamp, update.UpdateType)
				}
			}

			return nil
		},
	}
}

// formatSource returns a human-readable label for the collector source.
func formatSource(source string) string {
	switch source {
	case "git":
		return "Git Status"
	case "workspace":
		return "Workspace Discovery"
	case "session":
		return "Session"
	case "plan":
		return "Plan Stats"
	case "note":
		return "Note Counts"
	default:
		return source
	}
}
