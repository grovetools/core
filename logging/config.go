package logging

//go:generate sh -c "cd .. && go run ./tools/logging-schema-generator/"

// DefaultHide is the default list of components/groups to hide when no
// show or hide rules are configured. The current project is still visible
// due to ShowCurrentProject defaulting to true.
var DefaultHide = []string{"grove-ecosystem"}

// DefaultGroups contains built-in component groups that users can reference
// in show/hide without defining them. User-defined groups take precedence.
var DefaultGroups = map[string][]string{
	"grove-ecosystem": {
		"grove-agent-logs",
		"grove-context",
		"grove-core",
		"grove-docgen",
		"grove-flow",
		"grove-gemini",
		"grove-hooks",
		"grove-mcp",
		"grove-meta",
		"grove-notebook",
		"grove-notifications",
		"grove-nvim",
		"grove-proxy",
		"grove-sandbox",
		"grove-tend",
		"grove-tmux",
	},
	"ai": {
		"grove-gemini",
		"grove-context",
	},
}

// ComponentFilteringConfig defines rules for showing/hiding logs from components.
type ComponentFilteringConfig struct {
	// Only is a strict whitelist. If set, only logs from these components/groups will be shown.
	Only []string `yaml:"only,omitempty" toml:"only,omitempty" jsonschema:"description=Strict whitelist of components/groups to show (ignores show/hide)" jsonschema_extras:"x-layer=global,x-priority=85"`

	// Show ensures logs from these components/groups are visible, overriding any 'hide' rules.
	// It does not act as a whitelist.
	Show []string `yaml:"show,omitempty" toml:"show,omitempty" jsonschema:"description=Components/groups to always show (overrides hide)" jsonschema_extras:"x-layer=global,x-priority=86"`

	// Hide is a blacklist of components or groups to silence.
	// This is ignored if 'only' is set. 'show' overrides 'hide'.
	Hide []string `yaml:"hide,omitempty" toml:"hide,omitempty" jsonschema:"description=Components/groups to hide from log output" jsonschema_extras:"x-layer=global,x-priority=87"`
}

// Config defines the structure for logging configuration in grove.yml.
type Config struct {
	// Level is the minimum log level to output (e.g., "debug", "info", "warn", "error").
	// Can be overridden by the GROVE_LOG_LEVEL environment variable.
	Level string `yaml:"level" toml:"level" jsonschema:"description=Minimum log level (debug/info/warn/error),default=info,enum=debug,enum=info,enum=warn,enum=error" jsonschema_extras:"x-layer=global,x-priority=60"`

	// SystemLevel is the minimum log level for system-scoped logging (daemon, global tools).
	// When set, overrides Level for processes running in ScopeSystem.
	// Prefer file.level for targeted debug capture in the file sink, or the
	// GROVE_LOG_LEVEL=debug environment variable for one-shot debugging;
	// system_level=debug makes the daemon verbose on every sink.
	SystemLevel string `yaml:"system_level,omitempty" toml:"system_level,omitempty" jsonschema:"description=Minimum log level for system/daemon logs (debug/info/warn/error). Prefer file.level for targeted file capture or GROVE_LOG_LEVEL=debug for one-shot debugging,enum=debug,enum=info,enum=warn,enum=error" jsonschema_extras:"x-layer=global,x-priority=61"`

	// ReportCaller, if true, includes the file, line, and function name in the log output.
	// Can be enabled with the GROVE_LOG_CALLER=true environment variable.
	ReportCaller bool `yaml:"report_caller" toml:"report_caller" jsonschema:"description=Include file/line/function in log output,default=true" jsonschema_extras:"x-layer=global,x-priority=65"`

	// LogStartup, if true, logs "Grove binary started" on first logger initialization.
	// Defaults to false.
	LogStartup bool `yaml:"log_startup" toml:"log_startup" jsonschema:"description=Log 'Grove binary started' on first init,default=false" jsonschema_extras:"x-layer=global,x-priority=90"`

	// File configures logging to a file.
	File FileSinkConfig `yaml:"file" toml:"file" jsonschema:"description=File logging sink configuration" jsonschema_extras:"x-layer=global,x-priority=70"`

	// Format configures the appearance of the log output.
	Format FormatConfig `yaml:"format" toml:"format" jsonschema:"description=Log output format settings" jsonschema_extras:"x-layer=global,x-priority=75"`

	// StructuredPrettyFields, if true, embeds the console-rendered form of
	// each unified log entry into its structured record as pretty_ansi
	// (styled) and pretty_text (ANSI-stripped) fields, for log viewers that
	// want to replay the exact styled console line (`core logs
	// --format=pretty`, the TUI log detail pane). Off by default: the fields
	// add roughly 10% to log volume and cost a lipgloss render plus an ANSI
	// strip per entry, and viewers fall back to msg when they are absent.
	// The GROVE_LOG_PRETTY_FIELDS environment variable (true/false)
	// overrides this setting.
	StructuredPrettyFields bool `yaml:"structured_pretty_fields,omitempty" toml:"structured_pretty_fields,omitempty" jsonschema:"description=Embed rendered pretty_ansi/pretty_text fields in structured log entries (adds ~10% log volume; GROVE_LOG_PRETTY_FIELDS overrides),default=false" jsonschema_extras:"x-layer=global,x-priority=79"`

	// Groups defines named collections of component loggers for easy filtering.
	// Example:
	//   groups:
	//     ai: [grove-gemini, grove-context]
	//     devops: [grove-proxy, grove-deploy]
	Groups map[string][]string `yaml:"groups,omitempty" toml:"groups,omitempty" jsonschema:"description=Named collections of component loggers for filtering" jsonschema_extras:"x-layer=global,x-priority=80"`

	// ComponentFiltering contains all rules for filtering logs by component.
	ComponentFiltering *ComponentFilteringConfig `yaml:"component_filtering,omitempty" toml:"component_filtering,omitempty" jsonschema:"description=Rules for filtering logs by component" jsonschema_extras:"x-layer=global,x-priority=85"`

	// ShowCurrentProject, if true (default), always shows logs from the current project
	// regardless of show/hide settings. The current project is determined from grove.yml name.
	ShowCurrentProject *bool `yaml:"show_current_project,omitempty" toml:"show_current_project,omitempty" jsonschema:"description=Always show logs from current project regardless of filters" jsonschema_extras:"x-layer=global,x-priority=88"`
}

// FileSinkConfig configures the file logging sink.
type FileSinkConfig struct {
	Enabled bool `yaml:"enabled" toml:"enabled" jsonschema:"description=Enable file logging,default=true" jsonschema_extras:"x-layer=global,x-priority=70"`
	// Path is the full path to the log file.
	Path   string `yaml:"path" toml:"path" jsonschema:"description=Full path to the log file" jsonschema_extras:"x-layer=global,x-priority=71"`
	Format string `yaml:"format,omitempty" toml:"format,omitempty" jsonschema:"description=File log format: text or json,default=json,enum=text,enum=json" jsonschema_extras:"x-layer=global,x-priority=72"`
	// Level is the minimum log level for the file sink only. When unset, the
	// file sink follows the console level. Useful for capturing debug detail
	// in the audit trail without making the console verbose.
	// GROVE_LOG_LEVEL overrides both the console and file levels.
	Level string `yaml:"level,omitempty" toml:"level,omitempty" jsonschema:"description=Minimum log level for the file sink only (defaults to the console level; GROVE_LOG_LEVEL overrides both),enum=debug,enum=info,enum=warn,enum=error" jsonschema_extras:"x-layer=global,x-priority=73"`
	// RetentionDays is how many days of dated log files to keep. Old files
	// are swept by the grove daemon; files for the current day are never
	// removed. 0 means use the default (14).
	RetentionDays int `yaml:"retention_days,omitempty" toml:"retention_days,omitempty" jsonschema:"description=Days of dated log files to keep before the daemon sweeps them (0 = default of 14),default=14" jsonschema_extras:"x-layer=global,x-priority=74"`
}

// FormatConfig controls the log output format.
type FormatConfig struct {
	// Preset can be "default" (rich text), "simple" (minimal text), or "json".
	Preset string `yaml:"preset" toml:"preset" jsonschema:"description=Log format preset: default (rich)/simple/json,enum=default,enum=simple,enum=json" jsonschema_extras:"x-layer=global,x-priority=75"`
	// DisableTimestamp disables the timestamp from the "default" and "simple" formats.
	DisableTimestamp bool `yaml:"disable_timestamp" toml:"disable_timestamp" jsonschema:"description=Disable timestamp in log output,default=false" jsonschema_extras:"x-layer=global,x-priority=76"`
	// DisableComponent disables the component name from the "default" and "simple" formats.
	DisableComponent bool `yaml:"disable_component" toml:"disable_component" jsonschema:"description=Disable component name in log output,default=false" jsonschema_extras:"x-layer=global,x-priority=77"`
	// StructuredToStderr controls when structured logs are sent to stderr.
	// Can be "auto" (default), "always", or "never".
	StructuredToStderr string `yaml:"structured_to_stderr" toml:"structured_to_stderr" jsonschema:"description=When to send structured logs to stderr,enum=auto,enum=always,enum=never,default=auto" jsonschema_extras:"x-layer=global,x-priority=78"`
}

// GetDefaultLoggingConfig returns a Config with sensible defaults that enable
// file logging and caller reporting out of the box. This allows `core logs`
// to work immediately without any user configuration.
func GetDefaultLoggingConfig() Config {
	return Config{
		Level:        "info",
		ReportCaller: true,
		File: FileSinkConfig{
			Enabled:       true,
			Format:        "json",
			RetentionDays: 14,
		},
	}
}
