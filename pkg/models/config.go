package models

import "time"

// Config represents the complete application configuration
type Config struct {
	Server                    ServerConfig                    `yaml:"server"`
	Storage                   StorageConfig                   `yaml:"storage"`
	Database                  DatabaseConfig                  `yaml:"database"`
	ToolSummarization         ToolSummarizationConfig         `yaml:"tool_summarization"`
	MessageExtraction         MessageExtractionConfig         `yaml:"message_extraction"`
	ConversationSummarization ConversationSummarizationConfig `yaml:"conversation_summarization"`
	SDK                       SDKConfig                       `yaml:"sdk"`
}

// ServerConfig holds HTTP/WebSocket server settings
type ServerConfig struct {
	APIPort       int           `yaml:"api_port"`
	WebSocketPort int           `yaml:"websocket_port"`
	ReadTimeout   time.Duration `yaml:"read_timeout"`
	WriteTimeout  time.Duration `yaml:"write_timeout"`
}

// StorageConfig defines storage configuration options
type StorageConfig struct {
	Environment      string           `yaml:"environment"` // "production", "development", "test"
	StateFile        string           `yaml:"state_file"`
	TmuxConfigDir    string           `yaml:"tmux_config_dir"`
	TmuxSessionsFile string           `yaml:"tmux_sessions_file"`
	TestMode         TestModeConfig   `yaml:"test_mode"`
	Expiration       ExpirationConfig `yaml:"expiration"`
}

// DatabaseConfig holds database-specific configuration
type DatabaseConfig struct {
	Path         string `yaml:"path"`
	MaxOpenConns int    `yaml:"max_open_conns"`
	MaxIdleConns int    `yaml:"max_idle_conns"`
}

// TestModeConfig defines test-specific settings
type TestModeConfig struct {
	Enabled          bool   `yaml:"enabled"`
	EphemeralStorage bool   `yaml:"ephemeral_storage"`
	AutoCleanup      bool   `yaml:"auto_cleanup"`
	TestPrefix       string `yaml:"test_prefix"`
	ForceDelete      bool   `yaml:"force_delete"`
}

// ExpirationConfig defines data expiration settings
type ExpirationConfig struct {
	TestDataTTL     time.Duration `yaml:"test_data_ttl"`
	CleanupInterval time.Duration `yaml:"cleanup_interval"`
	StaleDataTTL    time.Duration `yaml:"stale_data_ttl"`
	TestSessionTTL  time.Duration `yaml:"test_session_ttl"`
}

// ToolSummarizationConfig holds configuration for LLM-based tool output summarization
type ToolSummarizationConfig struct {
	Enabled       bool     `yaml:"enabled"`
	LLMCommand    string   `yaml:"llm_command"`
	MaxOutputSize int      `yaml:"max_output_size"`
	ToolWhitelist []string `yaml:"tool_whitelist"`
}

// MessageExtractionConfig holds configuration for message extraction
type MessageExtractionConfig struct {
	Enabled       bool `yaml:"enabled"`
	CheckInterval int  `yaml:"check_interval"` // seconds
	Incremental   bool `yaml:"incremental"`    // use file offset tracking
}

// ConversationSummarizationConfig holds configuration for conversation summarization
type ConversationSummarizationConfig struct {
	Enabled            bool   `yaml:"enabled"`
	LLMCommand         string `yaml:"llm_command"`
	UpdateInterval     int    `yaml:"update_interval"` // update every N messages
	CurrentWindow      int    `yaml:"current_window"`  // messages for current activity
	RecentWindow       int    `yaml:"recent_window"`   // messages for recent context
	MaxInputTokens     int    `yaml:"max_input_tokens"`
	MilestoneDetection bool   `yaml:"milestone_detection"`
}

// SDKConfig holds configuration for Claude Code SDK integration
type SDKConfig struct {
	Enabled        bool       `yaml:"enabled"`
	NodeExecutable string     `yaml:"node_executable"`
	ScriptPath     string     `yaml:"script_path"`
	Options        SDKOptions `yaml:"options"`
}

// SDKOptions holds SDK-specific options
type SDKOptions struct {
	MaxTurns     int      `yaml:"max_turns"`
	AllowedTools []string `yaml:"allowed_tools"`
}

// NotificationConfig holds notification settings
type NotificationConfig struct {
	Enabled     bool            `yaml:"enabled"`
	MaxRetries  int             `yaml:"max_retries"`
	RetryDelay  time.Duration   `yaml:"retry_delay"`
	Preferences map[string]bool `yaml:"preferences"` // Per-type enable/disable
}

// SummaryConfig is an alias for ConversationSummarizationConfig
// to maintain compatibility while reducing duplication
type SummaryConfig = ConversationSummarizationConfig
