package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mattsolo1/grove-core/config"
	"github.com/mattsolo1/grove-tend/pkg/assert"
	"github.com/mattsolo1/grove-tend/pkg/fs"
	"github.com/mattsolo1/grove-tend/pkg/harness"
	"gopkg.in/yaml.v3"
)

// NotebooksConfigIsolationScenario tests that notebooks config is properly isolated from user's real config.
func NotebooksConfigIsolationScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "core-notebooks-config-isolation",
		Description: "Verifies that notebooks config respects XDG_CONFIG_HOME and doesn't leak user's real config.",
		Tags:        []string{"core", "notebooks", "config", "isolation"},
		Steps: []harness.Step{
			{
				Name: "Test empty global config doesn't leak user's notebooks",
				Func: func(ctx *harness.Context) error {
					projectDir := ctx.NewDir("test-project")

					// Create empty global config in sandboxed HOME
					globalConfigDir := filepath.Join(ctx.ConfigDir(), "grove")
					if err := fs.CreateDir(globalConfigDir); err != nil {
						return fmt.Errorf("failed to create global config dir: %w", err)
					}

					// Create minimal global config (no notebooks section)
					globalYAML := `version: "1.0"
`
					if err := fs.WriteString(filepath.Join(globalConfigDir, "grove.yml"), globalYAML); err != nil {
						return err
					}

					// Debug: Read back the global config
					globalWritten, _ := os.ReadFile(filepath.Join(globalConfigDir, "grove.yml"))
					fmt.Printf("=== Written global grove.yml ===\n%s\n=== End ===\n", string(globalWritten))

					// Create project config with local notebooks
					projectYAML := `name: test-project
version: "1.0"
notebooks:
  rules:
    default: "local"
  definitions:
    local:
      root_dir: ""
`
					if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), projectYAML); err != nil {
						return err
					}

					// Debug: Read back what was written
					writtenContent, err := os.ReadFile(filepath.Join(projectDir, "grove.yml"))
					if err != nil {
						return fmt.Errorf("failed to read back written file: %w", err)
					}
					fmt.Printf("=== Written project grove.yml ===\n%s\n=== End ===\n", string(writtenContent))

					// Debug: Load the config directly and inspect it
					layered, err := config.LoadLayered(projectDir)
					if err != nil {
						return fmt.Errorf("failed to load config for debugging: %w", err)
					}

					// Debug: Check what's loaded for global config
					if layered.Global != nil {
						globalData, _ := yaml.Marshal(layered.Global)
						fmt.Printf("=== Loaded global config ===\n%s\n=== End ===\n", string(globalData))
					}

					if layered.Project != nil && layered.Project.Notebooks != nil && layered.Project.Notebooks.Definitions != nil {
						if nb, ok := layered.Project.Notebooks.Definitions["local"]; ok {
							fmt.Printf("=== Loaded notebook 'local' ===\n")
							fmt.Printf("RootDir: '%s' (len=%d)\n", nb.RootDir, len(nb.RootDir))
							fmt.Printf("Notebook ptr: %p\n", nb)
							fmt.Printf("Full notebook: %+v\n", nb)

							// Test marshaling just this config
							data, _ := yaml.Marshal(layered.Project)
							fmt.Printf("=== Marshaled project config ===\n%s\n=== End ===\n", string(data))
						} else {
							fmt.Printf("=== No 'local' notebook found in definitions ===\n")
						}
					} else {
						fmt.Printf("=== Project notebooks config is nil or empty ===\n")
					}

					// Run config-layers to see what config is loaded
					coreBinary, err := FindProjectBinary()
					if err != nil {
						return err
					}

					cmd := ctx.Command(coreBinary, "config-layers").Dir(projectDir)
					result := cmd.Run()
					ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
					if result.Error != nil {
						return fmt.Errorf("`core config-layers` failed: %w", result.Error)
					}

					output := result.Stdout

					// Verify notebooks section is present from project config
					if err := assert.Contains(output, "notebooks:", "notebooks section should be present"); err != nil {
						return err
					}
					if err := assert.Contains(output, "local:", "local notebooks definition should be present"); err != nil {
						return err
					}
					if err := assert.Contains(output, `root_dir: ""`, "local mode should have empty root_dir"); err != nil {
						return err
					}

					// Verify user's real notebook paths are NOT present
					if err := assert.NotContains(output, "/Users/solom4/notebooks", "user's real notebooks path should NOT be present"); err != nil {
						return err
					}

					return nil
				},
			},
		},
	}
}

// NotebooksLocalModeScenario tests that notebook locator correctly resolves paths in local mode.
func NotebooksLocalModeScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "core-notebooks-local-mode",
		Description: "Verifies that NotebookLocator correctly resolves paths in local mode (root_dir: \"\").",
		Tags:        []string{"core", "notebooks", "locator"},
		Steps: []harness.Step{
			{
				Name: "Test notebooks path resolution in local mode",
				Func: func(ctx *harness.Context) error {
					projectDir := ctx.NewDir("my-workspace")

					// Create empty global config
					globalConfigDir := filepath.Join(ctx.ConfigDir(), "grove")
					if err := fs.CreateDir(globalConfigDir); err != nil {
						return fmt.Errorf("failed to create global config dir: %w", err)
					}

					emptyGlobalYAML := `version: "1.0"
`
					if err := fs.WriteString(filepath.Join(globalConfigDir, "grove.yml"), emptyGlobalYAML); err != nil {
						return err
					}

					// Create project config with local mode notebooks
					projectYAML := `name: my-workspace
version: "1.0"
notebooks:
  rules:
    default: "local"
  definitions:
    local:
      root_dir: ""
`
					if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), projectYAML); err != nil {
						return err
					}

					// Get core binary
					coreBinary, err := FindProjectBinary()
					if err != nil {
						return err
					}

					// Use config-layers to see the merged config
					cmd := ctx.Command(coreBinary, "config-layers").Dir(projectDir)
					result := cmd.Run()
					ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
					if result.Error != nil {
						return fmt.Errorf("`core config-layers` failed: %w", result.Error)
					}

					output := result.Stdout

					// Verify local mode is configured
					if err := assert.Contains(output, `root_dir: ""`, "local mode should have empty root_dir"); err != nil {
						return err
					}

					// Verify no centralized notebook paths are present
					if err := assert.NotContains(output, "~/notebooks", "should not contain user's centralized notebooks path"); err != nil {
						return err
					}
					if err := assert.NotContains(output, "/Users/", "should not contain absolute user paths"); err != nil {
						return err
					}

					return nil
				},
			},
		},
	}
}

// NotebooksXDGConfigHomeScenario tests that XDG_CONFIG_HOME is properly respected.
func NotebooksXDGConfigHomeScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "core-notebooks-xdg-config-home",
		Description: "Verifies that grove-core respects XDG_CONFIG_HOME for config loading.",
		Tags:        []string{"core", "notebooks", "xdg", "env"},
		Steps: []harness.Step{
			{
				Name: "Verify XDG_CONFIG_HOME is used for global config",
				Func: func(ctx *harness.Context) error {
					projectDir := ctx.NewDir("test-xdg")

					// Create global config in sandboxed XDG_CONFIG_HOME
					globalConfigDir := filepath.Join(ctx.ConfigDir(), "grove")
					if err := fs.CreateDir(globalConfigDir); err != nil {
						return fmt.Errorf("failed to create global config dir: %w", err)
					}

					// Create global config with a marker value
					globalYAML := `version: "1.0"
name: xdg-config-marker
logging:
  level: trace
`
					if err := fs.WriteString(filepath.Join(globalConfigDir, "grove.yml"), globalYAML); err != nil {
						return err
					}

					// Create minimal project config
					projectYAML := `name: test-project
version: "1.0"
`
					if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), projectYAML); err != nil {
						return err
					}

					// Run config-layers
					coreBinary, err := FindProjectBinary()
					if err != nil {
						return err
					}

					cmd := ctx.Command(coreBinary, "config-layers").Dir(projectDir)
					result := cmd.Run()
					ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
					if result.Error != nil {
						return fmt.Errorf("`core config-layers` failed: %w", result.Error)
					}

					output := result.Stdout

					// Verify the marker from XDG_CONFIG_HOME global config is present
					if err := assert.Contains(output, "level: trace", "global config from XDG_CONFIG_HOME should be loaded"); err != nil {
						return err
					}

					// Final name should be from project config (override)
					if err := assert.Contains(output, "name: test-project", "project name should override global"); err != nil {
						return err
					}

					return nil
				},
			},
		},
	}
}
