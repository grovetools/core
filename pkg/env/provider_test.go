package env

import (
	"testing"
)

func TestResolveProvider(t *testing.T) {
	// Mock client for built-in providers
	mock := &MockDaemonEnvClient{}

	tests := []struct {
		name         string
		providerName string
		wantType     string
	}{
		{"native returns DaemonProvider", "native", "*env.DaemonProvider"},
		{"docker returns DaemonProvider", "docker", "*env.DaemonProvider"},
		{"cloud returns ExecProvider", "cloud", "*env.ExecProvider"},
		{"custom-plugin returns ExecProvider", "custom-plugin", "*env.ExecProvider"},
		{"empty string returns ExecProvider", "", "*env.ExecProvider"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := ResolveProvider(tt.providerName, mock, "")
			gotType := typeString(p)
			if gotType != tt.wantType {
				t.Errorf("ResolveProvider(%q) = %s, want %s", tt.providerName, gotType, tt.wantType)
			}
		})
	}
}

func TestResolveProvider_NilClient(t *testing.T) {
	// Exec providers should work without a client
	p := ResolveProvider("my-exec-plugin", nil, "")
	if _, ok := p.(*ExecProvider); !ok {
		t.Errorf("expected *ExecProvider, got %T", p)
	}
}

func typeString(p Provider) string {
	switch p.(type) {
	case *DaemonProvider:
		return "*env.DaemonProvider"
	case *ExecProvider:
		return "*env.ExecProvider"
	default:
		return "unknown"
	}
}
