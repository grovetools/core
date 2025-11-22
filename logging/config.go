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
		"grove-openai",
		"grove-proxy",
		"grove-sandbox",
		"grove-tend",
		"grove-tmux",
	},
	"ai": {
		"grove-gemini",
		"grove-openai",
		"grove-context",
	},
}

// Config defines the structure for logging configuration in grove.yml.
type Config struct {
	// Level is the minimum log level to output (e.g., "debug", "info", "warn", "error").
	// Can be overridden by the GROVE_LOG_LEVEL environment variable.
	Level string `yaml:"level"`

	// ReportCaller, if true, includes the file, line, and function name in the log output.
	// Can be enabled with the GROVE_LOG_CALLER=true environment variable.
	ReportCaller bool `yaml:"report_caller"`

	// LogStartup, if true, logs "Grove binary started" on first logger initialization.
	// Defaults to false.
	LogStartup bool `yaml:"log_startup"`

	// File configures logging to a file.
	File FileSinkConfig `yaml:"file"`

	// Format configures the appearance of the log output.
	Format FormatConfig `yaml:"format"`

	// Groups defines named collections of component loggers for easy filtering.
	// Example:
	//   groups:
	//     ai: [grove-gemini, grove-context]
	//     devops: [grove-proxy, grove-deploy]
	Groups map[string][]string `yaml:"groups,omitempty"`

	// Show is a whitelist of components or groups to display in console logs.
	// If this is set, all other components will be silenced. Takes precedence over 'hide'.
	Show []string `yaml:"show,omitempty"`

	// Hide is a blacklist of components or groups to silence in console logs.
	// This is ignored if 'show' is set.
	Hide []string `yaml:"hide,omitempty"`

	// ShowCurrentProject, if true (default), always shows logs from the current project
	// regardless of show/hide settings. The current project is determined from grove.yml name.
	ShowCurrentProject *bool `yaml:"show_current_project,omitempty"`
}

// FileSinkConfig configures the file logging sink.
type FileSinkConfig struct {
	Enabled bool   `yaml:"enabled"`
	// Path is the full path to the log file.
	Path   string `yaml:"path"`
	Format string `yaml:"format,omitempty"` // "text" (default) or "json"
}

// FormatConfig controls the log output format.
type FormatConfig struct {
	// Preset can be "default" (rich text), "simple" (minimal text), or "json".
	Preset string `yaml:"preset"`
	// DisableTimestamp disables the timestamp from the "default" and "simple" formats.
	DisableTimestamp bool `yaml:"disable_timestamp"`
	// DisableComponent disables the component name from the "default" and "simple" formats.
	DisableComponent bool `yaml:"disable_component"`
	// StructuredToStderr controls when structured logs are sent to stderr.
	// Can be "auto" (default), "always", or "never".
	StructuredToStderr string `yaml:"structured_to_stderr"`
}