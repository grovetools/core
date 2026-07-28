package workspace

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grovetools/core/config"
)

func TestNotebookLocator_CustomTemplates(t *testing.T) {
	// Test the user's actual config with custom templates
	cfg := &config.Config{
		Notebooks: &config.NotebooksConfig{
			Definitions: map[string]*config.Notebook{
				"nb": {
					RootDir:           "~/Code/nb",
					ChatsPathTemplate: "repos/{{ .Workspace.Name }}/main/current",
					NotesPathTemplate: "repos/{{ .Workspace.Name }}/main/{{ .NoteType }}",
					PlansPathTemplate: "repos/{{ .Workspace.Name }}/main/plans",
				},
			},
			Rules: &config.NotebookRules{
				Default: "nb",
			},
		},
	}

	locator := NewNotebookLocator(cfg)

	// Create a test workspace node
	node := &WorkspaceNode{
		Name:         "grove-core",
		Path:         "/home/user/code/grove-core",
		Kind:         KindStandaloneProject,
		NotebookName: "nb",
	}

	// Test Plans Path
	plansDir, err := locator.GetPlansDir(node)
	require.NoError(t, err)
	// Should expand to something like /home/user/Code/nb/repos/grove-core/main/plans
	assert.Contains(t, plansDir, filepath.Join("Code", "nb", "repos", "grove-core", "main", "plans"))

	// Test Chats Path
	chatsDir, err := locator.GetChatsDir(node)
	require.NoError(t, err)
	assert.Contains(t, chatsDir, filepath.Join("Code", "nb", "repos", "grove-core", "main", "current"))

	// Test Notes Path
	notesDir, err := locator.GetNotesDir(node, "meeting")
	require.NoError(t, err)
	assert.Contains(t, notesDir, filepath.Join("Code", "nb", "repos", "grove-core", "main", "meeting"))
}

func TestNotebookLocator_DefaultPaths(t *testing.T) {
	// Test default behavior with no config
	locator := NewNotebookLocator(nil)

	node := &WorkspaceNode{
		Name: "my-project",
		Path: "/home/user/code/my-project",
		Kind: KindStandaloneProject,
	}

	// Test Plans Path - should use default location
	plansDir, err := locator.GetPlansDir(node)
	require.NoError(t, err)
	assert.Contains(t, plansDir, filepath.Join(".grove", "notebooks", "nb", "workspaces", "my-project", "plans"))

	// Test global notebook fallback
	globalNode := &WorkspaceNode{
		Name: "global",
		Path: "",
		Kind: KindStandaloneProject,
	}

	globalPlansDir, err := locator.GetPlansDir(globalNode)
	require.NoError(t, err)
	assert.Contains(t, globalPlansDir, filepath.Join(".grove", "notebooks", "global", "plans"))
}

func TestNotebookLocator_WorktreeHandling(t *testing.T) {
	// Test that worktrees use their parent project's notebook context
	cfg := &config.Config{
		Notebooks: &config.NotebooksConfig{
			Definitions: map[string]*config.Notebook{
				"nb": {
					RootDir:           "~/Code/nb",
					PlansPathTemplate: "repos/{{ .Workspace.Name }}/main/plans",
				},
			},
			Rules: &config.NotebookRules{
				Default: "nb",
			},
		},
	}

	locator := NewNotebookLocator(cfg)

	// Create a worktree node
	worktreeNode := &WorkspaceNode{
		Name:              "grove-core",
		Path:              "/home/user/code/grove-core/.grove-worktrees/my-feature",
		Kind:              KindStandaloneProjectWorktree,
		ParentProjectPath: "/home/user/code/grove-core",
		NotebookName:      "nb",
	}

	// Worktrees should use the parent project's name in the template
	plansDir, err := locator.GetPlansDir(worktreeNode)
	require.NoError(t, err)
	// Should still use "grove-core" (parent), not "my-feature"
	assert.Contains(t, plansDir, filepath.Join("Code", "nb", "repos", "grove-core", "main", "plans"))
}

func TestNotebookLocator_WorktreeContainerChildren(t *testing.T) {
	// Member repos inside a worktree container resolve plans in the origin
	// ecosystem's workspace — the same workspace the KindEcosystemWorktree
	// container itself resolves to — because the container's plan is created
	// there. Regression: these used to render the member repo's workspace
	// (workspaces/<repo>/plans), which never contains the container's plan.
	cfg := &config.Config{
		Notebooks: &config.NotebooksConfig{
			Definitions: map[string]*config.Notebook{
				"nb": {
					RootDir:           "~/notebooks/nb",
					PlansPathTemplate: "workspaces/{{ .Workspace.Name }}/plans",
				},
			},
			Rules: &config.NotebookRules{
				Default: "nb",
			},
		},
	}

	locator := NewNotebookLocator(cfg)
	container := "/home/user/.local/share/grove/worktrees/my-eco-0bd46c64/misc-fixes"
	ecoPlans := filepath.Join("notebooks", "nb", "workspaces", "my-eco", "plans")

	for _, tc := range []struct {
		name string
		node *WorkspaceNode
	}{
		{
			name: "linked worktree child",
			node: &WorkspaceNode{
				Name:                "treemux",
				Path:                filepath.Join(container, "treemux"),
				Kind:                KindEcosystemWorktreeSubProjectWorktree,
				ParentProjectPath:   "/home/user/code/my-eco/treemux",
				ParentEcosystemPath: container,
				RootEcosystemPath:   "/home/user/code/my-eco",
				NotebookName:        "nb",
			},
		},
		{
			name: "full clone child",
			node: &WorkspaceNode{
				Name:                "treemux",
				Path:                filepath.Join(container, "treemux"),
				Kind:                KindEcosystemWorktreeSubProject,
				ParentEcosystemPath: container,
				RootEcosystemPath:   "/home/user/code/my-eco",
				NotebookName:        "nb",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plansDir, err := locator.GetPlansDir(tc.node)
			require.NoError(t, err)
			assert.Contains(t, plansDir, ecoPlans)
		})
	}

	// A container child with no known origin ecosystem keeps the old
	// repo-scoped behavior (worktree of a true standalone repo).
	standaloneChild := &WorkspaceNode{
		Name:                "myrepo",
		Path:                "/home/user/.local/share/grove/worktrees/myrepo-9e7e892e/dev/myrepo",
		Kind:                KindEcosystemWorktreeSubProjectWorktree,
		ParentProjectPath:   "/home/user/code/myrepo",
		ParentEcosystemPath: "/home/user/.local/share/grove/worktrees/myrepo-9e7e892e/dev",
		NotebookName:        "nb",
	}
	plansDir, err := locator.GetPlansDir(standaloneChild)
	require.NoError(t, err)
	assert.Contains(t, plansDir, filepath.Join("notebooks", "nb", "workspaces", "myrepo", "plans"))
}
