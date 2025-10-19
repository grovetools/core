package workspace

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWorkspaceNode_Identifier(t *testing.T) {
	tests := []struct {
		name     string
		project  *WorkspaceNode
		expected string
	}{
		{
			name: "Standalone project",
			project: &WorkspaceNode{
				Name: "my-project",
				Path: "/path/to/my-project",
				Kind: KindStandaloneProject,
			},
			expected: "my-project",
		},
		{
			name: "Project worktree",
			project: &WorkspaceNode{
				Name:              "feature-branch",
				Path:              "/path/to/my-project/.grove-worktrees/feature-branch",
				Kind:              KindStandaloneProjectWorktree,
				ParentProjectPath: "/path/to/my-project",
			},
			expected: "my-project_feature-branch",
		},
		{
			name: "Ecosystem main repository",
			project: &WorkspaceNode{
				Name: "my-ecosystem",
				Path: "/path/to/my-ecosystem",
				Kind: KindEcosystemRoot,
			},
			expected: "my-ecosystem",
		},
		{
			name: "Ecosystem worktree",
			project: &WorkspaceNode{
				Name:                "eco-feature",
				Path:                "/path/to/my-ecosystem/.grove-worktrees/eco-feature",
				Kind:                KindEcosystemWorktree,
				ParentProjectPath:   "/path/to/my-ecosystem",
				ParentEcosystemPath: "/path/to/my-ecosystem",
			},
			expected: "my-ecosystem_eco-feature",
		},
		{
			name: "Sub-project within ecosystem worktree",
			project: &WorkspaceNode{
				Name:                "sub-project",
				Path:                "/path/to/my-ecosystem/.grove-worktrees/eco-feature/sub-project",
				Kind:                KindEcosystemWorktreeSubProject,
				ParentEcosystemPath: "/path/to/my-ecosystem/.grove-worktrees/eco-feature",
			},
			expected: "my-ecosystem_eco-feature_sub-project",
		},
		{
			name: "Sub-project within main ecosystem repo",
			project: &WorkspaceNode{
				Name:                "sub-project",
				Path:                "/path/to/my-ecosystem/sub-project",
				Kind:                KindEcosystemSubProject,
				ParentEcosystemPath: "/path/to/my-ecosystem",
			},
			expected: "my-ecosystem_sub-project",
		},
		{
			name: "Sub-project worktree within main ecosystem repo",
			project: &WorkspaceNode{
				Name:                "feature-branch",
				Path:                "/path/to/my-ecosystem/sub-project/.grove-worktrees/feature-branch",
				Kind:                KindEcosystemSubProjectWorktree,
				ParentProjectPath:   "/path/to/my-ecosystem/sub-project",
				ParentEcosystemPath: "/path/to/my-ecosystem",
			},
			expected: "my-ecosystem_sub-project_feature-branch",
		},
		{
			name: "Sub-project worktree within ecosystem worktree (linked development state)",
			project: &WorkspaceNode{
				Name:                "sub-project",
				Path:                "/path/to/my-ecosystem/.grove-worktrees/eco-feature/sub-project",
				Kind:                KindEcosystemWorktreeSubProjectWorktree,
				ParentProjectPath:   "/path/to/my-ecosystem/sub-project",
				ParentEcosystemPath: "/path/to/my-ecosystem/.grove-worktrees/eco-feature",
				RootEcosystemPath:   "/path/to/my-ecosystem",
			},
			expected: "my-ecosystem_eco-feature_sub-project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Correctly handle filepaths for the test environment
			if tt.project.ParentProjectPath != "" {
				tt.project.ParentProjectPath = filepath.FromSlash(tt.project.ParentProjectPath)
			}
			if tt.project.ParentEcosystemPath != "" {
				tt.project.ParentEcosystemPath = filepath.FromSlash(tt.project.ParentEcosystemPath)
			}
			if tt.project.RootEcosystemPath != "" {
				tt.project.RootEcosystemPath = filepath.FromSlash(tt.project.RootEcosystemPath)
			}
			tt.project.Path = filepath.FromSlash(tt.project.Path)

			assert.Equal(t, tt.expected, tt.project.Identifier())
		})
	}
}
