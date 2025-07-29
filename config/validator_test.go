package config

import (
	"strings"
	"testing"
)

func TestSchemaValidation(t *testing.T) {
	validator, err := NewSchemaValidator()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		config    map[string]interface{}
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid config",
			config: map[string]interface{}{
				"services": map[string]interface{}{
					"web": map[string]interface{}{
						"build": ".",
						"ports": []interface{}{"8080:80"},
					},
				},
			},
			wantError: false,
		},
		{
			name: "missing required services",
			config: map[string]interface{}{
				"version": "3.8",
			},
			wantError: true,
			errorMsg:  "missing required field 'services'",
		},
		{
			name: "invalid port format",
			config: map[string]interface{}{
				"services": map[string]interface{}{
					"web": map[string]interface{}{
						"ports": []interface{}{"invalid-port"},
					},
				},
			},
			wantError: true,
			errorMsg:  "does not match pattern",
		},
		{
			name: "invalid project name",
			config: map[string]interface{}{
				"services": map[string]interface{}{
					"web": map[string]interface{}{
						"build": ".",
					},
				},
				"settings": map[string]interface{}{
					"project_name": "My-Project!", // Invalid characters
				},
			},
			wantError: true,
			errorMsg:  "does not match pattern",
		},
		{
			name: "valid settings",
			config: map[string]interface{}{
				"services": map[string]interface{}{
					"web": map[string]interface{}{
						"build": ".",
					},
				},
				"settings": map[string]interface{}{
					"project_name":    "myproject",
					"enable_traefik":  true,
					"network_name":    "mynetwork",
					"domain_suffix":   "test.local",
					"default_profile": "dev",
				},
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.config)
			if tt.wantError {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestCircularDependencies(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		wantError bool
		errorMsg  string
	}{
		{
			name: "no dependencies",
			config: &Config{
				Services: map[string]ServiceConfig{
					"web": {},
					"db":  {},
				},
			},
			wantError: false,
		},
		{
			name: "simple dependency",
			config: &Config{
				Services: map[string]ServiceConfig{
					"web": {DependsOn: []string{"db"}},
					"db":  {},
				},
			},
			wantError: false,
		},
		{
			name: "circular dependency",
			config: &Config{
				Services: map[string]ServiceConfig{
					"web": {DependsOn: []string{"db"}},
					"db":  {DependsOn: []string{"web"}},
				},
			},
			wantError: true,
			errorMsg:  "circular dependency detected",
		},
		{
			name: "indirect circular dependency",
			config: &Config{
				Services: map[string]ServiceConfig{
					"web":   {DependsOn: []string{"api"}},
					"api":   {DependsOn: []string{"db"}},
					"db":    {DependsOn: []string{"cache"}},
					"cache": {DependsOn: []string{"web"}},
				},
			},
			wantError: true,
			errorMsg:  "circular dependency detected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.checkCircularDependencies()
			if tt.wantError {
				if err == nil {
					t.Error("expected error but got none")
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestPortConflicts(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		wantError bool
		errorMsg  string
	}{
		{
			name: "no port conflicts",
			config: &Config{
				Services: map[string]ServiceConfig{
					"web": {Ports: []string{"8080:80"}},
					"api": {Ports: []string{"8081:80"}},
				},
			},
			wantError: false,
		},
		{
			name: "port conflict",
			config: &Config{
				Services: map[string]ServiceConfig{
					"web": {Ports: []string{"8080:80"}},
					"api": {Ports: []string{"8080:3000"}},
				},
			},
			wantError: true,
			errorMsg:  "port 8080 is used by both",
		},
		{
			name: "multiple ports no conflict",
			config: &Config{
				Services: map[string]ServiceConfig{
					"web": {Ports: []string{"8080:80", "8443:443"}},
					"api": {Ports: []string{"3000:3000", "3001:3001"}},
				},
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.checkPortConflicts()
			if tt.wantError {
				if err == nil {
					t.Error("expected error but got none")
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestServiceReferences(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid references",
			config: &Config{
				Services: map[string]ServiceConfig{
					"web": {DependsOn: []string{"db"}},
					"db":  {},
				},
				Profiles: map[string]ProfileConfig{
					"dev": {Services: []string{"web", "db"}},
				},
			},
			wantError: false,
		},
		{
			name: "invalid dependency reference",
			config: &Config{
				Services: map[string]ServiceConfig{
					"web": {DependsOn: []string{"missing"}},
				},
			},
			wantError: true,
			errorMsg:  "depends on non-existent service 'missing'",
		},
		{
			name: "invalid profile reference",
			config: &Config{
				Services: map[string]ServiceConfig{
					"web": {},
				},
				Profiles: map[string]ProfileConfig{
					"dev": {Services: []string{"web", "missing"}},
				},
			},
			wantError: true,
			errorMsg:  "references non-existent service 'missing'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.validateServiceReferences()
			if tt.wantError {
				if err == nil {
					t.Error("expected error but got none")
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}
