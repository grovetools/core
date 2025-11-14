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
