package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFindRootEcosystemPathOnAMaterializedCheckout pins the contract a
// card-declared materialization has to satisfy for the worktree/plan tooling to
// work on the new machine.
//
// SetupSubmodules resolves every member repo against findRootEcosystemPath(gitRoot);
// if that returns "" (or the wrong root) the members are resolved against an
// arbitrary checkout instead — the Frankenstein-mix failure that function's
// comment describes. So "a materialize destination is usable" reduces to two
// concrete properties, asserted here rather than left implied:
//
//  1. a clone root carrying a manifest with a non-empty `workspaces` resolves to
//     ITSELF, and
//  2. because the walk keeps going up and keeps the TOP-most match, a
//     destination nested inside another ecosystem resolves to the OUTER one —
//     which is why materialize destinations belong outside existing ecosystem
//     roots (~/code/<eco>), not inside one.
func TestFindRootEcosystemPathOnAMaterializedCheckout(t *testing.T) {
	// A superrepo clone: manifest with workspaces at the root, members beside
	// it, .gitmodules owned by the root.
	root := t.TempDir()
	dest := filepath.Join(root, "code", "grovetools")
	require.NoError(t, os.MkdirAll(filepath.Join(dest, "alpha"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dest, "grove.toml"), []byte("workspaces = [\"*\"]\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dest, ".gitmodules"),
		[]byte("[submodule \"alpha\"]\n\tpath = alpha\n\turl = ../alpha.git\n"), 0o600))

	require.Equal(t, dest, findRootEcosystemPath(dest),
		"a materialized ecosystem root must resolve to itself")
	require.Equal(t, dest, findRootEcosystemPath(filepath.Join(dest, "alpha")),
		"a member repo must resolve to the ecosystem root that owns it")

	// A manifest without `workspaces` is not an ecosystem root: FindEcosystemConfig
	// keys on that field, so a flat materialization that skipped the seeded
	// manifest would leave the tooling with nothing to anchor on.
	bare := filepath.Join(root, "code", "flatco")
	require.NoError(t, os.MkdirAll(bare, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bare, "grove.toml"), []byte("name = \"flatco\"\n"), 0o600))
	require.Empty(t, findRootEcosystemPath(bare),
		"a manifest without `workspaces` must not pass as an ecosystem root")

	// Nesting: the walk takes the TOP-most ecosystem, so a destination chosen
	// inside an existing ecosystem is captured by it.
	outer := filepath.Join(root, "outer")
	inner := filepath.Join(outer, "nested", "grovetools")
	require.NoError(t, os.MkdirAll(inner, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(outer, "grove.toml"), []byte("workspaces = [\"*\"]\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(inner, "grove.toml"), []byte("workspaces = [\"*\"]\n"), 0o600))
	require.Equal(t, outer, findRootEcosystemPath(inner),
		"a destination nested inside an ecosystem resolves to the outer root — materialize destinations must not be nested")
}
