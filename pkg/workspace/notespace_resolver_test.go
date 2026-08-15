package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/notespace"
)

var resolverIDs = []string{
	"01J00000000000000000000001",
	"01J00000000000000000000002",
	"01J00000000000000000000003",
}

func makeStampedRoot(t *testing.T, notebookRoot, display, id, subj string) string {
	t.Helper()
	root := filepath.Join(notebookRoot, NotespaceDirectory, display)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := notespace.InstallNotespace(root, notespace.NotespaceStamp{ID: id, Name: display, Subject: subj, Kind: "repo"}); err != nil {
		t.Fatal(err)
	}
	return root
}

func resolverConfig(roots ...string) *config.Config {
	defs := make(map[string]*config.Notebook, len(roots))
	for i, root := range roots {
		defs[string(rune('a'+i))] = &config.Notebook{RootDir: root}
	}
	return &config.Config{Notebooks: &config.NotebooksConfig{Definitions: defs}}
}

func TestResolveNotespaceTripleSameNameRootsBySubject(t *testing.T) {
	var notebookRoots, repoRoots, subjects []string
	machine := &config.MachineConfig{Primaries: map[string]string{}, Subjects: map[string]string{}}
	for i := range 3 {
		nb := filepath.Join(t.TempDir(), "book")
		repo := filepath.Join(t.TempDir(), "repo")
		if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		subj := "example.com/org/repo" + string(rune('A'+i))
		makeStampedRoot(t, nb, "same-display", resolverIDs[i], subj)
		notebookRoots = append(notebookRoots, nb)
		repoRoots = append(repoRoots, repo)
		subjects = append(subjects, subj)
		machine.Subjects[repo] = subj
		machine.Primaries[subj] = resolverIDs[i]
	}
	cfg := resolverConfig(notebookRoots...)
	for i, repo := range repoRoots {
		got, err := ResolveNotespace(filepath.Join(repo, "subdir"), cfg, machine)
		if err != nil {
			t.Fatalf("root %d: %v", i, err)
		}
		if got.Subject != subjects[i] || got.NotespaceID != resolverIDs[i] || !strings.HasPrefix(got.Root, notebookRoots[i]) {
			t.Fatalf("root %d resolution = %+v", i, got)
		}
	}
}

func TestResolveNotespaceNestedSubmoduleUsesRepositorySubject(t *testing.T) {
	nb := t.TempDir()
	ecosystem := t.TempDir()
	if err := os.Mkdir(filepath.Join(ecosystem, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	submodule := filepath.Join(ecosystem, "daemon")
	deep := filepath.Join(submodule, "pkg", "service")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	gitFile := "gitdir: " + filepath.Join("..", ".git", "modules", "daemon") + "\n"
	if err := os.WriteFile(filepath.Join(submodule, ".git"), []byte(gitFile), 0o644); err != nil {
		t.Fatal(err)
	}

	const (
		ecosystemSubject = "example.com/org/ecosystem"
		submoduleSubject = "example.com/org/daemon"
	)
	makeStampedRoot(t, nb, "ecosystem", resolverIDs[0], ecosystemSubject)
	submoduleNotespace := makeStampedRoot(t, nb, "daemon", resolverIDs[1], submoduleSubject)
	machine := &config.MachineConfig{
		Subjects: map[string]string{
			ecosystem: ecosystemSubject,
			submodule: submoduleSubject,
		},
		Primaries: map[string]string{
			ecosystemSubject: resolverIDs[0],
			submoduleSubject: resolverIDs[1],
		},
	}

	got, err := ResolveNotespace(deep, resolverConfig(nb), machine)
	if err != nil {
		t.Fatal(err)
	}
	if got.Subject != submoduleSubject || got.NotespaceID != resolverIDs[1] || got.Root != submoduleNotespace {
		t.Fatalf("resolution = %+v; want nested submodule primary", got)
	}
}

func TestResolveNotespaceAbsentPrimary(t *testing.T) {
	nb, repo := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	makeStampedRoot(t, nb, "display", resolverIDs[0], "example.com/org/repo")
	_, err := ResolveNotespace(repo, resolverConfig(nb), &config.MachineConfig{Subjects: map[string]string{repo: "example.com/org/repo"}})
	if err == nil || !strings.Contains(err.Error(), "no recorded primary") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveNotespaceMalformedAndDuplicateStamps(t *testing.T) {
	t.Run("malformed", func(t *testing.T) {
		nb := t.TempDir()
		root := filepath.Join(nb, NotespaceDirectory, "bad")
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, notespace.NotespaceStampName), []byte("not toml = ["), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := ResolveNotespaceName("bad", resolverConfig(nb), &config.MachineConfig{}); err == nil {
			t.Fatal("expected malformed error")
		}
	})
	t.Run("duplicate", func(t *testing.T) {
		nb := t.TempDir()
		makeStampedRoot(t, nb, "one", resolverIDs[0], "example.com/a/one")
		makeStampedRoot(t, nb, "two", resolverIDs[0], "example.com/a/two")
		machine := &config.MachineConfig{Primaries: map[string]string{"example.com/a/one": resolverIDs[0]}}
		if _, err := ResolveNotespaceName("one", resolverConfig(nb), machine); err == nil || !strings.Contains(err.Error(), "duplicate notespace id") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestResolveNotespaceNameDiffersFromID(t *testing.T) {
	nb := t.TempDir()
	root := makeStampedRoot(t, nb, "friendly", resolverIDs[0], "example.com/org/repo")
	machine := &config.MachineConfig{Primaries: map[string]string{"example.com/org/repo": resolverIDs[0]}}
	got, err := ResolveNotespaceName("friendly", resolverConfig(nb), machine)
	if err != nil {
		t.Fatal(err)
	}
	if got.NotespaceID == "friendly" || got.Root != root {
		t.Fatalf("resolution = %+v", got)
	}
}

func TestNotespaceParserLocalExemptionAndOldLayoutGuard(t *testing.T) {
	local := filepath.Join(t.TempDir(), ".notebook")
	if err := os.MkdirAll(filepath.Join(local, "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, root, isLocal, err := ParseNotespaceRoot(filepath.Join(t.TempDir(), "central"), filepath.Join(local, "plans", "p.md"))
	if err != nil || !isLocal || root != local {
		t.Fatalf("local root=%q local=%v err=%v", root, isLocal, err)
	}

	legacy := t.TempDir()
	if err := os.Mkdir(filepath.Join(legacy, "workspaces"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ValidateNotespaceLayout(legacy); err == nil || !strings.Contains(err.Error(), "grove migrate") {
		t.Fatalf("err = %v", err)
	}
	if err := os.Mkdir(filepath.Join(legacy, NotespaceDirectory), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ValidateNotespaceLayout(legacy); err != nil {
		t.Fatal(err)
	}
}
