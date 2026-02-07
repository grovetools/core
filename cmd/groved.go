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

			// Parse intervals from config (0 means use default)
			var gitInterval, sessionInterval, workspaceInterval, planInterval, noteInterval time.Duration
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
