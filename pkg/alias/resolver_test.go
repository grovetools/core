package alias

import (
	"path/filepath"
	"testing"

	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/stretchr/testify/assert"
)

func TestAliasResolver_Resolve(t *testing.T) {
	// Create a mock provider by setting up a discovery result with our test data
	mockResult := &workspace.DiscoveryResult{
		Ecosystems: []workspace.Ecosystem{
			{Name: "my-ecosystem", Path: "/path/to/my-ecosystem", Type: "Grove"},
		},
		Projects: []workspace.Project{
			{
				Name:                "my-repo",
				Path:                "/path/to/my-ecosystem/my-repo",
				ParentEcosystemPath: "/path/to/my-ecosystem",
				Workspaces: []workspace.DiscoveredWorkspace{
					{Name: "my-repo", Path: "/path/to/my-ecosystem/my-repo", Type: workspace.WorkspaceTypePrimary},
					{Name: "feature-branch", Path: "/path/to/my-ecosystem/my-repo/.grove-worktrees/feature-branch", Type: workspace.WorkspaceTypeWorktree, ParentProjectPath: "/path/to/my-ecosystem/my-repo"},
				},
			},
			{
				Name: "standalone-project",
				Path: "/path/to/standalone-project",
				Workspaces: []workspace.DiscoveredWorkspace{
					{Name: "standalone-project", Path: "/path/to/standalone-project", Type: workspace.WorkspaceTypePrimary},
					{Name: "standalone-worktree", Path: "/path/to/standalone-project/.grove-worktrees/standalone-worktree", Type: workspace.WorkspaceTypeWorktree, ParentProjectPath: "/path/to/standalone-project"},
				},
			},
		},
	}

	provider := workspace.NewProvider(mockResult)
	resolver := &AliasResolver{Provider: provider}
	// Mark providerOnce as already done to prevent initProvider from running
	resolver.providerOnce.Do(func() {})

	tests := []struct {
		name      string
		alias     string
		expected  string
		expectErr bool
	}{
		{"1-level ecosystem", "my-ecosystem", "/path/to/my-ecosystem", false},
		{"1-level standalone", "standalone-project", "/path/to/standalone-project", false},
		{"2-level ecosystem:repo", "my-ecosystem:my-repo", "/path/to/my-ecosystem/my-repo", false},
		{"2-level repo:worktree", "standalone-project:standalone-worktree", "/path/to/standalone-project/.grove-worktrees/standalone-worktree", false},
		{"3-level ecosystem:repo:worktree", "my-ecosystem:my-repo:feature-branch", "/path/to/my-ecosystem/my-repo/.grove-worktrees/feature-branch", false},
		{"not found single", "non-existent", "", true},
		{"not found double", "my-ecosystem:non-existent", "", true},
		{"not found triple", "my-ecosystem:my-repo:non-existent", "", true},
		{"invalid format", "a:b:c:d", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// On Windows, paths should use backslashes.
			expected := filepath.FromSlash(tt.expected)

			result, err := resolver.Resolve(tt.alias)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, expected, result)
			}
		})
	}
}

func TestAliasResolver_ResolveLine(t *testing.T) {
	// Create a mock provider by setting up a discovery result with our test data
	mockResult := &workspace.DiscoveryResult{
		Ecosystems: []workspace.Ecosystem{
			{Name: "my-ecosystem", Path: "/path/to/my-ecosystem", Type: "Grove"},
		},
		Projects: []workspace.Project{
			{
				Name:                "my-repo",
				Path:                "/path/to/my-ecosystem/my-repo",
				ParentEcosystemPath: "/path/to/my-ecosystem",
				Workspaces: []workspace.DiscoveredWorkspace{
					{Name: "my-repo", Path: "/path/to/my-ecosystem/my-repo", Type: workspace.WorkspaceTypePrimary},
					{Name: "feature-branch", Path: "/path/to/my-ecosystem/my-repo/.grove-worktrees/feature-branch", Type: workspace.WorkspaceTypeWorktree, ParentProjectPath: "/path/to/my-ecosystem/my-repo"},
				},
			},
			{
				Name: "standalone-project",
				Path: "/path/to/standalone-project",
				Workspaces: []workspace.DiscoveredWorkspace{
					{Name: "standalone-project", Path: "/path/to/standalone-project", Type: workspace.WorkspaceTypePrimary},
					{Name: "standalone-worktree", Path: "/path/to/standalone-project/.grove-worktrees/standalone-worktree", Type: workspace.WorkspaceTypeWorktree, ParentProjectPath: "/path/to/standalone-project"},
				},
			},
		},
	}

	provider := workspace.NewProvider(mockResult)
	resolver := &AliasResolver{Provider: provider}
	// Mark providerOnce as already done to prevent initProvider from running
	resolver.providerOnce.Do(func() {})

	tests := []struct {
		name      string
		line      string
		expected  string
		expectErr bool
	}{
		{"alias with pattern", "@alias:my-ecosystem:my-repo/src/**/*.go", "/path/to/my-ecosystem/my-repo/src/**/*.go", false},
		{"alias with exclusion", "!@alias:standalone-project/vendor/**", "!/path/to/standalone-project/vendor/**", false},
		{"alias only", "@alias:my-ecosystem", "/path/to/my-ecosystem/**", false},
		{"line without alias", "*.go", "", true},
		{"commented alias", "# @alias:my-ecosystem", "", true},
		{"not found", "@alias:not-found/main.go", "", true},
		// Test cases for specific file paths (bug fix)
		{"alias with specific file", "@alias:standalone-project/src/main.go", "/path/to/standalone-project/src/main.go", false},
		{"alias with glob pattern no leading slash", "@alias:standalone-project/**/*.go", "/path/to/standalone-project/**/*.go", false},
		{"alias with nested path", "@alias:my-ecosystem:my-repo/src/components/Header.astro", "/path/to/my-ecosystem/my-repo/src/components/Header.astro", false},
		// Test cases for patterns without leading slash (should fail initially)
		{"alias with pattern no leading slash 1", "@alias:standalone-project**/*.go", "/path/to/standalone-project/**/*.go", false},
		{"alias with pattern no leading slash 2", "@a:my-ecosystem:my-repo*.go", "/path/to/my-ecosystem/my-repo/*.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// On Windows, paths should use backslashes.
			expected := filepath.FromSlash(tt.expected)

			result, err := resolver.ResolveLine(tt.line)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, expected, result)
			}
		})
	}
}
