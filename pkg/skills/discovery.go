package skills

import (
	"os"
	"path/filepath"

	"github.com/grovetools/core/pkg/workspace"
)

// ListSkillSources returns a map of skill names to their physical local source paths.
// It discovers skills from three sources in order of precedence (later overrides earlier):
//  1. User skills: ~/.config/grove/skills/
//  2. Ecosystem skills: {notebook}/workspaces/{ecosystem}/skills/
//  3. Project skills: {notebook}/workspaces/{project}/skills/
func ListSkillSources(locator *workspace.NotebookLocator, node *workspace.WorkspaceNode) map[string]SkillSource {
	sources := make(map[string]SkillSource)

	// 1. User skills (lowest precedence)
	home, _ := os.UserHomeDir()
	userSkillsPath := filepath.Join(home, ".config", "grove", "skills")
	addSkillSources(userSkillsPath, SourceTypeUser, sources)

	// 2. Ecosystem skills
	if node != nil && node.RootEcosystemPath != "" {
		ecoNode := &workspace.WorkspaceNode{
			Name:         filepath.Base(node.RootEcosystemPath),
			Path:         node.RootEcosystemPath,
			NotebookName: node.NotebookName,
		}
		if ecoSkillsDir, err := locator.GetSkillsDir(ecoNode); err == nil && ecoSkillsDir != "" {
			addSkillSources(ecoSkillsDir, SourceTypeEcosystem, sources)
		}
	}

	// 3. Project skills (highest precedence)
	if node != nil {
		if projectSkillsDir, err := locator.GetSkillsDir(node); err == nil && projectSkillsDir != "" {
			addSkillSources(projectSkillsDir, SourceTypeProject, sources)
		}
	}

	return sources
}

// addSkillSources scans a directory for skill subdirectories and adds them to the sources map.
// Each subdirectory is treated as a skill.
func addSkillSources(dir string, sourceType SourceType, sources map[string]SkillSource) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillName := entry.Name()
		sources[skillName] = SkillSource{
			Path: filepath.Join(dir, skillName),
			Type: sourceType,
		}
	}
}
