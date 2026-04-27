package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/grovetools/core/cli"
	"github.com/grovetools/core/config"
	"github.com/grovetools/core/logging"
	"github.com/grovetools/core/pkg/logging/logutil"
	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/workspace"
)

// NewLogsCmd creates the `logs` command.
func NewLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Aggregate and display logs from Grove workspaces",
		Long: `Streams logs from one or more workspaces. By default, shows logs from the
current workspace only. Use --ecosystem to show logs from all workspaces.

Examples:
  # Follow logs from current workspace
  core logs -f

  # Show logs from all workspaces in ecosystem
  core logs --ecosystem -f

  # Get the last 100 log lines in JSON format
  core logs --tail 100 --json

  # Follow logs from specific workspaces
  core logs -f -w my-project,another-project

  # Show only the pretty CLI output (styled)
  core logs --format=pretty

  # Show only the pretty CLI output (plain text, no ANSI)
  core logs --format=pretty-text

  # Show full details with pretty output indented below each line
  core logs --format=full
`,
		RunE: runLogsE,
	}

	cmd.Flags().Bool("json", false, "Output logs in JSON Lines format (shorthand for --format=json)")
	cmd.Flags().String("format", "text", "Output format: text, json, full, rich, pretty, pretty-text")
	cmd.Flags().Bool("compact", false, "Disable spacing between log entries (for pretty/full/rich formats)")
	cmd.Flags().BoolP("tui", "i", false, "Launch the interactive TUI")
	cmd.Flags().Bool("ecosystem", false, "Show logs from all workspaces in the ecosystem")
	cmd.Flags().StringSliceP("workspaces", "w", []string{}, "Filter by specific workspace names (comma-separated)")
	cmd.Flags().BoolP("follow", "f", false, "Follow log output")
	cmd.Flags().Int("tail", -1, "Number of lines to show from the end of the logs (default: all)")

	cmd.Flags().Bool("show-all", false, "Show all logs, ignoring any configured show/hide rules")
	cmd.Flags().StringSlice("component", []string{}, "Show logs only from these components (acts as a strict whitelist)")
	cmd.Flags().StringSlice("also-show", []string{}, "Temporarily show components/groups, overriding hide rules")
	cmd.Flags().StringSlice("ignore-hide", []string{}, "Temporarily show components/groups that would be hidden by config")

	cmd.Flags().Bool("system", false, "Only show global system logs (ignores workspace logs)")
	cmd.Flags().Bool("include-system", false, "Include global system events that have no specific workspace context")

	return cmd
}

// filterStats holds counters for logging statistics.
type filterStats struct {
	total      int
	shown      int
	hidden     int
	lastReason logging.VisibilityReason
	lastRule   []string
}

func runLogsE(cmd *cobra.Command, args []string) error {
	logger := cli.GetLogger(cmd)
	opts := cli.GetOptions(cmd)

	// Load logging config for component filtering, starting with defaults
	logCfg := logging.GetDefaultLoggingConfig()
	if cfg, err := config.LoadDefault(); err == nil {
		_ = cfg.UnmarshalExtension("logging", &logCfg)
	}

	// --- Filter Overrides & Statistics ---
	showAll, _ := cmd.Flags().GetBool("show-all")
	showOnly, _ := cmd.Flags().GetStringSlice("component")
	alsoShow, _ := cmd.Flags().GetStringSlice("also-show")
	ignoreHide, _ := cmd.Flags().GetStringSlice("ignore-hide")

	overrideOpts := &logging.OverrideOptions{
		ShowAll:    showAll,
		ShowOnly:   showOnly,
		AlsoShow:   alsoShow,
		IgnoreHide: ignoreHide,
	}
	stats := &filterStats{}

	ecosystem, _ := cmd.Flags().GetBool("ecosystem")
	wsFilter, _ := cmd.Flags().GetStringSlice("workspaces")
	systemOnly, _ := cmd.Flags().GetBool("system")
	includeSystem, _ := cmd.Flags().GetBool("include-system")

	var workspaces []*workspace.WorkspaceNode

	// Determine which workspaces to show
	if systemOnly {
		// Skip workspace discovery entirely - only show system logs
		workspaces = []*workspace.WorkspaceNode{}
	} else if ecosystem || len(wsFilter) > 0 {
		// 1. Discover all workspaces in ecosystem
		allWorkspaces, err := workspace.GetProjects(logger)
		if err != nil {
			return fmt.Errorf("failed to discover workspaces: %w", err)
		}

		// 2. Filter workspaces if requested
		if len(wsFilter) > 0 {
			filterMap := make(map[string]bool)
			for _, w := range wsFilter {
				filterMap[w] = true
			}
			for _, ws := range allWorkspaces {
				if filterMap[ws.Name] {
					workspaces = append(workspaces, ws)
				}
			}
		} else {
			workspaces = allWorkspaces
		}
	} else {
		// Default: show current workspace only
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}

		// Try to get workspace name from grove.yml, fall back to directory basename
		wsName := filepath.Base(cwd)
		if cfg, err := config.LoadFrom(cwd); err == nil && cfg.Name != "" {
			wsName = cfg.Name
		}

		// Create a WorkspaceNode for the current workspace
		// Note: We don't require grove.yml to exist here - findLogFileForWorkspace
		// handles missing configs gracefully by falling back to .grove/logs/
		workspaces = []*workspace.WorkspaceNode{
			{
				Path: cwd,
				Name: wsName,
			},
		}
	}

	if len(workspaces) == 0 && !systemOnly {
		logger.Info("No matching workspaces found.")
		return nil
	}

	// Check if TUI mode should be used
	tuiMode, _ := cmd.Flags().GetBool("tui")
	follow, _ := cmd.Flags().GetBool("follow")

	if tuiMode {
		return runLogsTUI(workspaces, follow, overrideOpts, systemOnly, includeSystem, ecosystem)
	}

	// 3. Find log files and start tailing
	lineChan := make(chan logutil.TailedLine, 100)
	var wg sync.WaitGroup

	tail, _ := cmd.Flags().GetInt("tail")
	// When following without an explicit `--tail` value, default to 0
	// (stream only new lines) instead of the historical -1 (full
	// replay). Dumping an entire day's log — or, with stale rotated
	// files left over in `.grove/logs/`, an entire multi-month backlog
	// — on every `-f` invocation is jarring and sometimes gigabytes of
	// output. Users who want the old behavior can pass `--tail=-1`
	// explicitly; users who want a bounded replay can pass a positive
	// count.
	if follow && !cmd.Flags().Changed("tail") {
		tail = 0
	}
	jsonOutput, _ := cmd.Flags().GetBool("json")
	format, _ := cmd.Flags().GetString("format")
	compact, _ := cmd.Flags().GetBool("compact")

	// --json is shorthand for --format=json
	if jsonOutput {
		format = "json"
	}

	for _, ws := range workspaces {
		logFile, logsDir, err := logutil.FindLogFileForWorkspace(ws)
		if err != nil {
			// If following and we have a logs directory path, use tailDirectory
			// to wait for files to appear
			if follow && logsDir != "" {
				logger.WithFields(logrus.Fields{
					"workspace": ws.Name,
					"logs_dir":  logsDir,
				}).Debug("Waiting for log files in directory")

				wg.Add(1)
				go logutil.TailDirectory(ws.Name, ws.Path, logsDir, lineChan, &wg, follow, tail)
				continue
			}
			logger.WithField("workspace", ws.Name).Debugf("Skipping: %v", err)
			continue
		}

		logger.WithFields(logrus.Fields{
			"workspace": ws.Name,
			"log_file":  logFile,
		}).Debug("Tailing log file")

		wg.Add(1)
		// Use TailDirectory to handle file rotation/switching
		if follow {
			go logutil.TailDirectory(ws.Name, ws.Path, logsDir, lineChan, &wg, follow, tail)
		} else {
			go logutil.TailFile(ws.Name, ws.Path, logFile, lineChan, &wg, follow, tail)
		}
	}

	// Also tail the central system log directory for daemon/system events
	systemLogsDir := filepath.Join(paths.StateDir(), "logs")
	if _, err := os.Stat(systemLogsDir); err == nil {
		wg.Add(1)
		if follow || systemOnly {
			go logutil.TailDirectory("system", "", systemLogsDir, lineChan, &wg, follow || systemOnly, tail)
		} else {
			if sysLogFile, err := logutil.FindLatestLogFile(systemLogsDir); err == nil {
				go logutil.TailFile("system", "", sysLogFile, lineChan, &wg, follow, tail)
			} else {
				wg.Done()
			}
		}
	} else if systemOnly {
		logger.Info("No system logs found yet.")
		return nil
	}

	// Close channel when all tailing goroutines are done
	go func() {
		wg.Wait()
		close(lineChan)
	}()

	// Build a set of workspace names for filtering system log entries
	wsNameSet := make(map[string]bool, len(workspaces))
	for _, w := range workspaces {
		wsNameSet[w.Name] = true
	}

	// 4. Process and print logs from the channel
	for tailedLine := range lineChan {
		stats.total++

		var logMap map[string]interface{}
		if err := json.Unmarshal([]byte(tailedLine.Line), &logMap); err != nil {
			// Non-JSON line, print raw
			stats.shown++
			fmt.Println(tailedLine.Line)
			continue
		}

		// Handle system log filtering
		if tailedLine.Workspace == "system" {
			wsContext, _ := logMap["workspace"].(string)
			if !systemOnly {
				if wsContext != "" {
					if !wsNameSet[wsContext] {
						continue
					}
				} else if !includeSystem && !ecosystem {
					continue
				}
			}
		} else if systemOnly {
			continue
		}

		// Filter based on component visibility config
		if component, ok := logMap["component"].(string); ok {
			result := logging.GetComponentVisibility(component, &logCfg, overrideOpts)
			if !result.Visible {
				stats.hidden++
				stats.lastReason = result.Reason
				stats.lastRule = result.Rule
				continue
			}
		}
		stats.shown++

		// Handle global JSON output option
		outputFormat := format
		if opts.JSONOutput {
			outputFormat = "json"
		}

		fmt.Print(logutil.FormatLogLine(logMap, tailedLine.Workspace, outputFormat, compact))
	}

	// For non-follow commands, print filter statistics at the end.
	if !follow && stats.hidden > 0 {
		reasonStr := strings.ReplaceAll(string(stats.lastReason), "_", " ")
		ruleStr := strings.Join(stats.lastRule, ", ")
		if len(stats.lastRule) > 0 {
			fmt.Fprintf(os.Stderr, "\n[%d log entries hidden by %s rule: [%s]]\n", stats.hidden, reasonStr, ruleStr)
		} else {
			fmt.Fprintf(os.Stderr, "\n[%d log entries hidden by %s]\n", stats.hidden, reasonStr)
		}
	}

	return nil
}
