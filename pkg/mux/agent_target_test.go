package mux

import "testing"

func TestAgentTargetFor(t *testing.T) {
	tests := []struct {
		name          string
		active        MuxType
		groveTerminal bool
		want          string
	}{
		{"tuimux pane", MuxTuimux, false, AgentTargetTuimux},
		{"tuimux pane also exporting GROVE_TERMINAL", MuxTuimux, true, AgentTargetTuimux},
		{"grove terminal pane", MuxNone, true, AgentTargetNative},
		{"tmux session", MuxTmux, false, AgentTargetTmux},
		{"bare shell", MuxNone, false, AgentTargetTmux},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := agentTargetFor(tt.active, tt.groveTerminal); got != tt.want {
				t.Errorf("agentTargetFor(%q, %v) = %q, want %q", tt.active, tt.groveTerminal, got, tt.want)
			}
		})
	}
}

func TestResolveAgentTarget_FromEnvironment(t *testing.T) {
	t.Setenv(EnvTuimuxPTY, "1")
	if got := ResolveAgentTarget(); got != AgentTargetTuimux {
		t.Errorf("ResolveAgentTarget() under tuimux = %q, want %q", got, AgentTargetTuimux)
	}

	t.Setenv(EnvTuimuxPTY, "")
	t.Setenv(EnvGroveTerminal, "1")
	if got := ResolveAgentTarget(); got != AgentTargetNative {
		t.Errorf("ResolveAgentTarget() under a grove terminal = %q, want %q", got, AgentTargetNative)
	}
}

// The TUI's hosted flag comes from the panel wrapper that constructed it and is
// trusted over the environment, which cannot distinguish a hosted pane from any
// other process a grove terminal spawned.
func TestResolveAgentTargetHosted(t *testing.T) {
	t.Setenv(EnvTuimuxPTY, "1")
	if got := ResolveAgentTargetHosted(true); got != AgentTargetNative {
		t.Errorf("ResolveAgentTargetHosted(true) = %q, want %q", got, AgentTargetNative)
	}
	if got := ResolveAgentTargetHosted(false); got != AgentTargetTuimux {
		t.Errorf("ResolveAgentTargetHosted(false) under tuimux = %q, want %q", got, AgentTargetTuimux)
	}

	t.Setenv(EnvTuimuxPTY, "")
	t.Setenv(EnvGroveTerminal, "1")
	if got := ResolveAgentTargetHosted(false); got != AgentTargetTmux {
		t.Errorf("ResolveAgentTargetHosted(false) = %q, want %q: an unhosted TUI must not claim a native pane", got, AgentTargetTmux)
	}
}
