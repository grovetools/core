package skills

import (
	"fmt"
	"strings"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/workspace"
)

// ResolveConfiguredSkills resolves configured skills to physical paths.
// It takes the skill configuration and available sources and returns a map
// of skill names to their resolved physical locations and target providers.
func ResolveConfiguredSkills(locator *workspace.NotebookLocator, node *workspace.WorkspaceNode, cfg *config.SkillsConfig) (map[string]ResolvedSkill, error) {
	if cfg == nil {
		return nil, nil
	}

	availableSources := ListSkillSources(locator, node)
	defaultProviders := cfg.Providers
	if len(defaultProviders) == 0 {
		defaultProviders = []string{"claude"}
	}

	resolved := make(map[string]ResolvedSkill)

	processSkill := func(skillName string, dep *config.DependencyConfig) error {
		targetProviders := defaultProviders
		expectedSource := ""
		resolveName := skillName

		if dep != nil {
			if len(dep.Providers) > 0 {
				targetProviders = dep.Providers
			}
			if dep.Source != "" {
				expectedSource = dep.Source
			}
			if dep.Name != "" {
				resolveName = dep.Name
			}
		}

		// Handle qualified names (e.g., "ecosystem:skill-name")
		parts := strings.SplitN(resolveName, ":", 2)
		unqualifiedName := resolveName
		if len(parts) == 2 {
			unqualifiedName = parts[1]
			// Cross-workspace skills fall back to unqualified name for daemon sync
			// since full index isn't available
			resolveName = unqualifiedName
		}

		src, found := availableSources[resolveName]
		if !found {
			// Skill not found locally - this is not an error for the daemon,
			// as the skill may be a builtin handled by the CLI only
			return nil
		}

		// If a specific source was requested, verify it matches
		if expectedSource != "" && string(src.Type) != expectedSource {
			// Find specifically requested source
			sourceFound := false
			for name, s := range availableSources {
				if name == resolveName && string(s.Type) == expectedSource {
					src = s
					sourceFound = true
					break
				}
			}
			if !sourceFound {
				return fmt.Errorf("skill '%s' requested from source '%s' but found in '%s'", skillName, expectedSource, src.Type)
			}
		}

		resolved[unqualifiedName] = ResolvedSkill{
			Name:         unqualifiedName,
			SourceType:   src.Type,
			PhysicalPath: src.Path,
			Providers:    targetProviders,
		}
		return nil
	}

	// Process skills listed in Use array
	for _, skillName := range cfg.Use {
		var dep *config.DependencyConfig
		if d, exists := cfg.Dependencies[skillName]; exists {
			dep = &d
		}
		if err := processSkill(skillName, dep); err != nil {
			return nil, err
		}
	}

	// Process any skills only in Dependencies (not in Use)
	for skillName, dep := range cfg.Dependencies {
		parts := strings.SplitN(skillName, ":", 2)
		unqualified := skillName
		if len(parts) == 2 {
			unqualified = parts[1]
		}
		if _, exists := resolved[unqualified]; exists {
			continue // Already processed from Use array
		}
		depCopy := dep
		if err := processSkill(skillName, &depCopy); err != nil {
			return nil, err
		}
	}

	return resolved, nil
}
