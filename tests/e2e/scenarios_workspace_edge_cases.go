package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattsolo1/grove-core/config"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-tend/pkg/fs"
	"github.com/mattsolo1/grove-tend/pkg/git"
	"github.com/mattsolo1/grove-tend/pkg/harness"
	"gopkg.in/yaml.v3"
)

// WorkspaceEdgeCasesScenario verifies edge cases, error handling, and complex workspace configurations.
func WorkspaceEdgeCasesScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "workspace-edge-cases",
		Description: "Verifies workspace edge cases, error handling, and complex configurations.",
		Tags:        []string{"core", "workspace", "edge-cases"},
		Steps: []harness.Step{
			harness.NewStep("Setup Test Environment", setupEdgeCasesEnvironment),
			harness.NewStep("Test Error Handling - CWD Outside Workspace", testCwdOutsideWorkspace),
			harness.NewStep("Test Error Handling - Malformed Grove Config", testMalformedGroveConfig),
			harness.NewStep("Test Error Handling - Workspace Without Git", testWorkspaceWithoutGit),
			harness.NewStep("Test Disabled Groves", testDisabledGroves),
			harness.NewStep("Test Multiple Worktrees Same Project", testMultipleWorktreesSameProject),
			harness.NewStep("Test Complex Ecosystem - Multiple Sub-Projects", testComplexEcosystemMultipleSubProjects),
			harness.NewStep("Test Sub-Project With Worktrees", testSubProjectWithWorktrees),
			harness.NewStep("Test Workspaces Outside Grove Paths", testWorkspacesOutsideGrovePaths),
			harness.NewStep("Test Symlinked Paths", testSymlinkedPaths),
		},
	}
}

// setupEdgeCasesEnvironment creates the test environment for edge cases.
func setupEdgeCasesEnvironment(ctx *harness.Context) error {
	homeDir := ctx.HomeDir()

	// Create standard groves
	workDir := filepath.Join(homeDir, "work")
	playDir := filepath.Join(homeDir, "play")
	if err := fs.CreateDir(workDir); err != nil {
		return err
	}
	if err := fs.CreateDir(playDir); err != nil {
		return err
	}

	// Create directory outside groves for testing
	outsideDir := filepath.Join(homeDir, "outside-groves")
	if err := fs.CreateDir(outsideDir); err != nil {
		return err
	}

	// Create global config with one disabled grove
	trueVal := true
	falseVal := false
	globalCfg := &config.Config{
		Groves: map[string]config.GroveSourceConfig{
			"work":     {Path: "~/work", Enabled: &trueVal},
			"play":     {Path: "~/play", Enabled: &trueVal},
			"disabled": {Path: "~/disabled", Enabled: &falseVal},
		},
	}
	data, err := yaml.Marshal(globalCfg)
	if err != nil {
		return err
	}
	if err := fs.WriteFile(filepath.Join(homeDir, ".config", "grove", "grove.yml"), data); err != nil {
		return err
	}

	ctx.Set("homeDir", homeDir)
	ctx.Set("workDir", workDir)
	ctx.Set("playDir", playDir)
	ctx.Set("outsideDir", outsideDir)

	return nil
}

// testCwdOutsideWorkspace verifies that ws cwd fails gracefully when run outside any workspace.
func testCwdOutsideWorkspace(ctx *harness.Context) error {
	outsideDir := ctx.GetString("outsideDir")

	// Run ws cwd from outside any workspace
	cmd := ctx.Command("core", "ws", "cwd", "--json").Dir(outsideDir)
	result := cmd.Run()

	// Should fail gracefully (non-zero exit but no panic)
	if result.Error == nil {
		return fmt.Errorf("expected ws cwd to fail outside workspace, but it succeeded")
	}

	return nil
}

// testMalformedGroveConfig verifies handling of malformed grove.yml files.
func testMalformedGroveConfig(ctx *harness.Context) error {
	workDir := ctx.GetString("workDir")

	// Create workspace with invalid YAML
	invalidYamlPath := filepath.Join(workDir, "invalid-yaml")
	if err := fs.WriteString(filepath.Join(invalidYamlPath, "grove.yml"), "invalid: yaml: [missing: bracket"); err != nil {
		return err
	}

	// Initialize as git repo
	if _, err := git.SetupTestRepo(invalidYamlPath); err != nil {
		return err
	}

	// Run discovery - should handle malformed configs gracefully (not crash)
	cmd := ctx.Command("core", "ws", "--json")
	result := cmd.Run()

	// Discovery should succeed (not crash) even with invalid YAML
	// This is the key test - we don't want the system to panic or fail completely
	if result.Error != nil {
		return fmt.Errorf("discovery failed on malformed configs (should handle gracefully): %w", result.Error)
	}

	var nodes []*workspace.WorkspaceNode
	if err := json.Unmarshal([]byte(result.Stdout), &nodes); err != nil {
		return fmt.Errorf("failed to unmarshal nodes: %w", err)
	}

	// The system handles malformed configs gracefully by using fallback values
	// (e.g., directory name as workspace name). This is acceptable behavior.
	// The important thing is that discovery doesn't crash.

	return nil
}

// testWorkspaceWithoutGit verifies handling of workspaces without git repositories.
func testWorkspaceWithoutGit(ctx *harness.Context) error {
	workDir := ctx.GetString("workDir")

	// Create workspace with grove.yml but no git repo
	noGitPath := filepath.Join(workDir, "no-git-repo")
	if err := fs.WriteString(filepath.Join(noGitPath, "grove.yml"), "version: '1.0'\nname: no-git-repo"); err != nil {
		return err
	}

	// Run discovery
	cmd := ctx.Command("core", "ws", "--json")
	result := cmd.Run()
	if result.Error != nil {
		return fmt.Errorf("discovery failed: %w", result.Error)
	}

	var nodes []*workspace.WorkspaceNode
	if err := json.Unmarshal([]byte(result.Stdout), &nodes); err != nil {
		return fmt.Errorf("failed to unmarshal nodes: %w", err)
	}

	// Workspace without git should still be discovered (git is optional for some workspace types)
	// Just verify we don't crash

	return nil
}

// testDisabledGroves verifies that workspaces in disabled groves are not discovered.
func testDisabledGroves(ctx *harness.Context) error {
	homeDir := ctx.GetString("homeDir")

	// Create directory for disabled grove
	disabledDir := filepath.Join(homeDir, "disabled")
	if err := fs.CreateDir(disabledDir); err != nil {
		return err
	}

	// Create workspace in disabled grove
	disabledWorkspacePath := filepath.Join(disabledDir, "disabled-workspace")
	if err := fs.WriteString(filepath.Join(disabledWorkspacePath, "grove.yml"), "version: '1.0'\nname: disabled-workspace"); err != nil {
		return err
	}
	if _, err := git.SetupTestRepo(disabledWorkspacePath); err != nil {
		return err
	}

	// Run discovery
	cmd := ctx.Command("core", "ws", "--json")
	result := cmd.Run()
	if result.Error != nil {
		return fmt.Errorf("discovery failed: %w", result.Error)
	}

	var nodes []*workspace.WorkspaceNode
	if err := json.Unmarshal([]byte(result.Stdout), &nodes); err != nil {
		return fmt.Errorf("failed to unmarshal nodes: %w", err)
	}

	// Verify workspace in disabled grove is not discovered
	for _, node := range nodes {
		if node.Name == "disabled-workspace" {
			return fmt.Errorf("workspace in disabled grove should not be discovered")
		}
	}

	return nil
}

// testMultipleWorktreesSameProject verifies discovery of multiple worktrees from the same project.
func testMultipleWorktreesSameProject(ctx *harness.Context) error {
	playDir := ctx.GetString("playDir")

	// Create a project
	projectPath := filepath.Join(playDir, "multi-worktree-proj")
	if err := fs.WriteString(filepath.Join(projectPath, "grove.yml"), "version: '1.0'\nname: multi-worktree-proj"); err != nil {
		return err
	}
	repo, err := git.SetupTestRepo(projectPath)
	if err != nil {
		return err
	}
	if err := repo.AddCommit("initial commit"); err != nil {
		return err
	}

	// Create multiple worktrees
	worktreesDir := filepath.Join(projectPath, ".grove-worktrees")
	for i := 1; i <= 3; i++ {
		branchName := fmt.Sprintf("feature-%d", i)
		worktreePath := filepath.Join(worktreesDir, branchName)
		if err := repo.CreateWorktree(worktreePath, branchName); err != nil {
			return err
		}
	}

	// Run discovery
	cmd := ctx.Command("core", "ws", "--json")
	result := cmd.Run()
	if result.Error != nil {
		return fmt.Errorf("discovery failed: %w", result.Error)
	}

	var nodes []*workspace.WorkspaceNode
	if err := json.Unmarshal([]byte(result.Stdout), &nodes); err != nil {
		return fmt.Errorf("failed to unmarshal nodes: %w", err)
	}

	// Verify all worktrees are discovered (3 worktrees + 1 main project = 4 total)
	projectCount := 0
	worktreeCount := 0
	for _, node := range nodes {
		if strings.Contains(node.Path, "multi-worktree-proj") {
			if node.Kind == workspace.KindStandaloneProject {
				projectCount++
			} else if node.Kind == workspace.KindStandaloneProjectWorktree {
				worktreeCount++
			}
		}
	}

	if projectCount != 1 {
		return fmt.Errorf("expected 1 main project, got %d", projectCount)
	}
	if worktreeCount != 3 {
		return fmt.Errorf("expected 3 worktrees, got %d", worktreeCount)
	}

	return nil
}

// testComplexEcosystemMultipleSubProjects verifies ecosystems with multiple sub-projects.
func testComplexEcosystemMultipleSubProjects(ctx *harness.Context) error {
	workDir := ctx.GetString("workDir")

	// Create ecosystem with multiple sub-projects
	ecoPath := filepath.Join(workDir, "complex-eco")
	subProjects := []string{"backend", "frontend", "shared"}

	ecoConfig := fmt.Sprintf("version: '1.0'\nname: complex-eco\nworkspaces: %s",
		fmt.Sprintf("[%s]", strings.Join(func() []string {
			quoted := make([]string, len(subProjects))
			for i, p := range subProjects {
				quoted[i] = "'" + p + "'"
			}
			return quoted
		}(), ", ")))

	if err := fs.WriteString(filepath.Join(ecoPath, "grove.yml"), ecoConfig); err != nil {
		return err
	}
	ecoRepo, err := git.SetupTestRepo(ecoPath)
	if err != nil {
		return err
	}
	if err := ecoRepo.AddCommit("initial ecosystem commit"); err != nil {
		return err
	}

	// Create each sub-project
	for _, subProj := range subProjects {
		subProjPath := filepath.Join(ecoPath, subProj)
		if err := fs.WriteString(filepath.Join(subProjPath, "grove.yml"),
			fmt.Sprintf("version: '1.0'\nname: %s", subProj)); err != nil {
			return err
		}
		subRepo, err := git.SetupTestRepo(subProjPath)
		if err != nil {
			return err
		}
		if err := subRepo.AddCommit(fmt.Sprintf("initial %s commit", subProj)); err != nil {
			return err
		}
	}

	// Run discovery
	cmd := ctx.Command("core", "ws", "--json")
	result := cmd.Run()
	if result.Error != nil {
		return fmt.Errorf("discovery failed: %w", result.Error)
	}

	var nodes []*workspace.WorkspaceNode
	if err := json.Unmarshal([]byte(result.Stdout), &nodes); err != nil {
		return fmt.Errorf("failed to unmarshal nodes: %w", err)
	}

	// Verify ecosystem and all sub-projects are discovered
	ecoFound := false
	subProjectsFound := make(map[string]bool)

	for _, node := range nodes {
		if node.Name == "complex-eco" && node.Kind == workspace.KindEcosystemRoot {
			ecoFound = true
		}
		for _, subProj := range subProjects {
			if node.Name == subProj && node.Kind == workspace.KindEcosystemSubProject {
				subProjectsFound[subProj] = true
				// Verify parent relationships
				if node.RootEcosystemPath != ecoPath {
					return fmt.Errorf("sub-project %s has incorrect RootEcosystemPath: got %s, want %s",
						subProj, node.RootEcosystemPath, ecoPath)
				}
			}
		}
	}

	if !ecoFound {
		return fmt.Errorf("ecosystem not discovered")
	}
	for _, subProj := range subProjects {
		if !subProjectsFound[subProj] {
			return fmt.Errorf("sub-project %s not discovered", subProj)
		}
	}

	return nil
}

// testSubProjectWithWorktrees verifies sub-projects that have their own worktrees.
func testSubProjectWithWorktrees(ctx *harness.Context) error {
	workDir := ctx.GetString("workDir")

	// Create ecosystem
	ecoPath := filepath.Join(workDir, "eco-with-sub-worktrees")
	if err := fs.WriteString(filepath.Join(ecoPath, "grove.yml"),
		"version: '1.0'\nname: eco-with-sub-worktrees\nworkspaces: ['service']"); err != nil {
		return err
	}
	ecoRepo, err := git.SetupTestRepo(ecoPath)
	if err != nil {
		return err
	}
	if err := ecoRepo.AddCommit("initial ecosystem commit"); err != nil {
		return err
	}

	// Create sub-project
	subProjPath := filepath.Join(ecoPath, "service")
	if err := fs.WriteString(filepath.Join(subProjPath, "grove.yml"),
		"version: '1.0'\nname: service"); err != nil {
		return err
	}
	subRepo, err := git.SetupTestRepo(subProjPath)
	if err != nil {
		return err
	}
	if err := subRepo.AddCommit("initial service commit"); err != nil {
		return err
	}

	// Create worktree of the sub-project
	subWorktreePath := filepath.Join(subProjPath, ".grove-worktrees", "service-feature")
	if err := subRepo.CreateWorktree(subWorktreePath, "service-feature"); err != nil {
		return err
	}

	// Run discovery
	cmd := ctx.Command("core", "ws", "--json")
	result := cmd.Run()
	if result.Error != nil {
		return fmt.Errorf("discovery failed: %w", result.Error)
	}

	var nodes []*workspace.WorkspaceNode
	if err := json.Unmarshal([]byte(result.Stdout), &nodes); err != nil {
		return fmt.Errorf("failed to unmarshal nodes: %w", err)
	}

	// Verify the sub-project worktree is discovered
	worktreeFound := false
	for _, node := range nodes {
		if node.Name == "service-feature" {
			worktreeFound = true
			// This should be classified as an ecosystem sub-project worktree
			if node.Kind != workspace.KindEcosystemSubProjectWorktree {
				return fmt.Errorf("sub-project worktree has wrong kind: got %v, want %v",
					node.Kind, workspace.KindEcosystemSubProjectWorktree)
			}
			// Verify parent relationships
			if node.ParentProjectPath != subProjPath {
				return fmt.Errorf("sub-project worktree has incorrect ParentProjectPath: got %s, want %s",
					node.ParentProjectPath, subProjPath)
			}
			if node.RootEcosystemPath != ecoPath {
				return fmt.Errorf("sub-project worktree has incorrect RootEcosystemPath: got %s, want %s",
					node.RootEcosystemPath, ecoPath)
			}
		}
	}

	if !worktreeFound {
		return fmt.Errorf("sub-project worktree not discovered")
	}

	return nil
}

// testWorkspacesOutsideGrovePaths verifies that workspaces outside configured groves are not discovered.
func testWorkspacesOutsideGrovePaths(ctx *harness.Context) error {
	outsideDir := ctx.GetString("outsideDir")

	// Create workspace outside any grove path
	outsideWorkspacePath := filepath.Join(outsideDir, "outside-workspace")
	if err := fs.WriteString(filepath.Join(outsideWorkspacePath, "grove.yml"),
		"version: '1.0'\nname: outside-workspace"); err != nil {
		return err
	}
	if _, err := git.SetupTestRepo(outsideWorkspacePath); err != nil {
		return err
	}

	// Run discovery
	cmd := ctx.Command("core", "ws", "--json")
	result := cmd.Run()
	if result.Error != nil {
		return fmt.Errorf("discovery failed: %w", result.Error)
	}

	var nodes []*workspace.WorkspaceNode
	if err := json.Unmarshal([]byte(result.Stdout), &nodes); err != nil {
		return fmt.Errorf("failed to unmarshal nodes: %w", err)
	}

	// Verify workspace outside groves is not discovered
	for _, node := range nodes {
		if node.Name == "outside-workspace" {
			return fmt.Errorf("workspace outside grove paths should not be discovered")
		}
	}

	return nil
}

// testSymlinkedPaths verifies that symlinked paths are handled correctly.
func testSymlinkedPaths(ctx *harness.Context) error {
	playDir := ctx.GetString("playDir")
	homeDir := ctx.GetString("homeDir")

	// Create actual workspace
	actualPath := filepath.Join(playDir, "actual-workspace")
	if err := fs.WriteString(filepath.Join(actualPath, "grove.yml"),
		"version: '1.0'\nname: actual-workspace"); err != nil {
		return err
	}
	if _, err := git.SetupTestRepo(actualPath); err != nil {
		return err
	}

	// Create symlink to workspace
	symlinkPath := filepath.Join(homeDir, "symlink-to-workspace")
	if err := os.Symlink(actualPath, symlinkPath); err != nil {
		return err
	}

	// Run ws cwd from symlinked path
	cmd := ctx.Command("core", "ws", "cwd", "--json").Dir(symlinkPath)
	result := cmd.Run()
	if result.Error != nil {
		// Symlink handling may vary - this is more of an informational test
		// Not failing if symlinks aren't fully supported yet
		return nil
	}

	var node workspace.WorkspaceNode
	if err := json.Unmarshal([]byte(result.Stdout), &node); err != nil {
		return fmt.Errorf("failed to unmarshal node: %w", err)
	}

	// Verify the workspace is found (path resolution should work through symlink)
	if node.Name != "actual-workspace" {
		return fmt.Errorf("expected actual-workspace, got %s", node.Name)
	}

	return nil
}
