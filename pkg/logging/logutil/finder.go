package logutil

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/logging"
	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/util/pathutil"
)

// GetSystemLogsDir returns the path to the central system logs directory.
func GetSystemLogsDir() string {
	return filepath.Join(paths.StateDir(), "logs")
}

// FindLogFileForWorkspace determines the log file path for a given workspace.
// Returns the log file path and the logs directory path.
func FindLogFileForWorkspace(ws *workspace.WorkspaceNode) (logFile, logsDir string, err error) {
	cfg, _ := config.LoadFrom(ws.Path)

	var logCfg logging.Config
	if cfg != nil {
		_ = cfg.UnmarshalExtension("logging", &logCfg)
	}

	if logCfg.File.Enabled && logCfg.File.Path != "" {
		expanded, expandErr := pathutil.Expand(logCfg.File.Path)
		if expandErr != nil {
			return "", "", expandErr
		}
		return expanded, filepath.Dir(expanded), nil
	}

	// Default path logic
	logsDir = filepath.Join(ws.Path, ".grove", "logs")
	logFile, err = FindLatestLogFile(logsDir)
	return logFile, logsDir, err
}

// FindLatestLogFile finds the latest log file in a directory by
// sorting filenames lexically (descending). Grove logs are named
// `<prefix>-YYYY-MM-DD.log`, so ISO-8601 date ordering matches lexical
// order — this is strictly correct and immune to spurious `ModTime`
// updates caused by IDE indexers, backup tools, or accidental
// `touch`. Prefers files with content over empty files (so an empty
// file freshly opened for today doesn't mask yesterday's populated
// log while today's process is still warming up). Entries that don't
// end in `.log` are skipped.
func FindLatestLogFile(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("could not read log directory %s: %w", dir, err)
	}

	// Collect candidate log filenames.
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".log") {
			continue
		}
		names = append(names, name)
	}

	if len(names) == 0 {
		return "", fmt.Errorf("no log files found in %s", dir)
	}

	// Sort descending: newest ISO date first.
	sort.Sort(sort.Reverse(sort.StringSlice(names)))

	// Walk the sorted list, preferring the first non-empty file. If
	// every file is empty (rare but possible right after rotation),
	// fall back to the lexically newest entry.
	var firstPath string
	for _, name := range names {
		path := filepath.Join(dir, name)
		if firstPath == "" {
			firstPath = path
		}
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.Size() > 0 {
			return path, nil
		}
	}
	return firstPath, nil
}
