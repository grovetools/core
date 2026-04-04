package workspace

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateAndWarnCollisions(t *testing.T) {
	tests := []struct {
		name              string
		nodes             []*WorkspaceNode
		expectWarning     bool
		expectedCollision string
	}{
		{
			name: "No collision: project and its worktree share the same name",
			nodes: []*WorkspaceNode{
				{
					Name: "my-project",
					Path: "/path/to/my-project",
					Kind: KindStandaloneProject,
				},
				{
					Name:              "my-project",
					Path:              "/path/to/my-project/.grove-worktrees/feature",
					Kind:              KindStandaloneProjectWorktree,
					ParentProjectPath: "/path/to/my-project",
				},
			},
			expectWarning: false,
		},
		{
			name: "No collision: ecosystem sub-project and its worktree",
			nodes: []*WorkspaceNode{
				{
					Name:                "grove-core",
					Path:                "/path/to/ecosystem/grove-core",
					Kind:                KindEcosystemSubProject,
					ParentEcosystemPath: "/path/to/ecosystem",
				},
				{
					Name:                "grove-core",
					Path:                "/path/to/ecosystem/grove-core/.grove-worktrees/fix-123",
					Kind:                KindEcosystemSubProjectWorktree,
					ParentProjectPath:   "/path/to/ecosystem/grove-core",
					ParentEcosystemPath: "/path/to/ecosystem",
				},
			},
			expectWarning: false,
		},
		{
			name: "True collision: two different projects with the same name",
			nodes: []*WorkspaceNode{
				{
					Name: "my-project",
					Path: "/path/to/my-project",
					Kind: KindStandaloneProject,
				},
				{
					Name: "my-project",
					Path: "/other/path/to/my-project",
					Kind: KindStandaloneProject,
				},
			},
			expectWarning:     true,
			expectedCollision: "my-project",
		},
		{
			name: "True collision: project in different ecosystems",
			nodes: []*WorkspaceNode{
				{
					Name:                "grove-core",
					Path:                "/path/to/ecosystem1/grove-core",
					Kind:                KindEcosystemSubProject,
					ParentEcosystemPath: "/path/to/ecosystem1",
				},
				{
					Name:                "grove-core",
					Path:                "/path/to/ecosystem2/grove-core",
					Kind:                KindEcosystemSubProject,
					ParentEcosystemPath: "/path/to/ecosystem2",
				},
			},
			expectWarning:     true,
			expectedCollision: "grove-core",
		},
		{
			name: "Complex: project with worktrees + separate project with same name",
			nodes: []*WorkspaceNode{
				{
					Name: "my-project",
					Path: "/path/to/my-project",
					Kind: KindStandaloneProject,
				},
				{
					Name:              "my-project",
					Path:              "/path/to/my-project/.grove-worktrees/feature",
					Kind:              KindStandaloneProjectWorktree,
					ParentProjectPath: "/path/to/my-project",
				},
				{
					Name: "my-project",
					Path: "/different/location/my-project",
					Kind: KindStandaloneProject,
				},
			},
			expectWarning:     true,
			expectedCollision: "my-project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout to check for warnings
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Create provider (which calls validateAndWarnCollisions)
			p := &Provider{
				nodes:   tt.nodes,
				pathMap: make(map[string]*WorkspaceNode),
			}
			p.validateAndWarnCollisions()

			// Restore stdout and read captured output
			w.Close()
			os.Stdout = oldStdout
			var buf bytes.Buffer
			io.Copy(&buf, r)
			output := buf.String()

			if tt.expectWarning {
				assert.Contains(t, output, "WARNING: workspace name collisions detected:")
				assert.Contains(t, output, tt.expectedCollision)
			} else {
				// Should not contain any collision warnings
				if strings.Contains(output, "WARNING: workspace name collisions") {
					t.Errorf("Expected no collision warning, but got: %s", output)
				}
			}
		})
	}
}

func TestValidateAndWarnCollisions_NoCollisions(t *testing.T) {
	// Test that no warnings are printed when there are no collisions
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	nodes := []*WorkspaceNode{
		{
			Name: "project-a",
			Path: "/path/to/project-a",
			Kind: KindStandaloneProject,
		},
		{
			Name: "project-b",
			Path: "/path/to/project-b",
			Kind: KindStandaloneProject,
		},
	}

	p := &Provider{
		nodes:   nodes,
		pathMap: make(map[string]*WorkspaceNode),
	}
	p.validateAndWarnCollisions()

	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	assert.Empty(t, output, "Expected no output when there are no collisions")
}

func TestProvider_FindByIdentifier(t *testing.T) {
	// Build a realistic set of workspace nodes
	nodes := []*WorkspaceNode{
		{
			Name: "my-ecosystem",
			Path: "/path/to/my-ecosystem",
			Kind: KindEcosystemRoot,
		},
		{
			Name:                "cx",
			Path:                "/path/to/my-ecosystem/cx",
			Kind:                KindEcosystemSubProject,
			ParentEcosystemPath: "/path/to/my-ecosystem",
			RootEcosystemPath:   "/path/to/my-ecosystem",
			Depth:               1,
		},
		{
			Name:                "core",
			Path:                "/path/to/my-ecosystem/core",
			Kind:                KindEcosystemSubProject,
			ParentEcosystemPath: "/path/to/my-ecosystem",
			RootEcosystemPath:   "/path/to/my-ecosystem",
			Depth:               1,
		},
		{
			Name:                "feature-dev",
			Path:                "/path/to/my-ecosystem/.grove-worktrees/feature-dev",
			Kind:                KindEcosystemWorktree,
			ParentProjectPath:   "/path/to/my-ecosystem",
			ParentEcosystemPath: "/path/to/my-ecosystem",
			RootEcosystemPath:   "/path/to/my-ecosystem",
			Depth:               1,
		},
		{
			Name:                "cx",
			Path:                "/path/to/my-ecosystem/.grove-worktrees/feature-dev/cx",
			Kind:                KindEcosystemWorktreeSubProject,
			ParentEcosystemPath: "/path/to/my-ecosystem/.grove-worktrees/feature-dev",
			RootEcosystemPath:   "/path/to/my-ecosystem",
			Depth:               2,
		},
		{
			Name: "standalone",
			Path: "/path/to/standalone",
			Kind: KindStandaloneProject,
		},
	}

	provider := NewProviderFromNodes(nodes)

	tests := []struct {
		name        string
		identifier  string
		currentPath string
		expectPath  string
		expectNil   bool
	}{
		{
			name:       "Fully qualified ecosystem sub-project",
			identifier: "my-ecosystem:cx",
			expectPath: "/path/to/my-ecosystem/cx",
		},
		{
			name:       "Fully qualified worktree sub-project",
			identifier: "my-ecosystem:feature-dev:cx",
			expectPath: "/path/to/my-ecosystem/.grove-worktrees/feature-dev/cx",
		},
		{
			name:       "Short name standalone (unambiguous)",
			identifier: "standalone",
			expectPath: "/path/to/standalone",
		},
		{
			name:       "Short name ecosystem root",
			identifier: "my-ecosystem",
			expectPath: "/path/to/my-ecosystem",
		},
		{
			name:        "Ambiguous short name resolved by current path (in ecosystem root)",
			identifier:  "cx",
			currentPath: "/path/to/my-ecosystem",
			expectPath:  "/path/to/my-ecosystem/cx",
		},
		{
			name:        "Ambiguous short name resolved by sibling context",
			identifier:  "cx",
			currentPath: "/path/to/my-ecosystem/core",
			expectPath:  "/path/to/my-ecosystem/cx",
		},
		{
			name:        "Ambiguous short name resolved by worktree context",
			identifier:  "cx",
			currentPath: "/path/to/my-ecosystem/.grove-worktrees/feature-dev",
			expectPath:  "/path/to/my-ecosystem/.grove-worktrees/feature-dev/cx",
		},
		{
			name:       "Suffix match for partial identifier",
			identifier: "feature-dev:cx",
			expectPath: "/path/to/my-ecosystem/.grove-worktrees/feature-dev/cx",
		},
		{
			name:      "Not found returns nil",
			identifier: "nonexistent",
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := provider.FindByIdentifier(tt.identifier, tt.currentPath)
			if tt.expectNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result, "expected node at %s", tt.expectPath)
				if result != nil {
					assert.Equal(t, tt.expectPath, result.Path)
				}
			}
		})
	}
}

func TestProvider_FindByIdentifier_EcosystemWorktreeRoot(t *testing.T) {
	// Reproduces the e2e scenario: CWD is an ecosystem worktree root,
	// alias "project-beta" should resolve to the child inside the worktree,
	// not the standalone decoy project with the same name.
	nodes := []*WorkspaceNode{
		{
			Name: "my-ecosystem",
			Path: "/path/to/my-ecosystem",
			Kind: KindEcosystemRoot,
		},
		{
			Name:                "feature-branch",
			Path:                "/path/to/my-ecosystem/.grove-worktrees/feature-branch",
			Kind:                KindEcosystemWorktree,
			ParentProjectPath:   "/path/to/my-ecosystem",
			ParentEcosystemPath: "/path/to/my-ecosystem",
			RootEcosystemPath:   "/path/to/my-ecosystem",
			Depth:               1,
		},
		{
			Name:                "project-beta",
			Path:                "/path/to/my-ecosystem/.grove-worktrees/feature-branch/project-beta",
			Kind:                KindEcosystemWorktreeSubProject,
			ParentEcosystemPath: "/path/to/my-ecosystem/.grove-worktrees/feature-branch",
			RootEcosystemPath:   "/path/to/my-ecosystem",
			Depth:               2,
		},
		{
			Name:  "project-beta",
			Path:  "/path/to/standalone/project-beta",
			Kind:  KindStandaloneProject,
			Depth: 0,
		},
	}

	provider := NewProviderFromNodes(nodes)

	// From the ecosystem worktree root, "project-beta" should resolve to the child
	result := provider.FindByIdentifier("project-beta", "/path/to/my-ecosystem/.grove-worktrees/feature-branch")
	assert.NotNil(t, result)
	assert.Equal(t, "/path/to/my-ecosystem/.grove-worktrees/feature-branch/project-beta", result.Path)
}
