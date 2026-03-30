package config

import (
	"testing"
)

func TestResolveEnvironment_Default(t *testing.T) {
	cfg := &Config{
		Environment: &EnvironmentConfig{
			Provider: "native",
			Config: map[string]interface{}{
				"port": 8080,
				"host": "localhost",
			},
			Commands: map[string]string{
				"build": "make build",
				"test":  "make test",
			},
		},
	}

	resolved, err := ResolveEnvironment(cfg, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.Provider != "native" {
		t.Errorf("expected provider 'native', got %q", resolved.Provider)
	}
	if resolved.Config["port"] != 8080 {
		t.Errorf("expected port 8080, got %v", resolved.Config["port"])
	}
	if resolved.Config["host"] != "localhost" {
		t.Errorf("expected host 'localhost', got %v", resolved.Config["host"])
	}
	if resolved.Commands["build"] != "make build" {
		t.Errorf("expected build command 'make build', got %q", resolved.Commands["build"])
	}
	if resolved.Commands["test"] != "make test" {
		t.Errorf("expected test command 'make test', got %q", resolved.Commands["test"])
	}
}

func TestResolveEnvironment_NamedProfile(t *testing.T) {
	cfg := &Config{
		Environment: &EnvironmentConfig{
			Provider: "native",
			Config: map[string]interface{}{
				"port": 8080,
				"host": "localhost",
			},
			Commands: map[string]string{
				"build": "make build",
				"test":  "make test",
				"logs":  "tail -f /tmp/logs",
			},
		},
		Environments: map[string]*EnvironmentConfig{
			"docker": {
				Provider: "docker",
				Config: map[string]interface{}{
					"port": 9090,
					"tag":  "latest",
				},
				Commands: map[string]string{
					"build": "docker compose build",
					"logs":  "docker compose logs -f",
				},
			},
		},
	}

	resolved, err := ResolveEnvironment(cfg, "docker")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Provider overridden
	if resolved.Provider != "docker" {
		t.Errorf("expected provider 'docker', got %q", resolved.Provider)
	}
	// Config: port overridden, host inherited, tag added
	if resolved.Config["port"] != 9090 {
		t.Errorf("expected port 9090, got %v", resolved.Config["port"])
	}
	if resolved.Config["host"] != "localhost" {
		t.Errorf("expected host 'localhost' (inherited), got %v", resolved.Config["host"])
	}
	if resolved.Config["tag"] != "latest" {
		t.Errorf("expected tag 'latest' (added), got %v", resolved.Config["tag"])
	}
	// Commands: build and logs overridden, test inherited
	if resolved.Commands["build"] != "docker compose build" {
		t.Errorf("expected build 'docker compose build', got %q", resolved.Commands["build"])
	}
	if resolved.Commands["test"] != "make test" {
		t.Errorf("expected test 'make test' (inherited), got %q", resolved.Commands["test"])
	}
	if resolved.Commands["logs"] != "docker compose logs -f" {
		t.Errorf("expected logs 'docker compose logs -f', got %q", resolved.Commands["logs"])
	}
}

func TestResolveEnvironment_NotFound(t *testing.T) {
	cfg := &Config{
		Environment: &EnvironmentConfig{
			Provider: "native",
		},
	}

	_, err := ResolveEnvironment(cfg, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing profile, got nil")
	}
}

func TestResolveEnvironment_NilDefault(t *testing.T) {
	// No default environment, but a named profile exists
	cfg := &Config{
		Environments: map[string]*EnvironmentConfig{
			"cloud": {
				Provider: "cloud",
				Config: map[string]interface{}{
					"region": "us-central1",
				},
			},
		},
	}

	resolved, err := ResolveEnvironment(cfg, "cloud")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.Provider != "cloud" {
		t.Errorf("expected provider 'cloud', got %q", resolved.Provider)
	}
	if resolved.Config["region"] != "us-central1" {
		t.Errorf("expected region 'us-central1', got %v", resolved.Config["region"])
	}
}

func TestResolveEnvironment_DeepMergeConfig(t *testing.T) {
	// Test that nested config maps are deep-merged, not replaced
	cfg := &Config{
		Environment: &EnvironmentConfig{
			Provider: "native",
			Config: map[string]interface{}{
				"services": map[string]interface{}{
					"web": map[string]interface{}{
						"command":  "npm run dev",
						"port_env": "PORT",
					},
					"api": map[string]interface{}{
						"command":  "go run ./cmd/api",
						"port_env": "API_PORT",
					},
				},
			},
		},
		Environments: map[string]*EnvironmentConfig{
			"docker": {
				Provider: "docker",
				Config: map[string]interface{}{
					"services": map[string]interface{}{
						"web": map[string]interface{}{
							"container_port": 3000,
						},
					},
				},
			},
		},
	}

	resolved, err := ResolveEnvironment(cfg, "docker")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	services, ok := resolved.Config["services"].(map[string]interface{})
	if !ok {
		t.Fatal("expected services to be a map")
	}

	// Web service should have both inherited and overridden fields
	web, ok := services["web"].(map[string]interface{})
	if !ok {
		t.Fatal("expected web to be a map")
	}
	if web["container_port"] != 3000 {
		t.Errorf("expected container_port 3000, got %v", web["container_port"])
	}
	if web["port_env"] != "PORT" {
		t.Errorf("expected port_env 'PORT' (inherited), got %v", web["port_env"])
	}

	// API service should be fully inherited
	api, ok := services["api"].(map[string]interface{})
	if !ok {
		t.Fatal("expected api to be a map (inherited)")
	}
	if api["command"] != "go run ./cmd/api" {
		t.Errorf("expected api command inherited, got %v", api["command"])
	}
}
