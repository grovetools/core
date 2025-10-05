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
			},
			expected: "my-project",
		},
		{
			name: "Project worktree",
			project: &ProjectInfo{
				Name:       "feature-branch",
				Path:       "/path/to/my-project/.grove-worktrees/feature-branch",
				ParentPath: "/path/to/my-project",
				IsWorktree: true,
			},
			expected: "my-project_feature-branch",
		},
		{
			name: "Ecosystem main repository",
			project: &ProjectInfo{
				Name:        "my-ecosystem",
				Path:        "/path/to/my-ecosystem",
				IsEcosystem: true,
			},
			expected: "my-ecosystem",
		},
		{
			name: "Ecosystem worktree",
			project: &ProjectInfo{
				Name:                "eco-feature",
				Path:                "/path/to/my-ecosystem/.grove-worktrees/eco-feature",
				ParentPath:          "/path/to/my-ecosystem",
				IsWorktree:          true,
				IsEcosystem:         true,
				WorktreeName:        "eco-feature",
				ParentEcosystemPath: "/path/to/my-ecosystem",
			},
			expected: "my-ecosystem_eco-feature",
		},
		{
			name: "Sub-project within ecosystem worktree",
			project: &ProjectInfo{
				Name:                "sub-project",
				Path:                "/path/to/my-ecosystem/.grove-worktrees/eco-feature/sub-project",
				WorktreeName:        "eco-feature",
				ParentEcosystemPath: "/path/to/my-ecosystem",
			},
			expected: "my-ecosystem_eco-feature_sub-project",
		},
		{
			name: "Sub-project within main ecosystem repo",
			project: &ProjectInfo{
				Name:                "sub-project",
				Path:                "/path/to/my-ecosystem/sub-project",
				ParentEcosystemPath: "/path/to/my-ecosystem",
			},
			expected: "my-ecosystem_sub-project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Correctly handle filepaths for the test environment
			if tt.project.ParentPath != "" {
				tt.project.ParentPath = filepath.FromSlash(tt.project.ParentPath)
			}
			if tt.project.ParentEcosystemPath != "" {
				tt.project.ParentEcosystemPath = filepath.FromSlash(tt.project.ParentEcosystemPath)
			}
			tt.project.Path = filepath.FromSlash(tt.project.Path)

			assert.Equal(t, tt.expected, tt.project.Identifier())
		})
	}
}
