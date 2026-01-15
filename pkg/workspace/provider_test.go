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
