package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/mattsolo1/grove-core/config"
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

// DiscoveryService scans the filesystem to find and classify Grove entities.
type DiscoveryService struct {
	logger *logrus.Logger
}

// NewDiscoveryService creates a new discovery service.
func NewDiscoveryService(logger *logrus.Logger) *DiscoveryService {
	return &DiscoveryService{logger: logger}
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
	layeredCfg, err := config.LoadLayered(os.Getenv("HOME"))
	if err != nil || layeredCfg.Global == nil {
		s.logger.Warn("No global grove.yml found or failed to load. No 'groves' to scan.")
		return result, nil // Not a fatal error, just means no paths to scan.
	}

	if len(layeredCfg.Global.Groves) == 0 {
		s.logger.Info("No 'groves' search paths defined in global configuration.")
		return result, nil
	}

	// 2. Parallel scan of each configured grove path.
	type groveResult struct {
		projects   []Project
		ecosystems []Ecosystem
		nonGrove   []string
	}

	var wg sync.WaitGroup
	resultsChan := make(chan groveResult, len(layeredCfg.Global.Groves))

	for key, groveCfg := range layeredCfg.Global.Groves {
		if !groveCfg.Enabled {
			continue
		}

		// Expand path, e.g., ~/Work -> /Users/user/Work
		expandedPath := expandPath(groveCfg.Path)
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

			// 3. Scan the directory.
			err := filepath.WalkDir(grovePath, func(path string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !d.IsDir() {
					return nil
				}

				// Special handling for .grove-worktrees directories
				if d.Name() == ".grove-worktrees" {
					// Check if parent directory is an ecosystem
					parentPath := filepath.Dir(path)
					parentGroveYml := filepath.Join(parentPath, "grove.yml")
					if _, statErr := os.Stat(parentGroveYml); statErr == nil {
						parentCfg, loadErr := config.Load(parentGroveYml)
						if loadErr == nil && len(parentCfg.Workspaces) > 0 {
							// Parent is an ecosystem - treat each worktree as a project
							if entries, readErr := os.ReadDir(path); readErr == nil {
								for _, entry := range entries {
									if entry.IsDir() {
										wtPath := filepath.Join(path, entry.Name())
										proj := Project{
											Name:                entry.Name(),
											Path:                wtPath,
											ParentEcosystemPath: parentPath,
											Workspaces: []DiscoveredWorkspace{
												{
													Name:              entry.Name(),
													Path:              wtPath,
													Type:              WorkspaceTypePrimary,
													ParentProjectPath: wtPath,
												},
											},
										}
										groveRes.projects = append(groveRes.projects, proj)
									}
								}
							}
							// Continue descending into ecosystem worktrees to discover repos/submodules within them
							// This allows focusing on an ecosystem worktree to show all its contained repos
							return nil
						}
					}
					// If not an ecosystem's .grove-worktrees, continue normally
					return nil
				}

				// Check for grove.yml to classify the directory
				groveYmlPath := filepath.Join(path, "grove.yml")
				if _, statErr := os.Stat(groveYmlPath); statErr == nil {
					// Skip re-processing only if this is a direct child of .grove-worktrees
					// (the worktree directory itself, which was already classified above)
					// But DO process subdirectories within worktrees (submodules, nested repos)
					if filepath.Base(filepath.Dir(path)) == ".grove-worktrees" {
						return nil
					}

					cfg, loadErr := config.Load(groveYmlPath)
					if loadErr == nil {
						if len(cfg.Workspaces) > 0 {
							// This is an Ecosystem
							ecosystemName := cfg.Name
							if ecosystemName == "" {
								// Fall back to directory name if grove.yml has no name
								ecosystemName = filepath.Base(path)
							}
							eco := Ecosystem{
								Name: ecosystemName,
								Path: path,
								Type: "User", // Default to User, can be refined
							}
							if eco.Name == "grove-ecosystem" {
								eco.Type = "Grove"
							}
							groveRes.ecosystems = append(groveRes.ecosystems, eco)

							// Continue descending to find projects within the ecosystem
							return nil
						} else {
							// This is a Project
							projectName := cfg.Name
							if projectName == "" {
								// Fall back to directory name if grove.yml has no name
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
							groveRes.projects = append(groveRes.projects, proj)
							return filepath.SkipDir // Don't descend further into a project
						}
					}
				} else {
					// Check for .git to classify as Non-Grove Directory
					if _, statErr := os.Stat(filepath.Join(path, ".git")); statErr == nil {
						groveRes.nonGrove = append(groveRes.nonGrove, path)
						return filepath.SkipDir
					}
				}

				return nil
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
	ecosystemPaths := make([]string, 0, len(result.Ecosystems)+len(result.Projects))
	for _, eco := range result.Ecosystems {
		ecosystemPaths = append(ecosystemPaths, eco.Path)
	}
	// Also include ecosystem worktrees (projects that have a ParentEcosystemPath set)
	// but exclude paths that are inside .grove-worktrees directories as those are
	// worktree checkouts, not ecosystem roots
	for _, proj := range result.Projects {
		if proj.ParentEcosystemPath != "" && !strings.Contains(proj.Path, ".grove-worktrees") {
			ecosystemPaths = append(ecosystemPaths, proj.Path)
		}
	}

	// Now link each project to its closest parent ecosystem
	for i := range result.Projects {
		// Find the most specific (longest) matching ecosystem path
		var bestMatch string
		for _, ecoPath := range ecosystemPaths {
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
