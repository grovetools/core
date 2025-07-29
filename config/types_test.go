package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAgentConfigDefaults(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		expected AgentConfig
	}{
		{
			name:   "agent disabled by default",
			config: Config{},
			expected: AgentConfig{
				Enabled: false,
			},
		},
		{
			name: "agent enabled with defaults",
			config: Config{
				Agent: AgentConfig{Enabled: true},
			},
			expected: AgentConfig{
				Enabled:  true,
				Image:    "ghcr.io/grovepm/grove-agent:v0.1.0",
				LogsPath: "~/.claude/projects",
			},
		},
		{
			name: "agent with custom config",
			config: Config{
				Agent: AgentConfig{
					Enabled:      true,
					Image:        "custom/agent:v1",
					LogsPath:     "~/custom/logs",
					ExtraVolumes: []string{"/data:/data"},
				},
			},
			expected: AgentConfig{
				Enabled:      true,
				Image:        "custom/agent:v1",
				LogsPath:     "~/custom/logs",
				ExtraVolumes: []string{"/data:/data"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.config.SetDefaults()
			assert.Equal(t, tt.expected, tt.config.Agent)
		})
	}
}

func TestMcpPortDefaults(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		expected int
	}{
		{
			name:     "default mcp port",
			config:   Config{},
			expected: 1667,
		},
		{
			name: "custom mcp port",
			config: Config{
				Settings: Settings{
					McpPort: 8080,
				},
			},
			expected: 8080,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.config.SetDefaults()
			assert.Equal(t, tt.expected, tt.config.Settings.McpPort)
		})
	}
}
