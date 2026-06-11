package alias

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/grovetools/core/pkg/workspace"
)

// TestAliasResolver_Resolve_XDGWorktrees verifies aliases resolve to
// XDG-located worktree paths. Identifiers derive from names and
// original-checkout parents, so the alias shapes are unchanged — only the
// resolved Path moves to the XDG location.
func TestAliasResolver_Resolve_XDGWorktrees(t *testing.T) {
	// Sandbox: GROVE_HOME beats XDG_DATA_HOME in paths.getDataHome().
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("GROVE_HOME", "")

	ecoPath := "/path/to/my-ecosystem"
	repoPath := ecoPath + "/my-repo"
	xdgEcoWt := workspace.ResolveNewWorktreePath(ecoPath, "wt-xdg", true)
	xdgRepoWt := workspace.ResolveNewWorktreePath(repoPath, "feature-xdg", true)

	mockResult := &workspace.DiscoveryResult{
		Ecosystems: []workspace.Ecosystem{
			{Name: "my-ecosystem", Path: ecoPath, Type: "Grove"},
		},
		Projects: []workspace.Project{
			{
				// XDG ecosystem worktree, classified via provenance.
				Name:                "wt-xdg",
				Path:                xdgEcoWt,
				ParentEcosystemPath: ecoPath,
				WorktreeSourceBase:  filepath.Dir(xdgEcoWt),
				WorktreeOwnerPath:   ecoPath,
				Workspaces: []workspace.DiscoveredWorkspace{
					{Name: "wt-xdg", Path: xdgEcoWt, Type: workspace.WorkspaceTypePrimary, ParentProjectPath: xdgEcoWt},
				},
			},
			{
				Name:                "my-repo",
				Path:                repoPath,
				ParentEcosystemPath: ecoPath,
				Workspaces: []workspace.DiscoveredWorkspace{
					{Name: "my-repo", Path: repoPath, Type: workspace.WorkspaceTypePrimary},
					{Name: "feature-xdg", Path: xdgRepoWt, Type: workspace.WorkspaceTypeWorktree, ParentProjectPath: repoPath},
				},
			},
		},
	}

	provider := workspace.NewProvider(mockResult)
	resolver := &AliasResolver{Provider: provider}
	resolver.providerOnce.Do(func() {})

	tests := []struct {
		name      string
		alias     string
		expected  string
		expectErr bool
	}{
		{"ecosystem worktree by full alias", "my-ecosystem:wt-xdg", xdgEcoWt, false},
		{"ecosystem worktree by short name", "wt-xdg", xdgEcoWt, false},
		{"repo worktree by full alias", "my-ecosystem:my-repo:feature-xdg", xdgRepoWt, false},
		{"repo worktree by repo:worktree", "my-repo:feature-xdg", xdgRepoWt, false},
		{"missing worktree", "my-ecosystem:no-such-wt", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolver.Resolve(tt.alias)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
