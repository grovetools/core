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

// VisibilityReason provides a clear reason for a filtering decision.
type VisibilityReason string

const (
	// ReasonVisibleDefault is for components visible because no rules hid them.
	ReasonVisibleDefault VisibilityReason = "visible_default"
	// ReasonVisibleByShow is for components visible due to a 'show' rule.
	ReasonVisibleByShow VisibilityReason = "visible_by_show"
	// ReasonVisibleByOnly is for components visible due to an 'only' whitelist rule.
	ReasonVisibleByOnly VisibilityReason = "visible_by_only"
	// ReasonVisibleByProject is for components visible because they are the current project.
	ReasonVisibleByProject VisibilityReason = "visible_by_project"
	// ReasonHiddenByHide is for components hidden by a 'hide' rule.
	ReasonHiddenByHide VisibilityReason = "hidden_by_hide"
	// ReasonHiddenByOnly is for components hidden by an 'only' whitelist.
	ReasonHiddenByOnly VisibilityReason = "hidden_by_only"
	// ReasonHiddenByDefault is for components hidden by the default 'grove-ecosystem' hide rule.
	ReasonHiddenByDefault VisibilityReason = "hidden_by_default"
	// ReasonVisibleByOverrideShowAll is for components visible due to --show-all override.
	ReasonVisibleByOverrideShowAll VisibilityReason = "visible_by_override_show_all"
	// ReasonVisibleByOverrideShowOnly is for components visible due to --component override.
	ReasonVisibleByOverrideShowOnly VisibilityReason = "visible_by_override_show_only"
	// ReasonVisibleByOverrideAlsoShow is for components visible due to --also-show override.
	ReasonVisibleByOverrideAlsoShow VisibilityReason = "visible_by_override_also_show"
	// ReasonVisibleByOverrideIgnore is for components visible due to --ignore-hide override.
	ReasonVisibleByOverrideIgnore VisibilityReason = "visible_by_override_ignore_hide"
	// ReasonHiddenByOverrideShowOnly is for components hidden by --component override.
	ReasonHiddenByOverrideShowOnly VisibilityReason = "hidden_by_override_show_only"
)

// VisibilityResult holds the outcome of a component visibility check.
type VisibilityResult struct {
	Visible bool
	Reason  VisibilityReason
	Rule    []string // The config rule that was matched, e.g., ["grove-ecosystem"]
}

// OverrideOptions holds runtime filter settings from CLI flags.
type OverrideOptions struct {
	ShowAll    bool
	ShowOnly   []string // For --component
	AlsoShow   []string // For --also-show
	IgnoreHide []string // For --ignore-hide
}

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
// based on the provided configuration.
func IsComponentVisible(component string, cfg *Config) bool {
	// Call the new detailed function with no overrides for backward compatibility.
	return GetComponentVisibility(component, cfg, nil).Visible
}

// GetComponentVisibility determines if a component's logs should be visible based on a hierarchy of rules.
func GetComponentVisibility(component string, cfg *Config, overrides *OverrideOptions) VisibilityResult {
	if cfg == nil {
		cfg = &Config{}
	}
	if cfg.ComponentFiltering == nil {
		cfg.ComponentFiltering = &ComponentFilteringConfig{}
	}
	if overrides == nil {
		overrides = &OverrideOptions{}
	}

	// 1. --show-all override
	if overrides.ShowAll {
		return VisibilityResult{Visible: true, Reason: ReasonVisibleByOverrideShowAll}
	}

	// 2. --component override (acts as a strict 'only' whitelist)
	if len(overrides.ShowOnly) > 0 {
		showOnlySet := resolveFilterSet(overrides.ShowOnly, cfg.Groups)
		if showOnlySet[component] {
			return VisibilityResult{Visible: true, Reason: ReasonVisibleByOverrideShowOnly, Rule: overrides.ShowOnly}
		}
		return VisibilityResult{Visible: false, Reason: ReasonHiddenByOverrideShowOnly, Rule: overrides.ShowOnly}
	}

	// 3. show_current_project config
	showCurrentProject := cfg.ShowCurrentProject == nil || *cfg.ShowCurrentProject
	if showCurrentProject && component == getCurrentProjectName() {
		return VisibilityResult{Visible: true, Reason: ReasonVisibleByProject}
	}

	// 4. --also-show and config 'show' rules (force visibility)
	alsoShowSet := resolveFilterSet(overrides.AlsoShow, cfg.Groups)
	if alsoShowSet[component] {
		return VisibilityResult{Visible: true, Reason: ReasonVisibleByOverrideAlsoShow, Rule: overrides.AlsoShow}
	}
	showSet := resolveFilterSet(cfg.ComponentFiltering.Show, cfg.Groups)
	if showSet[component] {
		return VisibilityResult{Visible: true, Reason: ReasonVisibleByShow, Rule: cfg.ComponentFiltering.Show}
	}

	// 5. Config 'only' rules (strict whitelist)
	onlySet := resolveFilterSet(cfg.ComponentFiltering.Only, cfg.Groups)
	if onlySet != nil {
		if onlySet[component] {
			return VisibilityResult{Visible: true, Reason: ReasonVisibleByOnly, Rule: cfg.ComponentFiltering.Only}
		}
		return VisibilityResult{Visible: false, Reason: ReasonHiddenByOnly, Rule: cfg.ComponentFiltering.Only}
	}

	// 6. --ignore-hide override (prevents subsequent hide rules from applying)
	ignoreHideSet := resolveFilterSet(overrides.IgnoreHide, cfg.Groups)
	if ignoreHideSet[component] {
		return VisibilityResult{Visible: true, Reason: ReasonVisibleByOverrideIgnore, Rule: overrides.IgnoreHide}
	}

	// 7. Config 'hide' rules
	hideSet := resolveFilterSet(cfg.ComponentFiltering.Hide, cfg.Groups)
	if hideSet[component] {
		return VisibilityResult{Visible: false, Reason: ReasonHiddenByHide, Rule: cfg.ComponentFiltering.Hide}
	}

	// 8. Default 'hide' rule for grove-ecosystem
	if len(cfg.ComponentFiltering.Hide) == 0 {
		defaultHideSet := resolveFilterSet(DefaultHide, cfg.Groups)
		if defaultHideSet[component] {
			return VisibilityResult{Visible: false, Reason: ReasonHiddenByDefault, Rule: DefaultHide}
		}
	}

	// 9. If no rules matched, default to visible.
	return VisibilityResult{Visible: true, Reason: ReasonVisibleDefault}
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

	// Use the global writer instead of os.Stderr to support TUI redirection
	if shouldLogToStderr && isVisible {
		logger.SetOutput(GetGlobalOutput())
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

