package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/tend/pkg/assert"
	"github.com/grovetools/tend/pkg/fs"
	"github.com/grovetools/tend/pkg/git"
	"github.com/grovetools/tend/pkg/harness"
	"gopkg.in/yaml.v3"
)

// WorkspaceModelScenario verifies the workspace discovery, classification, and lookup logic.
func WorkspaceModelScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "workspace-model-verification",
		Description: "Verifies the workspace discovery, classification, and lookup logic.",
		Tags:        []string{"core", "workspace"},
		Steps: []harness.Step{
			harness.NewStep("Setup Test Environment", setupTestEnvironment),
			harness.NewStep("Create Workspace Filesystem Structure", createWorkspaceFilesystemStructure),
			harness.NewStep("Run Discovery and Verify Structure", runDiscoveryAndVerifyStructure),
			harness.NewStep("Verify Lookups by Path", verifyLookupsByPath),
		},
	}
}

// setupTestEnvironment creates a sandboxed home directory and global grove.yml.
func setupTestEnvironment(ctx *harness.Context) error {
	homeDir := ctx.HomeDir()
	workDir := filepath.Join(homeDir, "work")
	playDir := filepath.Join(homeDir, "play")

	if err := fs.CreateDir(workDir); err != nil {
		return err
	}
	if err := fs.CreateDir(playDir); err != nil {
		return err
	}

	trueVal := true
	globalCfg := &config.Config{
		Groves: map[string]config.GroveSourceConfig{
			"work": {Path: "~/work", Enabled: &trueVal},
			"play": {Path: "~/play", Enabled: &trueVal},
		},
	}
	data, err := yaml.Marshal(globalCfg)
	if err != nil {
		return err
	}

	return fs.WriteFile(filepath.Join(homeDir, ".config", "grove", "grove.yml"), data)
}

// createWorkspaceFilesystemStructure builds a complex directory layout with different workspace kinds.
func createWorkspaceFilesystemStructure(ctx *harness.Context) error {
	homeDir := ctx.HomeDir()

	// --- 1. Create Ecosystem in ~/work ---
	workEcoPath := filepath.Join(homeDir, "work", "work-eco")
	subProj1Path := filepath.Join(workEcoPath, "sub-proj-1")
	if err := fs.WriteString(filepath.Join(workEcoPath, "grove.yml"), "version: '1.0'\nname: work-eco\nworkspaces: ['sub-proj-1']"); err != nil {
		return err
	}
	workEcoRepo, err := git.SetupTestRepo(workEcoPath)
	if err != nil {
		return err
	}
	if err := workEcoRepo.AddCommit("initial ecosystem commit"); err != nil {
		return err
	}

	// --- 2. Create Sub-Project in Ecosystem ---
	if err := fs.WriteString(filepath.Join(subProj1Path, "grove.yml"), "version: '1.0'\nname: sub-proj-1"); err != nil {
		return err
	}
	subProj1Repo, err := git.SetupTestRepo(subProj1Path)
	if err != nil {
		return err
	}
	if err := subProj1Repo.AddCommit("initial sub-project commit"); err != nil {
		return err
	}

	// --- 3. Create Standalone Project in ~/play ---
	playProjPath := filepath.Join(homeDir, "play", "play-proj")
	if err := fs.WriteString(filepath.Join(playProjPath, "grove.yml"), "version: '1.0'\nname: play-proj"); err != nil {
		return err
	}
	playProjRepo, err := git.SetupTestRepo(playProjPath)
	if err != nil {
		return err
	}
	if err := playProjRepo.AddCommit("initial standalone commit"); err != nil {
		return err
	}

	// --- 4. Create Worktrees ---
	// Standalone worktree
	if err := playProjRepo.CreateWorktree(filepath.Join(playProjPath, ".grove-worktrees", "feature-a"), "feature-a"); err != nil {
		return err
	}
	// Ecosystem worktree
	ecoFeaturePath := filepath.Join(workEcoPath, ".grove-worktrees", "eco-feature")
	if err := workEcoRepo.CreateWorktree(ecoFeaturePath, "eco-feature"); err != nil {
		return err
	}
	// Linked development worktree
	linkedSubProjPath := filepath.Join(ecoFeaturePath, "sub-proj-1")
	if err := subProj1Repo.CreateWorktree(linkedSubProjPath, "linked-dev-branch"); err != nil {
		return err
	}

	// Store paths for verification steps
	ctx.Set("workEcoPath", workEcoPath)
	ctx.Set("subProj1Path", subProj1Path)
	ctx.Set("playProjPath", playProjPath)
	ctx.Set("featureAPath", filepath.Join(playProjPath, ".grove-worktrees", "feature-a"))
	ctx.Set("ecoFeaturePath", ecoFeaturePath)
	ctx.Set("linkedSubProjPath", linkedSubProjPath)

	return nil
}

// runDiscoveryAndVerifyStructure runs `core ws --json` and validates the output.
func runDiscoveryAndVerifyStructure(ctx *harness.Context) error {
	cmd := ctx.Command("core", "ws", "--json")
	result := cmd.Run()
	if result.Error != nil {
		return fmt.Errorf("`core ws --json` failed: %w\nOutput:\n%s", result.Error, result.Stdout)
	}

	var nodes []*workspace.WorkspaceNode
	if err := json.Unmarshal([]byte(result.Stdout), &nodes); err != nil {
		return fmt.Errorf("failed to unmarshal workspace nodes: %w", err)
	}

	// Verification calls
	if err := verifyNode(nodes, "work-eco", workspace.KindEcosystemRoot, "", "", ctx.GetString("workEcoPath")); err != nil {
		return err
	}
	if err := verifyNode(nodes, "sub-proj-1", workspace.KindEcosystemSubProject, "", ctx.GetString("workEcoPath"), ctx.GetString("workEcoPath")); err != nil {
		return err
	}
	if err := verifyNode(nodes, "play-proj", workspace.KindStandaloneProject, "", "", ""); err != nil {
		return err
	}
	if err := verifyNode(nodes, "feature-a", workspace.KindStandaloneProjectWorktree, ctx.GetString("playProjPath"), "", ""); err != nil {
		return err
	}
	if err := verifyNode(nodes, "eco-feature", workspace.KindEcosystemWorktree, ctx.GetString("workEcoPath"), ctx.GetString("workEcoPath"), ctx.GetString("workEcoPath")); err != nil {
		return err
	}
	if err := verifyNode(nodes, "sub-proj-1", workspace.KindEcosystemWorktreeSubProjectWorktree, ctx.GetString("subProj1Path"), ctx.GetString("ecoFeaturePath"), ctx.GetString("workEcoPath")); err != nil {
		return err
	}

	return nil
}

// verifyLookupsByPath runs `core ws cwd --json` from various paths and validates the output.
func verifyLookupsByPath(ctx *harness.Context) error {
	// Create a test subdirectory for nested path testing
	someDirPath := filepath.Join(ctx.GetString("subProj1Path"), "some-dir")
	if err := fs.CreateDir(someDirPath); err != nil {
		return err
	}

	testCases := []struct {
		path         string
		expectedName string
		expectedKind workspace.WorkspaceKind
	}{
		{ctx.GetString("workEcoPath"), "work-eco", workspace.KindEcosystemRoot},
		{someDirPath, "sub-proj-1", workspace.KindEcosystemSubProject},
		{ctx.GetString("playProjPath"), "play-proj", workspace.KindStandaloneProject},
		{ctx.GetString("featureAPath"), "feature-a", workspace.KindStandaloneProjectWorktree},
		{ctx.GetString("ecoFeaturePath"), "eco-feature", workspace.KindEcosystemWorktree},
		{ctx.GetString("linkedSubProjPath"), "sub-proj-1", workspace.KindEcosystemWorktreeSubProjectWorktree},
	}

	for _, tc := range testCases {
		cmd := ctx.Command("core", "ws", "cwd", "--json").Dir(tc.path)
		result := cmd.Run()
		if result.Error != nil {
			return fmt.Errorf("`core ws cwd` failed in dir %s: %w", tc.path, result.Error)
		}

		var node workspace.WorkspaceNode
		if err := json.Unmarshal([]byte(result.Stdout), &node); err != nil {
			return fmt.Errorf("failed to unmarshal node for path %s: %w", tc.path, err)
		}

		if err := assert.Equal(tc.expectedName, node.Name, fmt.Sprintf("Name mismatch for path %s", tc.path)); err != nil {
			return err
		}
		if err := assert.Equal(tc.expectedKind, node.Kind, fmt.Sprintf("Kind mismatch for path %s", tc.path)); err != nil {
			return err
		}
	}
	return nil
}

// verifyNode is a helper to find a node by name and kind and verify its properties.
func verifyNode(nodes []*workspace.WorkspaceNode, name string, kind workspace.WorkspaceKind, parentProject, parentEco, rootEco string) error {
	for _, node := range nodes {
		if node.Name == name && node.Kind == kind {
			if err := assert.Equal(parentProject, node.ParentProjectPath, "ParentProjectPath mismatch for "+name); err != nil {
				return err
			}
			if err := assert.Equal(parentEco, node.ParentEcosystemPath, "ParentEcosystemPath mismatch for "+name); err != nil {
				return err
			}
			if err := assert.Equal(rootEco, node.RootEcosystemPath, "RootEcosystemPath mismatch for "+name); err != nil {
				return err
			}
			return nil // Found and verified
		}
	}
	return fmt.Errorf("failed to find workspace node with Name=%s and Kind=%s", name, kind)
}
