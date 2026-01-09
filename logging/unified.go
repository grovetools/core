package logging

import (
	"context"
	"fmt"
	"regexp"
	"runtime"

	"github.com/mattsolo1/grove-core/tui/theme"
	"github.com/sirupsen/logrus"
)

// ansiRegex matches ANSI escape sequences for stripping
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// UnifiedLogger creates log entries that write to both pretty and structured outputs.
// It provides a builder pattern API for creating log messages that are rendered
// both as user-facing styled output and as structured audit logs.
type UnifiedLogger struct {
	component  string
	pretty     *PrettyLogger
	structured *logrus.Entry
}

// NewUnifiedLogger creates a new unified logger for a specific component.
// The component name is used for structured log filtering and identification.
func NewUnifiedLogger(component string) *UnifiedLogger {
	structured := NewLogger(component)
	// Disable logrus's built-in ReportCaller - we manually track the caller
	// in logStructured() so that "file" and "func" point to the actual call site,
	// not the unified logger wrapper.
	structured.Logger.SetReportCaller(false)

	return &UnifiedLogger{
		component:  component,
		pretty:     NewPrettyLogger(),
		structured: structured,
	}
}

// Debug returns a LogEntry at DEBUG level.
// Debug messages are hidden in pretty output by default (shown with --verbose or GROVE_LOG_LEVEL=debug).
func (u *UnifiedLogger) Debug(msg string) *LogEntry {
	return &LogEntry{
		logger: u,
		msg:    msg,
		level:  logrus.DebugLevel,
		fields: logrus.Fields{},
	}
}

// Info returns a LogEntry at INFO level.
func (u *UnifiedLogger) Info(msg string) *LogEntry {
	return &LogEntry{
		logger: u,
		msg:    msg,
		level:  logrus.InfoLevel,
		fields: logrus.Fields{},
	}
}

// Warn returns a LogEntry at WARN level with IconWarning.
func (u *UnifiedLogger) Warn(msg string) *LogEntry {
	return &LogEntry{
		logger: u,
		msg:    msg,
		level:  logrus.WarnLevel,
		fields: logrus.Fields{},
		icon:   theme.IconWarning,
	}
}

// Error returns a LogEntry at ERROR level with IconError.
func (u *UnifiedLogger) Error(msg string) *LogEntry {
	return &LogEntry{
		logger: u,
		msg:    msg,
		level:  logrus.ErrorLevel,
		fields: logrus.Fields{},
		icon:   theme.IconError,
	}
}

// Success returns a LogEntry with IconSuccess pre-set.
// Success messages are logged at INFO level with status=success in structured output.
func (u *UnifiedLogger) Success(msg string) *LogEntry {
	return &LogEntry{
		logger: u,
		msg:    msg,
		level:  logrus.InfoLevel,
		fields: logrus.Fields{"status": "success"},
		icon:   theme.IconSuccess,
	}
}

// Progress returns a LogEntry with IconRunning pre-set.
// Progress messages indicate in-progress operations.
func (u *UnifiedLogger) Progress(msg string) *LogEntry {
	return &LogEntry{
		logger: u,
		msg:    msg,
		level:  logrus.InfoLevel,
		fields: logrus.Fields{"status": "progress"},
		icon:   theme.IconRunning,
	}
}

// Status returns a LogEntry with IconInfo pre-set.
// Status messages provide informational updates.
func (u *UnifiedLogger) Status(msg string) *LogEntry {
	return &LogEntry{
		logger: u,
		msg:    msg,
		level:  logrus.InfoLevel,
		fields: logrus.Fields{"status": "info"},
		icon:   theme.IconInfo,
	}
}

// LogEntry accumulates options before writing to both outputs.
// Use the chainable methods to configure the entry, then call Log(ctx) to execute.
type LogEntry struct {
	logger     *UnifiedLogger
	msg        string
	level      logrus.Level
	fields     logrus.Fields
	icon       string
	prettyMsg  string // Custom styled output for CLI/TUI (from .Pretty())
	prettyOnly bool
	structOnly bool
	noIcon     bool
	err        error
}

// Field adds a structured field (chainable).
// Fields appear in structured logs but not in pretty output unless included in .Pretty().
func (e *LogEntry) Field(key string, value interface{}) *LogEntry {
	e.fields[key] = value
	return e
}

// Fields adds multiple structured fields (chainable).
func (e *LogEntry) Fields(fields map[string]interface{}) *LogEntry {
	for k, v := range fields {
		e.fields[k] = v
	}
	return e
}

// Err attaches an error (chainable).
// The error message is added to structured output as the "error" field.
func (e *LogEntry) Err(err error) *LogEntry {
	if err != nil {
		e.err = err
		e.fields["error"] = err.Error()
	}
	return e
}

// Icon overrides the default icon (chainable).
// Use theme.Icon* constants for consistent styling.
func (e *LogEntry) Icon(icon string) *LogEntry {
	e.icon = icon
	return e
}

// NoIcon suppresses the icon in pretty output (chainable).
func (e *LogEntry) NoIcon() *LogEntry {
	e.noIcon = true
	return e
}

// Pretty sets a custom lipgloss-styled string for CLI/TUI output (chainable).
// The msg from Info/Warn/etc. is used for structured logs (clean, no ANSI).
// The Pretty string is used for user-facing display (fully styled).
//
// Example:
//
//	ulog.Info("API call completed").
//	    Field("status", 201).
//	    Pretty(theme.IconSuccess + " " + theme.DefaultTheme.Success.Render("201 Created")).
//	    Log(ctx)
func (e *LogEntry) Pretty(styled string) *LogEntry {
	e.prettyMsg = styled
	return e
}

// PrettyOnly skips structured output (chainable).
// Use this for messages that should only appear in user-facing output.
func (e *LogEntry) PrettyOnly() *LogEntry {
	e.prettyOnly = true
	return e
}

// StructuredOnly skips pretty output (chainable).
// Use this for detailed audit logging that shouldn't clutter the UI.
func (e *LogEntry) StructuredOnly() *LogEntry {
	e.structOnly = true
	return e
}

// Log executes the log entry, writing to both outputs based on configuration.
// This is the terminal method that must be called for the log to be written.
func (e *LogEntry) Log(ctx context.Context) {
	// Compute the pretty output once (used by both logPretty and logStructured)
	prettyOutput := e.computePrettyOutput()

	// Pretty output (to context writer -> CLI + job.log)
	// Add extra newline for visual spacing between log entries in TUI
	if !e.structOnly {
		writer := GetWriter(ctx)
		fmt.Fprintf(writer, "%s\n\n", prettyOutput)
	}

	// Structured output (to workspace log + core logs)
	if !e.prettyOnly {
		e.logStructured(prettyOutput)
	}
}

// computePrettyOutput generates the styled output string.
func (e *LogEntry) computePrettyOutput() string {
	var output string
	if e.prettyMsg != "" {
		// Use custom styled message provided via .Pretty()
		output = e.prettyMsg
	} else {
		// Auto-style based on level/icon
		output = e.msg
		if !e.noIcon {
			icon := e.icon
			if icon == "" {
				// Default icon for entries without a specific icon
				icon = theme.IconBullet
			}
			output = icon + " " + e.msg
		}
	}

	// Apply level-based styling if no custom pretty message was set
	if e.prettyMsg == "" {
		styles := DefaultPrettyStyles()
		switch e.level {
		case logrus.WarnLevel:
			output = styles.Warning.Render(output)
		case logrus.ErrorLevel:
			output = styles.Error.Render(output)
		case logrus.DebugLevel:
			// Debug messages may be filtered at a higher level
			// but we still style them if they make it through
			output = styles.Key.Render(output) // muted style for debug
		default:
			// Info level and semantic methods (Success, Progress, Status)
			// get their styling from their icons
			if e.icon == theme.IconSuccess {
				output = styles.Success.Render(output)
			} else if e.icon == theme.IconRunning {
				output = styles.Info.Render(output)
			} else if e.icon == theme.IconInfo {
				output = styles.Info.Render(output)
			}
		}
	}

	return output
}

// logStructured writes the structured log entry to logrus.
func (e *LogEntry) logStructured(prettyOutput string) {
	// Capture the actual caller (skip: 0=logStructured, 1=Log, 2=actual call site)
	// Since we disabled ReportCaller in NewUnifiedLogger, we can use "file" and "func"
	// directly without collision - same fields as regular log calls use.
	if pc, file, line, ok := runtime.Caller(2); ok {
		fn := runtime.FuncForPC(pc)
		funcName := ""
		if fn != nil {
			funcName = fn.Name()
		}
		e.fields["file"] = fmt.Sprintf("%s:%d", file, line)
		e.fields["func"] = funcName
	}

	// Add both pretty output versions for viewer flexibility
	// pretty_ansi: with ANSI escape codes (can be rendered in terminals)
	// pretty_text: stripped of ANSI (clean text for display/search)
	e.fields["pretty_ansi"] = prettyOutput
	e.fields["pretty_text"] = ansiRegex.ReplaceAllString(prettyOutput, "")

	entry := e.logger.structured.WithFields(e.fields)
	entry.Log(e.level, e.msg) // Always uses clean msg, never prettyMsg
}

// Component returns the component name for this logger.
func (u *UnifiedLogger) Component() string {
	return u.component
}

// WithStructured returns the underlying logrus entry for direct structured logging.
// Use this when you need to bypass the unified pattern for specific cases.
func (u *UnifiedLogger) WithStructured() *logrus.Entry {
	return u.structured
}

// WithPretty returns the underlying PrettyLogger for direct pretty logging.
// Use this when you need to bypass the unified pattern for specific cases.
func (u *UnifiedLogger) WithPretty() *PrettyLogger {
	return u.pretty
}
