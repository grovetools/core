package build

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mkctx(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Dockerfile"), "FROM scratch\nCOPY . /\n")
	write(t, filepath.Join(dir, "main.go"), "package main\n")
	write(t, filepath.Join(dir, "sub", "util.go"), "package sub\n")
	return dir, filepath.Join(dir, "Dockerfile")
}

func TestHashContextDeterministic(t *testing.T) {
	dir, df := mkctx(t)
	h1, err := HashContext(dir, df)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := HashContext(dir, df)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatalf("expected stable hash, got %s vs %s", h1, h2)
	}
}

func TestHashContextChangesOnAdd(t *testing.T) {
	dir, df := mkctx(t)
	before, _ := HashContext(dir, df)
	write(t, filepath.Join(dir, "new.go"), "package x\n")
	after, _ := HashContext(dir, df)
	if before == after {
		t.Fatal("expected hash change after adding a file")
	}
}

func TestHashContextIgnoredFileDoesNotChange(t *testing.T) {
	dir, df := mkctx(t)
	write(t, filepath.Join(dir, ".dockerignore"), "ignored/**\n")
	before, err := HashContext(dir, df)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "ignored", "a.txt"), "hello\n")
	after, err := HashContext(dir, df)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatalf("expected ignored file to not change hash: %s vs %s", before, after)
	}
}

func TestHashContextDockerfileChange(t *testing.T) {
	dir, df := mkctx(t)
	before, _ := HashContext(dir, df)
	write(t, df, "FROM scratch\nCOPY . /\n# changed\n")
	after, _ := HashContext(dir, df)
	if before == after {
		t.Fatal("expected hash change after editing Dockerfile")
	}
}

func TestHashContextDockerfileOutside(t *testing.T) {
	dir, _ := mkctx(t)
	outside := t.TempDir()
	outsideDf := filepath.Join(outside, "Dockerfile.api")
	write(t, outsideDf, "FROM scratch\n")

	before, err := HashContext(dir, outsideDf)
	if err != nil {
		t.Fatal(err)
	}
	write(t, outsideDf, "FROM scratch\n# v2\n")
	after, err := HashContext(dir, outsideDf)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("expected hash change when outside Dockerfile changes")
	}
}

func TestHashContextMissingDockerignore(t *testing.T) {
	dir, df := mkctx(t)
	// No .dockerignore; ensure walk still succeeds and is stable.
	h, err := HashContext(dir, df)
	if err != nil {
		t.Fatal(err)
	}
	if h == "" {
		t.Fatal("expected non-empty hash")
	}
}

func TestHashContextSymlinkEscape(t *testing.T) {
	dir, df := mkctx(t)
	escape := t.TempDir()
	target := filepath.Join(escape, "secret.txt")
	write(t, target, "nope\n")
	if err := os.Symlink(target, filepath.Join(dir, "leak")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	_, err := HashContext(dir, df)
	if err == nil {
		t.Fatal("expected error for escaping symlink")
	}
}
