package skills

import (
	"path/filepath"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/workspace"
)

// LoadSkillsConfig extracts the skills configuration from grove.toml in the workspace.
// It handles inheritance by merging configurations in strict precedence order:
//
//  1. global.skills (base)
//  2. global.skills.ecosystems.<name> (user-scoped ecosystem overrides)
//  3. ecosystem grove.toml (team-shared ecosystem config)
//  4. global.skills.projects.<name> (user-scoped project overrides)
//  5. project grove.toml (team-shared project config, highest precedence)
//
// User config merges before actual project/ecosystem config, so team-configured
// skills take precedence but user preferences fill in the gaps.
func LoadSkillsConfig(cfg *config.Config, node *workspace.WorkspaceNode) (*config.SkillsConfig, error) {
	// Load global config first (contains both base skills and user-scoped overrides)
	globalConfig := config.LoadSkillsFromGlobalConfig(cfg)

	// If no node, just return base global config (without project/ecosystem scopes)
	if node == nil {
		return config.ApplySkillsDefaults(config.CopySkillsConfig(globalConfig)), nil
	}

	// Start with a copy of the base global config
	merged := config.CopySkillsConfig(globalConfig)

	// Determine ecosystem name for user-scoped lookups
	var ecoName string
	if node.RootEcosystemPath != "" && node.RootEcosystemPath != node.Path {
		ecoName = filepath.Base(node.RootEcosystemPath)
	} else if node.IsEcosystem() {
		ecoName = node.Name
	}

	// 1. Apply global ecosystem overrides (user-scoped, from ~/.config/grove/grove.toml)
	if ecoName != "" && globalConfig != nil && globalConfig.Ecosystems != nil {
		if ecoCfg, ok := globalConfig.Ecosystems[ecoName]; ok {
			merged = config.MergeSkillsConfig(merged, ecoCfg)
		}
	}

	// 2. Apply local ecosystem config (team-shared, from ecosystem grove.toml)
	if node.RootEcosystemPath != "" && node.RootEcosystemPath != node.Path {
		ecosystemConfig, _ := config.LoadSkillsFromPath(node.RootEcosystemPath)
		merged = config.MergeSkillsConfig(merged, ecosystemConfig)
	}

	// 3. Apply global project overrides (user-scoped, from ~/.config/grove/grove.toml)
	// Use repository name, not worktree name
	if globalConfig != nil && globalConfig.Projects != nil {
		projectName := node.Name
		if node.ParentProjectPath != "" {
			projectName = filepath.Base(node.ParentProjectPath)
		}
		if projCfg, ok := globalConfig.Projects[projectName]; ok {
			merged = config.MergeSkillsConfig(merged, projCfg)
		}
	}

	// 4. Apply local project config (team-shared, from project grove.toml, highest precedence)
	projectConfig, _ := config.LoadSkillsFromPath(node.Path)
	merged = config.MergeSkillsConfig(merged, projectConfig)

	return config.ApplySkillsDefaults(merged), nil
}
