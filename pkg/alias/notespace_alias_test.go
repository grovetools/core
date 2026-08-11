package alias

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/notespace"
	"github.com/grovetools/core/pkg/workspace"
)

func TestResolveResourceAliasUsesStampedNameNotCodeProvider(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("GROVE_HOME", "")
	configDir := filepath.Join(xdg, "grove")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	id := "01J00000000000000000000001"
	subj := "example.com/org/repo"
	machine := "[primaries]\n\"" + subj + "\" = \"" + id + "\"\n"
	if err := os.WriteFile(filepath.Join(configDir, config.MachineConfigFileName), []byte(machine), 0o600); err != nil {
		t.Fatal(err)
	}

	notebookRoot := t.TempDir()
	root := filepath.Join(notebookRoot, workspace.NotespaceDirectory, "friendly")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := notespace.InstallNotespace(root, notespace.NotespaceStamp{ID: id, Name: "friendly", Subject: subj, Kind: "repo"}); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Notebooks: &config.NotebooksConfig{Definitions: map[string]*config.Notebook{"nb": {RootDir: notebookRoot}}}}

	// Provider deliberately contains no node called "friendly". Notes aliases
	// must not consult the code-plane name index.
	r := &AliasResolver{Provider: workspace.NewProviderFromNodes(nil), config: cfg}
	r.providerOnce.Do(func() {})
	got, err := r.ResolveResourceAlias("friendly:plans/p.md")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "plans", "p.md")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
