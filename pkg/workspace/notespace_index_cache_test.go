package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/notespace"
)

// replaceStampSameStat atomically replaces a stamp with same-length bytes and
// restores its original mtime. Size and mtime therefore cannot distinguish the
// versions; only the filesystem entry's stable identity can.
func replaceStampSameStat(t *testing.T, root, content string) {
	t.Helper()
	path := notespace.NotespaceStampPath(root)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(content)) != before.Size() {
		t.Fatalf("probe content is %d bytes, stamp is %d", len(content), before.Size())
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".cache-identity-probe-*")
	if err != nil {
		t.Fatal(err)
	}
	tmpPath := tmp.Name()
	t.Cleanup(func() { _ = os.Remove(tmpPath) })
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		t.Fatal(err)
	}
	if err := tmp.Chmod(before.Mode()); err != nil {
		_ = tmp.Close()
		t.Fatal(err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(tmpPath, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("replacement did not preserve stat tuple: before=(%d, %s), after=(%d, %s)",
			before.Size(), before.ModTime(), after.Size(), after.ModTime())
	}
	if os.SameFile(before, after) {
		t.Fatal("atomic replacement unexpectedly retained file identity")
	}
}

func TestNotespaceIndexCacheServesUnchangedInputs(t *testing.T) {
	ResetNotespaceIndexCache()
	nb := t.TempDir()
	makeStampedRoot(t, nb, "friendly", resolverIDs[0], "example.com/org/repo")
	cfg := resolverConfig(nb)

	first, _, err := configuredNotespaceIndex(cfg)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := configuredNotespaceIndex(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatal("unchanged inputs rebuilt the memoized index")
	}
}

func TestNotespaceIndexCacheInvalidatesOnAtomicSameStatReplacement(t *testing.T) {
	ResetNotespaceIndexCache()
	nb := t.TempDir()
	root := makeStampedRoot(t, nb, "friendly", resolverIDs[0], "example.com/org/repo")
	cfg := resolverConfig(nb)
	machine := &config.MachineConfig{Primaries: map[string]string{"example.com/org/repo": resolverIDs[0]}}

	if _, err := ResolveNotespaceName("friendly", cfg, machine); err != nil {
		t.Fatal(err)
	}
	stamp, err := os.ReadFile(notespace.NotespaceStampPath(root))
	if err != nil {
		t.Fatal(err)
	}
	replaced := strings.Replace(string(stamp), "name = 'friendly'", "name = 'replaced'", 1)
	if replaced == string(stamp) {
		t.Fatalf("stamp fixture lacks expected name: %s", stamp)
	}
	replaceStampSameStat(t, root, replaced)

	if _, err := ResolveNotespaceName("friendly", cfg, machine); err == nil {
		t.Fatal("atomically replaced stamp still resolved under its old display name")
	}
	got, err := ResolveNotespaceName("replaced", cfg, machine)
	if err != nil || got.Root != root {
		t.Fatalf("replacement resolution = %+v, %v; want %s", got, err, root)
	}
}

func TestNotespaceIndexCacheInvalidatesOnDiskChange(t *testing.T) {
	ResetNotespaceIndexCache()
	nb := t.TempDir()
	cfg := resolverConfig(nb)
	machine := &config.MachineConfig{Primaries: map[string]string{
		"example.com/org/one": resolverIDs[0],
		"example.com/org/two": resolverIDs[1],
	}}
	first := makeStampedRoot(t, nb, "one", resolverIDs[0], "example.com/org/one")
	if _, err := ResolveNotespaceName("one", cfg, machine); err != nil {
		t.Fatal(err)
	}

	// A notespace minted after the first resolution: the notespaces/ directory
	// mtime moved, so the next call rebuilds rather than reporting it absent.
	second := makeStampedRoot(t, nb, "two", resolverIDs[1], "example.com/org/two")
	got, err := ResolveNotespaceName("two", cfg, machine)
	if err != nil || got.Root != second {
		t.Fatalf("minted notespace = %+v, %v; want %s", got, err, second)
	}

	// A stamp rewritten in place (a re-mint, a renamed display name): the stamp
	// file's own (mtime, size) is an input, so this is picked up too.
	if _, err := notespace.UpdateNotespace(first, resolverIDs[0], notespace.NotespaceMutable{
		Name: "renamed", Subject: "example.com/org/one", Kind: "repo",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveNotespaceName("one", cfg, machine); err == nil {
		t.Fatal("renamed notespace still resolved under its old display name")
	}
	if got, err := ResolveNotespaceName("renamed", cfg, machine); err != nil || got.Root != first {
		t.Fatalf("renamed resolution = %+v, %v; want %s", got, err, first)
	}

	// A notespace removed entirely.
	if err := os.RemoveAll(second); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveNotespaceName("two", cfg, machine); err == nil {
		t.Fatal("removed notespace still resolved")
	}
}

func TestNotespaceIndexCacheDoesNotCacheFailures(t *testing.T) {
	ResetNotespaceIndexCache()
	nb := t.TempDir()
	cfg := resolverConfig(nb)
	machine := &config.MachineConfig{Primaries: map[string]string{"example.com/org/repo": resolverIDs[0]}}

	bad := filepath.Join(nb, NotespaceDirectory, "bad")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notespace.NotespaceStampPath(bad), []byte("not toml = ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveNotespaceName("bad", cfg, machine); err == nil {
		t.Fatal("expected malformed stamp error")
	}
	// Repairing the malformed stamp must take effect immediately: the failure
	// left nothing behind to serve.
	if err := os.RemoveAll(bad); err != nil {
		t.Fatal(err)
	}
	good := makeStampedRoot(t, nb, "good", resolverIDs[0], "example.com/org/repo")
	if got, err := ResolveNotespaceName("good", cfg, machine); err != nil || got.Root != good {
		t.Fatalf("resolution after repair = %+v, %v; want %s", got, err, good)
	}
}

// TestNotespaceNameRoutesMatchesResolveNotespaceName pins the bulk table to the
// single-name resolver: an entry exists exactly where ResolveNotespaceName
// answers, and carries the same answer.
func TestNotespaceNameRoutesMatchesResolveNotespaceName(t *testing.T) {
	ResetNotespaceIndexCache()
	nb, other := t.TempDir(), t.TempDir()
	primary := makeStampedRoot(t, nb, "primary", resolverIDs[0], "example.com/org/primary")
	makeStampedRoot(t, nb, "not-primary", resolverIDs[1], "example.com/org/other")
	// Same display name, two roots, both recorded primaries for their subject:
	// ambiguous, so neither surface answers it.
	makeStampedRoot(t, nb, "ambiguous", resolverIDs[2], "example.com/org/amb-a")
	makeStampedRoot(t, other, "ambiguous", "01J00000000000000000000004", "example.com/org/amb-b")

	cfg := resolverConfig(nb, other)
	machine := &config.MachineConfig{Primaries: map[string]string{
		"example.com/org/primary": resolverIDs[0],
		"example.com/org/amb-a":   resolverIDs[2],
		"example.com/org/amb-b":   "01J00000000000000000000004",
	}}

	routes, err := NotespaceNameRoutes(cfg, machine)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"primary", "not-primary", "ambiguous", "absent", ""} {
		resolution, resolveErr := ResolveNotespaceName(name, cfg, machine)
		route, ok := routes[name]
		if (resolveErr == nil) != ok {
			t.Fatalf("%q: routes has entry = %v, ResolveNotespaceName err = %v", name, ok, resolveErr)
		}
		if ok && route != resolution {
			t.Fatalf("%q: route = %+v, resolution = %+v", name, route, resolution)
		}
	}
	if routes["primary"].Root != primary {
		t.Fatalf("primary route = %+v", routes["primary"])
	}
}

// BenchmarkResolveNotespaceName measures the steady-state cost the daemon's
// watch-set refresh pays per resolution, against a notebook the size of a real
// one. Before the cache this was a ReadDir plus two reads and TOML parses of
// every stamp; after it, one stat per notebook and per stamp.
func BenchmarkResolveNotespaceName(b *testing.B) {
	nb := b.TempDir()
	machine := &config.MachineConfig{Primaries: map[string]string{}}
	for i := range 76 {
		name := fmt.Sprintf("notespace-%02d", i)
		subj := "example.com/org/" + name
		root := filepath.Join(nb, NotespaceDirectory, name)
		if err := os.MkdirAll(root, 0o755); err != nil {
			b.Fatal(err)
		}
		id := fmt.Sprintf("01J000000000000000000%05d", i)
		if _, err := notespace.InstallNotespace(root, notespace.NotespaceStamp{ID: id, Name: name, Subject: subj, Kind: "repo"}); err != nil {
			b.Fatal(err)
		}
		machine.Primaries[subj] = id
	}
	cfg := resolverConfig(nb)

	b.Run("cached", func(b *testing.B) {
		for b.Loop() {
			if _, err := ResolveNotespaceName("notespace-42", cfg, machine); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("cold", func(b *testing.B) {
		for b.Loop() {
			ResetNotespaceIndexCache()
			if _, err := ResolveNotespaceName("notespace-42", cfg, machine); err != nil {
				b.Fatal(err)
			}
		}
	})
}
