package logutil

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mattsolo1/grove-core/config"
	"github.com/mattsolo1/grove-core/logging"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-core/util/pathutil"
)

// FindLogFileForWorkspace determines the log file path for a given workspace.
// Returns the log file path and the logs directory path.
func FindLogFileForWorkspace(ws *workspace.WorkspaceNode) (logFile string, logsDir string, err error) {
	cfg, cfgErr := config.LoadFrom(ws.Path)
	if cfgErr != nil {
		// A config might not exist, but we can still check default log path.
	}

	var logCfg logging.Config
	if cfg != nil {
		if unmarshalErr := cfg.UnmarshalExtension("logging", &logCfg); unmarshalErr != nil {
			// Continue with default config if parsing fails.
		}
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

// FindLatestLogFile finds the most recently modified non-empty file in a directory.
// Prefers files with content over empty files.
func FindLatestLogFile(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("could not read log directory %s: %w", dir, err)
	}

	var latestFile os.FileInfo
	var latestPath string
	var latestNonEmptyFile os.FileInfo
	var latestNonEmptyPath string

	for _, entry := range entries {
		if !entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			// Track latest file overall
			if latestFile == nil || info.ModTime().After(latestFile.ModTime()) {
				latestFile = info
				latestPath = filepath.Join(dir, entry.Name())
			}
			// Track latest non-empty file
			if info.Size() > 0 {
				if latestNonEmptyFile == nil || info.ModTime().After(latestNonEmptyFile.ModTime()) {
					latestNonEmptyFile = info
					latestNonEmptyPath = filepath.Join(dir, entry.Name())
				}
			}
		}
	}

	// Prefer non-empty files
	if latestNonEmptyFile != nil {
		return latestNonEmptyPath, nil
	}

	if latestFile == nil {
		return "", fmt.Errorf("no log files found in %s", dir)
	}

	return latestPath, nil
}
