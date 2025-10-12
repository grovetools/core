package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/mattsolo1/grove-core/config"
	"github.com/mattsolo1/grove-core/pkg/repo"
	"github.com/sirupsen/logrus"
)

// expandPath expands ~ to home directory and environment variables
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	return os.ExpandEnv(path)
}

// directoryType represents the classification of a directory during discovery
type directoryType int

const (
	typeUnknown directoryType = iota
	typeEcosystem
	typeProject
	typeEcosystemWorktreeDir // The .grove-worktrees directory itself
	typeNonGroveRepo
	typeSkip // Already processed or should be skipped
)

// classifyWorkspaceRoot examines a directory and returns its type based on filesystem markers.
// This is the single source of truth for workspace classification, used by both targeted lookups
// and full discovery scans to ensure consistency.
//
// Note: This function classifies repository roots and does NOT handle worktree directory logic
// (.grove-worktrees). That special case is handled separately by the walker.
func classifyWorkspaceRoot(path string) (directoryType, *config.Config, error) {
	// Check for grove.yml first to classify the directory
	groveYmlPath := filepath.Join(path, "grove.yml")
	if _, statErr := os.Stat(groveYmlPath); statErr == nil {
		cfg, loadErr := config.Load(groveYmlPath)
		if loadErr != nil {
			return typeUnknown, nil, fmt.Errorf("failed to load grove.yml: %w", loadErr)
		}

		// Check if it's an ecosystem (has workspaces key)
		if len(cfg.Workspaces) > 0 {
			return typeEcosystem, cfg, nil
		}

		// It's a project
		return typeProject, cfg, nil
	}

	// Check for .git to classify as Non-Grove Directory
	if _, statErr := os.Stat(filepath.Join(path, ".git")); statErr == nil {
		return typeNonGroveRepo, nil, nil
	}

	return typeUnknown, nil, nil
}

// classifyDirectory is a wrapper around classifyWorkspaceRoot that handles special cases
// for the directory walker, including .grove-worktrees detection.
func classifyDirectory(path string, d os.DirEntry) (directoryType, *config.Config, error) {
	if !d.IsDir() {
		return typeUnknown, nil, nil
	}

	// Special case: .grove-worktrees directory inside an ecosystem
	if d.Name() == ".grove-worktrees" {
		// Check if parent directory is an ecosystem
		parentPath := filepath.Dir(path)
		parentGroveYml := filepath.Join(parentPath, "grove.yml")
		if _, statErr := os.Stat(parentGroveYml); statErr == nil {
			parentCfg, loadErr := config.Load(parentGroveYml)
			if loadErr == nil && len(parentCfg.Workspaces) > 0 {
				// Parent is an ecosystem - this is an ecosystem worktree directory
				return typeEcosystemWorktreeDir, parentCfg, nil
			}
		}
		return typeUnknown, nil, nil
	}

	// Skip re-processing if this is a direct child of .grove-worktrees
	// (the worktree directory itself, which was already classified)
	if filepath.Base(filepath.Dir(path)) == ".grove-worktrees" {
		return typeSkip, nil, nil
	}

	// Use the central classification logic
	return classifyWorkspaceRoot(path)
}

// processEcosystem handles discovery of an ecosystem root directory
func processEcosystem(path string, cfg *config.Config) Ecosystem {
	ecosystemName := cfg.Name
	if ecosystemName == "" {
		ecosystemName = filepath.Base(path)
	}

	eco := Ecosystem{
		Name: ecosystemName,
		Path: path,
		Type: "User",
	}

	if eco.Name == "grove-ecosystem" {
		eco.Type = "Grove"
	}

	return eco
}

// processProject handles discovery of a project directory and its worktrees
func processProject(path string, cfg *config.Config) Project {
	projectName := cfg.Name
	if projectName == "" {
		projectName = filepath.Base(path)
	}

	proj := Project{
		Name:       projectName,
		Path:       path,
		Workspaces: []DiscoveredWorkspace{},
	}

	// Add the Primary Workspace
	proj.Workspaces = append(proj.Workspaces, DiscoveredWorkspace{
		Name:              "main",
		Path:              path,
		Type:              WorkspaceTypePrimary,
		ParentProjectPath: path,
	})

	// Scan for Worktree Workspaces
	worktreeBase := filepath.Join(path, ".grove-worktrees")
	if entries, readErr := os.ReadDir(worktreeBase); readErr == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				wtPath := filepath.Join(worktreeBase, entry.Name())
				proj.Workspaces = append(proj.Workspaces, DiscoveredWorkspace{
					Name:              entry.Name(),
					Path:              wtPath,
					Type:              WorkspaceTypeWorktree,
					ParentProjectPath: path,
				})
			}
		}
	}

	return proj
}

// processEcosystemWorktreeDir handles the special case of .grove-worktrees directory
// inside an ecosystem, treating each subdirectory as a project
func processEcosystemWorktreeDir(path string, parentEcoPath string) []Project {
	var projects []Project

	if entries, readErr := os.ReadDir(path); readErr == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				wtPath := filepath.Join(path, entry.Name())
				proj := Project{
					Name:                entry.Name(),
					Path:                wtPath,
					ParentEcosystemPath: parentEcoPath,
					Workspaces: []DiscoveredWorkspace{
						{
							Name:              entry.Name(),
							Path:              wtPath,
							Type:              WorkspaceTypePrimary,
							ParentProjectPath: wtPath,
						},
					},
				}
				projects = append(projects, proj)
			}
		}
	}

	return projects
}

// processNonGroveRepo records a non-Grove git repository
func processNonGroveRepo(path string) string {
	return path
}

// DiscoveryService scans the filesystem to find and classify Grove entities.
type DiscoveryService struct {
	logger     *logrus.Logger
	configPath string // Optional: if set, used instead of HOME for config discovery
}

// NewDiscoveryService creates a new discovery service.
func NewDiscoveryService(logger *logrus.Logger) *DiscoveryService {
	return &DiscoveryService{logger: logger}
}

// WithConfigPath returns a new DiscoveryService with a custom config path for testing.
// If configPath is set, it will be used instead of HOME directory when loading config.
func (s *DiscoveryService) WithConfigPath(configPath string) *DiscoveryService {
	return &DiscoveryService{
		logger:     s.logger,
		configPath: configPath,
	}
}

// DiscoverAll scans all configured 'groves' and returns a comprehensive result.
func (s *DiscoveryService) DiscoverAll() (*DiscoveryResult, error) {
	result := &DiscoveryResult{
		Projects:            []Project{},
		Ecosystems:          []Ecosystem{},
		NonGroveDirectories: []string{},
	}

	// Track seen paths to avoid duplicates when groves overlap
	seenProjects := make(map[string]bool)
	seenEcosystems := make(map[string]bool)
	seenNonGrove := make(map[string]bool)

	// 1. Load the global configuration to find 'groves' search paths.
	// We use LoadLayered to ensure we get the global config reliably.
	// If configPath is set (for testing), use it instead of HOME.
	configDir := os.Getenv("HOME")
	if s.configPath != "" {
		configDir = s.configPath
	}
	layeredCfg, err := config.LoadLayered(configDir)
	if err != nil || layeredCfg.Global == nil {
		s.logger.Warn("No global grove.yml found or failed to load. No 'groves' to scan.")
		return result, nil // Not a fatal error, just means no paths to scan.
	}

	if len(layeredCfg.Global.SearchPaths) == 0 {
		s.logger.Info("No 'search_paths' defined in global configuration.")
		return result, nil
	}

	// 2. Parallel scan of each configured grove path.
	type groveResult struct {
		projects   []Project
		ecosystems []Ecosystem
		nonGrove   []string
	}

	var wg sync.WaitGroup
	resultsChan := make(chan groveResult, len(layeredCfg.Global.SearchPaths)+1) // +1 for cloned repos

	// Discover cloned repositories concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		cloned, err := s.discoverClonedProjects()
		if err != nil {
			s.logger.Warnf("Could not discover cloned repositories: %v", err)
			return
		}
		if len(cloned) > 0 {
			resultsChan <- groveResult{projects: cloned}
		}
	}()

	for key, searchCfg := range layeredCfg.Global.SearchPaths {
		if !searchCfg.Enabled {
			continue
		}

		// Expand path, e.g., ~/Work -> /Users/user/Work
		expandedPath := expandPath(searchCfg.Path)
		absPath, err := filepath.Abs(expandedPath)
		if err != nil {
			s.logger.Warnf("Could not resolve path for grove '%s': %v", key, err)
			continue
		}

		wg.Add(1)
		go func(groveName, grovePath string) {
			defer wg.Done()

			groveRes := groveResult{
				projects:   []Project{},
				ecosystems: []Ecosystem{},
				nonGrove:   []string{},
			}

			// 3. Scan the directory using the new helper-based approach.
			err := filepath.WalkDir(grovePath, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}

				// Classify the directory
				entityType, groveCfg, classifyErr := classifyDirectory(path, d)
				if classifyErr != nil {
					// Log but continue on classification errors
					s.logger.Warnf("Error classifying directory %s: %v", path, classifyErr)
					return nil
				}

				// Handle based on classification
				switch entityType {
				case typeEcosystem:
					// This is an ecosystem root - add it and continue descending
					eco := processEcosystem(path, groveCfg)
					groveRes.ecosystems = append(groveRes.ecosystems, eco)
					return nil // Continue descending to find projects within

				case typeProject:
					// This is a project - add it and all its worktrees, then skip descending
					proj := processProject(path, groveCfg)
					groveRes.projects = append(groveRes.projects, proj)
					return filepath.SkipDir

				case typeEcosystemWorktreeDir:
					// This is an ecosystem's .grove-worktrees directory
					// Process each subdirectory as an ecosystem worktree project
					parentPath := filepath.Dir(path)
					projects := processEcosystemWorktreeDir(path, parentPath)
					groveRes.projects = append(groveRes.projects, projects...)
					// Continue descending to discover repos/submodules within ecosystem worktrees
					return nil

				case typeNonGroveRepo:
					// This is a git repo without grove.yml
					nonGrovePath := processNonGroveRepo(path)
					groveRes.nonGrove = append(groveRes.nonGrove, nonGrovePath)
					return filepath.SkipDir

				case typeSkip:
					// Already processed, skip this directory
					return nil

				case typeUnknown:
					// Not a grove entity, continue scanning
					return nil

				default:
					return nil
				}
			})
			if err != nil {
				s.logger.Warnf("Error walking path for grove '%s': %v", groveName, err)
			}

			resultsChan <- groveRes
		}(key, absPath)
	}

	// Wait for all goroutines to complete and close channel
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results with deduplication (merge worktrees from duplicates)
	projectMap := make(map[string]*Project) // Map by path for easy lookup

	for groveRes := range resultsChan {
		for _, eco := range groveRes.ecosystems {
			if !seenEcosystems[eco.Path] {
				result.Ecosystems = append(result.Ecosystems, eco)
				seenEcosystems[eco.Path] = true
			}
		}
		for _, proj := range groveRes.projects {
			if existing, found := projectMap[proj.Path]; found {
				// Merge worktrees from duplicate project
				for _, ws := range proj.Workspaces {
					// Check if this worktree is already in the existing project
					isDuplicate := false
					for _, existingWs := range existing.Workspaces {
						if existingWs.Path == ws.Path {
							isDuplicate = true
							break
						}
					}
					if !isDuplicate {
						existing.Workspaces = append(existing.Workspaces, ws)
					}
				}
			} else {
				// New project - add to map and result
				projectMap[proj.Path] = &proj
				result.Projects = append(result.Projects, proj)
				seenProjects[proj.Path] = true
			}
		}
		for _, path := range groveRes.nonGrove {
			if !seenNonGrove[path] {
				result.NonGroveDirectories = append(result.NonGroveDirectories, path)
				seenNonGrove[path] = true
			}
		}
	}

	// 4. Process explicit projects from global config
	if layeredCfg.Global != nil {
		for _, ep := range layeredCfg.Global.ExplicitProjects {
			if !ep.Enabled {
				continue
			}

			expandedPath := expandPath(ep.Path)
			absPath, err := filepath.Abs(expandedPath)
			if err != nil {
				s.logger.Warnf("Could not resolve explicit project path '%s': %v", ep.Path, err)
				continue
			}

			// Check if directory exists
			if info, err := os.Stat(absPath); err != nil || !info.IsDir() {
				s.logger.Warnf("Explicit project path does not exist or is not a directory: %s", absPath)
				continue
			}

			// Skip if already discovered
			if seenProjects[absPath] {
				continue
			}

			// Create project entry
			projectName := ep.Name
			if projectName == "" {
				projectName = filepath.Base(absPath)
			}

			proj := Project{
				Name:       projectName,
				Path:       absPath,
				Workspaces: []DiscoveredWorkspace{},
			}

			// Add the Primary Workspace
			proj.Workspaces = append(proj.Workspaces, DiscoveredWorkspace{
				Name:              "main",
				Path:              absPath,
				Type:              WorkspaceTypePrimary,
				ParentProjectPath: absPath,
			})

			// Scan for Worktree Workspaces
			worktreeBase := filepath.Join(absPath, ".grove-worktrees")
			if entries, readErr := os.ReadDir(worktreeBase); readErr == nil {
				for _, entry := range entries {
					if entry.IsDir() {
						wtPath := filepath.Join(worktreeBase, entry.Name())
						proj.Workspaces = append(proj.Workspaces, DiscoveredWorkspace{
							Name:              entry.Name(),
							Path:              wtPath,
							Type:              WorkspaceTypeWorktree,
							ParentProjectPath: absPath,
						})
					}
				}
			}

			result.Projects = append(result.Projects, proj)
			seenProjects[absPath] = true
		}
	}

	// 5. Final pass to link Projects to their parent Ecosystems.
	// First build a list of all potential ecosystem paths (ecosystems + ecosystem worktrees)
	ecosystemPaths := make(map[string]bool)
	for _, eco := range result.Ecosystems {
		ecosystemPaths[eco.Path] = true
	}

	for _, proj := range result.Projects {
		// A project is an ecosystem worktree if it's a direct child of a .grove-worktrees directory.
		// The discovery process ensures that .grove-worktrees is only processed when it's inside an ecosystem.
		if filepath.Base(filepath.Dir(proj.Path)) == ".grove-worktrees" {
			ecosystemPaths[proj.Path] = true
		}
	}

	// Convert map to slice for matching
	ecoPathSlice := make([]string, 0, len(ecosystemPaths))
	for p := range ecosystemPaths {
		ecoPathSlice = append(ecoPathSlice, p)
	}

	// Now link each project to its closest parent ecosystem
	for i := range result.Projects {
		// Find the most specific (longest) matching ecosystem path
		var bestMatch string
		for _, ecoPath := range ecoPathSlice {
			if strings.HasPrefix(result.Projects[i].Path, ecoPath+string(filepath.Separator)) {
				if len(ecoPath) > len(bestMatch) {
					bestMatch = ecoPath
				}
			}
		}
		if bestMatch != "" {
			result.Projects[i].ParentEcosystemPath = bestMatch
		}
	}

	return result, nil
}

// FindEcosystemRoot searches upward from startDir to find a grove.yml containing a 'workspaces' key.
func FindEcosystemRoot(startDir string) (string, error) {
	if startDir == "" {
		var err error
		startDir, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	// Make startDir absolute
	absStart, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	current := absStart
	for {
		groveYmlPath := filepath.Join(current, "grove.yml")
		if _, err := os.Stat(groveYmlPath); err == nil {
			// Load config to check if it has workspaces
			cfg, err := config.Load(groveYmlPath)
			if err == nil && len(cfg.Workspaces) > 0 {
				return current, nil
			}
		}

		// Move up one directory
		parent := filepath.Dir(current)
		if parent == current {
			// Reached root of filesystem
			break
		}
		current = parent
	}

	return "", fmt.Errorf("no grove.yml with workspaces found in %s or parent directories", absStart)
}

// GetProjects performs discovery and transformation in a single call,
// returning a flat list of WorkspaceNodes ready for consumption with
// pre-calculated tree prefixes for rendering.
func GetProjects(logger *logrus.Logger) ([]*WorkspaceNode, error) {
	discoveryService := NewDiscoveryService(logger)
	result, err := discoveryService.DiscoverAll()
	if err != nil {
		return nil, err
	}
	nodes := TransformToWorkspaceNodes(result)
	return BuildWorkspaceTree(nodes), nil
}

// discoverClonedProjects finds all repositories cloned and managed by `cx repo`.
func (s *DiscoveryService) discoverClonedProjects() ([]Project, error) {
	manager, err := repo.NewManager()
	if err != nil {
		return nil, err
	}

	cloned, err := manager.List()
	if err != nil {
		return nil, err
	}

	var projects []Project
	for _, r := range cloned {
		// Extract a simpler name from the URL
		name := r.URL
		if parts := strings.Split(name, "/"); len(parts) > 1 {
			name = parts[len(parts)-1]
		}
		name = strings.TrimSuffix(name, ".git")

		// Convert repo.RepoInfo to workspace.Project
		proj := Project{
			Name:        name,
			Path:        r.LocalPath,
			Type:        "Cloned",
			Version:     r.PinnedVersion,
			Commit:      r.ResolvedCommit,
			AuditStatus: r.Audit.Status,
			ReportPath:  r.Audit.ReportPath,
			Workspaces: []DiscoveredWorkspace{
				{
					Name:              "main",
					Path:              r.LocalPath,
					Type:              WorkspaceTypePrimary,
					ParentProjectPath: r.LocalPath,
				},
			},
		}
		projects = append(projects, proj)
	}

	return projects, nil
}
