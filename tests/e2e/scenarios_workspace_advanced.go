package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/tend/pkg/fs"
	"github.com/grovetools/tend/pkg/git"
	"github.com/grovetools/tend/pkg/harness"
	"gopkg.in/yaml.v3"
)

// WorkspaceAdvancedScenario verifies advanced workspace behaviors and potential issues.
func WorkspaceAdvancedScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "workspace-advanced",
		Description: "Verifies advanced workspace behaviors: name conflicts, orphans, validation, and overlaps.",
		Tags:        []string{"core", "workspace", "advanced"},
		Steps: []harness.Step{
			harness.NewStep("Setup Test Environment", setupAdvancedEnvironment),
			harness.NewStep("Test Duplicate Workspace Names Across Groves", testDuplicateWorkspaceNames),
			harness.NewStep("Test Orphaned Worktrees", testOrphanedWorktrees),
			harness.NewStep("Test Ecosystem Workspace List Validation", testEcosystemWorkspaceValidation),
			harness.NewStep("Test Grove Path Overlaps", testGrovePathOverlaps),
		},
	}
}

// setupAdvancedEnvironment creates the test environment for advanced tests.
func setupAdvancedEnvironment(ctx *harness.Context) error {
	homeDir := ctx.HomeDir()

	// Create standard groves
	workDir := filepath.Join(homeDir, "work")
	playDir := filepath.Join(homeDir, "play")
	codeDir := filepath.Join(homeDir, "code")
	codeWorkDir := filepath.Join(homeDir, "code", "work") // Nested for overlap test

	for _, dir := range []string{workDir, playDir, codeDir, codeWorkDir} {
		if err := fs.CreateDir(dir); err != nil {
			return err
		}
	}

	// Create global config with overlapping groves
	trueVal := true
	globalCfg := &config.Config{
		Groves: map[string]config.GroveSourceConfig{
			"work":      {Path: "~/work", Enabled: &trueVal},
			"play":      {Path: "~/play", Enabled: &trueVal},
			"code":      {Path: "~/code", Enabled: &trueVal},
			"code-work": {Path: "~/code/work", Enabled: &trueVal}, // Overlaps with "code"
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
	ctx.Set("codeDir", codeDir)
	ctx.Set("codeWorkDir", codeWorkDir)

	return nil
}

// testDuplicateWorkspaceNames verifies behavior when multiple workspaces have the same name.
func testDuplicateWorkspaceNames(ctx *harness.Context) error {
	workDir := ctx.GetString("workDir")
	playDir := ctx.GetString("playDir")

	// Create two workspaces with the same name in different groves
	workMyAppPath := filepath.Join(workDir, "my-app")
	playMyAppPath := filepath.Join(playDir, "my-app")

	// Create work/my-app as an ecosystem with a sub-project
	if err := fs.WriteString(filepath.Join(workMyAppPath, "grove.yml"),
		"version: '1.0'\nname: my-app\nworkspaces: ['sub']"); err != nil {
		return err
	}
	workRepo, err := git.SetupTestRepo(workMyAppPath)
	if err != nil {
		return err
	}
	if err := workRepo.AddCommit("initial commit"); err != nil {
		return err
	}

	// Create the sub-project to make it a valid ecosystem
	subPath := filepath.Join(workMyAppPath, "sub")
	if err := fs.WriteString(filepath.Join(subPath, "grove.yml"),
		"version: '1.0'\nname: sub"); err != nil {
		return err
	}
	if _, err := git.SetupTestRepo(subPath); err != nil {
		return err
	}

	// Create play/my-app as a standalone project
	if err := fs.WriteString(filepath.Join(playMyAppPath, "grove.yml"),
		"version: '1.0'\nname: my-app"); err != nil {
		return err
	}
	if _, err := git.SetupTestRepo(playMyAppPath); err != nil {
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

	// Verify both workspaces are discovered
	myAppCount := 0
	ecosystemFound := false
	standaloneFound := false

	for _, node := range nodes {
		if node.Name == "my-app" {
			myAppCount++
			if node.Kind == workspace.KindEcosystemRoot {
				ecosystemFound = true
			} else if node.Kind == workspace.KindStandaloneProject {
				standaloneFound = true
			}
		}
	}

	if myAppCount != 2 {
		return fmt.Errorf("expected 2 workspaces named 'my-app', got %d", myAppCount)
	}
	if !ecosystemFound || !standaloneFound {
		return fmt.Errorf("expected both ecosystem and standalone 'my-app' workspaces")
	}

	// Test lookup behavior from work/my-app (should find the correct one)
	cwdCmd := ctx.Command("core", "ws", "cwd", "--json").Dir(workMyAppPath)
	cwdResult := cwdCmd.Run()
	if cwdResult.Error != nil {
		return fmt.Errorf("ws cwd failed: %w", cwdResult.Error)
	}

	var cwdNode workspace.WorkspaceNode
	if err := json.Unmarshal([]byte(cwdResult.Stdout), &cwdNode); err != nil {
		return fmt.Errorf("failed to unmarshal cwd node: %w", err)
	}

	// Should find the ecosystem version when running from work/my-app
	if cwdNode.Kind != workspace.KindEcosystemRoot {
		return fmt.Errorf("expected to find ecosystem when running from work/my-app, got %v", cwdNode.Kind)
	}
	// Compare paths after resolving symlinks and normalizing case (macOS /var -> /private/var, T -> t)
	expectedPath, _ := filepath.EvalSymlinks(workMyAppPath)
	actualPath, _ := filepath.EvalSymlinks(cwdNode.Path)
	if !strings.EqualFold(actualPath, expectedPath) {
		return fmt.Errorf("expected path %s, got %s", expectedPath, actualPath)
	}

	// Test lookup from play/my-app (should find the standalone one)
	cwdCmd2 := ctx.Command("core", "ws", "cwd", "--json").Dir(playMyAppPath)
	cwdResult2 := cwdCmd2.Run()
	if cwdResult2.Error != nil {
		return fmt.Errorf("ws cwd failed: %w", cwdResult2.Error)
	}

	var cwdNode2 workspace.WorkspaceNode
	if err := json.Unmarshal([]byte(cwdResult2.Stdout), &cwdNode2); err != nil {
		return fmt.Errorf("failed to unmarshal cwd node: %w", err)
	}

	if cwdNode2.Kind != workspace.KindStandaloneProject {
		return fmt.Errorf("expected to find standalone when running from play/my-app, got %v", cwdNode2.Kind)
	}
	expectedPath2, _ := filepath.EvalSymlinks(playMyAppPath)
	actualPath2, _ := filepath.EvalSymlinks(cwdNode2.Path)
	if !strings.EqualFold(actualPath2, expectedPath2) {
		return fmt.Errorf("expected path %s, got %s", expectedPath2, actualPath2)
	}

	return nil
}

// testOrphanedWorktrees verifies handling of worktrees whose parent project has been deleted.
func testOrphanedWorktrees(ctx *harness.Context) error {
	playDir := ctx.GetString("playDir")

	// Create a project with a worktree
	projectPath := filepath.Join(playDir, "temp-project")
	if err := fs.WriteString(filepath.Join(projectPath, "grove.yml"),
		"version: '1.0'\nname: temp-project"); err != nil {
		return err
	}
	repo, err := git.SetupTestRepo(projectPath)
	if err != nil {
		return err
	}
	if err := repo.AddCommit("initial commit"); err != nil {
		return err
	}

	// Create a worktree
	worktreePath := filepath.Join(projectPath, ".grove-worktrees", "feature")
	if err := repo.CreateWorktree(worktreePath, "feature"); err != nil {
		return err
	}

	// Verify both are discovered
	cmd := ctx.Command("core", "ws", "--json")
	result := cmd.Run()
	if result.Error != nil {
		return fmt.Errorf("initial discovery failed: %w", result.Error)
	}

	var nodes []*workspace.WorkspaceNode
	if err := json.Unmarshal([]byte(result.Stdout), &nodes); err != nil {
		return fmt.Errorf("failed to unmarshal nodes: %w", err)
	}

	tempProjectFound := false
	worktreeFound := false
	for _, node := range nodes {
		if node.Name == "temp-project" {
			tempProjectFound = true
		}
		if node.Name == "feature" {
			worktreeFound = true
		}
	}

	if !tempProjectFound || !worktreeFound {
		return fmt.Errorf("expected both project and worktree to be found initially")
	}

	// Delete the main project directory (but leave the worktree)
	// We need to delete the .git directory and grove.yml, but leave .grove-worktrees
	if err := os.Remove(filepath.Join(projectPath, "grove.yml")); err != nil {
		return err
	}
	gitDir := filepath.Join(projectPath, ".git")
	if err := os.RemoveAll(gitDir); err != nil {
		return err
	}

	// Run discovery again
	cmd2 := ctx.Command("core", "ws", "--json")
	result2 := cmd2.Run()

	// Discovery should succeed (not crash) even with orphaned worktree
	if result2.Error != nil {
		return fmt.Errorf("discovery failed with orphaned worktree (should handle gracefully): %w", result2.Error)
	}

	var nodes2 []*workspace.WorkspaceNode
	if err := json.Unmarshal([]byte(result2.Stdout), &nodes2); err != nil {
		return fmt.Errorf("failed to unmarshal nodes: %w", err)
	}

	// The orphaned worktree should either:
	// 1. Not be discovered (acceptable - parent is gone)
	// 2. Be discovered but marked somehow (also acceptable)
	// The important thing is that discovery doesn't crash

	orphanedWorktreeFound := false
	for _, node := range nodes2 {
		if node.Name == "feature" || node.Path == worktreePath {
			orphanedWorktreeFound = true
		}
	}

	// Log behavior for documentation purposes
	if orphanedWorktreeFound {
		// System discovers orphaned worktrees - this is fine as long as it doesn't crash
		ctx.Set("orphaned_worktree_behavior", "discovered")
	} else {
		// System filters out orphaned worktrees - also fine
		ctx.Set("orphaned_worktree_behavior", "filtered")
	}

	return nil
}

// testEcosystemWorkspaceValidation verifies handling of invalid workspace declarations in ecosystems.
func testEcosystemWorkspaceValidation(ctx *harness.Context) error {
	workDir := ctx.GetString("workDir")

	// Create ecosystem with various invalid workspace declarations
	ecoPath := filepath.Join(workDir, "validation-eco")
	ecoConfig := `version: '1.0'
name: validation-eco
workspaces:
  - 'valid-workspace'
  - 'does-not-exist'
  - '../outside-ecosystem'
  - '/absolute/path'
`
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

	// Create only the valid workspace
	validWorkspacePath := filepath.Join(ecoPath, "valid-workspace")
	if err := fs.WriteString(filepath.Join(validWorkspacePath, "grove.yml"),
		"version: '1.0'\nname: valid-workspace"); err != nil {
		return err
	}
	if _, err := git.SetupTestRepo(validWorkspacePath); err != nil {
		return err
	}

	// Run discovery - should handle invalid declarations gracefully
	cmd := ctx.Command("core", "ws", "--json")
	result := cmd.Run()

	// Discovery should succeed (not crash)
	if result.Error != nil {
		return fmt.Errorf("discovery failed on invalid workspace declarations (should handle gracefully): %w", result.Error)
	}

	var nodes []*workspace.WorkspaceNode
	if err := json.Unmarshal([]byte(result.Stdout), &nodes); err != nil {
		return fmt.Errorf("failed to unmarshal nodes: %w", err)
	}

	// Verify ecosystem is discovered
	ecoFound := false
	validWorkspaceFound := false
	invalidWorkspacesFound := 0

	for _, node := range nodes {
		if node.Name == "validation-eco" {
			ecoFound = true
		}
		if node.Name == "valid-workspace" {
			validWorkspaceFound = true
		}
		// Check if any invalid workspaces were somehow discovered
		if node.Name == "does-not-exist" || node.Name == "outside-ecosystem" || node.Name == "path" {
			invalidWorkspacesFound++
		}
	}

	if !ecoFound {
		return fmt.Errorf("ecosystem should be discovered")
	}
	if !validWorkspaceFound {
		return fmt.Errorf("valid workspace should be discovered")
	}
	if invalidWorkspacesFound > 0 {
		return fmt.Errorf("invalid workspace declarations should not be discovered")
	}

	return nil
}

// testGrovePathOverlaps verifies behavior when grove paths overlap/nest.
func testGrovePathOverlaps(ctx *harness.Context) error {
	codeDir := ctx.GetString("codeDir")
	codeWorkDir := ctx.GetString("codeWorkDir")

	// Create a workspace in the parent grove (code)
	parentWorkspacePath := filepath.Join(codeDir, "parent-workspace")
	if err := fs.WriteString(filepath.Join(parentWorkspacePath, "grove.yml"),
		"version: '1.0'\nname: parent-workspace"); err != nil {
		return err
	}
	if _, err := git.SetupTestRepo(parentWorkspacePath); err != nil {
		return err
	}

	// Create a workspace in the child grove (code/work)
	childWorkspacePath := filepath.Join(codeWorkDir, "child-workspace")
	if err := fs.WriteString(filepath.Join(childWorkspacePath, "grove.yml"),
		"version: '1.0'\nname: child-workspace"); err != nil {
		return err
	}
	if _, err := git.SetupTestRepo(childWorkspacePath); err != nil {
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

	// Both workspaces should be discovered
	parentFound := false
	childFound := false
	duplicateChild := false

	childCount := 0
	for _, node := range nodes {
		if node.Name == "parent-workspace" {
			parentFound = true
		}
		if node.Name == "child-workspace" {
			childCount++
			childFound = true
		}
	}

	if childCount > 1 {
		duplicateChild = true
	}

	if !parentFound {
		return fmt.Errorf("parent workspace should be discovered in 'code' grove")
	}
	if !childFound {
		return fmt.Errorf("child workspace should be discovered")
	}

	// The child workspace should only appear once, even though it's technically
	// in both the 'code' and 'code-work' grove paths
	if duplicateChild {
		return fmt.Errorf("child workspace should not be discovered twice due to overlapping groves")
	}

	// Verify lookup works correctly from both locations
	parentCwdCmd := ctx.Command("core", "ws", "cwd", "--json").Dir(parentWorkspacePath)
	parentResult := parentCwdCmd.Run()
	if parentResult.Error != nil {
		return fmt.Errorf("ws cwd failed for parent: %w", parentResult.Error)
	}

	var parentNode workspace.WorkspaceNode
	if err := json.Unmarshal([]byte(parentResult.Stdout), &parentNode); err != nil {
		return fmt.Errorf("failed to unmarshal parent node: %w", err)
	}

	if parentNode.Name != "parent-workspace" {
		return fmt.Errorf("expected parent-workspace, got %s", parentNode.Name)
	}

	childCwdCmd := ctx.Command("core", "ws", "cwd", "--json").Dir(childWorkspacePath)
	childResult := childCwdCmd.Run()
	if childResult.Error != nil {
		return fmt.Errorf("ws cwd failed for child: %w", childResult.Error)
	}

	var childNode workspace.WorkspaceNode
	if err := json.Unmarshal([]byte(childResult.Stdout), &childNode); err != nil {
		return fmt.Errorf("failed to unmarshal child node: %w", err)
	}

	if childNode.Name != "child-workspace" {
		return fmt.Errorf("expected child-workspace, got %s", childNode.Name)
	}

	return nil
}
