package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/mattsolo1/grove-core/config"
	"github.com/mattsolo1/grove-core/version"
	"github.com/sirupsen/logrus"
)

var (
	loggers   = make(map[string]*logrus.Entry)
	loggersMu sync.Mutex
	initOnce  sync.Once
)

// resolveFilterSet expands a list of items (which can be component or group names)
// into a flat set of component names. User-defined groups take precedence over DefaultGroups.
func resolveFilterSet(items []string, groups map[string][]string) map[string]bool {
	if len(items) == 0 {
		return nil
	}
	set := make(map[string]bool)
	for _, item := range items {
		// Check user-defined groups first
		if components, ok := groups[item]; ok {
			for _, c := range components {
				set[c] = true
			}
		} else if components, ok := DefaultGroups[item]; ok {
			// Fall back to default groups
			for _, c := range components {
				set[c] = true
			}
		} else {
			// This is a single component
			set[item] = true
		}
	}
	return set
}

// currentProjectName caches the current project name from grove.yml
var currentProjectName string
var currentProjectOnce sync.Once

// getCurrentProjectName returns the name of the current project from grove.yml
func getCurrentProjectName() string {
	currentProjectOnce.Do(func() {
		cfg, err := config.LoadDefault()
		if err == nil {
			currentProjectName = cfg.Name
		}
	})
	return currentProjectName
}

// IsComponentVisible determines if a component should be visible in console logs
// based on the provided configuration. 'show' takes precedence over 'hide'.
// If ShowCurrentProject is true (default), the current project is always visible.
// If no show/hide rules are configured, DefaultHide is applied.
func IsComponentVisible(component string, cfg *Config) bool {
	// Check if this is the current project and ShowCurrentProject is enabled (default true)
	showCurrentProject := cfg.ShowCurrentProject == nil || *cfg.ShowCurrentProject
	if showCurrentProject && component == getCurrentProjectName() {
		return true
	}

	showSet := resolveFilterSet(cfg.Show, cfg.Groups)
	if showSet != nil {
		return showSet[component]
	}

	hideSet := resolveFilterSet(cfg.Hide, cfg.Groups)
	if hideSet != nil {
		return !hideSet[component]
	}

	// Apply default hide if no explicit rules are set
	defaultHideSet := resolveFilterSet(DefaultHide, cfg.Groups)
	if defaultHideSet != nil {
		return !defaultHideSet[component]
	}

	return true
}

// VersionFields represents the fields logged when a Grove binary starts
type VersionFields struct {
	Branch    string `json:"branch" verbosity:"3"`
	Commit    string `json:"commit" verbosity:"3"`
	Binary    string `json:"binary" verbosity:"3"`
	Version   string `json:"version" verbosity:"0"`
	Platform  string `json:"platform" verbosity:"3"`
	GoVersion string `json:"goVersion" verbosity:"3"`
	BuildDate string `json:"buildDate" verbosity:"1"`
	Compiler  string `json:"compiler" verbosity:"3"`
}

// StructToLogrusFields converts a struct with verbosity tags to logrus.Fields
// including a _verbosity metadata field that maps field names to verbosity levels
func StructToLogrusFields(v interface{}) logrus.Fields {
	fields := logrus.Fields{}
	verbosityMap := make(map[string]int)

	val := reflect.ValueOf(v)
	typ := reflect.TypeOf(v)

	for i := 0; i < val.NumField(); i++ {
		field := typ.Field(i)
		value := val.Field(i)

		// Get JSON tag for field name
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" {
			continue
		}

		// Get verbosity tag for verbosity level
		verbosityTag := field.Tag.Get("verbosity")
		verbosityLevel := 0
		if verbosityTag != "" {
			if level, err := strconv.Atoi(verbosityTag); err == nil {
				verbosityLevel = level
			}
		}

		// Add field to logrus fields
		fields[jsonTag] = value.Interface()
		verbosityMap[jsonTag] = verbosityLevel
	}

	// Add verbosity metadata
	fields["_verbosity"] = verbosityMap

	return fields
}

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

	// Configure File Sink
	if logCfg.File.Enabled {
		var logFilePath string
		if logCfg.File.Path != "" {
			// Use explicitly configured path
			logFilePath = expandPath(logCfg.File.Path)
		} else {
			// Default to .grove/logs/workspace-<date>.log in the current working directory
			cwd, err := os.Getwd()
			if err == nil {
				now := time.Now()
				dateStr := now.Format("2006-01-02")
				logFilePath = filepath.Join(cwd, ".grove", "logs", fmt.Sprintf("workspace-%s.log", dateStr))
			}
		}

		if logFilePath != "" {
			dir := filepath.Dir(logFilePath)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				logger.Warnf("Failed to create log directory %s: %v", dir, err)
			} else {
				// Use a file writer that is safe for concurrent writes
				file, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
				if err == nil {
					var fileFormatter logrus.Formatter
					if logCfg.File.Format == "json" {
						fileFormatter = &logrus.JSONFormatter{}
					} else {
						// Default to a simple text formatter for files if not JSON
						fileFormatter = &TextFormatter{Config: FormatConfig{DisableTimestamp: false}}
					}
					logger.AddHook(&FileHook{
						Writer:    file,
						LogLevels: logrus.AllLevels,
						Formatter: fileFormatter,
					})
				} else {
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
		isDebug := os.Getenv("GROVE_DEBUG") == "1" || logger.GetLevel() == logrus.DebugLevel
		isInteractive := isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())
		if isDebug || !isInteractive {
			shouldLogToStderr = true
		}
	}

	// Check component visibility based on show/hide configuration
	isVisible := IsComponentVisible(component, &logCfg)

	if shouldLogToStderr && isVisible {
		logger.SetOutput(os.Stderr)
	} else {
		logger.SetOutput(io.Discard)
	}

	// Log version information once on first logger initialization (if enabled)
	initOnce.Do(func() {
		// Only log startup message if explicitly enabled in config
		if !logCfg.LogStartup {
			return
		}

		info := version.GetInfo()

		// Get binary name for more useful logging
		binaryName := "unknown"
		if len(os.Args) > 0 {
			binaryName = filepath.Base(os.Args[0])
		}

		// Use grove-<binary> as component for clearer identification
		// Don't prepend grove- if binary name already starts with grove-
		componentName := binaryName
		if !strings.HasPrefix(binaryName, "grove-") && binaryName != "grove" {
			componentName = fmt.Sprintf("grove-%s", binaryName)
		}

		// Create version fields struct with verbosity tags
		versionFields := VersionFields{
			Branch:    info.Branch,
			Commit:    info.Commit,
			Binary:    binaryName,
			Version:   info.Version,
			Platform:  info.Platform,
			GoVersion: info.GoVersion,
			BuildDate: info.BuildDate,
			Compiler:  info.Compiler,
		}

		// Convert struct to logrus fields with verbosity metadata
		fields := StructToLogrusFields(versionFields)
		fields["component"] = componentName

		logger.WithFields(fields).Info("Grove binary started")
	})

	entry := logger.WithField("component", component)
	loggers[component] = entry
	return entry
}

// FileHook is a logrus hook for writing logs to a file with a specific formatter.
// It includes a mutex to handle concurrent writes from different tool processes.
type FileHook struct {
	Writer    io.Writer
	LogLevels []logrus.Level
	Formatter logrus.Formatter
	mu        sync.Mutex
}

// Fire is called by logrus when a log entry is created.
func (hook *FileHook) Fire(entry *logrus.Entry) error {
	hook.mu.Lock()
	defer hook.mu.Unlock()

	line, err := hook.Formatter.Format(entry)
	if err != nil {
		return err
	}
	_, err = hook.Writer.Write(line)
	return err
}

// Levels returns the log levels that this hook will fire for.
func (hook *FileHook) Levels() []logrus.Level {
	return hook.LogLevels
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

// Reset clears the logger cache and resets the init state.
// This is primarily useful for testing when you need to reinitialize
// loggers with different configurations.
func Reset() {
	loggersMu.Lock()
	defer loggersMu.Unlock()
	loggers = make(map[string]*logrus.Entry)
	initOnce = sync.Once{}
	currentProjectOnce = sync.Once{}
	currentProjectName = ""
}

