package logging

import (
	"flag"
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
	"github.com/sirupsen/logrus"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/version"
)

// LogScope defines the execution scope for log routing.
type LogScope int

const (
	// ScopeWorkspace routes logs to the XDG state directory (~/.local/state/grove/logs/workspaces/<identifier>/).
	ScopeWorkspace LogScope = iota
	// ScopeSystem routes logs to the central XDG state directory (~/.local/state/grove/logs).
	ScopeSystem
)

var (
	activeScope LogScope = ScopeWorkspace
	scopeMu     sync.RWMutex
)

// SetGlobalScope configures the destination scope for all loggers created in this process.
func SetGlobalScope(scope LogScope) {
	scopeMu.Lock()
	activeScope = scope
	scopeMu.Unlock()
}

// GetGlobalScope returns the current global log scope.
func GetGlobalScope() LogScope {
	scopeMu.RLock()
	defer scopeMu.RUnlock()
	return activeScope
}

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
var (
	currentProjectName string
	currentProjectOnce sync.Once
)

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

// resolvedConsoleLevel caches the console level resolved by the most recent
// NewLogger call so the unified logger can gate pretty output without
// re-loading configuration. Levels are process-wide, not per component.
var (
	resolvedConsoleLevel   = logrus.InfoLevel
	resolvedConsoleLevelMu sync.RWMutex
)

func setResolvedConsoleLevel(level logrus.Level) {
	resolvedConsoleLevelMu.Lock()
	resolvedConsoleLevel = level
	resolvedConsoleLevelMu.Unlock()
}

// ConsoleLevel returns the console log level resolved by the most recent
// NewLogger call (info before any logger has been created).
func ConsoleLevel() logrus.Level {
	resolvedConsoleLevelMu.RLock()
	defer resolvedConsoleLevelMu.RUnlock()
	return resolvedConsoleLevel
}

// resolvedPrettyFields caches whether structured entries should embed the
// rendered pretty_ansi/pretty_text fields, as resolved by the most recent
// NewLogger call. Like the console level, this is process-wide.
var (
	resolvedPrettyFields   = false
	resolvedPrettyFieldsMu sync.RWMutex
)

func setResolvedPrettyFields(enabled bool) {
	resolvedPrettyFieldsMu.Lock()
	resolvedPrettyFields = enabled
	resolvedPrettyFieldsMu.Unlock()
}

// PrettyFieldsEnabled reports whether unified log entries embed their
// rendered pretty_ansi/pretty_text forms into structured output, as resolved
// by the most recent NewLogger call (false — the default — before any logger
// has been created). See Config.StructuredPrettyFields.
func PrettyFieldsEnabled() bool {
	resolvedPrettyFieldsMu.RLock()
	defer resolvedPrettyFieldsMu.RUnlock()
	return resolvedPrettyFields
}

// resolvePrettyFields resolves whether structured entries embed the rendered
// pretty fields: GROVE_LOG_PRETTY_FIELDS env (true/false) >
// structured_pretty_fields config > off.
func resolvePrettyFields(logCfg *Config) bool {
	if env := os.Getenv("GROVE_LOG_PRETTY_FIELDS"); env != "" {
		if v, err := strconv.ParseBool(env); err == nil {
			return v
		}
	}
	return logCfg.StructuredPrettyFields
}

// parseLevelOrInfo parses a level string, falling back to info.
func parseLevelOrInfo(s string) logrus.Level {
	level, err := logrus.ParseLevel(s)
	if err != nil {
		return logrus.InfoLevel
	}
	return level
}

// resolveLevels resolves the per-sink log levels from config and scope.
//
// consoleLevel follows the chain: GROVE_LOG_LEVEL env > system_level (for
// ScopeSystem) > level > "info". fileLevel is file.level when set, otherwise
// consoleLevel. GROVE_LOG_LEVEL overrides both sinks.
func resolveLevels(logCfg *Config, scope LogScope) (consoleLevel, fileLevel logrus.Level) {
	if env := os.Getenv("GROVE_LOG_LEVEL"); env != "" {
		level := parseLevelOrInfo(env)
		return level, level
	}

	levelStr := "info" // Default level
	if scope == ScopeSystem && logCfg.SystemLevel != "" {
		levelStr = logCfg.SystemLevel
	} else if logCfg.Level != "" {
		levelStr = logCfg.Level
	}
	consoleLevel = parseLevelOrInfo(levelStr)

	fileLevel = consoleLevel
	if logCfg.File.Level != "" {
		fileLevel = parseLevelOrInfo(logCfg.File.Level)
	}
	return consoleLevel, fileLevel
}

// mostVerbose returns the more verbose of two levels (logrus levels are
// numerically inverted: Debug=5 > Info=4).
func mostVerbose(a, b logrus.Level) logrus.Level {
	if a > b {
		return a
	}
	return b
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

	// Load configuration from grove.yml, starting with defaults
	cfg, err := config.LoadDefault()
	logCfg := GetDefaultLoggingConfig() // Start with defaults
	if err == nil {
		// Use UnmarshalExtension to safely decode the logging part
		// This overlays user config on top of the defaults
		if err := cfg.UnmarshalExtension("logging", &logCfg); err != nil {
			// Log a warning if parsing fails, but continue with defaults
			logrus.Warnf("Failed to parse 'logging' config: %v", err)
		}
	}

	scopeMu.RLock()
	currentScope := activeScope
	scopeMu.RUnlock()

	// Configure Level. The logrus level must admit the most verbose sink;
	// the console output is filtered back down to consoleLevel via
	// levelFilteringFormatter, and the file sink via FileHook.LogLevels.
	consoleLevel, fileLevel := resolveLevels(&logCfg, currentScope)
	logger.SetLevel(mostVerbose(consoleLevel, fileLevel))
	setResolvedConsoleLevel(consoleLevel)
	setResolvedPrettyFields(resolvePrettyFields(&logCfg))

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

	// Configure File Sink.
	//
	// In `go test` binaries the IMPLICIT default sinks — the XDG
	// system/workspace log files the daemon TUI's log view tails — are
	// disabled. Executor-style tests run real job lifecycles in-process, and
	// without this every `go test` run sprays lifecycle events ("Job
	// launched/finished") and deliberate failure-path errors into the live
	// log stream, where they read exactly like production traffic. Explicit
	// destinations still work under test: GROVE_LOG_FILE and a configured
	// File.Path are honored, so tests that want file logs can opt in.
	fileSinkAllowed := os.Getenv("GROVE_LOG_FILE") != "" || logCfg.File.Path != "" || !IsTestBinary()
	if logCfg.File.Enabled && fileSinkAllowed {
		// pathFn derives the log file path for a point in time so the
		// dateRotatingWriter can reopen date-patterned paths when the day
		// changes. Fixed paths (env override, explicit config) never roll.
		var pathFn func(time.Time) string

		if envPath := os.Getenv("GROVE_LOG_FILE"); envPath != "" {
			p := expandPath(envPath)
			pathFn = func(time.Time) string { return p }
		} else if currentScope == ScopeSystem {
			// System scope: write to central XDG state directory
			pathFn = func(now time.Time) string {
				return filepath.Join(paths.StateDir(), "logs", fmt.Sprintf("system-%s.log", now.Format("2006-01-02")))
			}
		} else if logCfg.File.Path != "" {
			// Use explicitly configured path
			p := expandPath(logCfg.File.Path)
			pathFn = func(time.Time) string { return p }
		} else {
			// Default to XDG state directory organized by workspace identifier
			cwd, err := os.Getwd()
			if err == nil {
				if _, err := os.Stat(cwd); err != nil {
					cwd = ""
				}
			}

			if cwd != "" {
				node, err := workspace.GetProjectByPath(cwd)
				if err == nil && node != nil {
					identifier := node.Identifier("/")
					pathFn = func(now time.Time) string {
						return filepath.Join(paths.StateDir(), "logs", "workspaces", identifier, fmt.Sprintf("workspace-%s.log", now.Format("2006-01-02")))
					}
				} else {
					pathFn = func(now time.Time) string {
						return filepath.Join(paths.StateDir(), "logs", fmt.Sprintf("system-%s.log", now.Format("2006-01-02")))
					}
				}
			}
		}

		if pathFn != nil {
			writer, err := newDateRotatingWriter(pathFn, nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "grove-log: failed to open log file: %v\n", err)
			} else {
				var fileFormatter logrus.Formatter
				if logCfg.File.Format == "json" {
					fileFormatter = &logrus.JSONFormatter{}
				} else {
					fileFormatter = &TextFormatter{Config: FormatConfig{DisableTimestamp: false}}
				}
				logger.AddHook(&FileHook{
					Writer:    writer,
					LogLevels: logrus.AllLevels[:fileLevel+1],
					Formatter: fileFormatter,
				})
			}
		}
	}

	// Determine if we should write structured logs to stderr
	shouldLogToStderr := false
	suppressDualEmit := false
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
		// Use consoleLevel, not logger.GetLevel(): the logrus level may be
		// raised to satisfy a more verbose file sink (file.level=debug)
		// without the console being in debug mode.
		isDebug := os.Getenv("GROVE_DEBUG") == "1" || consoleLevel >= logrus.DebugLevel
		isInteractive := isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())
		if isDebug || !isInteractive {
			shouldLogToStderr = true
		}
		// Piped/redirected human-facing output (non-TTY, not debugging):
		// unified log entries already print a pretty line to the same
		// stream, so skip the raw structured duplicate on the console.
		// File sinks (FileHook) are unaffected and still capture
		// everything; StructuredOnly entries are not marked and still
		// reach the console.
		suppressDualEmit = !isDebug && !isInteractive
	}

	// Check component visibility based on show/hide configuration
	isVisible := IsComponentVisible(component, &logCfg)

	// Use the global writer instead of os.Stderr to support TUI redirection
	if shouldLogToStderr && isVisible {
		logger.SetOutput(GetGlobalOutput())
		if suppressDualEmit {
			logger.SetFormatter(&dualEmitSuppressingFormatter{inner: logger.Formatter})
		}
		if consoleLevel < logger.GetLevel() {
			// The logrus level admits entries for a more verbose file sink;
			// filter them out of the console output here (outermost wrapper).
			logger.SetFormatter(&levelFilteringFormatter{maxLevel: consoleLevel, inner: logger.Formatter})
		}
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

	// Config-load schema warnings buffer inside the config package until a
	// logging pipeline exists (config cannot import this package). The first
	// fully-configured logger becomes their sink, so early-load warnings —
	// including ones fired at package-init time — reach the file sink and
	// honor the TUI-safe console gating above.
	schemaWarnSinkOnce.Do(func() { registerSchemaWarningSink(logger) })

	entry := logger.WithField("component", component)
	loggers[component] = entry
	return entry
}

var schemaWarnSinkOnce sync.Once

// registerSchemaWarningSink points config's schema-warning channel at a
// configured logger. The closure can fire inside a config.Load made by
// NewLogger itself (loggersMu held), so it must log through the captured
// logger directly and never call NewLogger.
func registerSchemaWarningSink(logger *logrus.Logger) {
	entry := logger.WithField("component", "config")
	config.SetSchemaWarningSink(func(source string, err error) {
		entry.WithError(err).WithField("source", source).
			Warn("configuration does not fully conform to the schema (continuing; validation is advisory)")
	})
}

// dualEmitSuppressingFormatter wraps a formatter and emits nothing for
// entries already rendered via the unified pretty path (see dualEmitKey in
// unified.go). It is installed only on the console output of loggers running
// in "auto" stderr mode with a non-interactive stderr, where the pretty line
// and the raw structured line would otherwise both land on the same stream.
// File sinks use their own formatter via FileHook and are not affected.
type dualEmitSuppressingFormatter struct {
	inner logrus.Formatter
}

// Format implements logrus.Formatter.
func (f *dualEmitSuppressingFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	if isDualEmit(entry) {
		return nil, nil
	}
	return f.inner.Format(entry)
}

// levelFilteringFormatter wraps the console formatter and emits nothing for
// entries more verbose than maxLevel. It is installed when the file sink's
// level is more verbose than the console level: the logrus logger level must
// admit the verbose entries so the FileHook can capture them, so the console
// output filters them here instead. File sinks use their own formatter via
// FileHook and are not affected.
type levelFilteringFormatter struct {
	maxLevel logrus.Level
	inner    logrus.Formatter
}

// Format implements logrus.Formatter.
func (f *levelFilteringFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	if entry.Level > f.maxLevel {
		return nil, nil
	}
	return f.inner.Format(entry)
}

// dateRotatingWriter writes to a path derived from the current time and
// reopens the file when the derived path changes (i.e. at midnight for
// date-patterned paths). There is intentionally NO intra-day size-based
// rename rotation: log tailers follow a single fd and only switch when a
// file with a different name appears, so renaming the live file would
// silently detach them. Retention of old dated files is handled by the
// grove daemon sweep (see FileSinkConfig.RetentionDays), not here.
type dateRotatingWriter struct {
	mu      sync.Mutex
	pathFn  func(time.Time) string
	now     func() time.Time
	curPath string
	file    *os.File
}

// newDateRotatingWriter opens the file for the current time. nowFn is
// injectable for tests; nil means time.Now.
func newDateRotatingWriter(pathFn func(time.Time) string, nowFn func() time.Time) (*dateRotatingWriter, error) {
	if nowFn == nil {
		nowFn = time.Now
	}
	w := &dateRotatingWriter{pathFn: pathFn, now: nowFn}
	if err := w.reopen(w.pathFn(w.now())); err != nil {
		return nil, err
	}
	return w, nil
}

// reopen opens path (creating parent directories) and swaps it in as the
// current file. Callers must hold w.mu (or be the constructor).
func (w *dateRotatingWriter) reopen(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
	if err != nil {
		return err
	}
	if w.file != nil {
		w.file.Close()
	}
	w.file = f
	w.curPath = path
	return nil
}

// Write implements io.Writer, rolling to the new path first when the
// derived path has changed since the last write.
func (w *dateRotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if path := w.pathFn(w.now()); path != w.curPath {
		if err := w.reopen(path); err != nil && w.file == nil {
			return 0, err
		}
		// On reopen failure with a still-open previous file, keep writing
		// to the old fd rather than dropping the entry.
	}
	return w.file.Write(p)
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

// IsTestBinary reports whether this process is a `go test` binary. Compiled
// test executables are named <pkg>.test (also on the cached-build path,
// .../bNNN/<pkg>.test); as a fallback the testing package's registered
// test.v flag is checked, which covers `go test -o` renames once flags are
// registered. Deliberately dependency-free: importing the testing package
// from production code would link it into every grove binary.
//
// Exported so other side-effecting subsystems (e.g. flow's job-completion
// ntfy pushes) can refuse to touch production resources from plain unit
// tests — only the tend e2e harness sandboxes XDG/config paths; `go test`
// runs otherwise inherit the developer's real environment.
func IsTestBinary() bool {
	if strings.HasSuffix(os.Args[0], ".test") {
		return true
	}
	return flag.Lookup("test.v") != nil
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
	setResolvedConsoleLevel(logrus.InfoLevel)
	setResolvedPrettyFields(false)

	scopeMu.Lock()
	activeScope = ScopeWorkspace
	scopeMu.Unlock()
}
