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
current workspace only.

Examples:
  # Stream current workspace logs
  core logs -f

  # Stream all ecosystem workspaces
  core logs --scope ecosystem -f

  # Stream ecosystem + system logs
  core logs --scope ecosystem --system -f

  # System logs only
  core logs --scope system

  # Live daemon event stream
  core logs --scope daemon -f

  # Include debug entries
  core logs --level debug -f

  # Last 50 errors
  core logs --level error --tail 50

  # Filter to a single component
  core logs --component groved.server -f

  # Specific workspaces
  core logs -w api,worker -f

  # Styled output, last 100 lines
  core logs --format pretty --tail 100
`,
		RunE: runLogsE,
	}

	// Scope
	cmd.Flags().String("scope", "workspace", "Log scope: workspace, ecosystem, all, system, daemon")
	cmd.Flags().StringSliceP("workspace", "w", []string{}, "Filter to specific workspace names (comma-separated)")
	cmd.Flags().Bool("system", false, "Include system logs alongside workspace scope")

	// Filtering
	cmd.Flags().String("level", "", "Minimum log level: debug, info, warn, error")
	cmd.Flags().StringSlice("component", []string{}, "Show only these components (comma-separated whitelist)")
	cmd.Flags().Bool("show-all", false, "Ignore all configured hide/show rules")

	// Output
	cmd.Flags().BoolP("follow", "f", false, "Follow log output")
	cmd.Flags().Int("tail", -1, "Number of lines to show from the end of the logs (default: all)")
	cmd.Flags().String("format", "text", "Output format: text, json, full, rich, pretty, pretty-text")
	cmd.Flags().Bool("json", false, "Shorthand for --format=json")
	cmd.Flags().Bool("compact", false, "Disable spacing between entries (pretty/full/rich)")

	// Mode
	cmd.Flags().BoolP("tui", "i", false, "Launch the interactive TUI")

	return cmd
}

// validLevels maps level name to its severity rank for threshold filtering.
var validLevels = map[string]int{
	"debug":   0,
	"info":    1,
	"warn":    2,
	"warning": 2,
	"error":   3,
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

	// Load logging config for component filtering
	logCfg := logging.GetDefaultLoggingConfig()
	if cfg, err := config.LoadDefault(); err == nil {
		_ = cfg.UnmarshalExtension("logging", &logCfg)
	}

	// --- Parse flags ---
	scope, _ := cmd.Flags().GetString("scope")
	wsFilter, _ := cmd.Flags().GetStringSlice("workspace")
	includeSystem, _ := cmd.Flags().GetBool("system")
	level, _ := cmd.Flags().GetString("level")
	showAll, _ := cmd.Flags().GetBool("show-all")
	showOnly, _ := cmd.Flags().GetStringSlice("component")
	follow, _ := cmd.Flags().GetBool("follow")
	tuiMode, _ := cmd.Flags().GetBool("tui")

	// Validate scope
	switch scope {
	case "workspace", "ecosystem", "all", "system", "daemon":
	default:
		return fmt.Errorf("invalid --scope %q: must be workspace, ecosystem, all, system, or daemon", scope)
	}

	// Validate level
	var minLevelRank int = -1
	if level != "" {
		level = strings.ToLower(level)
		rank, ok := validLevels[level]
		if !ok {
			return fmt.Errorf("invalid --level %q: must be debug, info, warn, or error", level)
		}
		minLevelRank = rank
	}

	// -w implies ecosystem scope for workspace discovery
	if len(wsFilter) > 0 && !cmd.Flags().Changed("scope") {
		scope = "ecosystem"
	}

	systemOnly := scope == "system"

	overrideOpts := &logging.OverrideOptions{
		ShowAll:  showAll,
		ShowOnly: showOnly,
	}
	stats := &filterStats{}

	var workspaces []*workspace.WorkspaceNode

	if scope == "daemon" {
		return fmt.Errorf("--scope daemon is not yet supported in CLI mode; use the TUI (core logs -i --scope daemon)")
	}

	// Determine which workspaces to show
	if systemOnly {
		workspaces = []*workspace.WorkspaceNode{}
	} else if scope == "ecosystem" || scope == "all" || len(wsFilter) > 0 {
		allWorkspaces, err := workspace.GetProjects(logger)
		if err != nil {
			return fmt.Errorf("failed to discover workspaces: %w", err)
		}

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
		// Default: current workspace
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}

		wsName := filepath.Base(cwd)
		if cfg, err := config.LoadFrom(cwd); err == nil && cfg.Name != "" {
			wsName = cfg.Name
		}

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

	if tuiMode {
		return runLogsTUI(workspaces, follow, overrideOpts, scope, includeSystem, level)
	}

	// --- Non-TUI file tailing mode ---
	lineChan := make(chan logutil.TailedLine, 100)
	var wg sync.WaitGroup

	tail, _ := cmd.Flags().GetInt("tail")
	// When following without explicit --tail, default to 0 (stream new only)
	if follow && !cmd.Flags().Changed("tail") {
		tail = 0
	}
	jsonOutput, _ := cmd.Flags().GetBool("json")
	format, _ := cmd.Flags().GetString("format")
	compact, _ := cmd.Flags().GetBool("compact")

	if jsonOutput {
		format = "json"
	}

	for _, ws := range workspaces {
		logFile, logsDir, err := logutil.FindLogFileForWorkspace(ws)
		if err != nil {
			if follow && logsDir != "" {
				logger.WithFields(logrus.Fields{
					"workspace": ws.Name,
					"logs_dir":  logsDir,
				}).Debug("Waiting for log files in directory")

				wg.Add(1)
				go logutil.TailDirectory(cmd.Context(), ws.Name, ws.Path, logsDir, lineChan, &wg, follow, tail)
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
		if follow {
			go logutil.TailDirectory(cmd.Context(), ws.Name, ws.Path, logsDir, lineChan, &wg, follow, tail)
		} else {
			go logutil.TailFile(cmd.Context(), ws.Name, ws.Path, logFile, lineChan, &wg, follow, tail)
		}
	}

	// Also tail system logs when scope includes them
	systemLogsDir := filepath.Join(paths.StateDir(), "logs")
	if _, err := os.Stat(systemLogsDir); err == nil {
		wg.Add(1)
		if follow || systemOnly {
			go logutil.TailDirectory(cmd.Context(), "system", "", systemLogsDir, lineChan, &wg, follow || systemOnly, tail)
		} else {
			if sysLogFile, err := logutil.FindLatestLogFile(systemLogsDir); err == nil {
				go logutil.TailFile(cmd.Context(), "system", "", sysLogFile, lineChan, &wg, follow, tail)
			} else {
				wg.Done()
			}
		}
	} else if systemOnly {
		logger.Info("No system logs found yet.")
		return nil
	}

	go func() {
		wg.Wait()
		close(lineChan)
	}()

	wsNameSet := make(map[string]bool, len(workspaces))
	for _, w := range workspaces {
		wsNameSet[w.Name] = true
	}

	for tailedLine := range lineChan {
		stats.total++

		var logMap map[string]interface{}
		if err := json.Unmarshal([]byte(tailedLine.Line), &logMap); err != nil {
			stats.shown++
			fmt.Println(tailedLine.Line)
			continue
		}

		// System log filtering
		if tailedLine.Workspace == "system" {
			wsContext, _ := logMap["workspace"].(string)
			if !systemOnly {
				if wsContext != "" {
					if !wsNameSet[wsContext] {
						continue
					}
				} else if !includeSystem && scope != "ecosystem" && scope != "all" {
					continue
				}
			}
		} else if systemOnly {
			continue
		}

		// Level filtering
		if minLevelRank >= 0 {
			if entryLevel, ok := logMap["level"].(string); ok {
				entryRank, known := validLevels[strings.ToLower(entryLevel)]
				if known && entryRank < minLevelRank {
					continue
				}
			}
		}

		// Component visibility filtering
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

		outputFormat := format
		if opts.JSONOutput {
			outputFormat = "json"
		}

		fmt.Print(logutil.FormatLogLine(logMap, tailedLine.Workspace, outputFormat, compact))
	}

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
