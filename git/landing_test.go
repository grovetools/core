package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func landingGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v: %s", args, err, out)
	}
}

func landingCommit(t *testing.T, dir, name, body, message string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	landingGit(t, dir, "add", ".")
	landingGit(t, dir, "commit", "-m", message)
}

func landingRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	landingGit(t, dir, "init", "-b", "main")
	landingGit(t, dir, "config", "user.email", "t@example.com")
	landingGit(t, dir, "config", "user.name", "Test")
	landingCommit(t, dir, "f.txt", "one", "c1")
	return dir
}

// The bases are the contract: local main/master, never origin/main, and the
// push distance measured against origin/<branch>.
func TestGetLandingStateMeasuresAgainstLocalMain(t *testing.T) {
	dir := landingRepo(t)
	landingGit(t, dir, "checkout", "-b", "feature")
	landingCommit(t, dir, "g.txt", "branch", "c2")
	landingGit(t, dir, "checkout", "main")
	landingCommit(t, dir, "h.txt", "main", "c3")
	landingGit(t, dir, "checkout", "feature")

	land := GetLandingState(dir, "feature")
	if !land.Computed {
		t.Fatal("landing state reported itself uncomputed")
	}
	if land.Onto != "main" {
		t.Fatalf("Onto = %q, want the repo's own local main", land.Onto)
	}
	if land.Ahead != 1 || land.Behind != 1 {
		t.Fatalf("ahead/behind = %d/%d, want 1/1 against local main", land.Ahead, land.Behind)
	}
	if land.HasRemote {
		t.Fatal("an un-pushed branch reported a remote")
	}
	if land.LastCommitAt.IsZero() {
		t.Fatal("last-commit time is missing; the age column would need its own fork")
	}
}

// A main checkout is by definition at its own local main — 0/0 — even when
// origin/main has moved ahead. This is the case where the coarse status'
// counters mean something else entirely.
func TestGetLandingStateOnMainIsZeroAgainstItself(t *testing.T) {
	origin := t.TempDir()
	landingGit(t, origin, "init", "--bare", "-b", "main")
	seed := landingRepo(t)
	landingCommit(t, seed, "f.txt", "two", "c2")
	landingGit(t, seed, "remote", "add", "origin", origin)
	landingGit(t, seed, "push", "origin", "main")

	dir := t.TempDir()
	landingGit(t, dir, "init", "-b", "main")
	landingGit(t, dir, "config", "user.email", "t@example.com")
	landingGit(t, dir, "config", "user.name", "Test")
	landingGit(t, dir, "remote", "add", "origin", origin)
	landingGit(t, dir, "fetch", "origin")
	landingGit(t, dir, "checkout", "--no-track", "-B", "main", "origin/main~1")

	land := GetLandingState(dir, "main")
	if land.Ahead != 0 || land.Behind != 0 {
		t.Fatalf("ahead/behind = %d/%d, want 0/0: a main checkout is its own onto ref", land.Ahead, land.Behind)
	}
	if !land.HasRemote || land.BehindOrigin != 1 {
		t.Fatalf("push distance = (%v, %d), want (true, 1)", land.HasRemote, land.BehindOrigin)
	}
}

// The memo is pinned to the refs the state was computed from: unchanged refs
// serve a byte-identical answer with no forks, and a moved ref invalidates it.
func TestGetLandingStateCacheFollowsRefs(t *testing.T) {
	dir := landingRepo(t)
	landingGit(t, dir, "checkout", "-b", "feature")
	landingCommit(t, dir, "g.txt", "branch", "c2")

	first := GetLandingState(dir, "feature")
	if first.Ahead != 1 {
		t.Fatalf("ahead = %d, want 1", first.Ahead)
	}
	fp, ok := resolveLandingFingerprint(filepath.Clean(dir), "feature")
	if !ok {
		t.Fatal("fingerprint did not resolve for an ordinary repo; the memo would never warm")
	}
	if _, cached := lookupLanding(filepath.Clean(dir), fp); !cached {
		t.Fatal("a computed landing state was not memoized")
	}

	second := GetLandingState(dir, "feature")
	if *second != *first {
		t.Fatalf("cached answer %+v differs from the computed one %+v", second, first)
	}

	landingCommit(t, dir, "i.txt", "more", "c3")
	if moved := GetLandingState(dir, "feature"); moved.Ahead != 2 {
		t.Fatalf("ahead after a new commit = %d, want 2: the memo outlived its refs", moved.Ahead)
	}
}

// A repo with no local main/master must not be memoized: the fingerprint has no
// base to pin to, and caching "nothing here" would survive main appearing.
func TestGetLandingStateRefusesCacheWithoutLocalMain(t *testing.T) {
	dir := landingRepo(t)
	landingGit(t, dir, "checkout", "-b", "feature")
	landingGit(t, dir, "branch", "-D", "main")

	if _, ok := resolveLandingFingerprint(filepath.Clean(dir), "feature"); ok {
		t.Fatal("a repo with no local main/master was fingerprinted")
	}
	if land := GetLandingState(dir, "feature"); land.Onto != "" || !land.Computed {
		t.Fatalf("landing state = %+v, want a computed state with no onto ref", land)
	}
}
