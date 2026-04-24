package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/tend/pkg/fs"
	"github.com/grovetools/tend/pkg/harness"
	"github.com/grovetools/tend/pkg/verify"
)

// --- Helpers ---

// nbCreateGitRepo creates a minimal .git directory marker at the given path.
func nbCreateGitRepo(repoPath string) error {
	gitDir := filepath.Join(repoPath, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		return fmt.Errorf("failed to create .git dir at %s: %w", repoPath, err)
	}
	return fs.WriteString(filepath.Join(gitDir, "HEAD"), "ref: refs/heads/main\n")
}

// nbCreateEcosystemDir creates a directory with a grove.toml ecosystem marker.
func nbCreateEcosystemDir(path string, name string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	content := fmt.Sprintf("name = %q\nworkspaces = [\"*\"]\n", name)
	return fs.WriteString(filepath.Join(path, "grove.toml"), content)
}

// nbCreateGroveProject creates a git repo with a grove.toml config file.
func nbCreateGroveProject(path string, name string) error {
	if err := nbCreateGitRepo(path); err != nil {
		return err
	}
	return fs.WriteString(filepath.Join(path, "grove.toml"), fmt.Sprintf("name = %q\n", name))
}

// nbSetupGlobalConfig creates a global grove.toml with the given TOML content.
func nbSetupGlobalConfig(ctx *harness.Context, configTOML string) error {
	globalConfigDir := filepath.Join(ctx.ConfigDir(), "grove")
	if err := os.MkdirAll(globalConfigDir, 0o755); err != nil {
		return fmt.Errorf("failed to create global config dir: %w", err)
	}
	return fs.WriteString(filepath.Join(globalConfigDir, "grove.toml"), configTOML)
}

// nbResolveSymlinks resolves symlinks in a path to avoid macOS /var → /private/var mismatches.
func nbResolveSymlinks(path string) string {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return path
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}
	return resolved
}

// nbRunDiscovery runs `core ws --json` from the given directory and returns the parsed projects.
func nbRunDiscovery(ctx *harness.Context, dir string) ([]workspace.Project, error) {
	cmd := ctx.Bin("ws", "--json")
	cmd.Dir(dir)
	result := cmd.Run()
	ctx.ShowCommandOutput("core ws --json", result.Stdout, result.Stderr)
	if err := result.AssertSuccess(); err != nil {
		return nil, fmt.Errorf("core ws --json failed: %w\nStdout: %s\nStderr: %s", err, result.Stdout, result.Stderr)
	}
	var projects []workspace.Project
	if err := json.Unmarshal([]byte(result.Stdout), &projects); err != nil {
		return nil, fmt.Errorf("failed to parse ws JSON: %w\nStdout: %s", err, result.Stdout)
	}
	return projects, nil
}

// nbHasProject checks if a project with the given name exists in the slice.
func nbHasProject(projects []workspace.Project, name string) bool {
	for _, p := range projects {
		if p.Name == name {
			return true
		}
	}
	return false
}

// --- Scenarios ---

// NbTomlsRelaxedDiscoveryScenario exercises depth, include_repos, exclude_repos, skip-list,
// and backward compatibility in a single filesystem layout.
//
// Layout:
//
//	grove-root/                (ecosystem with grove.toml)
//	├── project-a/             (grove project)
//	├── included-repo/         (naked git repo, in include_repos)
//	├── other-repo/            (naked git repo, NOT in include_repos)
//	├── excluded-project/      (grove project, in exclude_repos)
//	├── node_modules/hidden/   (grove project inside skip-list dir)
//	└── subdir/deep-project/   (naked git repo at depth 2)
func NbTomlsRelaxedDiscoveryScenario() *harness.Scenario {
	return harness.NewScenario(
		"nb-tomls-relaxed-discovery",
		"Exercises depth, include/exclude repos, skip-list, and backward compat in one layout",
		[]string{"nb-tomls", "discovery"},
		[]harness.Step{
			harness.NewStep("Create filesystem", func(ctx *harness.Context) error {
				root := nbResolveSymlinks(ctx.NewDir("grove-disc"))
				ctx.Set("grove_root", root)

				if err := nbCreateEcosystemDir(root, "disc-eco"); err != nil {
					return err
				}
				if err := nbCreateGitRepo(root); err != nil {
					return err
				}

				// grove project (always visible)
				if err := nbCreateGroveProject(filepath.Join(root, "project-a"), "project-a"); err != nil {
					return err
				}
				// naked repo to be included explicitly
				if err := nbCreateGitRepo(filepath.Join(root, "included-repo")); err != nil {
					return err
				}
				// naked repo NOT included (should stay hidden without depth)
				if err := nbCreateGitRepo(filepath.Join(root, "other-repo")); err != nil {
					return err
				}
				// grove project that will be excluded
				if err := nbCreateGroveProject(filepath.Join(root, "excluded-project"), "excluded-project"); err != nil {
					return err
				}
				// project inside node_modules (skip-list)
				if err := nbCreateGroveProject(filepath.Join(root, "node_modules", "hidden"), "hidden"); err != nil {
					return err
				}
				// naked repo at depth 2
				if err := os.MkdirAll(filepath.Join(root, "subdir"), 0o755); err != nil {
					return err
				}
				if err := nbCreateGitRepo(filepath.Join(root, "subdir", "deep-project")); err != nil {
					return err
				}

				return nil
			}),

			// ---------- Run 1: depth=1, include_repos, exclude_repos ----------
			harness.NewStep("Configure depth=1 + include + exclude", func(ctx *harness.Context) error {
				root := ctx.Get("grove_root").(string)
				return nbSetupGlobalConfig(ctx, fmt.Sprintf(`[groves.work]
path = %q
enabled = true
depth = 1
include_repos = ["included-repo"]
exclude_repos = ["excluded-project"]
`, root))
			}),
			harness.NewStep("Verify depth + include + exclude + skip-list", func(ctx *harness.Context) error {
				root := ctx.Get("grove_root").(string)
				projects, err := nbRunDiscovery(ctx, root)
				if err != nil {
					return err
				}
				return ctx.Verify(func(v *verify.Collector) {
					v.True("project-a discovered", nbHasProject(projects, "project-a"))
					v.True("included-repo promoted by include_repos", nbHasProject(projects, "included-repo"))
					v.True("other-repo promoted by depth=1", nbHasProject(projects, "other-repo"))
					v.True("excluded-project skipped", !nbHasProject(projects, "excluded-project"))
					v.True("hidden inside node_modules skipped", !nbHasProject(projects, "hidden"))
					v.True("deep-project beyond depth=1", !nbHasProject(projects, "deep-project"))
				})
			}),

			// ---------- Run 2: no depth, no include, no exclude (backward compat) ----------
			harness.NewStep("Reconfigure without depth/include/exclude", func(ctx *harness.Context) error {
				root := ctx.Get("grove_root").(string)
				return nbSetupGlobalConfig(ctx, fmt.Sprintf(`[groves.work]
path = %q
enabled = true
`, root))
			}),
			harness.NewStep("Verify backward compat: only grove projects found", func(ctx *harness.Context) error {
				root := ctx.Get("grove_root").(string)
				projects, err := nbRunDiscovery(ctx, root)
				if err != nil {
					return err
				}
				return ctx.Verify(func(v *verify.Collector) {
					v.True("project-a still discovered", nbHasProject(projects, "project-a"))
					v.True("excluded-project now visible (no exclude)", nbHasProject(projects, "excluded-project"))
					v.True("included-repo not promoted without depth/include", !nbHasProject(projects, "included-repo"))
					v.True("other-repo not promoted without depth", !nbHasProject(projects, "other-repo"))
					v.True("deep-project still hidden", !nbHasProject(projects, "deep-project"))
				})
			}),
		},
	)
}

// NbTomlsNotebookConfigScenario tests notebook config resolution, TOML parsing,
// merge ordering, and fallback format support.
//
// Layout:
//
//	grove-root/                       (ecosystem)
//	├── proj-local/                   (has local grove.toml + notebook grove.toml)
//	├── proj-nb-only/                 (naked git repo, notebook provides config)
//	├── proj-yml-nb/                  (grove project, notebook in .yml format)
//	└── proj-yaml-nb/                 (grove project, notebook in .yaml format)
//	notebook-root/workspaces/
//	├── proj-local/grove.toml
//	├── proj-nb-only/grove.toml
//	├── proj-yml-nb/grove.yml
//	└── proj-yaml-nb/grove.yaml
func NbTomlsNotebookConfigScenario() *harness.Scenario {
	return harness.NewScenario(
		"nb-tomls-notebook-config",
		"Notebook config resolution, TOML parsing, merge order, and format fallback",
		[]string{"nb-tomls", "notebook", "config"},
		[]harness.Step{
			harness.NewStep("Create filesystem", func(ctx *harness.Context) error {
				root := nbResolveSymlinks(ctx.NewDir("grove-nb"))
				nbRoot := nbResolveSymlinks(ctx.NewDir("nb-root"))
				ctx.Set("grove_root", root)
				ctx.Set("notebook_root", nbRoot)

				// Ecosystem
				if err := nbCreateEcosystemDir(root, "nb-eco"); err != nil {
					return err
				}
				if err := nbCreateGitRepo(root); err != nil {
					return err
				}

				// proj-local: has local grove.toml AND notebook grove.toml
				if err := nbCreateGroveProject(filepath.Join(root, "proj-local"), "proj-local"); err != nil {
					return err
				}
				nbLocal := filepath.Join(nbRoot, "workspaces", "proj-local")
				if err := os.MkdirAll(nbLocal, 0o755); err != nil {
					return err
				}
				if err := fs.WriteString(filepath.Join(nbLocal, "grove.toml"), `name = "proj-local-from-nb"

[extensions]
nb_setting = "from-notebook"
`); err != nil {
					return err
				}

				// proj-nb-only: naked git repo, config only in notebook
				if err := nbCreateGitRepo(filepath.Join(root, "proj-nb-only")); err != nil {
					return err
				}
				nbOnly := filepath.Join(nbRoot, "workspaces", "proj-nb-only")
				if err := os.MkdirAll(nbOnly, 0o755); err != nil {
					return err
				}
				if err := fs.WriteString(filepath.Join(nbOnly, "grove.toml"), "name = \"proj-nb-only-from-nb\"\n"); err != nil {
					return err
				}

				// proj-yml-nb: grove project with .yml notebook config
				if err := nbCreateGroveProject(filepath.Join(root, "proj-yml-nb"), "proj-yml-nb"); err != nil {
					return err
				}
				nbYml := filepath.Join(nbRoot, "workspaces", "proj-yml-nb")
				if err := os.MkdirAll(nbYml, 0o755); err != nil {
					return err
				}
				if err := fs.WriteString(filepath.Join(nbYml, "grove.yml"), "name: proj-yml-nb-from-nb\n"); err != nil {
					return err
				}

				// proj-yaml-nb: grove project with .yaml notebook config
				if err := nbCreateGroveProject(filepath.Join(root, "proj-yaml-nb"), "proj-yaml-nb"); err != nil {
					return err
				}
				nbYaml := filepath.Join(nbRoot, "workspaces", "proj-yaml-nb")
				if err := os.MkdirAll(nbYaml, 0o755); err != nil {
					return err
				}
				if err := fs.WriteString(filepath.Join(nbYaml, "grove.yaml"), "name: proj-yaml-nb-from-nb\n"); err != nil {
					return err
				}

				return nil
			}),
			harness.NewStep("Setup global config with depth + notebook", func(ctx *harness.Context) error {
				root := ctx.Get("grove_root").(string)
				nbRoot := ctx.Get("notebook_root").(string)
				return nbSetupGlobalConfig(ctx, fmt.Sprintf(`[groves.work]
path = %q
enabled = true
depth = 1
notebook = "nb"

[notebooks.definitions.nb]
root_dir = %q
`, root, nbRoot))
			}),
			harness.NewStep("Verify all projects discovered (including depth-promoted)", func(ctx *harness.Context) error {
				root := ctx.Get("grove_root").(string)
				projects, err := nbRunDiscovery(ctx, root)
				if err != nil {
					return err
				}
				return ctx.Verify(func(v *verify.Collector) {
					v.True("proj-local discovered", nbHasProject(projects, "proj-local"))
					v.True("proj-nb-only promoted via depth", nbHasProject(projects, "proj-nb-only"))
					v.True("proj-yml-nb discovered", nbHasProject(projects, "proj-yml-nb"))
					v.True("proj-yaml-nb discovered", nbHasProject(projects, "proj-yaml-nb"))
				})
			}),
			harness.NewStep("Verify notebook config files on disk", func(ctx *harness.Context) error {
				nbRoot := ctx.Get("notebook_root").(string)
				return ctx.Verify(func(v *verify.Collector) {
					v.Equal("TOML notebook exists", nil,
						fs.AssertExists(filepath.Join(nbRoot, "workspaces", "proj-local", "grove.toml")))
					v.Equal("TOML notebook for nb-only exists", nil,
						fs.AssertExists(filepath.Join(nbRoot, "workspaces", "proj-nb-only", "grove.toml")))
					v.Equal("YML notebook exists", nil,
						fs.AssertExists(filepath.Join(nbRoot, "workspaces", "proj-yml-nb", "grove.yml")))
					v.Equal("YAML notebook exists", nil,
						fs.AssertExists(filepath.Join(nbRoot, "workspaces", "proj-yaml-nb", "grove.yaml")))
				})
			}),
		},
	)
}

// NbTomlsFullFeatureFlowScenario validates the complete loop: configure depth, discover a naked
// git repo, bind its configuration from a centralized notebook, and verify exclusions.
func NbTomlsFullFeatureFlowScenario() *harness.Scenario {
	return harness.NewScenario(
		"nb-tomls-full-flow",
		"End-to-end: depth discovery + notebook config + exclude",
		[]string{"nb-tomls", "integration"},
		[]harness.Step{
			harness.NewStep("Create complete environment", func(ctx *harness.Context) error {
				root := nbResolveSymlinks(ctx.NewDir("grove-full"))
				nbRoot := nbResolveSymlinks(ctx.NewDir("nb-full"))
				ctx.Set("grove_root", root)
				ctx.Set("notebook_root", nbRoot)

				if err := nbCreateEcosystemDir(root, "full-eco"); err != nil {
					return err
				}
				if err := nbCreateGitRepo(root); err != nil {
					return err
				}

				// Grove project with local config
				if err := nbCreateGroveProject(filepath.Join(root, "configured-project"), "configured-project"); err != nil {
					return err
				}

				// Naked git repo - promoted by depth, config from notebook
				if err := nbCreateGitRepo(filepath.Join(root, "my-service")); err != nil {
					return err
				}
				nbSvc := filepath.Join(nbRoot, "workspaces", "my-service")
				if err := os.MkdirAll(nbSvc, 0o755); err != nil {
					return err
				}
				if err := fs.WriteString(filepath.Join(nbSvc, "grove.toml"), `name = "my-service-from-notebook"

[extensions]
managed_by = "notebook"
service_port = "8080"
`); err != nil {
					return err
				}

				// Excluded repo
				if err := nbCreateGitRepo(filepath.Join(root, "excluded-service")); err != nil {
					return err
				}

				// Deep repo (depth 2, should not be found with depth=1)
				if err := os.MkdirAll(filepath.Join(root, "libs"), 0o755); err != nil {
					return err
				}
				if err := nbCreateGitRepo(filepath.Join(root, "libs", "deep-lib")); err != nil {
					return err
				}

				return nil
			}),
			harness.NewStep("Setup global config", func(ctx *harness.Context) error {
				root := ctx.Get("grove_root").(string)
				nbRoot := ctx.Get("notebook_root").(string)
				return nbSetupGlobalConfig(ctx, fmt.Sprintf(`[groves.work]
path = %q
enabled = true
depth = 1
notebook = "nb"
exclude_repos = ["excluded-service"]

[notebooks.definitions.nb]
root_dir = %q
`, root, nbRoot))
			}),
			harness.NewStep("Verify discovery with all features", func(ctx *harness.Context) error {
				root := ctx.Get("grove_root").(string)
				projects, err := nbRunDiscovery(ctx, root)
				if err != nil {
					return err
				}
				return ctx.Verify(func(v *verify.Collector) {
					v.True("configured-project discovered", nbHasProject(projects, "configured-project"))
					v.True("my-service promoted via depth", nbHasProject(projects, "my-service"))
					v.True("excluded-service skipped", !nbHasProject(projects, "excluded-service"))
					v.True("deep-lib beyond depth=1", !nbHasProject(projects, "deep-lib"))
				})
			}),
			harness.NewStep("Verify notebook config on disk", func(ctx *harness.Context) error {
				nbRoot := ctx.Get("notebook_root").(string)
				nbPath := filepath.Join(nbRoot, "workspaces", "my-service", "grove.toml")
				return ctx.Verify(func(v *verify.Collector) {
					v.Equal("notebook config exists", nil, fs.AssertExists(nbPath))
					v.Equal("has notebook name", nil, fs.AssertContains(nbPath, "my-service-from-notebook"))
					v.Equal("has extensions", nil, fs.AssertContains(nbPath, "managed_by"))
				})
			}),
		},
	)
}
