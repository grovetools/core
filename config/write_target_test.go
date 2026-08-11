package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grovetools/core/pkg/coderoot"
)

// dotfilesLink stages the canonical layout this contract exists for: the real
// file lives in a machine-specific dotfiles directory and the canonical config
// path is only a symlink to it.
func dotfilesLink(t *testing.T, canonicalDir, name, body string) (link, target string) {
	t.Helper()
	dotfiles := t.TempDir()
	target = filepath.Join(dotfiles, name)
	if err := os.WriteFile(target, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	link = filepath.Join(canonicalDir, name)
	if err := os.MkdirAll(canonicalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	return link, target
}

func assertStillLinkedTo(t *testing.T, link, target string) {
	t.Helper()
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat %s: %v", link, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is no longer a symlink; the writer replaced the link", link)
	}
	got, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	if got != target {
		t.Fatalf("%s -> %q, want %q", link, got, target)
	}
}

func TestResolveConfigWriteTargetClassifiesPaths(t *testing.T) {
	dir := t.TempDir()

	missing := filepath.Join(dir, "absent.toml")
	if target, linked, err := ResolveConfigWriteTarget(missing); err != nil || linked || target != missing {
		t.Fatalf("missing path = (%q, %t, %v), want (%q, false, nil)", target, linked, err, missing)
	}

	plain := filepath.Join(dir, "plain.toml")
	if err := os.WriteFile(plain, []byte("x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if target, linked, err := ResolveConfigWriteTarget(plain); err != nil || linked || target != plain {
		t.Fatalf("regular file = (%q, %t, %v), want (%q, false, nil)", target, linked, err, plain)
	}

	link, want := dotfilesLink(t, filepath.Join(dir, "canonical"), "roots.toml", "")
	wantResolved, err := filepath.EvalSymlinks(want)
	if err != nil {
		t.Fatal(err)
	}
	target, linked, err := ResolveConfigWriteTarget(link)
	if err != nil || !linked || target != wantResolved {
		t.Fatalf("symlink = (%q, %t, %v), want (%q, true, nil)", target, linked, err, wantResolved)
	}

	// A chain resolves to the regular file at its end; every link in the chain
	// is left for the caller to preserve.
	hop := filepath.Join(dir, "hop.toml")
	if err := os.Symlink(link, hop); err != nil {
		t.Fatal(err)
	}
	if target, linked, err := ResolveConfigWriteTarget(hop); err != nil || !linked || target != wantResolved {
		t.Fatalf("symlink chain = (%q, %t, %v), want (%q, true, nil)", target, linked, err, wantResolved)
	}
}

func TestResolveConfigWriteTargetRefusesUnreviewableTargets(t *testing.T) {
	dir := t.TempDir()

	dangling := filepath.Join(dir, "dangling.toml")
	if err := os.Symlink(filepath.Join(dir, "nowhere.toml"), dangling); err != nil {
		t.Fatal(err)
	}
	_, _, err := ResolveConfigWriteTarget(dangling)
	if err == nil || !strings.Contains(err.Error(), "unreviewable config symlink") {
		t.Fatalf("dangling symlink error = %v, want an unreviewable-symlink refusal", err)
	}

	a, b := filepath.Join(dir, "a.toml"), filepath.Join(dir, "b.toml")
	if err := os.Symlink(b, a); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(a, b); err != nil {
		t.Fatal(err)
	}
	_, _, err = ResolveConfigWriteTarget(a)
	if err == nil || !strings.Contains(err.Error(), "unreviewable config symlink") {
		t.Fatalf("cyclic symlink error = %v, want an unreviewable-symlink refusal", err)
	}

	toDir := filepath.Join(dir, "to-dir.toml")
	if err := os.Symlink(t.TempDir(), toDir); err != nil {
		t.Fatal(err)
	}
	_, _, err = ResolveConfigWriteTarget(toDir)
	if err == nil || !strings.Contains(err.Error(), "target is not a regular file") {
		t.Fatalf("directory-target error = %v, want a non-regular-target refusal", err)
	}

	notFile := filepath.Join(dir, "plain-dir")
	if err := os.Mkdir(notFile, 0o755); err != nil {
		t.Fatal(err)
	}
	_, _, err = ResolveConfigWriteTarget(notFile)
	if err == nil || !strings.Contains(err.Error(), "expected a regular file or symlink") {
		t.Fatalf("non-file path error = %v, want a non-regular-path refusal", err)
	}
}

func TestWriteCodeRootsUpdatesSymlinkTargetAndKeepsLink(t *testing.T) {
	canonical := filepath.Join(t.TempDir(), "grove")
	link, target := dotfilesLink(t, canonical, coderoot.RootsFileName, "# dotfiles-owned\n")
	// The recorded pair is cross-validated at the canonical directory, so the
	// sibling lives beside the link, not beside the dotfiles file.
	if err := os.WriteFile(filepath.Join(canonical, coderoot.NotebooksFileName), []byte("default = \"nb\"\n\n[notebooks.nb]\nroot = \"/notes/nb\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}

	changed, err := WriteCodeRoots(link, CodeRootEdits{Upserts: map[string]coderoot.Root{
		"work": {Path: "/code/work", Notebook: "nb"},
	}})
	if err != nil || !changed {
		t.Fatalf("WriteCodeRoots = (%t, %v)", changed, err)
	}

	assertStillLinkedTo(t, link, target)
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "[roots.work]") || !strings.Contains(string(body), "# dotfiles-owned") {
		t.Fatalf("dotfiles target did not receive the surgical edit:\n%s", body)
	}
	after, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if after.Mode().Perm() != before.Mode().Perm() {
		t.Fatalf("dotfiles target mode %v, want the pre-existing %v", after.Mode().Perm(), before.Mode().Perm())
	}
	// No stray temp file may be left in either directory.
	for _, dir := range []string{canonical, filepath.Dir(target)} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.Contains(entry.Name(), ".tmp-") {
				t.Fatalf("temp file %s survived in %s", entry.Name(), dir)
			}
		}
	}
}

func TestWriteNotebooksUpdatesSymlinkTargetAndKeepsLink(t *testing.T) {
	canonical := filepath.Join(t.TempDir(), "grove")
	link, target := dotfilesLink(t, canonical, coderoot.NotebooksFileName, "")
	def := "nb"

	changed, err := WriteNotebooks(link, NotebookEdits{Default: &def, Upserts: map[string]coderoot.Notebook{
		"nb": {Root: "/notes/nb"},
	}})
	if err != nil || !changed {
		t.Fatalf("WriteNotebooks = (%t, %v)", changed, err)
	}
	assertStillLinkedTo(t, link, target)
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "[notebooks.nb]") {
		t.Fatalf("dotfiles target did not receive the edit:\n%s", body)
	}
}

func TestEditMachineConfigUpdatesSymlinkTargetAndKeepsLink(t *testing.T) {
	canonical := filepath.Join(t.TempDir(), "grove")
	link, target := dotfilesLink(t, canonical, "machine.toml", "[machine]\nname = \"laptop\"\n")

	_, changed, err := EditMachineConfig(link, MachineEditOptions{}, func(cfg *MachineConfig) error {
		cfg.Primaries = map[string]string{testLocalSubject: testNotespaceID}
		return nil
	})
	if err != nil || !changed {
		t.Fatalf("EditMachineConfig = (%t, %v)", changed, err)
	}
	assertStillLinkedTo(t, link, target)
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "[primaries]") || !strings.Contains(string(body), "name = \"laptop\"") {
		t.Fatalf("dotfiles machine.toml did not receive the surgical edit:\n%s", body)
	}

	changedName, err := WriteMachineName(link, "renamed")
	if err != nil || !changedName {
		t.Fatalf("WriteMachineName = (%t, %v)", changedName, err)
	}
	assertStillLinkedTo(t, link, target)
	if body, err := os.ReadFile(target); err != nil || !strings.Contains(string(body), "name = \"renamed\"") {
		t.Fatalf("dotfiles machine.toml kept the old name: %s (%v)", body, err)
	}
}

func TestConfigWritersRefuseUnreviewableSymlinks(t *testing.T) {
	dir := t.TempDir()
	canonical := filepath.Join(dir, "grove")
	if err := os.MkdirAll(canonical, 0o755); err != nil {
		t.Fatal(err)
	}
	rootsLink := filepath.Join(canonical, coderoot.RootsFileName)
	if err := os.Symlink(filepath.Join(dir, "dotfiles-not-checked-out", coderoot.RootsFileName), rootsLink); err != nil {
		t.Fatal(err)
	}

	_, err := WriteCodeRoots(rootsLink, CodeRootEdits{Upserts: map[string]coderoot.Root{"work": {Path: "/code/work"}}})
	if err == nil || !strings.Contains(err.Error(), "unreviewable config symlink") {
		t.Fatalf("WriteCodeRoots over a dangling link = %v, want a refusal", err)
	}
	// The refusal must leave the dangling link exactly as it was; recreating a
	// regular file here is the data loss the contract prevents.
	info, lerr := os.Lstat(rootsLink)
	if lerr != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("refusal replaced the dangling link: mode=%v err=%v", info.Mode(), lerr)
	}

	machineLink := filepath.Join(canonical, "machine.toml")
	loop := filepath.Join(canonical, "machine.loop.toml")
	if err := os.Symlink(loop, machineLink); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(machineLink, loop); err != nil {
		t.Fatal(err)
	}
	if _, _, err := EditMachineConfig(machineLink, MachineEditOptions{}, func(cfg *MachineConfig) error {
		cfg.Primaries = map[string]string{testLocalSubject: testNotespaceID}
		return nil
	}); err == nil || !strings.Contains(err.Error(), "unreviewable config symlink") {
		t.Fatalf("EditMachineConfig over a cyclic link = %v, want a refusal", err)
	}
}

func TestAtomicWriteVerifiedPreservesRegularFileBehaviour(t *testing.T) {
	dir := t.TempDir()

	created := filepath.Join(dir, "created.toml")
	if err := atomicWriteVerified(created, "a = 1\n", nil); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(created)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("created file mode %v, want 0644", info.Mode().Perm())
	}

	if err := atomicWriteVerified(created, "a = 2\n", nil); err != nil {
		t.Fatal(err)
	}
	if body, err := os.ReadFile(created); err != nil || string(body) != "a = 2\n" {
		t.Fatalf("in-place rewrite = %q (%v)", body, err)
	}
	if lst, err := os.Lstat(created); err != nil || lst.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("in-place write produced a link: %v %v", lst.Mode(), err)
	}

	// A candidate that does not reload is refused before anything is staged.
	refused := filepath.Join(dir, "refused.toml")
	err = atomicWriteVerified(refused, "a = 3\n", func(string) error { return os.ErrInvalid })
	if err == nil || !strings.Contains(err.Error(), "result would not reload") {
		t.Fatalf("verify failure = %v, want a reload refusal", err)
	}
	if _, statErr := os.Stat(refused); !os.IsNotExist(statErr) {
		t.Fatalf("refused write created %s: %v", refused, statErr)
	}
}
