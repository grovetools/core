package logging

//go:generate sh -c "cd .. && go run ./tools/logging-schema-generator/"

// Config defines the structure for logging configuration in grove.yml.
type Config struct {
	// Level is the minimum log level to output (e.g., "debug", "info", "warn", "error").
	// Can be overridden by the GROVE_LOG_LEVEL environment variable.
	Level string `yaml:"level"`

	// ReportCaller, if true, includes the file, line, and function name in the log output.
	// Can be enabled with the GROVE_LOG_CALLER=true environment variable.
	ReportCaller bool `yaml:"report_caller"`

	// File configures logging to a file.
	File FileSinkConfig `yaml:"file"`

	// Format configures the appearance of the log output.
	Format FormatConfig `yaml:"format"`
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