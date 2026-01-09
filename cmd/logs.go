package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattsolo1/grove-core/cli"
	"github.com/mattsolo1/grove-core/config"
	"github.com/mattsolo1/grove-core/logging"
	"github.com/mattsolo1/grove-core/pkg/logging/logutil"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/tui/theme"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// TailedLine represents a line of log output from a specific workspace.
type TailedLine struct {
	Workspace string
	Line      string
}

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
	cmd.Flags().String("format", "text", "Output format: text, json, full, pretty, pretty-text")
	cmd.Flags().BoolP("tui", "i", false, "Launch the interactive TUI")
	cmd.Flags().Bool("ecosystem", false, "Show logs from all workspaces in the ecosystem")
	cmd.Flags().StringSliceP("workspaces", "w", []string{}, "Filter by specific workspace names (comma-separated)")
	cmd.Flags().BoolP("follow", "f", false, "Follow log output")
	cmd.Flags().Int("tail", -1, "Number of lines to show from the end of the logs (default: all)")

	cmd.Flags().Bool("show-all", false, "Show all logs, ignoring any configured show/hide rules")
	cmd.Flags().StringSlice("component", []string{}, "Show logs only from these components (acts as a strict whitelist)")
	cmd.Flags().StringSlice("also-show", []string{}, "Temporarily show components/groups, overriding hide rules")
	cmd.Flags().StringSlice("ignore-hide", []string{}, "Temporarily show components/groups that would be hidden by config")

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

	// Load logging config for component filtering
	var logCfg logging.Config
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

	var workspaces []*workspace.WorkspaceNode

	// Determine which workspaces to show
	if ecosystem || len(wsFilter) > 0 {
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

		// Create a WorkspaceNode for the current workspace
		// Note: We don't require grove.yml to exist here - findLogFileForWorkspace
		// handles missing configs gracefully by falling back to .grove/logs/
		workspaces = []*workspace.WorkspaceNode{
			{
				Path: cwd,
				Name: filepath.Base(cwd),
			},
		}
	}

	if len(workspaces) == 0 {
		logger.Info("No matching workspaces found.")
		return nil
	}

	// Check if TUI mode should be used
	tuiMode, _ := cmd.Flags().GetBool("tui")
	follow, _ := cmd.Flags().GetBool("follow")

	if tuiMode {
		// Convert WorkspaceNode slice to string slice of paths
		workspacePaths := make([]string, len(workspaces))
		for i, ws := range workspaces {
			workspacePaths[i] = ws.Path
		}
		return runLogsTUI(workspacePaths, follow, overrideOpts)
	}

	// 3. Find log files and start tailing
	lineChan := make(chan TailedLine, 100)
	var wg sync.WaitGroup

	tail, _ := cmd.Flags().GetInt("tail")
	jsonOutput, _ := cmd.Flags().GetBool("json")
	format, _ := cmd.Flags().GetString("format")

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
				go tailDirectory(ws.Name, logsDir, lineChan, &wg, follow, tail)
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
		// Use tailDirectory to handle file rotation/switching
		if follow {
			go tailDirectory(ws.Name, logsDir, lineChan, &wg, follow, tail)
		} else {
			go tailFile(ws.Name, logFile, lineChan, &wg, follow, tail)
		}
	}

	// Close channel when all tailing goroutines are done
	go func() {
		wg.Wait()
		close(lineChan)
	}()

	// 4. Process and print logs from the channel
	for tailedLine := range lineChan {
		stats.total++
		// Filter based on component visibility config
		var logMap map[string]interface{}
		if err := json.Unmarshal([]byte(tailedLine.Line), &logMap); err == nil {
			if component, ok := logMap["component"].(string); ok {
				result := logging.GetComponentVisibility(component, &logCfg, overrideOpts)
				if !result.Visible {
					stats.hidden++
					stats.lastReason = result.Reason
					stats.lastRule = result.Rule
					continue
				}
			}
		}
		stats.shown++

		// Handle global JSON output option
		outputFormat := format
		if opts.JSONOutput {
			outputFormat = "json"
		}

		switch outputFormat {
		case "json":
			printLogJSON(tailedLine)
		case "pretty":
			printLogPretty(tailedLine, true) // with ANSI
		case "pretty-text":
			printLogPretty(tailedLine, false) // without ANSI
		case "full":
			printLogFull(tailedLine)
		default: // "text"
			printLogText(tailedLine)
		}
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


// tailFile reads a file and sends new lines to a channel.
func tailFile(wsName, path string, lineChan chan<- TailedLine, wg *sync.WaitGroup, follow bool, tailLines int) {
	defer wg.Done()

	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	reader := bufio.NewReader(f)

	if tailLines >= 0 {
		// This is an inefficient way to tail, but simple for this implementation.
		// A more robust solution would read from the end of the file.
		allLines, _ := io.ReadAll(reader)
		lines := strings.Split(string(allLines), "\n")
		start := len(lines) - tailLines - 1
		if tailLines == 0 { // tail 0 means from start
			start = 0
		}
		if start < 0 {
			start = 0
		}
		for _, line := range lines[start:] {
			if line != "" {
				lineChan <- TailedLine{Workspace: wsName, Line: line}
			}
		}
		// If not following, we are done.
		if !follow {
			return
		}
		// To follow, we need to seek to the end. Re-open is easiest.
		f.Close()
		f, _ = os.Open(path)
		f.Seek(0, io.SeekEnd)
		reader = bufio.NewReader(f)
	}

	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			lineChan <- TailedLine{Workspace: wsName, Line: strings.TrimSpace(line)}
		}

		if err == io.EOF {
			if !follow {
				break
			}
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if err != nil {
			break
		}
	}
}

// tailDirectory watches a log directory for files and tails them.
// It handles the case where the directory or files don't exist yet.
func tailDirectory(wsName, logsDir string, lineChan chan<- TailedLine, wg *sync.WaitGroup, follow bool, tailLines int) {
	defer wg.Done()

	var currentFile string
	var f *os.File
	var reader *bufio.Reader
	var fileOffset int64

	// Wait for directory and files to appear
	for {
		logFile, err := logutil.FindLatestLogFile(logsDir)
		if err == nil {
			currentFile = logFile
			break
		}
		if !follow {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Open the initial file
	f, err := os.Open(currentFile)
	if err != nil {
		return
	}

	reader = bufio.NewReader(f)

	// Handle initial tail lines
	if tailLines >= 0 {
		allLines, _ := io.ReadAll(reader)
		lines := strings.Split(string(allLines), "\n")
		start := len(lines) - tailLines - 1
		if tailLines == 0 {
			start = 0
		}
		if start < 0 {
			start = 0
		}
		for _, line := range lines[start:] {
			if line != "" {
				lineChan <- TailedLine{Workspace: wsName, Line: line}
			}
		}
		if !follow {
			f.Close()
			return
		}
		// Seek to end for following
		f.Close()
		f, _ = os.Open(currentFile)
		fileOffset, _ = f.Seek(0, io.SeekEnd)
		reader = bufio.NewReader(f)
	}

	checkInterval := time.NewTicker(500 * time.Millisecond)
	defer checkInterval.Stop()

	for {
		// Read any available lines
		for {
			line, err := reader.ReadString('\n')
			if len(line) > 0 {
				lineChan <- TailedLine{Workspace: wsName, Line: strings.TrimSpace(line)}
				fileOffset += int64(len(line))
			}
			if err != nil {
				break
			}
		}

		if !follow {
			break
		}

		<-checkInterval.C

		// Check for newer log file
		latestFile, err := logutil.FindLatestLogFile(logsDir)
		if err == nil && latestFile != currentFile {
			// Switch to the newer file
			f.Close()
			currentFile = latestFile
			f, err = os.Open(currentFile)
			if err != nil {
				continue
			}
			reader = bufio.NewReader(f)
			fileOffset = 0
		}
	}

	if f != nil {
		f.Close()
	}
}

// printLogJSON prints a log line in JSON format, enriched with the workspace name.
// Filters out pretty_ansi (keeps pretty_text for clean searchable output).
func printLogJSON(tailedLine TailedLine) {
	var logMap map[string]interface{}
	err := json.Unmarshal([]byte(tailedLine.Line), &logMap)
	if err != nil {
		// Fallback for non-JSON lines
		fallback := map[string]interface{}{
			"workspace": tailedLine.Workspace,
			"raw_line":  tailedLine.Line,
			"error":     "failed to parse original log line as JSON",
		}
		jsonData, _ := json.Marshal(fallback)
		fmt.Println(string(jsonData))
		return
	}

	// Filter out pretty_ansi (ANSI codes don't belong in JSON output)
	delete(logMap, "pretty_ansi")

	logMap["workspace"] = tailedLine.Workspace
	jsonData, _ := json.Marshal(logMap)
	fmt.Println(string(jsonData))
}

// printLogPretty prints only the pretty output from the unified logger.
// If withANSI is true, uses pretty_ansi (styled); otherwise uses pretty_text (plain).
func printLogPretty(tailedLine TailedLine, withANSI bool) {
	var logMap map[string]interface{}
	if err := json.Unmarshal([]byte(tailedLine.Line), &logMap); err != nil {
		// Not JSON, print raw line
		fmt.Println(tailedLine.Line)
		return
	}

	// Choose which pretty field to use
	var prettyOutput string
	if withANSI {
		if v, ok := logMap["pretty_ansi"].(string); ok && v != "" {
			prettyOutput = v
		}
	} else {
		if v, ok := logMap["pretty_text"].(string); ok && v != "" {
			prettyOutput = v
		}
	}

	// If no pretty output available, fall back to msg
	if prettyOutput == "" {
		if msg, ok := logMap["msg"].(string); ok {
			prettyOutput = msg
		} else {
			return // Nothing to print
		}
	}

	fmt.Println(prettyOutput)
}

// printLogFull prints the standard text format plus the pretty output indented below.
func printLogFull(tailedLine TailedLine) {
	var logMap map[string]interface{}
	if err := json.Unmarshal([]byte(tailedLine.Line), &logMap); err != nil {
		// Print as a raw line if not JSON
		fmt.Printf("[%s] %s\n",
			theme.DefaultTheme.Accent.Render(tailedLine.Workspace),
			tailedLine.Line,
		)
		return
	}

	// Extract common fields
	ts, _ := logMap["time"].(string)
	level, _ := logMap["level"].(string)
	msg, _ := logMap["msg"].(string)
	component, _ := logMap["component"].(string)

	// Parse time for formatting
	parsedTime, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		parsedTime, _ = time.Parse(time.RFC3339, ts)
	}
	timeStr := parsedTime.Format("15:04:05")

	// Style level
	var levelStyle lipgloss.Style
	switch strings.ToLower(level) {
	case "error", "fatal", "panic":
		levelStyle = theme.DefaultTheme.Error
	case "warning":
		levelStyle = theme.DefaultTheme.Warning
	case "info":
		levelStyle = theme.DefaultTheme.Info
	default:
		levelStyle = theme.DefaultTheme.Muted
	}
	levelStr := levelStyle.Render(strings.ToUpper(level))

	// Get other fields (excluding pretty_* and standard fields)
	otherFields := []string{}
	sortedKeys := []string{}
	excludeFields := map[string]bool{
		"time": true, "level": true, "msg": true, "component": true,
		"workspace": true, "pretty_ansi": true, "pretty_text": true,
	}
	for k := range logMap {
		if !excludeFields[k] {
			sortedKeys = append(sortedKeys, k)
		}
	}
	sort.Strings(sortedKeys)

	for _, k := range sortedKeys {
		otherFields = append(otherFields, fmt.Sprintf("%s=%v", theme.DefaultTheme.Muted.Render(k), logMap[k]))
	}

	fieldsStr := strings.Join(otherFields, " ")

	// Print the main log line
	fmt.Printf("%s [%s] %s %s [%s] %s\n",
		timeStr,
		theme.DefaultTheme.Accent.Render(tailedLine.Workspace),
		levelStr,
		msg,
		theme.DefaultTheme.Muted.Render(component),
		fieldsStr,
	)

	// Print pretty output indented below if available
	if prettyAnsi, ok := logMap["pretty_ansi"].(string); ok && prettyAnsi != "" {
		fmt.Printf("         %s %s\n\n", theme.DefaultTheme.Muted.Render("└─"), prettyAnsi)
	}
}

// printLogText pretty-prints a log line for human consumption.
func printLogText(tailedLine TailedLine) {
	var logMap map[string]interface{}
	if err := json.Unmarshal([]byte(tailedLine.Line), &logMap); err != nil {
		// Print as a raw line if not JSON
		fmt.Printf("[%s] %s\n",
			theme.DefaultTheme.Accent.Render(tailedLine.Workspace),
			tailedLine.Line,
		)
		return
	}

	// Extract common fields
	ts, _ := logMap["time"].(string)
	level, _ := logMap["level"].(string)
	msg, _ := logMap["msg"].(string)
	component, _ := logMap["component"].(string)

	// Parse time for formatting
	parsedTime, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		parsedTime, _ = time.Parse(time.RFC3339, ts)
	}
	timeStr := parsedTime.Format("15:04:05")

	// Style level
	var levelStyle lipgloss.Style
	switch strings.ToLower(level) {
	case "error", "fatal", "panic":
		levelStyle = theme.DefaultTheme.Error
	case "warning":
		levelStyle = theme.DefaultTheme.Warning
	case "info":
		levelStyle = theme.DefaultTheme.Info
	default:
		levelStyle = theme.DefaultTheme.Muted
	}
	levelStr := levelStyle.Render(strings.ToUpper(level))

	// Get other fields (excluding pretty_* which are handled separately)
	otherFields := []string{}
	sortedKeys := []string{}
	excludeFields := map[string]bool{
		"time": true, "level": true, "msg": true, "component": true,
		"workspace": true, "pretty_ansi": true, "pretty_text": true,
	}
	for k := range logMap {
		if !excludeFields[k] {
			sortedKeys = append(sortedKeys, k)
		}
	}
	sort.Strings(sortedKeys)

	for _, k := range sortedKeys {
		otherFields = append(otherFields, fmt.Sprintf("%s=%v", theme.DefaultTheme.Muted.Render(k), logMap[k]))
	}

	fieldsStr := strings.Join(otherFields, " ")

	fmt.Printf("%s [%s] %s %s [%s] %s\n",
		timeStr,
		theme.DefaultTheme.Accent.Render(tailedLine.Workspace),
		levelStr,
		msg,
		theme.DefaultTheme.Muted.Render(component),
		fieldsStr,
	)
}
