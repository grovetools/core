package logging

import (
	"io"
	"os"
	"path/filepath"
	"sync"

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
	logger.SetOutput(os.Stderr) // Default to stderr for all diagnostics

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

	// Configure File Sink
	if logCfg.File.Enabled && logCfg.File.Path != "" {
		// Expand tilde in path
		path := expandPath(logCfg.File.Path)
		
		// Create directory if it doesn't exist
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			logger.Warnf("Failed to create log directory %s: %v", dir, err)
		} else {
			file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
			if err == nil {
				// Create a multi-writer to write to both stderr and file
				mw := io.MultiWriter(os.Stderr, file)
				logger.SetOutput(mw)
			} else {
				logger.Warnf("Failed to open log file %s: %v", path, err)
			}
		}
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