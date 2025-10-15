package workspace

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/mattsolo1/grove-core/config"
	"github.com/mattsolo1/grove-core/util/pathutil"
)

const (
	defaultNotesPathTemplate       = "repos/{{ .Workspace.Name }}/main/notes/{{ .NoteType }}"
	defaultPlansPathTemplate       = "repos/{{ .Workspace.Name }}/main/plans"
	defaultChatsPathTemplate       = "repos/{{ .Workspace.Name }}/main/chats"
	defaultGlobalNotesPathTemplate = "global/notes/{{ .NoteType }}"
	defaultGlobalPlansPathTemplate = "global/plans"
	defaultGlobalChatsPathTemplate = "global/chats"
)

// NotebookLocator resolves paths for notes, plans, and chats based on configuration.
// It operates in two modes:
//   - Local Mode (default): Plans/chats are stored within the project directory (e.g., ./plans)
//   - Centralized Mode (opt-in): Plans/chats are stored in a centralized notebook directory
//
// The mode is determined by whether notebook.root_dir is configured.
type NotebookLocator struct {
	config *config.NotebookConfig
}

// NewNotebookLocator creates a new locator. It gracefully handles a nil config.
func NewNotebookLocator(cfg *config.Config) *NotebookLocator {
	var notebookCfg *config.NotebookConfig
	if cfg != nil {
		notebookCfg = cfg.Notebook
	}

	// Ensure we have a config object to work with, even if it's empty.
	if notebookCfg == nil {
		notebookCfg = &config.NotebookConfig{}
	}

	// Populate with defaults if templates are not provided by the user.
	if notebookCfg.NotesPathTemplate == "" {
		notebookCfg.NotesPathTemplate = defaultNotesPathTemplate
	}
	if notebookCfg.PlansPathTemplate == "" {
		notebookCfg.PlansPathTemplate = defaultPlansPathTemplate
	}
	if notebookCfg.ChatsPathTemplate == "" {
		notebookCfg.ChatsPathTemplate = defaultChatsPathTemplate
	}
	if notebookCfg.GlobalNotesPathTemplate == "" {
		notebookCfg.GlobalNotesPathTemplate = defaultGlobalNotesPathTemplate
	}
	if notebookCfg.GlobalPlansPathTemplate == "" {
		notebookCfg.GlobalPlansPathTemplate = defaultGlobalPlansPathTemplate
	}
	if notebookCfg.GlobalChatsPathTemplate == "" {
		notebookCfg.GlobalChatsPathTemplate = defaultGlobalChatsPathTemplate
	}

	return &NotebookLocator{
		config: notebookCfg,
	}
}

// isCentralized returns true if the system is configured for centralized storage.
func (l *NotebookLocator) isCentralized() bool {
	return l.config != nil && l.config.RootDir != ""
}

// GetPlansDir returns the absolute path to the plans directory for a given workspace node.
// In Local Mode, it returns the plans directory within the project (using GetGroupingKey to handle worktrees).
// In Centralized Mode, it uses the configured root_dir and path templates.
func (l *NotebookLocator) GetPlansDir(node *WorkspaceNode) (string, error) {
	if !l.isCentralized() {
		// Local Mode: Plans are inside the project's root directory.
		// Use GetGroupingKey to correctly handle worktrees.
		return filepath.Join(node.GetGroupingKey(), "plans"), nil
	}

	// Centralized Mode
	rootDir, err := pathutil.Expand(l.config.RootDir)
	if err != nil {
		return "", fmt.Errorf("expanding notebook root_dir: %w", err)
	}

	var tplStr string
	var data interface{}

	if node.Name == "global" {
		tplStr = l.config.GlobalPlansPathTemplate
		data = struct{}{}
	} else {
		tplStr = l.config.PlansPathTemplate

		// Determine the correct workspace name for different node kinds
		contextNode := node

		// For worktrees, we need to use the parent project/ecosystem name
		if node.IsWorktree() {
			if node.Kind == KindEcosystemWorktreeSubProjectWorktree {
				// This is a subproject within an ecosystem worktree
				// Use the subproject's own name, not the root ecosystem
				// The subproject has its own notebook directory
				contextNode = &WorkspaceNode{
					Name: node.Name, // Keep the subproject's name
					Path: node.Path,
				}
			} else if node.RootEcosystemPath != "" {
				// This is an ecosystem worktree (not a subproject)
				// Use the root ecosystem's name
				contextNode = &WorkspaceNode{
					Name: filepath.Base(node.RootEcosystemPath),
					Path: node.RootEcosystemPath,
				}
			} else if node.ParentProjectPath != "" {
				// This is a standalone project worktree
				// Use the parent project's name
				contextNode = &WorkspaceNode{
					Name: filepath.Base(node.ParentProjectPath),
					Path: node.ParentProjectPath,
				}
			}
		}

		data = struct {
			Workspace *WorkspaceNode
		}{
			Workspace: contextNode,
		}
	}

	renderedPath, err := renderPath(tplStr, data)
	if err != nil {
		return "", err
	}

	return filepath.Join(rootDir, renderedPath), nil
}

// GetChatsDir is analogous to GetPlansDir.
func (l *NotebookLocator) GetChatsDir(node *WorkspaceNode) (string, error) {
	if !l.isCentralized() {
		// Local Mode: Chats are inside the project's root directory.
		return filepath.Join(node.GetGroupingKey(), "chats"), nil
	}

	// Centralized Mode
	rootDir, err := pathutil.Expand(l.config.RootDir)
	if err != nil {
		return "", fmt.Errorf("expanding notebook root_dir: %w", err)
	}

	var tplStr string
	var data interface{}

	if node.Name == "global" {
		tplStr = l.config.GlobalChatsPathTemplate
		data = struct{}{}
	} else {
		tplStr = l.config.ChatsPathTemplate

		// Determine the correct workspace name for different node kinds
		contextNode := node

		// For worktrees, we need to use the parent project/ecosystem name
		if node.IsWorktree() {
			if node.Kind == KindEcosystemWorktreeSubProjectWorktree {
				// Subproject within ecosystem worktree - use subproject's name
				contextNode = &WorkspaceNode{
					Name: node.Name,
					Path: node.Path,
				}
			} else if node.RootEcosystemPath != "" {
				// Ecosystem worktree - use root ecosystem's name
				contextNode = &WorkspaceNode{
					Name: filepath.Base(node.RootEcosystemPath),
					Path: node.RootEcosystemPath,
				}
			} else if node.ParentProjectPath != "" {
				// Standalone project worktree - use parent project's name
				contextNode = &WorkspaceNode{
					Name: filepath.Base(node.ParentProjectPath),
					Path: node.ParentProjectPath,
				}
			}
		}

		data = struct {
			Workspace *WorkspaceNode
		}{
			Workspace: contextNode,
		}
	}

	renderedPath, err := renderPath(tplStr, data)
	if err != nil {
		return "", err
	}

	return filepath.Join(rootDir, renderedPath), nil
}

// GetNotesDir returns the absolute path to the notes directory for a given workspace node and note type.
// In Local Mode, it returns the notes directory within the project (e.g., ./notes/{noteType}).
// In Centralized Mode, it uses the configured root_dir and path templates.
func (l *NotebookLocator) GetNotesDir(node *WorkspaceNode, noteType string) (string, error) {
	if !l.isCentralized() {
		// Local Mode: Notes are inside the project's root directory.
		return filepath.Join(node.GetGroupingKey(), "notes", noteType), nil
	}

	// Centralized Mode
	rootDir, err := pathutil.Expand(l.config.RootDir)
	if err != nil {
		return "", fmt.Errorf("expanding notebook root_dir: %w", err)
	}

	var tplStr string
	var data interface{}

	if node.Name == "global" {
		tplStr = l.config.GlobalNotesPathTemplate
		data = struct {
			NoteType string
		}{
			NoteType: noteType,
		}
	} else {
		tplStr = l.config.NotesPathTemplate

		// Determine the correct workspace name for different node kinds
		contextNode := node

		// For worktrees, we need to use the parent project/ecosystem name
		if node.IsWorktree() {
			if node.Kind == KindEcosystemWorktreeSubProjectWorktree {
				// Subproject within ecosystem worktree - use subproject's name
				contextNode = &WorkspaceNode{
					Name: node.Name,
					Path: node.Path,
				}
			} else if node.RootEcosystemPath != "" {
				// Ecosystem worktree - use root ecosystem's name
				contextNode = &WorkspaceNode{
					Name: filepath.Base(node.RootEcosystemPath),
					Path: node.RootEcosystemPath,
				}
			} else if node.ParentProjectPath != "" {
				// Standalone project worktree - use parent project's name
				contextNode = &WorkspaceNode{
					Name: filepath.Base(node.ParentProjectPath),
					Path: node.ParentProjectPath,
				}
			}
		}

		data = struct {
			Workspace *WorkspaceNode
			NoteType  string
		}{
			Workspace: contextNode,
			NoteType:  noteType,
		}
	}

	renderedPath, err := renderPath(tplStr, data)
	if err != nil {
		return "", err
	}

	return filepath.Join(rootDir, renderedPath), nil
}

// renderPath executes the Go template for path generation.
func renderPath(tplStr string, data interface{}) (string, error) {
	tpl, err := template.New("path").Parse(tplStr)
	if err != nil {
		return "", fmt.Errorf("parsing path template: %w", err)
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing path template: %w", err)
	}

	return buf.String(), nil
}

// ScanForAllPlans discovers all plan directories across all known workspaces.
// It returns a list of absolute paths to plan directories that actually exist on disk.
// This method properly handles both Local Mode and Centralized Mode.
func (l *NotebookLocator) ScanForAllPlans(provider *Provider) ([]string, error) {
	if provider == nil {
		return nil, fmt.Errorf("workspace provider is required")
	}

	var planDirs []string
	seen := make(map[string]bool)

	for _, node := range provider.All() {
		// We only need to check the root of a project group (not every worktree)
		// as they share the same plan directory.
		groupKey := node.GetGroupingKey()
		if seen[groupKey] {
			continue
		}

		// Use the node representing the group key to resolve the path
		groupNode := provider.FindByPath(groupKey)
		if groupNode == nil {
			continue // Should not happen
		}

		dir, err := l.GetPlansDir(groupNode)
		if err != nil {
			continue // Log this? For now, skip.
		}

		if _, err := os.Stat(dir); err == nil {
			planDirs = append(planDirs, dir)
		}
		seen[groupKey] = true
	}
	return planDirs, nil
}

// ScanForAllChats discovers all chat directories across all known workspaces.
// It returns a list of absolute paths to chat directories that actually exist on disk.
// This method properly handles both Local Mode and Centralized Mode.
func (l *NotebookLocator) ScanForAllChats(provider *Provider) ([]string, error) {
	if provider == nil {
		return nil, fmt.Errorf("workspace provider is required")
	}

	var chatDirs []string
	seen := make(map[string]bool)

	for _, node := range provider.All() {
		// We only need to check the root of a project group (not every worktree)
		// as they share the same chat directory.
		groupKey := node.GetGroupingKey()
		if seen[groupKey] {
			continue
		}

		// Use the node representing the group key to resolve the path
		groupNode := provider.FindByPath(groupKey)
		if groupNode == nil {
			continue // Should not happen
		}

		dir, err := l.GetChatsDir(groupNode)
		if err != nil {
			continue // Log this? For now, skip.
		}

		if _, err := os.Stat(dir); err == nil {
			chatDirs = append(chatDirs, dir)
		}
		seen[groupKey] = true
	}
	return chatDirs, nil
}
