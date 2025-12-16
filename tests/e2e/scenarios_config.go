package main

import (
	"fmt"
	"path/filepath"

	"github.com/mattsolo1/grove-tend/pkg/assert"
	"github.com/mattsolo1/grove-tend/pkg/fs"
	"github.com/mattsolo1/grove-tend/pkg/harness"
)

// CoreConfigLayeringScenario creates a scenario to test the config layering.
func CoreConfigLayeringScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "core-config-layering",
		Description: "Verifies that global, project, and override configs are merged correctly.",
		Tags:        []string{"core", "config"},
		Steps: []harness.Step{
			{
				Name: "Setup layered configuration and verify merge logic",
				Func: func(ctx *harness.Context) error {
					// 1. Setup file structure in sandboxed environment
					projectDir := ctx.NewDir("test-project")
					globalConfigDir := filepath.Join(ctx.HomeDir(), ".config", "grove")
					if err := fs.CreateDir(globalConfigDir); err != nil {
						return fmt.Errorf("failed to create global config dir: %w", err)
					}

					// 2. Create Global Config (~/.config/grove/grove.yml)
					globalYAML := `name: global-name
version: "1.0"
groves:
  global_code:
    path: /tmp/code
    enabled: true
`
					if err := fs.WriteString(filepath.Join(globalConfigDir, "grove.yml"), globalYAML); err != nil {
						return err
					}

					// 3. Create Project Config (./grove.yml)
					projectYAML := `name: project-name
version: "1.0"
extensions:
  proxy:
    enabled: true
`
					if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), projectYAML); err != nil {
						return err
					}

					// 4. Create Override Config (./grove.override.yml)
					overrideYAML := `name: override-name`
					if err := fs.WriteString(filepath.Join(projectDir, "grove.override.yml"), overrideYAML); err != nil {
						return err
					}

					// 5. Execute 'config-layers' command and verify output
					coreBinary, err := findCoreBinary()
					if err != nil {
						return err
					}

					// ctx.Command automatically uses the sandboxed HOME directory.
					cmd := ctx.Command(coreBinary, "config-layers").Dir(projectDir)
					result := cmd.Run()
					ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
					if result.Error != nil {
						return fmt.Errorf("`core config-layers` failed: %w", result.Error)
					}

					// 6. Assertions
					output := result.Stdout
					if err := assert.Contains(output, "FINAL MERGED CONFIG", "final config block should exist"); err != nil {
						return err
					}
					if err := assert.Contains(output, "name: override-name", "override name should be used"); err != nil {
						return err
					}
					if err := assert.Contains(output, "proxy:", "project extension should be present"); err != nil {
						return err
					}
					if err := assert.Contains(output, "global_code:", "global groves should be present"); err != nil {
						return err
					}

					return nil
				},
			},
		},
	}
}

// CoreConfigOverrideScenario tests that project config overrides global config for the same key.
func CoreConfigOverrideScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "core-config-override",
		Description: "Verifies that project-level config values override global config values.",
		Tags:        []string{"core", "config", "override"},
		Steps: []harness.Step{
			{
				Name: "Setup configs with overlapping keys and verify override behavior",
				Func: func(ctx *harness.Context) error {
					projectDir := ctx.NewDir("override-test")
					globalConfigDir := filepath.Join(ctx.HomeDir(), ".config", "grove")
					if err := fs.CreateDir(globalConfigDir); err != nil {
						return fmt.Errorf("failed to create global config dir: %w", err)
					}

					// Global config defines a grove and logging level
					globalYAML := `name: global-name
logging:
  level: info
groves:
  shared_grove:
    path: /global/path
    enabled: false
`
					if err := fs.WriteString(filepath.Join(globalConfigDir, "grove.yml"), globalYAML); err != nil {
						return err
					}

					// Project config overrides the same grove and logging level
					projectYAML := `name: project-name
logging:
  level: debug
groves:
  shared_grove:
    path: /project/path
    enabled: true
`
					if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), projectYAML); err != nil {
						return err
					}

					coreBinary, err := findCoreBinary()
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
					// Verify project values override global values
					if err := assert.Contains(output, "name: project-name", "project name should override global"); err != nil {
						return err
					}
					if err := assert.Contains(output, "level: debug", "project logging level should override global"); err != nil {
						return err
					}
					if err := assert.Contains(output, "path: /project/path", "project grove path should override global"); err != nil {
						return err
					}
					if err := assert.Contains(output, "enabled: true", "project grove enabled should override global"); err != nil {
						return err
					}

					return nil
				},
			},
		},
	}
}

// CoreConfigMissingScenario tests that missing configs are handled gracefully.
func CoreConfigMissingScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "core-config-missing",
		Description: "Verifies that missing config files are handled gracefully without errors.",
		Tags:        []string{"core", "config", "edge-cases"},
		Steps: []harness.Step{
			{
				Name: "Run config-layers with no config files",
				Func: func(ctx *harness.Context) error {
					projectDir := ctx.NewDir("no-config-test")
					if err := fs.CreateDir(projectDir); err != nil {
						return fmt.Errorf("failed to create project dir: %w", err)
					}

					coreBinary, err := findCoreBinary()
					if err != nil {
						return err
					}

					cmd := ctx.Command(coreBinary, "config-layers").Dir(projectDir)
					result := cmd.Run()
					ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)

					// Should succeed even with no config files
					if result.Error != nil {
						return fmt.Errorf("`core config-layers` should succeed with no configs, but failed: %w", result.Error)
					}

					// Should still show output structure
					output := result.Stdout
					if err := assert.Contains(output, "FINAL MERGED CONFIG", "should show final config even with no configs"); err != nil {
						return err
					}

					return nil
				},
			},
		},
	}
}

// CoreConfigVersionScenario tests version field handling in configs.
func CoreConfigVersionScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "core-config-version",
		Description: "Verifies that version field is properly loaded and displayed.",
		Tags:        []string{"core", "config", "version"},
		Steps: []harness.Step{
			{
				Name: "Setup config with version and verify it's displayed",
				Func: func(ctx *harness.Context) error {
					projectDir := ctx.NewDir("version-test")

					projectYAML := `name: versioned-project
version: "2.1.0"
groves:
  test_grove:
    path: /test/path
    enabled: true
`
					if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), projectYAML); err != nil {
						return err
					}

					coreBinary, err := findCoreBinary()
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
					if err := assert.Contains(output, "version: 2.1.0", "version should be displayed"); err != nil {
						return err
					}
					if err := assert.Contains(output, "name: versioned-project", "name should be displayed"); err != nil {
						return err
					}

					return nil
				},
			},
		},
	}
}

// CoreConfigGlobalOverrideScenario tests that global override config is loaded and displayed.
func CoreConfigGlobalOverrideScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "core-config-global-override",
		Description: "Verifies that grove.override.yml in global config directory is loaded and displayed.",
		Tags:        []string{"core", "config", "global-override"},
		Steps: []harness.Step{
			{
				Name: "Setup global and global override configs and verify both are displayed",
				Func: func(ctx *harness.Context) error {
					projectDir := ctx.NewDir("global-override-test")
					if err := fs.CreateDir(projectDir); err != nil {
						return fmt.Errorf("failed to create project dir: %w", err)
					}
					globalConfigDir := filepath.Join(ctx.HomeDir(), ".config", "grove")
					if err := fs.CreateDir(globalConfigDir); err != nil {
						return fmt.Errorf("failed to create global config dir: %w", err)
					}

					// Create Global Config
					globalYAML := `name: global-name
logging:
  level: info
groves:
  global_grove:
    path: /global/path
    enabled: true
`
					if err := fs.WriteString(filepath.Join(globalConfigDir, "grove.yml"), globalYAML); err != nil {
						return err
					}

					// Create Global Override Config
					globalOverrideYAML := `name: global-override-name
logging:
  level: debug
extensions:
  global_extension:
    enabled: true
`
					if err := fs.WriteString(filepath.Join(globalConfigDir, "grove.override.yml"), globalOverrideYAML); err != nil {
						return err
					}

					coreBinary, err := findCoreBinary()
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
					// Verify both global and global override sections are displayed
					if err := assert.Contains(output, "GLOBAL CONFIG", "global config section should exist"); err != nil {
						return err
					}
					if err := assert.Contains(output, "GLOBAL OVERRIDE CONFIG", "global override config section should exist"); err != nil {
						return err
					}
					// Verify global override values take precedence in final config
					if err := assert.Contains(output, "name: global-override-name", "global override name should be in final config"); err != nil {
						return err
					}
					if err := assert.Contains(output, "level: debug", "global override logging level should be in final config"); err != nil {
						return err
					}
					// Verify global config values are still merged (not overridden)
					if err := assert.Contains(output, "global_grove:", "global grove should be in final config"); err != nil {
						return err
					}
					// Verify global override-specific values are present
					if err := assert.Contains(output, "global_extension:", "global override extension should be in final config"); err != nil {
						return err
					}

					return nil
				},
			},
		},
	}
}

// CoreConfigGlobalOverridePrecedenceScenario tests the precedence order with global override.
func CoreConfigGlobalOverridePrecedenceScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "core-config-global-override-precedence",
		Description: "Verifies that global override takes precedence over global but not over project configs.",
		Tags:        []string{"core", "config", "global-override", "precedence"},
		Steps: []harness.Step{
			{
				Name: "Setup all config layers and verify precedence order",
				Func: func(ctx *harness.Context) error {
					projectDir := ctx.NewDir("precedence-test")
					if err := fs.CreateDir(projectDir); err != nil {
						return fmt.Errorf("failed to create project dir: %w", err)
					}
					globalConfigDir := filepath.Join(ctx.HomeDir(), ".config", "grove")
					if err := fs.CreateDir(globalConfigDir); err != nil {
						return fmt.Errorf("failed to create global config dir: %w", err)
					}

					// Create Global Config
					globalYAML := `name: global-name
logging:
  level: info
groves:
  test_grove:
    path: /global/path
    enabled: false
`
					if err := fs.WriteString(filepath.Join(globalConfigDir, "grove.yml"), globalYAML); err != nil {
						return err
					}

					// Create Global Override Config
					globalOverrideYAML := `name: global-override-name
logging:
  level: warn
groves:
  test_grove:
    path: /global-override/path
    enabled: true
`
					if err := fs.WriteString(filepath.Join(globalConfigDir, "grove.override.yml"), globalOverrideYAML); err != nil {
						return err
					}

					// Create Project Config (should override both global and global override)
					projectYAML := `name: project-name
logging:
  level: debug
`
					if err := fs.WriteString(filepath.Join(projectDir, "grove.yml"), projectYAML); err != nil {
						return err
					}

					coreBinary, err := findCoreBinary()
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
					// Verify project config takes highest precedence for name and logging level
					if err := assert.Contains(output, "name: project-name", "project name should override global override and global"); err != nil {
						return err
					}
					if err := assert.Contains(output, "level: debug", "project logging level should override global override and global"); err != nil {
						return err
					}
					// Verify global override takes precedence over global for grove settings
					// (project doesn't define groves, so global override values should win)
					if err := assert.Contains(output, "path: /global-override/path", "global override grove path should override global"); err != nil {
						return err
					}
					if err := assert.Contains(output, "enabled: true", "global override grove enabled should override global"); err != nil {
						return err
					}

					return nil
				},
			},
		},
	}
}

// CoreConfigGlobalOverrideYamlExtensionScenario tests that .yaml extension is also supported.
func CoreConfigGlobalOverrideYamlExtensionScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "core-config-global-override-yaml-extension",
		Description: "Verifies that grove.override.yaml (with .yaml extension) is also supported.",
		Tags:        []string{"core", "config", "global-override"},
		Steps: []harness.Step{
			{
				Name: "Setup global override with .yaml extension and verify it's loaded",
				Func: func(ctx *harness.Context) error {
					projectDir := ctx.NewDir("yaml-extension-test")
					if err := fs.CreateDir(projectDir); err != nil {
						return fmt.Errorf("failed to create project dir: %w", err)
					}
					globalConfigDir := filepath.Join(ctx.HomeDir(), ".config", "grove")
					if err := fs.CreateDir(globalConfigDir); err != nil {
						return fmt.Errorf("failed to create global config dir: %w", err)
					}

					// Create Global Config
					globalYAML := `name: global-name`
					if err := fs.WriteString(filepath.Join(globalConfigDir, "grove.yml"), globalYAML); err != nil {
						return err
					}

					// Create Global Override Config with .yaml extension
					globalOverrideYAML := `name: override-from-yaml-ext
extensions:
  yaml_test:
    enabled: true
`
					if err := fs.WriteString(filepath.Join(globalConfigDir, "grove.override.yaml"), globalOverrideYAML); err != nil {
						return err
					}

					coreBinary, err := findCoreBinary()
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
					// Verify .yaml extension is loaded
					if err := assert.Contains(output, "GLOBAL OVERRIDE CONFIG", "global override config section should exist"); err != nil {
						return err
					}
					if err := assert.Contains(output, "name: override-from-yaml-ext", "override name from .yaml file should be in final config"); err != nil {
						return err
					}
					if err := assert.Contains(output, "yaml_test:", "extension from .yaml file should be in final config"); err != nil {
						return err
					}

					return nil
				},
			},
		},
	}
}
