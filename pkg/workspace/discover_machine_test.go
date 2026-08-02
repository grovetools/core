package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/grovetools/core/config"
)

// Discovery must walk groves compiled from machine.toml, including on a
// machine that has NO global grove.toml at all — the dotfiles-restore shape,
// where machine.toml is the only declaration of intent.
func TestDiscoverAllWalksMachineSubscriptions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GROVE_HOME", home)
	config.ResetLoadCache()
	t.Cleanup(config.ResetLoadCache)

	configDir := filepath.Join(home, "config", "grove")
	codeRoot := filepath.Join(home, "code")
	ecoDir := filepath.Join(codeRoot, "myeco")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	if err := os.MkdirAll(ecoDir, 0o755); err != nil {
		t.Fatalf("mkdir ecosystem: %v", err)
	}
	// An ecosystem is a grove config carrying `workspaces`.
	write(t, filepath.Join(ecoDir, "grove.toml"), "name = \"myeco\"\nworkspaces = [\"*\"]\n")
	write(t, filepath.Join(configDir, "machine.toml"), `[machine]
name = "fixture"

[machine.ecosystems.myeco]
path = "`+ecoDir+`"
notebook = "nb"
`)

	// The compiled entry is in Final.Groves...
	layered, err := config.LoadLayered(home)
	if err != nil {
		t.Fatalf("LoadLayered: %v", err)
	}
	if _, ok := layered.Final.Groves["myeco"]; !ok {
		t.Fatalf("Final.Groves does not contain the compiled subscription: %v", layered.Final.Groves)
	}

	// ...and discovery walks it, with no global grove.toml in play.
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)
	result, err := NewDiscoveryService(logger).WithConfigPath(home).DiscoverAll()
	if err != nil {
		t.Fatalf("DiscoverAll: %v", err)
	}
	for _, eco := range result.Ecosystems {
		if eco.Name == "myeco" {
			return
		}
	}
	t.Fatalf("DiscoverAll did not find the subscribed ecosystem; ecosystems=%+v projects=%+v", result.Ecosystems, result.Projects)
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
