package skills

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/fs"
	"github.com/grovetools/core/git"
	"github.com/grovetools/core/internal/daemon/store"
	"github.com/grovetools/core/pkg/workspace"
)

// GetSkillsDirectoryForWorktree returns the standard skills directory path for a worktree.
func GetSkillsDirectoryForWorktree(worktreePath, provider string) string {
	switch provider {
	case "codex":
		return filepath.Join(worktreePath, ".codex", "skills")
	case "opencode":
		return filepath.Join(worktreePath, ".opencode", "skill")
	default: // claude
		return filepath.Join(worktreePath, ".claude", "skills")
	}
}

// SyncForNode performs the skill resolution and synchronization for a single workspace node.
// It returns a SkillSyncPayload containing the results of the sync operation.
func SyncForNode(cfg *config.Config, node *workspace.WorkspaceNode) (store.SkillSyncPayload, error) {
	payload := store.SkillSyncPayload{
		Workspace: "global",
	}
	if node != nil {
		payload.Workspace = node.Name
	}

	if node == nil {
		return payload, fmt.Errorf("global skill sync not supported without a target node")
	}

	locator := workspace.NewNotebookLocator(cfg)

	gitRoot, err := git.GetGitRoot(node.Path)
	if err != nil {
		return payload, fmt.Errorf("failed to determine git root: %w", err)
	}

	skillsCfg, err := LoadSkillsConfig(cfg, node)
	if err != nil {
		return payload, fmt.Errorf("failed to load skills config: %w", err)
	}

	// Determine default providers for cleanup
	providers := []string{"claude"}
	if skillsCfg != nil && len(skillsCfg.Providers) > 0 {
		providers = skillsCfg.Providers
	}

	// If no skills configured, clean up all skills from destination
	if skillsCfg == nil || (len(skillsCfg.Use) == 0 && len(skillsCfg.Dependencies) == 0) {
		for _, provider := range providers {
			destBaseDir := GetSkillsDirectoryForWorktree(gitRoot, provider)
			cleanupRemovedSkills(destBaseDir, nil) // nil means remove all
		}
		return payload, nil
	}

	resolved, err := ResolveConfiguredSkills(locator, node, skillsCfg)
	if err != nil {
		return payload, fmt.Errorf("failed to resolve skills: %w", err)
	}

	// Even if no skills resolved, we may need to clean up
	if len(resolved) == 0 {
		for _, provider := range providers {
			destBaseDir := GetSkillsDirectoryForWorktree(gitRoot, provider)
			cleanupRemovedSkills(destBaseDir, nil) // nil means remove all
		}
		return payload, nil
	}

	var synced []string
	destPathsMap := make(map[string]bool)

	// Build set of configured skill names for cleanup
	configuredSkills := make(map[string]bool)
	for skillName := range resolved {
		configuredSkills[skillName] = true
	}

	// Copy to provider directories
	for skillName, r := range resolved {
		for _, provider := range r.Providers {
			destBaseDir := GetSkillsDirectoryForWorktree(gitRoot, provider)
			destPath := filepath.Join(destBaseDir, skillName)
			destPathsMap[destBaseDir] = true

			if err := os.MkdirAll(destBaseDir, 0755); err != nil {
				return payload, fmt.Errorf("failed to create directory %s: %w", destBaseDir, err)
			}

			// Remove existing skill directory before copying
			os.RemoveAll(destPath)
			if err := fs.CopyDir(r.PhysicalPath, destPath); err != nil {
				return payload, fmt.Errorf("failed to copy skill %s: %w", skillName, err)
			}
		}
		synced = append(synced, skillName)
	}

	// Clean up skills that are no longer configured
	for destBaseDir := range destPathsMap {
		cleanupRemovedSkills(destBaseDir, configuredSkills)
	}

	var destPaths []string
	for p := range destPathsMap {
		destPaths = append(destPaths, p)
	}

	payload.SyncedSkills = synced
	payload.DestPaths = destPaths

	return payload, nil
}

// cleanupRemovedSkills removes skill directories that are no longer in the configured set.
// If configuredSkills is nil, removes ALL skill directories.
func cleanupRemovedSkills(skillsDir string, configuredSkills map[string]bool) {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillName := entry.Name()
		// If configuredSkills is nil, remove all; otherwise check if configured
		if configuredSkills == nil || !configuredSkills[skillName] {
			// This skill is no longer configured, remove it
			os.RemoveAll(filepath.Join(skillsDir, skillName))
		}
	}
}
