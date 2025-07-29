package config

// Config represents the grove.yml configuration
type Config struct {
	Version       string                    `yaml:"version"`
	Networks      map[string]NetworkConfig  `yaml:"networks"`
	Services      map[string]ServiceConfig  `yaml:"services"`
	Volumes       map[string]VolumeConfig   `yaml:"volumes"`
	Profiles      map[string]ProfileConfig  `yaml:"profiles"`
	Agent         AgentConfig               `yaml:"agent,omitempty"`
	Settings      Settings                  `yaml:"settings"`
	Orchestration OrchestrationConfig       `yaml:"orchestration,omitempty"`
}

type NetworkConfig struct {
	External bool   `yaml:"external"`
	Driver   string `yaml:"driver"`
}

type VolumeConfig struct {
	External bool   `yaml:"external"`
	Driver   string `yaml:"driver"`
}

type ServiceConfig struct {
	Build       interface{}       `yaml:"build"`
	Image       string            `yaml:"image"`
	Ports       []string          `yaml:"ports"`
	Environment []string          `yaml:"environment"`
	EnvFile     []string          `yaml:"env_file"`
	Volumes     []string          `yaml:"volumes"`
	DependsOn   []string          `yaml:"depends_on"`
	Labels      map[string]string `yaml:"labels"`
	Command     interface{}       `yaml:"command"`
	Profiles    []string          `yaml:"profiles"`
}

type ProfileConfig struct {
	Services []string `yaml:"services"`
	EnvFile  []string `yaml:"env_file"`
}

// AgentConfig defines the configuration for the built-in Grove agent
type AgentConfig struct {
	Enabled      bool     `yaml:"enabled"`
	Image        string   `yaml:"image"`
	LogsPath     string   `yaml:"logs_path"`
	ExtraVolumes []string `yaml:"extra_volumes"`
	NotesDir     string   `yaml:"notes_dir,omitempty"`
	Args         []string `yaml:"args"`
	OutputFormat string   `yaml:"output_format"` // text (default), json, or stream-json
}

type Settings struct {
	ProjectName    string `yaml:"project_name"`
	DefaultProfile string `yaml:"default_profile"`
	TraefikEnabled *bool  `yaml:"traefik_enabled,omitempty"`
	NetworkName    string `yaml:"network_name"`
	DomainSuffix   string `yaml:"domain_suffix"`
	AutoInference  *bool  `yaml:"auto_inference,omitempty"`
	Concurrency    int    `yaml:"concurrency,omitempty"`
	McpPort        int    `yaml:"mcp_port,omitempty"`
}

// OrchestrationConfig defines settings for orchestration jobs
type OrchestrationConfig struct {
	OneshotModel         string `yaml:"oneshot_model"`
	TargetAgentContainer string `yaml:"target_agent_container,omitempty"`
	PlansDirectory       string `yaml:"plans_directory,omitempty"`
	MaxConsecutiveSteps  int    `yaml:"max_consecutive_steps,omitempty"`
}

// SetDefaults sets default values for configuration
func (c *Config) SetDefaults() {
	if c.Version == "" {
		c.Version = "1.0"
	}
	if c.Settings.DomainSuffix == "" {
		c.Settings.DomainSuffix = "localhost"
	}
	if c.Settings.NetworkName == "" {
		c.Settings.NetworkName = "grove"
	}
	// Enable Traefik by default
	if c.Settings.TraefikEnabled == nil {
		enabled := true
		c.Settings.TraefikEnabled = &enabled
	}

	// Set Agent defaults
	if c.Agent.Enabled {
		if c.Agent.Image == "" {
			// Default to v0.1.0 for now - this should be injected from CLI
			c.Agent.Image = "ghcr.io/grovepm/grove-agent:v0.1.0"
		}
		if c.Agent.LogsPath == "" {
			c.Agent.LogsPath = "~/.claude/projects"
		}
	}

	// Set Settings defaults
	if c.Settings.McpPort == 0 {
		c.Settings.McpPort = 1667
	}
}
