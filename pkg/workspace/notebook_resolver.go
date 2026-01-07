package workspace

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mattsolo1/grove-core/config"
	"github.com/mattsolo1/grove-core/util/pathutil"
)

// GetProjectFromNotebookPath finds the project associated with a notebook path.
// Given a path inside a notebook (e.g., /notebooks/nb/workspaces/zooboo2/plans/...),
// it extracts the workspace name and finds the corresponding project.
//
// Returns:
//   - *WorkspaceNode: the project node if found (nil if not found)
//   - string: the notebook root path (empty if not in a notebook)
//   - error: only for unexpected errors
func GetProjectFromNotebookPath(path string) (*WorkspaceNode, string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, "", err
	}

	// Load config for notebook definitions
	cfg, _ := config.LoadDefault()

	// 1. Find the notebook root from config (preferred method)
	notebookRoot, notebook := FindNotebookRootFromConfig(absPath, cfg)

	// 2. Extract workspace name from the path
	var workspaceName string
	if notebookRoot != "" && notebook != nil {
		// Use template-aware extraction
		workspaceName = ExtractWorkspaceFromNotebook(absPath, notebookRoot, notebook)
	}

	// 3. Fallback: check for notebook.yml marker (for notebooks not in config)
	if notebookRoot == "" {
		notebookRoot = FindNotebookMarker(absPath)
		if notebookRoot != "" {
			workspaceName = extractWorkspaceNameFallback(absPath, notebookRoot)
		}
	}

	if notebookRoot == "" {
		return nil, "", nil // Not in a notebook
	}

	if workspaceName == "" || workspaceName == "global" {
		return nil, notebookRoot, nil
	}

	// 4. Find the project with this workspace name
	project := findProjectByWorkspaceName(workspaceName, cfg)
	return project, notebookRoot, nil
}

// FindNotebookRootFromConfig checks if a path is under any configured notebook's root_dir.
// Returns the notebook root path and the notebook config if found.
func FindNotebookRootFromConfig(absPath string, cfg *config.Config) (string, *config.Notebook) {
	if cfg == nil || cfg.Notebooks == nil || cfg.Notebooks.Definitions == nil {
		return "", nil
	}

	for _, nb := range cfg.Notebooks.Definitions {
		if nb == nil || nb.RootDir == "" {
			continue
		}

		rootDir, err := pathutil.Expand(nb.RootDir)
		if err != nil {
			continue
		}

		// Normalize paths for comparison
		rootDir = filepath.Clean(rootDir)

		// Check if path is under this notebook's root
		if strings.HasPrefix(absPath, rootDir+string(filepath.Separator)) || absPath == rootDir {
			return rootDir, nb
		}
	}

	return "", nil
}

// ExtractWorkspaceFromNotebook extracts the workspace name from a path within a notebook
// using the notebook's configured templates.
func ExtractWorkspaceFromNotebook(absPath, notebookRoot string, notebook *config.Notebook) string {
	relPath, err := filepath.Rel(notebookRoot, absPath)
	if err != nil || relPath == "." {
		return ""
	}

	// Try each template to extract workspace name
	templates := getNotebookTemplates(notebook)
	for _, tmpl := range templates {
		if name := extractWorkspaceFromTemplate(relPath, tmpl); name != "" {
			return name
		}
	}

	return ""
}

// FindNotebookMarker walks up from path looking for notebook.yml marker.
// This is a fallback for notebooks not defined in config.
// Returns the directory containing the marker, or empty string if not found.
func FindNotebookMarker(path string) string {
	current := path
	for {
		if IsNotebookRepo(current) {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "" // Reached filesystem root
		}
		current = parent
	}
}

// findProjectByWorkspaceName searches for a project matching the workspace name.
// It first tries common paths, then falls back to full discovery.
func findProjectByWorkspaceName(workspaceName string, cfg *config.Config) *WorkspaceNode {
	// Fast path: Check common project locations
	homeDir, _ := os.UserHomeDir()
	commonPaths := []string{
		filepath.Join(homeDir, "code", workspaceName),
		filepath.Join(homeDir, "Code", workspaceName),
		filepath.Join(homeDir, "projects", workspaceName),
	}

	// Also check paths from groves config
	if cfg != nil && cfg.Groves != nil {
		for _, grove := range cfg.Groves {
			if grove.Enabled != nil && !*grove.Enabled {
				continue
			}
			grovePath, err := pathutil.Expand(grove.Path)
			if err != nil {
				continue
			}
			commonPaths = append(commonPaths, filepath.Join(grovePath, workspaceName))
		}
	}

	for _, projectPath := range commonPaths {
		if node, err := GetProjectByPath(projectPath); err == nil && node != nil {
			return node
		}
	}

	// Fallback: Full discovery to find project by name
	ds := NewDiscoveryService(nil)
	result, err := ds.DiscoverAll()
	if err != nil {
		return nil
	}

	provider := NewProvider(result)
	return provider.FindByName(workspaceName)
}

// getNotebookTemplates returns all path templates for a notebook configuration.
func getNotebookTemplates(nb *config.Notebook) []string {
	templates := []string{}

	// Add configured templates or defaults
	if nb.PlansPathTemplate != "" {
		templates = append(templates, nb.PlansPathTemplate)
	} else {
		templates = append(templates, defaultPlansPathTemplate)
	}

	if nb.ChatsPathTemplate != "" {
		templates = append(templates, nb.ChatsPathTemplate)
	} else {
		templates = append(templates, defaultChatsPathTemplate)
	}

	if nb.NotesPathTemplate != "" {
		templates = append(templates, nb.NotesPathTemplate)
	} else {
		templates = append(templates, defaultNotesPathTemplate)
	}

	if nb.TemplatesPathTemplate != "" {
		templates = append(templates, nb.TemplatesPathTemplate)
	} else {
		templates = append(templates, defaultTemplatesPathTemplate)
	}

	if nb.RecipesPathTemplate != "" {
		templates = append(templates, nb.RecipesPathTemplate)
	} else {
		templates = append(templates, defaultRecipesPathTemplate)
	}

	return templates
}

// extractWorkspaceFromTemplate tries to extract the workspace name from a path
// by reverse-parsing against a template like "workspaces/{{ .Workspace.Name }}/plans".
func extractWorkspaceFromTemplate(relPath, tmpl string) string {
	// Convert template to regex pattern
	pattern := tmpl
	pattern = strings.ReplaceAll(pattern, "{{ .Workspace.Name }}", "(?P<workspace>[^/]+)")
	pattern = strings.ReplaceAll(pattern, "{{.Workspace.Name}}", "(?P<workspace>[^/]+)")
	pattern = strings.ReplaceAll(pattern, "{{ .NoteType }}", "[^/]+")
	pattern = strings.ReplaceAll(pattern, "{{.NoteType}}", "[^/]+")

	// Create patterns for different matching scenarios:
	// 1. Path is under template dir (workspaces/zooboo2/plans/myplan)
	// 2. Path is exactly template dir (workspaces/zooboo2/plans)
	// 3. Path is partial (workspaces/zooboo2) - just the workspace part
	patterns := []string{
		"^" + pattern + "(/|$)", // Full template match with optional subdirs
		"^" + pattern + "$",     // Exact template match
	}

	// Add prefix pattern that just captures up to the workspace name
	if idx := strings.Index(pattern, "(?P<workspace>[^/]+)"); idx != -1 {
		prefixEnd := idx + len("(?P<workspace>[^/]+)")
		prefixPattern := "^" + pattern[:prefixEnd] + "(/|$)"
		patterns = append(patterns, prefixPattern)
	}

	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			continue
		}

		match := re.FindStringSubmatch(relPath)
		if match == nil {
			continue
		}

		for i, name := range re.SubexpNames() {
			if name == "workspace" && i < len(match) {
				return match[i]
			}
		}
	}

	return ""
}

// extractWorkspaceNameFallback extracts workspace name using path heuristics
// when template-based extraction isn't available.
func extractWorkspaceNameFallback(absPath, notebookRoot string) string {
	relPath, err := filepath.Rel(notebookRoot, absPath)
	if err == nil && relPath != "." {
		parts := strings.Split(relPath, string(filepath.Separator))

		// Look for "workspaces/<name>" pattern
		for i, part := range parts {
			if part == "workspaces" && i+1 < len(parts) {
				return parts[i+1]
			}
		}

		// Fallback: first non-empty directory component
		for _, part := range parts {
			if part != "" && part != "." {
				return part
			}
		}
	}

	// If we're at the notebook root itself, extract from the root path
	notebookParts := strings.Split(notebookRoot, string(filepath.Separator))
	for i := len(notebookParts) - 1; i >= 0; i-- {
		part := notebookParts[i]
		if part != "" && part != "." {
			if i > 0 && notebookParts[i-1] == "workspaces" {
				return part
			}
			return part
		}
	}

	return ""
}

// ExtractWorkspaceNameFromNotebookPath is the public interface for extracting
// workspace name from a notebook path. It uses config-based lookup when possible.
func ExtractWorkspaceNameFromNotebookPath(absPath, notebookRoot string) string {
	cfg, _ := config.LoadDefault()

	// Try config-based extraction first
	if root, notebook := FindNotebookRootFromConfig(absPath, cfg); root != "" && notebook != nil {
		if name := ExtractWorkspaceFromNotebook(absPath, root, notebook); name != "" {
			return name
		}
	}

	// Fall back to heuristic extraction
	return extractWorkspaceNameFallback(absPath, notebookRoot)
}

// FindNotebookRoot is kept for backwards compatibility.
// Prefer FindNotebookRootFromConfig for config-aware lookup.
func FindNotebookRoot(path string) string {
	// First try config-based lookup
	cfg, _ := config.LoadDefault()
	if root, _ := FindNotebookRootFromConfig(path, cfg); root != "" {
		return root
	}
	// Fall back to marker-based lookup
	return FindNotebookMarker(path)
}
