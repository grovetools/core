package config

import (
	"reflect"
	"testing"
)

func TestInferDefaults(t *testing.T) {
	tests := []struct {
		name     string
		input    Config
		expected Config
	}{
		{
			name: "create default profile when none exist",
			input: Config{
				Services: map[string]ServiceConfig{
					"web": {Image: "nginx"},
					"api": {Image: "node"},
				},
				Profiles: map[string]ProfileConfig{},
				Settings: Settings{},
			},
			expected: Config{
				Services: map[string]ServiceConfig{
					"web": {Image: "nginx"},
					"api": {Image: "node"},
				},
				Profiles: map[string]ProfileConfig{
					"default": {Services: []string{"web", "api"}},
				},
				Settings: Settings{
					DefaultProfile: "default",
				},
			},
		},
		{
			name: "use existing default profile",
			input: Config{
				Services: map[string]ServiceConfig{
					"web": {Image: "nginx"},
				},
				Profiles: map[string]ProfileConfig{
					"default": {Services: []string{"web"}},
					"dev":     {Services: []string{"web"}},
				},
				Settings: Settings{},
			},
			expected: Config{
				Services: map[string]ServiceConfig{
					"web": {Image: "nginx"},
				},
				Profiles: map[string]ProfileConfig{
					"default": {Services: []string{"web"}},
					"dev":     {Services: []string{"web"}},
				},
				Settings: Settings{
					DefaultProfile: "default",
				},
			},
		},
		{
			name: "use first profile when no default exists",
			input: Config{
				Services: map[string]ServiceConfig{
					"web": {Image: "nginx"},
				},
				Profiles: map[string]ProfileConfig{
					"prod": {Services: []string{"web"}},
					"dev":  {Services: []string{"web"}},
				},
				Settings: Settings{},
			},
			expected: Config{
				Services: map[string]ServiceConfig{
					"web": {Image: "nginx"},
				},
				Profiles: map[string]ProfileConfig{
					"prod": {Services: []string{"web"}},
					"dev":  {Services: []string{"web"}},
				},
				Settings: Settings{
					DefaultProfile: "prod", // or "dev" - map iteration order isn't guaranteed
				},
			},
		},
		{
			name: "don't override existing settings",
			input: Config{
				Services: map[string]ServiceConfig{
					"web": {Image: "nginx"},
				},
				Profiles: map[string]ProfileConfig{
					"custom": {Services: []string{"web"}},
				},
				Settings: Settings{
					DefaultProfile: "custom",
				},
			},
			expected: Config{
				Services: map[string]ServiceConfig{
					"web": {Image: "nginx"},
				},
				Profiles: map[string]ProfileConfig{
					"custom": {Services: []string{"web"}},
				},
				Settings: Settings{
					DefaultProfile: "custom",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := tt.input
			config.InferDefaults()

			// Check services
			if !reflect.DeepEqual(config.Services, tt.expected.Services) {
				t.Errorf("Services mismatch: got %v, want %v", config.Services, tt.expected.Services)
			}

			// Check profiles exist
			if len(config.Profiles) != len(tt.expected.Profiles) {
				t.Errorf("Profile count mismatch: got %d, want %d", len(config.Profiles), len(tt.expected.Profiles))
			}

			// For the "use first profile" test, just verify a default was set
			if tt.name == "use first profile when no default exists" {
				if config.Settings.DefaultProfile == "" {
					t.Error("Expected default profile to be set")
				}
				if _, exists := config.Profiles[config.Settings.DefaultProfile]; !exists {
					t.Errorf("Default profile %s doesn't exist", config.Settings.DefaultProfile)
				}
			} else {
				// Check default profile
				if config.Settings.DefaultProfile != tt.expected.Settings.DefaultProfile {
					t.Errorf("DefaultProfile mismatch: got %s, want %s",
						config.Settings.DefaultProfile, tt.expected.Settings.DefaultProfile)
				}
			}
		})
	}
}

func TestInferServiceDefaults(t *testing.T) {
	tests := []struct {
		name     string
		service  ServiceConfig
		expected ServiceConfig
		skipTest bool // For tests that depend on filesystem
	}{
		{
			name: "keep existing build context",
			service: ServiceConfig{
				Build: ".",
			},
			expected: ServiceConfig{
				Build: ".",
			},
		},
		{
			name: "keep existing image",
			service: ServiceConfig{
				Image: "nginx:latest",
			},
			expected: ServiceConfig{
				Image: "nginx:latest",
			},
		},
		{
			name: "keep existing environment",
			service: ServiceConfig{
				Build:       ".",
				Environment: []string{"NODE_ENV=production", "PORT=3000"},
			},
			expected: ServiceConfig{
				Build:       ".",
				Environment: []string{"NODE_ENV=production", "PORT=3000"},
			},
		},
	}

	for _, tt := range tests {
		if tt.skipTest {
			continue
		}

		t.Run(tt.name, func(t *testing.T) {
			service := tt.service
			inferServiceDefaults("test", &service)

			if !reflect.DeepEqual(service, tt.expected) {
				t.Errorf("Service mismatch:\ngot  %+v\nwant %+v", service, tt.expected)
			}
		})
	}
}
