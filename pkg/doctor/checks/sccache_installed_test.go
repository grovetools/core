package checks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/grovetools/core/pkg/doctor"
)

const cargoServiceToml = `
[environments.hybrid-api.config.services.api]
command = "cd kitchen-app/api && cargo run"
`

const noCargoToml = `
[environments.docker-local.config.services.web]
command = "npm run dev"
`

func newSccacheFixture(t *testing.T, tomlBody, sccachePath string) (*sccacheInstalledCheck, string) {
	t.Helper()
	scope := t.TempDir()
	if tomlBody != "" {
		if err := os.WriteFile(filepath.Join(scope, "grove.toml"), []byte(tomlBody), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c := &sccacheInstalledCheck{
		getenv: func(k string) string {
			if k == "GROVE_SCOPE" {
				return scope
			}
			return ""
		},
		getwd:        func() (string, error) { return scope, nil },
		resolveScope: func(string) string { return scope },
		lookPath: func(name string) (string, error) {
			if sccachePath == "" {
				return "", exec.ErrNotFound
			}
			return sccachePath, nil
		},
		runVersion: func(path string) (string, error) { return "sccache 0.8.2", nil },
	}
	return c, scope
}

func TestSccache_CargoServiceSccacheAbsent_Warn(t *testing.T) {
	c, _ := newSccacheFixture(t, cargoServiceToml, "")
	res := c.Run(context.Background(), doctor.RunOptions{})
	if res.Status != doctor.StatusWarn {
		t.Fatalf("expected Warn, got %s: %s", res.Status, res.Message)
	}
	if res.Resolution == "" {
		t.Fatalf("expected Resolution hint")
	}
}

func TestSccache_NoCargoServices_OK(t *testing.T) {
	c, _ := newSccacheFixture(t, noCargoToml, "")
	res := c.Run(context.Background(), doctor.RunOptions{})
	if res.Status != doctor.StatusOK {
		t.Fatalf("expected OK for no cargo services, got %s: %s", res.Status, res.Message)
	}
}

func TestSccache_CargoServiceSccachePresent_OK(t *testing.T) {
	c, _ := newSccacheFixture(t, cargoServiceToml, "/usr/local/bin/sccache")
	res := c.Run(context.Background(), doctor.RunOptions{})
	if res.Status != doctor.StatusOK {
		t.Fatalf("expected OK, got %s: %s", res.Status, res.Message)
	}
	if res.Message == "" || res.Message[:7] != "sccache" {
		t.Fatalf("expected version line, got %q", res.Message)
	}
}

func TestSccache_DotCargoConfigTriggers(t *testing.T) {
	c, scope := newSccacheFixture(t, noCargoToml, "")
	// Drop a .cargo/config.toml in a subdir — simulates the kitchen-app layout.
	sub := filepath.Join(scope, "kitchen-app", ".cargo")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "config.toml"), []byte(`[build]
rustc-wrapper = "sccache"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	res := c.Run(context.Background(), doctor.RunOptions{})
	if res.Status != doctor.StatusWarn {
		t.Fatalf("expected Warn (cargo config present, no sccache), got %s: %s", res.Status, res.Message)
	}
}

func TestSccache_AutoFixReturnsNotFixable(t *testing.T) {
	c, _ := newSccacheFixture(t, cargoServiceToml, "")
	err := c.AutoFix(context.Background())
	if err == nil || !errors.Is(err, doctor.ErrNotFixable) {
		t.Fatalf("expected ErrNotFixable, got %v", err)
	}
}

// Silence "unused" warnings for fmt when not needed.
var _ = fmt.Sprintf
