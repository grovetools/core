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
			Commands: map[string]interface{}{
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

func TestResolveEnvironment_DefaultAlias(t *testing.T) {
	// "default" should alias to the unnamed base [environment] block,
	// matching the behavior of passing "".
	cfg := &Config{
		Environment: &EnvironmentConfig{
			Provider: "native",
			Config: map[string]interface{}{
				"port": 8080,
			},
			Commands: map[string]interface{}{"build": "make build"},
		},
		Environments: map[string]*EnvironmentConfig{
			"docker": {Provider: "docker"},
		},
	}

	resolved, err := ResolveEnvironment(cfg, "default")
	if err != nil {
		t.Fatalf("expected \"default\" to resolve without error, got %v", err)
	}
	if resolved.Provider != "native" {
		t.Errorf("expected provider 'native' from base env, got %q", resolved.Provider)
	}
	if resolved.Config["port"] != 8080 {
		t.Errorf("expected port 8080 from base env, got %v", resolved.Config["port"])
	}
	if resolved.Commands["build"] != "make build" {
		t.Errorf("expected build command inherited, got %q", resolved.Commands["build"])
	}
}

func TestResolveEnvironmentWithProvenance_DefaultAlias(t *testing.T) {
	layered := &LayeredConfig{
		Project: &Config{
			Environment: &EnvironmentConfig{
				Provider: "native",
				Config:   map[string]interface{}{"port": 8080},
			},
		},
	}

	resolved, _, _, err := ResolveEnvironmentWithProvenance(layered, "default")
	if err != nil {
		t.Fatalf("expected \"default\" to resolve without error, got %v", err)
	}
	if resolved.Provider != "native" {
		t.Errorf("expected provider native, got %q", resolved.Provider)
	}
	if resolved.Config["port"] != 8080 {
		t.Errorf("expected port 8080, got %v", resolved.Config["port"])
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
			Commands: map[string]interface{}{
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
				Commands: map[string]interface{}{
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

// TestResolveEnvironmentWithProvenance_NotebookOverridesEcosystem verifies a
// project-notebook layer overrides ecosystem defaults and that provenance
// reflects the correct origin for each key.
func TestResolveEnvironmentWithProvenance_NotebookOverridesEcosystem(t *testing.T) {
	layered := &LayeredConfig{
		Ecosystem: &Config{
			Environment: &EnvironmentConfig{
				Provider: "native",
				Config: map[string]interface{}{
					"services": map[string]interface{}{
						"api": map[string]interface{}{
							"command": "cargo run",
							"port":    8080,
						},
					},
				},
				Commands: map[string]interface{}{"build": "make build"},
			},
		},
		ProjectNotebook: &Config{
			Environment: &EnvironmentConfig{
				Config: map[string]interface{}{
					"services": map[string]interface{}{
						"api": map[string]interface{}{
							"port": 9090,
						},
					},
				},
				Commands: map[string]interface{}{"logs": "tail -f /tmp/logs"},
			},
		},
	}

	resolved, prov, deleted, err := ResolveEnvironmentWithProvenance(layered, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.Provider != "native" {
		t.Errorf("expected provider native, got %q", resolved.Provider)
	}
	api := resolved.Config["services"].(map[string]interface{})["api"].(map[string]interface{})
	if api["command"] != "cargo run" {
		t.Errorf("expected ecosystem command inherited, got %v", api["command"])
	}
	if api["port"] != 9090 {
		t.Errorf("expected notebook port override, got %v", api["port"])
	}

	if got := prov["provider"]; got != "ecosystem (environment)" {
		t.Errorf("expected provider prov = ecosystem, got %q", got)
	}
	if got := prov["config.services.api.command"]; got != "ecosystem (environment)" {
		t.Errorf("expected api.command ecosystem, got %q", got)
	}
	if got := prov["config.services.api.port"]; got != "project-notebook (environment)" {
		t.Errorf("expected api.port project-notebook, got %q", got)
	}
	if got := prov["commands.build"]; got != "ecosystem (environment)" {
		t.Errorf("expected commands.build ecosystem, got %q", got)
	}
	if got := prov["commands.logs"]; got != "project-notebook (environment)" {
		t.Errorf("expected commands.logs project-notebook, got %q", got)
	}
	if len(deleted) != 0 {
		t.Errorf("expected no deletions, got %v", deleted)
	}
}

// TestResolveEnvironmentWithProvenance_NamedProfileDelete verifies that a
// named profile can drop inherited blocks with _delete = true and that the
// deleted map records the dropping layer.
func TestResolveEnvironmentWithProvenance_NamedProfileDelete(t *testing.T) {
	layered := &LayeredConfig{
		Project: &Config{
			Environment: &EnvironmentConfig{
				Provider: "native",
				Config: map[string]interface{}{
					"services": map[string]interface{}{
						"clickhouse": map[string]interface{}{"command": "clickhouse server"},
						"api":        map[string]interface{}{"command": "cargo run"},
					},
				},
			},
			Environments: map[string]*EnvironmentConfig{
				"hybrid-api": {
					Provider: "terraform",
					Config: map[string]interface{}{
						"services": map[string]interface{}{
							"clickhouse": map[string]interface{}{"_delete": true},
							"web":        map[string]interface{}{"command": "npm run dev"},
						},
					},
				},
			},
		},
	}

	resolved, prov, deleted, err := ResolveEnvironmentWithProvenance(layered, "hybrid-api")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.Provider != "terraform" {
		t.Errorf("expected provider terraform from profile overlay, got %q", resolved.Provider)
	}
	services := resolved.Config["services"].(map[string]interface{})
	if _, present := services["clickhouse"]; present {
		t.Errorf("expected services.clickhouse dropped")
	}
	if _, present := services["api"]; !present {
		t.Errorf("expected services.api inherited from default env")
	}
	if _, present := services["web"]; !present {
		t.Errorf("expected services.web added from hybrid-api profile")
	}

	if got := prov["provider"]; got != "project (environments.hybrid-api)" {
		t.Errorf("expected provider prov = hybrid-api, got %q", got)
	}
	if got := prov["config.services.api.command"]; got != "project (environment)" {
		t.Errorf("expected api.command default env prov, got %q", got)
	}
	if got := prov["config.services.web.command"]; got != "project (environments.hybrid-api)" {
		t.Errorf("expected web.command profile prov, got %q", got)
	}
	if _, present := prov["config.services.clickhouse.command"]; present {
		t.Errorf("expected clickhouse.command provenance pruned after delete")
	}
	if got := deleted["config.services.clickhouse"]; got != "project (environments.hybrid-api)" {
		t.Errorf("expected delete recorded against profile layer, got %q", got)
	}
}

// TestResolveEnvironmentWithProvenance_MultiLayerStack verifies a realistic
// stack (ecosystem default + notebook overlay + project profile) produces
// the correct per-key provenance across all three layers.
func TestResolveEnvironmentWithProvenance_MultiLayerStack(t *testing.T) {
	layered := &LayeredConfig{
		Ecosystem: &Config{
			Environment: &EnvironmentConfig{
				Provider: "native",
				Config: map[string]interface{}{
					"domain": "grove.local",
					"port":   8080,
				},
				Commands: map[string]interface{}{"build": "make build"},
			},
		},
		ProjectNotebook: &Config{
			Environment: &EnvironmentConfig{
				Config: map[string]interface{}{
					"port": 9000,
				},
			},
		},
		Project: &Config{
			Environments: map[string]*EnvironmentConfig{
				"cloud": {
					Provider: "terraform",
					Config: map[string]interface{}{
						"port":   9090,
						"region": "us-central1",
					},
					Commands: map[string]interface{}{"deploy": "terraform apply"},
				},
			},
		},
	}

	resolved, prov, _, err := ResolveEnvironmentWithProvenance(layered, "cloud")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.Provider != "terraform" {
		t.Errorf("expected provider terraform, got %q", resolved.Provider)
	}
	if resolved.Config["domain"] != "grove.local" {
		t.Errorf("expected domain inherited from ecosystem, got %v", resolved.Config["domain"])
	}
	if resolved.Config["port"] != 9090 {
		t.Errorf("expected port overridden by profile, got %v", resolved.Config["port"])
	}
	if resolved.Config["region"] != "us-central1" {
		t.Errorf("expected region from profile, got %v", resolved.Config["region"])
	}

	if got := prov["provider"]; got != "project (environments.cloud)" {
		t.Errorf("expected provider prov = project profile, got %q", got)
	}
	if got := prov["config.domain"]; got != "ecosystem (environment)" {
		t.Errorf("expected domain prov = ecosystem, got %q", got)
	}
	if got := prov["config.port"]; got != "project (environments.cloud)" {
		t.Errorf("expected port prov = project profile, got %q", got)
	}
	if got := prov["config.region"]; got != "project (environments.cloud)" {
		t.Errorf("expected region prov = project profile, got %q", got)
	}
	if got := prov["commands.build"]; got != "ecosystem (environment)" {
		t.Errorf("expected build prov = ecosystem, got %q", got)
	}
	if got := prov["commands.deploy"]; got != "project (environments.cloud)" {
		t.Errorf("expected deploy prov = project profile, got %q", got)
	}
}

// TestResolveEnvironmentWithProvenance_MissingProfile errors when a profile
// name is given but no layer defines it.
func TestResolveEnvironmentWithProvenance_MissingProfile(t *testing.T) {
	layered := &LayeredConfig{
		Project: &Config{Environment: &EnvironmentConfig{Provider: "native"}},
	}
	_, _, _, err := ResolveEnvironmentWithProvenance(layered, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing profile")
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

func TestIsSharedProfile(t *testing.T) {
	tru := true
	fals := false

	cfg := &Config{
		Environments: map[string]*EnvironmentConfig{
			"terraform-infra": {Provider: "terraform", Shared: &tru},
			"legacy-shared": {
				Provider: "terraform",
				Config: map[string]interface{}{
					"state_bucket": "kitchen-env-state",
				},
			},
			"hybrid-api": {
				Provider: "terraform",
				Config: map[string]interface{}{
					"shared_env": "legacy-shared",
				},
			},
			"terraform": {
				Provider: "terraform",
				Config: map[string]interface{}{
					"shared_env": "terraform-infra",
				},
			},
			"docker-local":  {Provider: "docker"},
			"explicit-leaf": {Provider: "terraform", Shared: &fals},
		},
	}

	tests := []struct {
		name    string
		profile string
		want    bool
	}{
		{"explicit shared=true wins", "terraform-infra", true},
		{"implicit via shared_env pointing here", "legacy-shared", true},
		{"per-worktree profile is not shared", "hybrid-api", false},
		{"per-worktree terraform profile is not shared", "terraform", false},
		{"docker profile is not shared", "docker-local", false},
		{"explicit shared=false stays not shared even if referenced", "explicit-leaf", false},
		{"unknown profile is not shared", "ghost", false},
		{"empty profile is not shared", "", false},
		{"literal 'default' is not shared", "default", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsSharedProfile(cfg, tc.profile); got != tc.want {
				t.Errorf("IsSharedProfile(%q) = %v, want %v", tc.profile, got, tc.want)
			}
		})
	}

	if IsSharedProfile(nil, "terraform-infra") {
		t.Error("IsSharedProfile(nil, …) should return false")
	}
}
