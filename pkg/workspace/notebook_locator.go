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
	defaultNotesPathTemplate       = "notebooks/{{ .Workspace.Name }}/notes/{{ .NoteType }}"
	defaultPlansPathTemplate       = "notebooks/{{ .Workspace.Name }}/plans"
	defaultChatsPathTemplate       = "notebooks/{{ .Workspace.Name }}/chats"
	defaultGlobalNotesPathTemplate = "global/notes/{{ .NoteType }}"
	defaultGlobalPlansPathTemplate = "global/plans"
	defaultGlobalChatsPathTemplate = "global/chats"
)

// ScannedDir represents a directory found by the locator, linking it
// to the WorkspaceNode that owns it.
type ScannedDir struct {
	Path  string
	Owner *WorkspaceNode
}

// NotebookLocator resolves paths for notes, plans, and chats based on configuration.
// It operates in two modes:
//   - Local Mode (default): Plans/chats are stored within the project directory (e.g., ./plans)
//   - Centralized Mode (opt-in): Plans/chats are stored in a centralized notebook directory
//
// The mode is determined by whether notebook.root_dir is configured.
type NotebookLocator struct {
	config *config.Notebook
}

// NewNotebookLocator creates a new locator. It gracefully handles a nil config.
// It uses the "default" notebook from the Notebooks map, maintaining backward compatibility.
func NewNotebookLocator(cfg *config.Config) *NotebookLocator {
	var notebookCfg *config.Notebook
	if cfg != nil && cfg.Notebooks != nil {
		// Use the "default" notebook for backward compatibility
		notebookCfg = cfg.Notebooks["default"]
	}

	// Ensure we have a config object to work with, even if it's empty.
	if notebookCfg == nil {
		notebookCfg = &config.Notebook{}
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
		// Local Mode: Plans are inside the project's root .notebook directory.
		// Use GetGroupingKey to correctly handle worktrees.
		return filepath.Join(node.GetGroupingKey(), ".notebook", "plans"), nil
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
			if node.Kind == KindEcosystemWorktree {
				// This is an ecosystem worktree. It acts as a container.
				// Its notebook context is that of the root ecosystem.
				contextNode = &WorkspaceNode{
					Name:                filepath.Base(node.RootEcosystemPath),
					Path:                node.RootEcosystemPath,
					ParentEcosystemPath: node.ParentEcosystemPath,
					RootEcosystemPath:   node.RootEcosystemPath,
				}
			} else if node.ParentProjectPath != "" {
				// This correctly handles all other worktree kinds that are children of a project:
				// - KindStandaloneProjectWorktree
				// - KindEcosystemSubProjectWorktree (This fixes the bug)
				// - KindEcosystemWorktreeSubProjectWorktree
				// In all these cases, the notebook context belongs to the parent project.
				contextNode = &WorkspaceNode{
					Name:                filepath.Base(node.ParentProjectPath),
					Path:                node.ParentProjectPath,
					ParentEcosystemPath: node.ParentEcosystemPath,
					RootEcosystemPath:   node.RootEcosystemPath,
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
		// Local Mode: Chats are inside the project's root .notebook directory.
		return filepath.Join(node.GetGroupingKey(), ".notebook", "chats"), nil
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
			if node.Kind == KindEcosystemWorktree {
				// This is an ecosystem worktree. It acts as a container.
				// Its notebook context is that of the root ecosystem.
				contextNode = &WorkspaceNode{
					Name:                filepath.Base(node.RootEcosystemPath),
					Path:                node.RootEcosystemPath,
					ParentEcosystemPath: node.ParentEcosystemPath,
					RootEcosystemPath:   node.RootEcosystemPath,
				}
			} else if node.ParentProjectPath != "" {
				// This correctly handles all other worktree kinds that are children of a project:
				// - KindStandaloneProjectWorktree
				// - KindEcosystemSubProjectWorktree (This fixes the bug)
				// - KindEcosystemWorktreeSubProjectWorktree
				// In all these cases, the notebook context belongs to the parent project.
				contextNode = &WorkspaceNode{
					Name:                filepath.Base(node.ParentProjectPath),
					Path:                node.ParentProjectPath,
					ParentEcosystemPath: node.ParentEcosystemPath,
					RootEcosystemPath:   node.RootEcosystemPath,
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
		// Local Mode: Notes are inside the project's root .notebook directory.
		return filepath.Join(node.GetGroupingKey(), ".notebook", "notes", noteType), nil
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
			if node.Kind == KindEcosystemWorktree {
				// This is an ecosystem worktree. It acts as a container.
				// Its notebook context is that of the root ecosystem.
				contextNode = &WorkspaceNode{
					Name:                filepath.Base(node.RootEcosystemPath),
					Path:                node.RootEcosystemPath,
					ParentEcosystemPath: node.ParentEcosystemPath,
					RootEcosystemPath:   node.RootEcosystemPath,
				}
			} else if node.ParentProjectPath != "" {
				// This correctly handles all other worktree kinds that are children of a project:
				// - KindStandaloneProjectWorktree
				// - KindEcosystemSubProjectWorktree (This fixes the bug)
				// - KindEcosystemWorktreeSubProjectWorktree
				// In all these cases, the notebook context belongs to the parent project.
				contextNode = &WorkspaceNode{
					Name:                filepath.Base(node.ParentProjectPath),
					Path:                node.ParentProjectPath,
					ParentEcosystemPath: node.ParentEcosystemPath,
					RootEcosystemPath:   node.RootEcosystemPath,
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
// It returns a list of ScannedDir structs, linking each directory to its owner.
// This method properly handles both Local Mode and Centralized Mode.
func (l *NotebookLocator) ScanForAllPlans(provider *Provider) ([]ScannedDir, error) {
	if provider == nil {
		return nil, fmt.Errorf("workspace provider is required")
	}

	// Map of directory path -> owner node. We'll use this to deduplicate and prefer main projects.
	dirOwners := make(map[string]*WorkspaceNode)
	seenGroupKeys := make(map[string]bool)

	for _, node := range provider.All() {
		// We only need to check the root of a project group (not every worktree)
		// as they share the same plan directory.
		groupKey := node.GetGroupingKey()
		if seenGroupKeys[groupKey] {
			continue
		}

		// Use the node representing the group key to resolve the path.
		// Prefer the main project node over worktree nodes.
		groupNode := provider.FindByPath(groupKey)
		if groupNode == nil {
			continue // Should not happen
		}

		// If FindByPath returned a worktree, look for the main project instead
		if groupNode.IsWorktree() {
			// Find the main project with this exact path
			for _, n := range provider.All() {
				if n.Path == groupKey && !n.IsWorktree() {
					groupNode = n
					break
				}
			}
		}

		dir, err := l.GetPlansDir(groupNode)
		if err != nil {
			continue // Log this? For now, skip.
		}

		if _, err := os.Stat(dir); err == nil {
			// Check if we've already seen this directory path
			if existingOwner, exists := dirOwners[dir]; exists {
				// Prefer main projects over worktrees
				if existingOwner.IsWorktree() && !groupNode.IsWorktree() {
					dirOwners[dir] = groupNode
				}
			} else {
				dirOwners[dir] = groupNode
			}
		}
		seenGroupKeys[groupKey] = true
	}

	// Convert map to slice
	var planDirs []ScannedDir
	for dir, owner := range dirOwners {
		planDirs = append(planDirs, ScannedDir{Path: dir, Owner: owner})
	}
	return planDirs, nil
}

// ScanForAllChats discovers all chat directories across all known workspaces.
// It returns a list of ScannedDir structs, linking each directory to its owner.
// This method properly handles both Local Mode and Centralized Mode.
func (l *NotebookLocator) ScanForAllChats(provider *Provider) ([]ScannedDir, error) {
	if provider == nil {
		return nil, fmt.Errorf("workspace provider is required")
	}

	// Map of directory path -> owner node. We'll use this to deduplicate and prefer main projects.
	dirOwners := make(map[string]*WorkspaceNode)
	seenGroupKeys := make(map[string]bool)

	for _, node := range provider.All() {
		// We only need to check the root of a project group (not every worktree)
		// as they share the same chat directory.
		groupKey := node.GetGroupingKey()
		if seenGroupKeys[groupKey] {
			continue
		}

		// Use the node representing the group key to resolve the path.
		// Prefer the main project node over worktree nodes.
		groupNode := provider.FindByPath(groupKey)
		if groupNode == nil {
			continue // Should not happen
		}

		// If FindByPath returned a worktree, look for the main project instead
		if groupNode.IsWorktree() {
			// Find the main project with this exact path
			for _, n := range provider.All() {
				if n.Path == groupKey && !n.IsWorktree() {
					groupNode = n
					break
				}
			}
		}

		dir, err := l.GetChatsDir(groupNode)
		if err != nil {
			continue // Log this? For now, skip.
		}

		if _, err := os.Stat(dir); err == nil {
			// Check if we've already seen this directory path
			if existingOwner, exists := dirOwners[dir]; exists {
				// Prefer main projects over worktrees
				if existingOwner.IsWorktree() && !groupNode.IsWorktree() {
					dirOwners[dir] = groupNode
				}
			} else {
				dirOwners[dir] = groupNode
			}
		}
		seenGroupKeys[groupKey] = true
	}

	// Convert map to slice
	var chatDirs []ScannedDir
	for dir, owner := range dirOwners {
		chatDirs = append(chatDirs, ScannedDir{Path: dir, Owner: owner})
	}
	return chatDirs, nil
}
