package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/mattsolo1/grove-core/config"
	"github.com/sirupsen/logrus"
)

var (
	loggers   = make(map[string]*logrus.Entry)
	loggersMu sync.Mutex
)

// NewLogger creates and returns a pre-configured logger for a specific component.
// It uses a singleton pattern per component to avoid re-initializing.
func NewLogger(component string) *logrus.Entry {
	loggersMu.Lock()
	defer loggersMu.Unlock()

	if logger, exists := loggers[component]; exists {
		return logger
	}

	logger := logrus.New()

	// Load configuration from grove.yml
	cfg, err := config.LoadDefault()
	var logCfg Config
	if err == nil {
		// Use UnmarshalExtension to safely decode the logging part
		if err := cfg.UnmarshalExtension("logging", &logCfg); err != nil {
			// Log a warning if parsing fails, but continue with defaults
			logrus.Warnf("Failed to parse 'logging' config: %v", err)
		}
	}

	// Configure Level
	levelStr := "info" // Default level
	if os.Getenv("GROVE_LOG_LEVEL") != "" {
		levelStr = os.Getenv("GROVE_LOG_LEVEL")
	} else if logCfg.Level != "" {
		levelStr = logCfg.Level
	}
	level, err := logrus.ParseLevel(levelStr)
	if err != nil {
		level = logrus.InfoLevel
	}
	logger.SetLevel(level)

	// Configure Caller Reporting
	if os.Getenv("GROVE_LOG_CALLER") == "true" || logCfg.ReportCaller {
		logger.SetReportCaller(true)
	}

	// Configure Formatter
	switch logCfg.Format.Preset {
	case "json":
		logger.SetFormatter(&logrus.JSONFormatter{})
	case "simple":
		logger.SetFormatter(&TextFormatter{Config: FormatConfig{
			DisableTimestamp: true,
			DisableComponent: true,
		}})
	default:
		logger.SetFormatter(&TextFormatter{Config: logCfg.Format})
	}

	// Configure Output Sinks
	var writers []io.Writer
	
	// Configure File Sink
	var logFilePath string
	if logCfg.File.Enabled && logCfg.File.Path != "" {
		// Use explicitly configured path
		logFilePath = expandPath(logCfg.File.Path)
	} else {
		// Default to .grove/logs/<component>-<date>.log in the current working directory
		// This keeps logs with the project rather than centralizing them
		cwd, err := os.Getwd()
		if err == nil {
			// Create date-based log file name
			now := time.Now()
			dateStr := now.Format("2006-01-02")
			logFilePath = filepath.Join(cwd, ".grove", "logs", fmt.Sprintf("%s-%s.log", component, dateStr))
		} else {
			// Fallback to home directory if we can't get working directory
			home, homeErr := os.UserHomeDir()
			if homeErr == nil {
				now := time.Now()
				dateStr := now.Format("2006-01-02")
				logFilePath = filepath.Join(home, ".grove", "logs", fmt.Sprintf("%s-%s.log", component, dateStr))
			}
		}
	}
	
	// Create and open the log file
	if logFilePath != "" {
		dir := filepath.Dir(logFilePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			// Don't warn about default log dir creation failures
			if logCfg.File.Enabled {
				logger.Warnf("Failed to create log directory %s: %v", dir, err)
			}
		} else {
			file, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
			if err == nil {
				writers = append(writers, file)
			} else {
				// Only warn if explicitly configured
				if logCfg.File.Enabled {
					logger.Warnf("Failed to open log file %s: %v", logFilePath, err)
				}
			}
		}
	}

	// Determine if we should write structured logs to stderr
	shouldLogToStderr := false
	stderrMode := "auto"
	if logCfg.Format.StructuredToStderr != "" {
		stderrMode = logCfg.Format.StructuredToStderr
	}

	switch stderrMode {
	case "always":
		shouldLogToStderr = true
	case "never":
		shouldLogToStderr = false
	case "auto":
		// "auto" mode: log to stderr if debug is enabled, or if not in an interactive terminal
		isDebug := os.Getenv("GROVE_DEBUG") == "1" || logger.GetLevel() == logrus.DebugLevel
		isInteractive := isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())
		// Only show structured logs to stderr if:
		// 1. Debug mode is enabled, OR
		// 2. We're NOT in an interactive terminal (e.g., output is piped or in CI)
		// This suppresses structured logs in normal interactive use
		if isDebug || !isInteractive {
			shouldLogToStderr = true
		}
	}

	if shouldLogToStderr {
		writers = append(writers, os.Stderr)
	}

	// Configure the output based on the number of writers
	if len(writers) == 0 {
		// No writers configured - this is intentional in auto mode for interactive terminals
		// Use io.Discard to suppress all output rather than defaulting to stderr
		logger.SetOutput(io.Discard)
	} else if len(writers) == 1 {
		logger.SetOutput(writers[0])
	} else {
		mw := io.MultiWriter(writers...)
		logger.SetOutput(mw)
	}

	entry := logger.WithField("component", component)
	loggers[component] = entry
	return entry
}

// expandPath expands tilde in file paths
func expandPath(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}