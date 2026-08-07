package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

// link builds one of the installer's version layouts and points a bin dir entry
// at it, which is the shape BuiltCommit reads.
func link(t *testing.T, root, name, version, binary string) string {
	t.Helper()
	versionBin := filepath.Join(root, "data", "grove", "plugins", "versions", name, version, "bin", binary)
	if err := os.MkdirAll(filepath.Dir(versionBin), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(versionBin, []byte("#!/bin/sh\n"), 0o755); err != nil { //nolint:gosec // test fixture
		t.Fatalf("write %s: %v", versionBin, err)
	}
	binPath := filepath.Join(root, "bin", binary)
	if err := os.MkdirAll(filepath.Dir(binPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(versionBin, binPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	return binPath
}

func TestBuiltCommitReadsTheVersionLink(t *testing.T) {
	root := t.TempDir()
	const commit = "274ca8258f1149a0d5ca6d5f0f6d3a7b4c8e9f01"
	pin := &Pin{Binary: link(t, root, "demo", commit, "grove-panel-demo")}

	if got := pin.BuiltCommit(); got != commit {
		t.Errorf("BuiltCommit() = %q, want the commit the link points into (%q)", got, commit)
	}
}

// A dev install keys its version directory on the literal "dev", and reporting
// that verbatim is the point: it says the binary came from a working tree that
// nothing is pinned to.
func TestBuiltCommitReportsDev(t *testing.T) {
	root := t.TempDir()
	pin := &Pin{Binary: link(t, root, "demo", "dev", "grove-panel-demo")}

	if got := pin.BuiltCommit(); got != "dev" {
		t.Errorf("BuiltCommit() = %q, want \"dev\"", got)
	}
}

// The pinned commit and the built one disagree exactly when it matters — a
// half-finished rebuild leaves the link on the previous version — so
// BuiltCommit must report what the LINK says, never what the pin says.
func TestBuiltCommitDoesNotFallBackToThePin(t *testing.T) {
	root := t.TempDir()
	pin := &Pin{
		Commit: "9f0c1a2b3d4e5f60718293a4b5c6d7e8f9012345",
		Binary: link(t, root, "demo", "274ca8258f1149a0d5ca6d5f0f6d3a7b4c8e9f01", "grove-panel-demo"),
	}

	if got := pin.BuiltCommit(); got != "274ca8258f1149a0d5ca6d5f0f6d3a7b4c8e9f01" {
		t.Errorf("BuiltCommit() = %q, want the commit on disk rather than the pinned one", got)
	}
}

func TestBuiltCommitIsEmptyWhenTheLinkIsMissingOrForeign(t *testing.T) {
	root := t.TempDir()

	if got := (&Pin{}).BuiltCommit(); got != "" {
		t.Errorf("a pin with no binary reported %q", got)
	}
	if got := (&Pin{Binary: filepath.Join(root, "nothing-here")}).BuiltCommit(); got != "" {
		t.Errorf("a missing link reported %q", got)
	}

	// A binary someone put in the bin dir themselves: a real link, pointing
	// somewhere that is not one of grove's version directories. Guessing a
	// commit for it would be worse than saying nothing.
	foreign := filepath.Join(root, "elsewhere", "grove-panel-demo")
	if err := os.MkdirAll(filepath.Dir(foreign), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(foreign, []byte("#!/bin/sh\n"), 0o755); err != nil { //nolint:gosec // test fixture
		t.Fatalf("write: %v", err)
	}
	binPath := filepath.Join(root, "bin", "grove-panel-demo")
	if err := os.MkdirAll(filepath.Dir(binPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(foreign, binPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if got := (&Pin{Binary: binPath}).BuiltCommit(); got != "" {
		t.Errorf("a link outside the version layout reported %q", got)
	}

	// A plain file, not a link at all — what `claimBinPath` refuses to install
	// over.
	plain := filepath.Join(root, "bin", "plain")
	if err := os.WriteFile(plain, []byte("#!/bin/sh\n"), 0o755); err != nil { //nolint:gosec // test fixture
		t.Fatalf("write: %v", err)
	}
	if got := (&Pin{Binary: plain}).BuiltCommit(); got != "" {
		t.Errorf("a plain file reported %q", got)
	}
}
