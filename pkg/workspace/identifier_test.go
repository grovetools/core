package workspace

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProjectInfo_Identifier(t *testing.T) {
	tests := []struct {
		name     string
		project  *ProjectInfo
		expected string
	}{
		{
			name: "Standalone project",
			project: &ProjectInfo{
				Name: "my-project",
				Path: "/path/to/my-project",
				Kind: KindStandaloneProject,
			},
			expected: "my-project",
		},
		{
			name: "Project worktree",
			project: &ProjectInfo{
				Name:              "feature-branch",
				Path:              "/path/to/my-project/.grove-worktrees/feature-branch",
				Kind:              KindStandaloneProjectWorktree,
				ParentProjectPath: "/path/to/my-project",
			},
			expected: "my-project_feature-branch",
		},
		{
			name: "Ecosystem main repository",
			project: &ProjectInfo{
				Name: "my-ecosystem",
				Path: "/path/to/my-ecosystem",
				Kind: KindEcosystemRoot,
			},
			expected: "my-ecosystem",
		},
		{
			name: "Ecosystem worktree",
			project: &ProjectInfo{
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
			project: &ProjectInfo{
				Name:                "sub-project",
				Path:                "/path/to/my-ecosystem/.grove-worktrees/eco-feature/sub-project",
				Kind:                KindEcosystemWorktreeSubProject,
				ParentEcosystemPath: "/path/to/my-ecosystem/.grove-worktrees/eco-feature",
			},
			expected: "my-ecosystem_eco-feature_sub-project",
		},
		{
			name: "Sub-project within main ecosystem repo",
			project: &ProjectInfo{
				Name:                "sub-project",
				Path:                "/path/to/my-ecosystem/sub-project",
				Kind:                KindEcosystemSubProject,
				ParentEcosystemPath: "/path/to/my-ecosystem",
			},
			expected: "my-ecosystem_sub-project",
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
			tt.project.Path = filepath.FromSlash(tt.project.Path)

			assert.Equal(t, tt.expected, tt.project.Identifier())
		})
	}
}
