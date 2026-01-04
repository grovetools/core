package workspace

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/mattsolo1/grove-core/config"
	"github.com/mattsolo1/grove-core/util/pathutil"
)

const (
	defaultNotesPathTemplate       = "workspaces/{{ .Workspace.Name }}/{{ .NoteType }}"
	defaultPlansPathTemplate       = "workspaces/{{ .Workspace.Name }}/plans"
	defaultChatsPathTemplate       = "workspaces/{{ .Workspace.Name }}/chats"
	defaultTemplatesPathTemplate   = "workspaces/{{ .Workspace.Name }}/templates"
	defaultRecipesPathTemplate     = "workspaces/{{ .Workspace.Name }}/recipes"
	defaultInProgressPathTemplate  = "workspaces/{{ .Workspace.Name }}/in_progress"
	defaultCompletedPathTemplate   = "workspaces/{{ .Workspace.Name }}/completed"
	defaultGlobalNotesPathTemplate = "global/{{ .NoteType }}"
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
	config *config.Config
}

// NewNotebookLocator creates a new locator. It gracefully handles a nil config.
// It now stores the full config to support dynamic notebook resolution based on WorkspaceNode.NotebookName.
func NewNotebookLocator(cfg *config.Config) *NotebookLocator {
	// Ensure we have a config object to work with, even if it's empty.
	if cfg == nil {
		cfg = &config.Config{}
	}

	return &NotebookLocator{
		config: cfg,
	}
}

// getNotebookForNode retrieves the notebook configuration for a given node.
// It uses the node's NotebookName field to look up the correct notebook definition.
func (l *NotebookLocator) getNotebookForNode(node *WorkspaceNode) *config.Notebook {
	if l.config == nil || l.config.Notebooks == nil {
		// Default to centralized notebook at ~/.grove/notebooks/nb when no config exists
		return &config.Notebook{
			RootDir: "~/.grove/notebooks/nb",
		}
	}

	// Use the node's NotebookName to look up the notebook
	if node.NotebookName != "" && l.config.Notebooks.Definitions != nil {
		if nb, exists := l.config.Notebooks.Definitions[node.NotebookName]; exists && nb != nil {
			return nb
		}
	}

	// Fallback to default notebook if specified
	if l.config.Notebooks.Rules != nil && l.config.Notebooks.Rules.Default != "" {
		if nb, exists := l.config.Notebooks.Definitions[l.config.Notebooks.Rules.Default]; exists && nb != nil {
			return nb
		}
	}

	// No notebook configuration found, default to centralized notebook at ~/.grove/notebooks/nb
	return &config.Notebook{
		RootDir: "~/.grove/notebooks/nb",
	}
}

// isCentralized returns true if the system is configured for centralized storage for a given node.
func (l *NotebookLocator) isCentralized(node *WorkspaceNode) bool {
	nb := l.getNotebookForNode(node)
	return nb != nil && nb.RootDir != ""
}

// GetPlansDir returns the absolute path to the plans directory for a given workspace node.
// In Local Mode, it returns the plans directory within the project (using GetGroupingKey to handle worktrees).
// In Centralized Mode, it uses the configured root_dir and path templates.
func (l *NotebookLocator) GetPlansDir(node *WorkspaceNode) (string, error) {
	// Handle global case first
	if node.Name == "global" {
		if l.config != nil && l.config.Notebooks != nil && l.config.Notebooks.Rules != nil && l.config.Notebooks.Rules.Global != nil {
			rootDir, err := pathutil.Expand(l.config.Notebooks.Rules.Global.RootDir)
			if err != nil {
				return "", fmt.Errorf("expanding global notebook root_dir: %w", err)
			}
			return filepath.Join(rootDir, "plans"), nil
		}
		// Fallback for when global is not explicitly configured
		return pathutil.Expand("~/.grove/notebooks/global/plans")
	}

	// For non-global nodes, check mode based on resolved notebook
	if !l.isCentralized(node) {
		// Local Mode: Plans are inside the project's root .notebook directory.
		// Use GetGroupingKey to correctly handle worktrees.
		return filepath.Join(node.GetGroupingKey(), ".notebook", "plans"), nil
	}

	// Centralized Mode
	notebook := l.getNotebookForNode(node)
	rootDir, err := pathutil.Expand(notebook.RootDir)
	if err != nil {
		return "", fmt.Errorf("expanding notebook root_dir for '%s': %w", node.NotebookName, err)
	}

	tplStr := notebook.PlansPathTemplate
	if tplStr == "" {
		tplStr = defaultPlansPathTemplate
	}

	// Determine the correct workspace name for different node kinds
	contextNode := getContextNodeForPath(node)

	data := struct {
		Workspace *WorkspaceNode
	}{
		Workspace: contextNode,
	}

	renderedPath, err := renderPath(tplStr, data)
	if err != nil {
		return "", err
	}

	return filepath.Join(rootDir, renderedPath), nil
}

// GetChatsDir is analogous to GetPlansDir.
func (l *NotebookLocator) GetChatsDir(node *WorkspaceNode) (string, error) {
	// Handle global case first
	if node.Name == "global" {
		if l.config != nil && l.config.Notebooks != nil && l.config.Notebooks.Rules != nil && l.config.Notebooks.Rules.Global != nil {
			rootDir, err := pathutil.Expand(l.config.Notebooks.Rules.Global.RootDir)
			if err != nil {
				return "", fmt.Errorf("expanding global notebook root_dir: %w", err)
			}
			return filepath.Join(rootDir, "chats"), nil
		}
		// Fallback for when global is not explicitly configured
		return pathutil.Expand("~/.grove/notebooks/global/chats")
	}

	// For non-global nodes, check mode based on resolved notebook
	if !l.isCentralized(node) {
		// Local Mode: Chats are inside the project's root .notebook directory.
		return filepath.Join(node.GetGroupingKey(), ".notebook", "chats"), nil
	}

	// Centralized Mode
	notebook := l.getNotebookForNode(node)
	rootDir, err := pathutil.Expand(notebook.RootDir)
	if err != nil {
		return "", fmt.Errorf("expanding notebook root_dir for '%s': %w", node.NotebookName, err)
	}

	tplStr := notebook.ChatsPathTemplate
	if tplStr == "" {
		tplStr = defaultChatsPathTemplate
	}

	// Determine the correct workspace name for different node kinds
	contextNode := getContextNodeForPath(node)

	data := struct {
		Workspace *WorkspaceNode
	}{
		Workspace: contextNode,
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
	// Handle global case first
	if node.Name == "global" {
		if l.config != nil && l.config.Notebooks != nil && l.config.Notebooks.Rules != nil && l.config.Notebooks.Rules.Global != nil {
			rootDir, err := pathutil.Expand(l.config.Notebooks.Rules.Global.RootDir)
			if err != nil {
				return "", fmt.Errorf("expanding global notebook root_dir: %w", err)
			}
			return filepath.Join(rootDir, "notes", noteType), nil
		}
		// Fallback for when global is not explicitly configured
		expandedPath, err := pathutil.Expand("~/.grove/notebooks/global/notes")
		if err != nil {
			return "", err
		}
		return filepath.Join(expandedPath, noteType), nil
	}

	// For non-global nodes, check mode based on resolved notebook
	if !l.isCentralized(node) {
		// Local Mode: Notes are inside the project's root .notebook directory.
		return filepath.Join(node.GetGroupingKey(), ".notebook", "notes", noteType), nil
	}

	// Centralized Mode
	notebook := l.getNotebookForNode(node)
	rootDir, err := pathutil.Expand(notebook.RootDir)
	if err != nil {
		return "", fmt.Errorf("expanding notebook root_dir for '%s': %w", node.NotebookName, err)
	}

	tplStr := notebook.NotesPathTemplate
	if tplStr == "" {
		tplStr = defaultNotesPathTemplate
	}

	// Determine the correct workspace name for different node kinds
	contextNode := getContextNodeForPath(node)

	data := struct {
		Workspace *WorkspaceNode
		NoteType  string
	}{
		Workspace: contextNode,
		NoteType:  noteType,
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

// ContentDirectory represents a directory containing notebook content
type ContentDirectory struct {
	Path string
	Type string // "notes", "plans", "chats"
}

// GetAllContentDirs returns all directories that contain content for a workspace.
// This includes the notes directory (containing subdirs like current, learn, etc.),
// the plans directory, and the chats directory.
func (l *NotebookLocator) GetAllContentDirs(node *WorkspaceNode) ([]ContentDirectory, error) {
	var dirs []ContentDirectory

	// Add notes directory (which contains subdirs like inbox, learn, etc.)
	// We get one note type and go up a level to get the parent notes directory
	notesPath, err := l.GetNotesDir(node, "inbox")
	if err == nil {
		// Go up one level to get the parent notes directory
		dirs = append(dirs, ContentDirectory{
			Path: filepath.Dir(notesPath),
			Type: "notes",
		})
	}

	// Add plans directory
	plansPath, err := l.GetPlansDir(node)
	if err == nil {
		dirs = append(dirs, ContentDirectory{
			Path: plansPath,
			Type: "plans",
		})
	}

	// Add chats directory
	chatsPath, err := l.GetChatsDir(node)
	if err == nil {
		dirs = append(dirs, ContentDirectory{
			Path: chatsPath,
			Type: "chats",
		})
	}

	return dirs, nil
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

// ScanForAllNotes discovers all notes directories across all known workspaces.
// It returns a list of ScannedDir structs, linking each directory to its owner.
func (l *NotebookLocator) ScanForAllNotes(provider *Provider) ([]ScannedDir, error) {
	if provider == nil {
		return nil, fmt.Errorf("workspace provider is required")
	}

	// Map of directory path -> owner node. We'll use this to deduplicate and prefer main projects.
	dirOwners := make(map[string]*WorkspaceNode)
	seenGroupKeys := make(map[string]bool)

	for _, node := range provider.All() {
		// We only need to check the root of a project group (not every worktree)
		// as they share the same notes directory.
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

		// Get the parent notes directory by finding inbox and going up one level
		notesInboxDir, err := l.GetNotesDir(groupNode, "inbox")
		if err != nil {
			continue // Skip if we can't get notes directory
		}
		notesRootDir := filepath.Dir(notesInboxDir)

		if _, err := os.Stat(notesRootDir); err == nil {
			// Check if we've already seen this directory path
			if existingOwner, exists := dirOwners[notesRootDir]; exists {
				// Prefer main projects over worktrees
				if existingOwner.IsWorktree() && !groupNode.IsWorktree() {
					dirOwners[notesRootDir] = groupNode
				}
			} else {
				dirOwners[notesRootDir] = groupNode
			}
		}
		seenGroupKeys[groupKey] = true
	}

	// Convert map to slice
	var noteDirs []ScannedDir
	for dir, owner := range dirOwners {
		noteDirs = append(noteDirs, ScannedDir{Path: dir, Owner: owner})
	}
	return noteDirs, nil
}

// getContextNodeForPath is a helper to determine the correct context node for path rendering.
func getContextNodeForPath(node *WorkspaceNode) *WorkspaceNode {
	contextNode := node
	// For worktrees, we need to use the parent project/ecosystem name
	if node.IsWorktree() {
		if node.Kind == KindEcosystemWorktree {
			contextNode = &WorkspaceNode{
				Name:                filepath.Base(node.RootEcosystemPath),
				Path:                node.RootEcosystemPath,
				ParentEcosystemPath: node.ParentEcosystemPath,
				RootEcosystemPath:   node.RootEcosystemPath,
			}
		} else if node.ParentProjectPath != "" {
			contextNode = &WorkspaceNode{
				Name:                filepath.Base(node.ParentProjectPath),
				Path:                node.ParentProjectPath,
				ParentEcosystemPath: node.ParentEcosystemPath,
				RootEcosystemPath:   node.RootEcosystemPath,
			}
		}
	}
	return contextNode
}

// GetTemplatesDir is analogous to GetPlansDir but for templates.
func (l *NotebookLocator) GetTemplatesDir(node *WorkspaceNode) (string, error) {
	// For non-global nodes, check mode based on resolved notebook
	if !l.isCentralized(node) {
		// Local Mode: Templates are inside the project's root .notebook directory.
		return filepath.Join(node.GetGroupingKey(), ".notebook", "templates"), nil
	}

	// Centralized Mode
	notebook := l.getNotebookForNode(node)
	rootDir, err := pathutil.Expand(notebook.RootDir)
	if err != nil {
		return "", fmt.Errorf("expanding notebook root_dir for '%s': %w", node.NotebookName, err)
	}

	tplStr := notebook.TemplatesPathTemplate
	if tplStr == "" {
		tplStr = defaultTemplatesPathTemplate
	}

	contextNode := getContextNodeForPath(node)
	data := struct {
		Workspace *WorkspaceNode
	}{
		Workspace: contextNode,
	}

	renderedPath, err := renderPath(tplStr, data)
	if err != nil {
		return "", err
	}

	return filepath.Join(rootDir, renderedPath), nil
}

// GetRecipesDir is analogous to GetPlansDir but for recipes.
func (l *NotebookLocator) GetRecipesDir(node *WorkspaceNode) (string, error) {
	// For non-global nodes, check mode based on resolved notebook
	if !l.isCentralized(node) {
		// Local Mode: Recipes are inside the project's root .notebook directory.
		return filepath.Join(node.GetGroupingKey(), ".notebook", "recipes"), nil
	}

	// Centralized Mode
	notebook := l.getNotebookForNode(node)
	rootDir, err := pathutil.Expand(notebook.RootDir)
	if err != nil {
		return "", fmt.Errorf("expanding notebook root_dir for '%s': %w", node.NotebookName, err)
	}

	tplStr := notebook.RecipesPathTemplate
	if tplStr == "" {
		tplStr = defaultRecipesPathTemplate
	}

	contextNode := getContextNodeForPath(node)
	data := struct {
		Workspace *WorkspaceNode
	}{
		Workspace: contextNode,
	}

	renderedPath, err := renderPath(tplStr, data)
	if err != nil {
		return "", err
	}

	return filepath.Join(rootDir, renderedPath), nil
}

// GetInProgressDir is analogous to GetPlansDir but for in_progress notes.
func (l *NotebookLocator) GetInProgressDir(node *WorkspaceNode) (string, error) {
	if !l.isCentralized(node) {
		return filepath.Join(node.GetGroupingKey(), ".notebook", "in_progress"), nil
	}
	notebook := l.getNotebookForNode(node)
	rootDir, err := pathutil.Expand(notebook.RootDir)
	if err != nil {
		return "", fmt.Errorf("expanding notebook root_dir for '%s': %w", node.NotebookName, err)
	}
	tplStr := notebook.InProgressPathTemplate
	if tplStr == "" {
		tplStr = defaultInProgressPathTemplate
	}
	contextNode := getContextNodeForPath(node)
	data := struct {
		Workspace *WorkspaceNode
	}{
		Workspace: contextNode,
	}
	renderedPath, err := renderPath(tplStr, data)
	if err != nil {
		return "", err
	}
	return filepath.Join(rootDir, renderedPath), nil
}

// GetCompletedDir is analogous to GetPlansDir but for completed notes.
func (l *NotebookLocator) GetCompletedDir(node *WorkspaceNode) (string, error) {
	if !l.isCentralized(node) {
		return filepath.Join(node.GetGroupingKey(), ".notebook", "completed"), nil
	}
	notebook := l.getNotebookForNode(node)
	rootDir, err := pathutil.Expand(notebook.RootDir)
	if err != nil {
		return "", fmt.Errorf("expanding notebook root_dir for '%s': %w", node.NotebookName, err)
	}
	tplStr := notebook.CompletedPathTemplate
	if tplStr == "" {
		tplStr = defaultCompletedPathTemplate
	}
	contextNode := getContextNodeForPath(node)
	data := struct {
		Workspace *WorkspaceNode
	}{
		Workspace: contextNode,
	}
	renderedPath, err := renderPath(tplStr, data)
	if err != nil {
		return "", err
	}
	return filepath.Join(rootDir, renderedPath), nil
}

// GetSkillsDir returns the absolute path to the skills directory for a given workspace node.
// Skills are stored alongside plans and chats in the notebook structure.
func (l *NotebookLocator) GetSkillsDir(node *WorkspaceNode) (string, error) {
	// For non-global nodes, check mode based on resolved notebook
	if !l.isCentralized(node) {
		// Local Mode: Skills are inside the project's root .notebook directory.
		return filepath.Join(node.GetGroupingKey(), ".notebook", "skills"), nil
	}

	// Centralized Mode
	notebook := l.getNotebookForNode(node)
	rootDir, err := pathutil.Expand(notebook.RootDir)
	if err != nil {
		return "", fmt.Errorf("expanding notebook root_dir for '%s': %w", node.NotebookName, err)
	}

	// Use a default template similar to plans
	tplStr := "workspaces/{{ .Workspace.Name }}/skills"

	contextNode := getContextNodeForPath(node)
	data := struct {
		Workspace *WorkspaceNode
	}{
		Workspace: contextNode,
	}

	renderedPath, err := renderPath(tplStr, data)
	if err != nil {
		return "", err
	}

	return filepath.Join(rootDir, renderedPath), nil
}

// GetGroupDir resolves the absolute path for any group directory (e.g., "inbox", "plans", "plans/my-feature").
// This is the centralized method for resolving all note-related directory paths.
func (l *NotebookLocator) GetGroupDir(node *WorkspaceNode, groupName string) (string, error) {
	parts := strings.SplitN(groupName, "/", 2)
	baseType := parts[0]
	subPath := ""
	if len(parts) > 1 {
		subPath = parts[1]
	}

	var basePath string
	var err error

	switch baseType {
	case "plans":
		basePath, err = l.GetPlansDir(node)
	case "chats":
		basePath, err = l.GetChatsDir(node)
	case "templates":
		basePath, err = l.GetTemplatesDir(node)
	case "recipes":
		basePath, err = l.GetRecipesDir(node)
	case "in_progress":
		basePath, err = l.GetInProgressDir(node)
	case "completed":
		basePath, err = l.GetCompletedDir(node)
	case "skills":
		basePath, err = l.GetSkillsDir(node)
	default:
		// For all other types, it's a subdirectory under the main notes directory.
		basePath, err = l.GetNotesDir(node, groupName)
		// Since GetNotesDir already includes the full groupName, we don't need to join subPath.
		if err != nil {
			return "", err
		}
		return basePath, nil
	}

	if err != nil {
		return "", err
	}

	if subPath != "" {
		return filepath.Join(basePath, subPath), nil
	}

	return basePath, nil
}
