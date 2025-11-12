package workspace

import (
	"path/filepath"
	"testing"

	"github.com/mattsolo1/grove-core/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotebookLocator_CustomTemplates(t *testing.T) {
	// Test the user's actual config with custom templates
	cfg := &config.Config{
		Notebooks: &config.NotebooksConfig{
			Definitions: map[string]*config.Notebook{
				"nb": {
					RootDir:            "~/Code/nb",
					ChatsPathTemplate:  "repos/{{ .Workspace.Name }}/main/current",
					NotesPathTemplate:  "repos/{{ .Workspace.Name }}/main/{{ .NoteType }}",
					PlansPathTemplate:  "repos/{{ .Workspace.Name }}/main/plans",
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
		Path:         "/Users/solom4/code/grove-core",
		Kind:         KindStandaloneProject,
		NotebookName: "nb",
	}

	// Test Plans Path
	plansDir, err := locator.GetPlansDir(node)
	require.NoError(t, err)
	// Should expand to something like /Users/solom4/Code/nb/repos/grove-core/main/plans
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
	assert.Contains(t, plansDir, filepath.Join(".grove", "notebooks", "nb", "notebooks", "my-project"))

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
		Path:              "/Users/solom4/code/grove-core/.grove-worktrees/my-feature",
		Kind:              KindStandaloneProjectWorktree,
		ParentProjectPath: "/Users/solom4/code/grove-core",
		NotebookName:      "nb",
	}

	// Worktrees should use the parent project's name in the template
	plansDir, err := locator.GetPlansDir(worktreeNode)
	require.NoError(t, err)
	// Should still use "grove-core" (parent), not "my-feature"
	assert.Contains(t, plansDir, filepath.Join("Code", "nb", "repos", "grove-core", "main", "plans"))
}
