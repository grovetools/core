package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentConfig(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		want    AgentConfig
		wantErr bool
	}{
		{
			name: "agent disabled by default",
			yaml: `
version: "1.0"
services:
  test:
    image: alpine:latest`,
			want: AgentConfig{
				Enabled: false,
			},
		},
		{
			name: "agent with minimal config",
			yaml: `
version: "1.0"
services:
  test:
    image: alpine:latest
agent:
  enabled: true`,
			want: AgentConfig{
				Enabled:  true,
				Image:    "ghcr.io/grovepm/grove-agent:v0.1.0",
				LogsPath: "~/.claude/projects",
			},
		},
		{
			name: "agent with full config",
			yaml: `
version: "1.0"
services:
  test:
    image: alpine:latest
agent:
  enabled: true
  image: custom/agent:v2
  logs_path: ~/custom/logs
  extra_volumes:
    - /data:/data:ro
    - /config:/config`,
			want: AgentConfig{
				Enabled:  true,
				Image:    "custom/agent:v2",
				LogsPath: "~/custom/logs",
				ExtraVolumes: []string{
					"/data:/data:ro",
					"/config:/config",
				},
			},
		},
		{
			name: "agent with empty volumes array",
			yaml: `
version: "1.0"
services:
  test:
    image: alpine:latest
agent:
  enabled: true
  extra_volumes: []`,
			want: AgentConfig{
				Enabled:      true,
				Image:        "ghcr.io/grovepm/grove-agent:v0.1.0",
				LogsPath:     "~/.claude/projects",
				ExtraVolumes: []string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := LoadFromBytes([]byte(tt.yaml))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			cfg.SetDefaults()
			assert.Equal(t, tt.want, cfg.Agent)
		})
	}
}

func TestAgentConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		agent   AgentConfig
		wantErr string
	}{
		{
			name: "valid disabled agent",
			agent: AgentConfig{
				Enabled: false,
			},
		},
		{
			name: "valid enabled agent",
			agent: AgentConfig{
				Enabled: true,
				Image:   "agent:latest",
			},
		},
		{
			name: "invalid - empty image when enabled",
			agent: AgentConfig{
				Enabled: true,
				Image:   "",
			},
			wantErr: "agent.image cannot be empty when agent is enabled",
		},
		{
			name: "valid - disabled with empty image",
			agent: AgentConfig{
				Enabled: false,
				Image:   "",
			},
		},
		{
			name: "valid with extra volumes",
			agent: AgentConfig{
				Enabled:      true,
				Image:        "agent:latest",
				ExtraVolumes: []string{"/data:/data:ro", "/config:/config"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Version: "1.0",
				Agent:   tt.agent,
				Services: map[string]ServiceConfig{
					"test": {Image: "alpine:latest"},
				},
				Settings: Settings{
					NetworkName:  "grove",
					DomainSuffix: "localhost",
				},
			}

			// Only set defaults for valid cases to test validation properly
			if tt.wantErr == "" {
				cfg.SetDefaults()
			}

			err := cfg.Validate()

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAgentConfigDefaultsExtended(t *testing.T) {
	tests := []struct {
		name     string
		input    AgentConfig
		expected AgentConfig
	}{
		{
			name: "disabled agent gets no defaults",
			input: AgentConfig{
				Enabled: false,
			},
			expected: AgentConfig{
				Enabled: false,
			},
		},
		{
			name: "enabled agent gets defaults",
			input: AgentConfig{
				Enabled: true,
			},
			expected: AgentConfig{
				Enabled:  true,
				Image:    "ghcr.io/grovepm/grove-agent:v0.1.0",
				LogsPath: "~/.claude/projects",
			},
		},
		{
			name: "enabled agent with custom image keeps image",
			input: AgentConfig{
				Enabled: true,
				Image:   "custom:latest",
			},
			expected: AgentConfig{
				Enabled:  true,
				Image:    "custom:latest",
				LogsPath: "~/.claude/projects",
			},
		},
		{
			name: "enabled agent with custom logs path keeps path",
			input: AgentConfig{
				Enabled:  true,
				LogsPath: "/custom/logs",
			},
			expected: AgentConfig{
				Enabled:  true,
				Image:    "ghcr.io/grovepm/grove-agent:v0.1.0",
				LogsPath: "/custom/logs",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Version: "1.0",
				Agent:   tt.input,
			}
			cfg.SetDefaults()
			assert.Equal(t, tt.expected, cfg.Agent)
		})
	}
}