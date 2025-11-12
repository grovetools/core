package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransformToWorkspaceNodes_HierarchyAndClassification(t *testing.T) {
	// Create a temporary filesystem structure to test with
	tempDir := t.TempDir()

	// Create the ecosystem root
	ecoPath := filepath.Join(tempDir, "my-ecosystem")
	require.NoError(t, os.MkdirAll(ecoPath, 0755))

	// Create .grove-worktrees directory
	worktreesDir := filepath.Join(ecoPath, ".grove-worktrees")
	require.NoError(t, os.MkdirAll(worktreesDir, 0755))

	// Create ecosystem worktree
	ecoWorktreePath := filepath.Join(worktreesDir, "eco-feature")
	require.NoError(t, os.MkdirAll(ecoWorktreePath, 0755))

	// Create sub-project with .git file (worktree - linked development)
	subProjectWtPath := filepath.Join(ecoWorktreePath, "sub-project-wt")
	require.NoError(t, os.MkdirAll(subProjectWtPath, 0755))
	gitFilePath := filepath.Join(subProjectWtPath, ".git")
	require.NoError(t, os.WriteFile(gitFilePath, []byte("gitdir: /some/path"), 0644))

	// Create sub-project with .git directory (full checkout)
	subProjectCheckoutPath := filepath.Join(ecoWorktreePath, "sub-project-checkout")
	require.NoError(t, os.MkdirAll(subProjectCheckoutPath, 0755))
	gitDirPath := filepath.Join(subProjectCheckoutPath, ".git")
	require.NoError(t, os.MkdirAll(gitDirPath, 0755))

	// Construct a DiscoveryResult reflecting this structure
	result := &DiscoveryResult{
		Ecosystems: []Ecosystem{
			{
				Name: "my-ecosystem",
				Path: ecoPath,
				Type: "User",
			},
		},
		Projects: []Project{
			{
				Name:                "eco-feature",
				Path:                ecoWorktreePath,
				ParentEcosystemPath: ecoPath,
				Workspaces: []DiscoveredWorkspace{
					{
						Name:              "eco-feature",
						Path:              ecoWorktreePath,
						Type:              WorkspaceTypePrimary,
						ParentProjectPath: ecoWorktreePath,
					},
				},
			},
			{
				Name:                "sub-project-wt",
				Path:                subProjectWtPath,
				ParentEcosystemPath: ecoWorktreePath,
				Workspaces: []DiscoveredWorkspace{
					{
						Name:              "main",
						Path:              subProjectWtPath,
						Type:              WorkspaceTypePrimary,
						ParentProjectPath: subProjectWtPath,
					},
				},
			},
			{
				Name:                "sub-project-checkout",
				Path:                subProjectCheckoutPath,
				ParentEcosystemPath: ecoWorktreePath,
				Workspaces: []DiscoveredWorkspace{
					{
						Name:              "main",
						Path:              subProjectCheckoutPath,
						Type:              WorkspaceTypePrimary,
						ParentProjectPath: subProjectCheckoutPath,
					},
				},
			},
		},
	}

	// Execute transformation
	nodes := TransformToWorkspaceNodes(result, nil)

	// Create a map for easy lookup
	nodeMap := make(map[string]*WorkspaceNode)
	for _, node := range nodes {
		nodeMap[node.Path] = node
	}

	// Verify the ecosystem root
	ecoNode := nodeMap[ecoPath]
	require.NotNil(t, ecoNode, "Ecosystem root node should exist")
	assert.Equal(t, "my-ecosystem", ecoNode.Name)
	assert.Equal(t, KindEcosystemRoot, ecoNode.Kind)
	assert.Equal(t, ecoPath, ecoNode.RootEcosystemPath)
	assert.Empty(t, ecoNode.ParentEcosystemPath)

	// Verify the ecosystem worktree
	ecoWtNode := nodeMap[ecoWorktreePath]
	require.NotNil(t, ecoWtNode, "Ecosystem worktree node should exist")
	assert.Equal(t, "eco-feature", ecoWtNode.Name)
	assert.Equal(t, KindEcosystemWorktree, ecoWtNode.Kind)
	assert.Equal(t, ecoPath, ecoWtNode.ParentEcosystemPath)
	assert.Equal(t, ecoPath, ecoWtNode.RootEcosystemPath)

	// Verify sub-project-wt (should be KindEcosystemWorktreeSubProjectWorktree due to .git file)
	subWtNode := nodeMap[subProjectWtPath]
	require.NotNil(t, subWtNode, "Sub-project worktree node should exist")
	assert.Equal(t, "sub-project-wt", subWtNode.Name)
	assert.Equal(t, KindEcosystemWorktreeSubProjectWorktree, subWtNode.Kind,
		"Sub-project with .git file should be classified as KindEcosystemWorktreeSubProjectWorktree")
	assert.Equal(t, ecoWorktreePath, subWtNode.ParentEcosystemPath)
	assert.Equal(t, ecoPath, subWtNode.RootEcosystemPath,
		"RootEcosystemPath should point to the top-level ecosystem root")

	// Verify sub-project-checkout (should be KindEcosystemWorktreeSubProject due to .git directory)
	subCheckoutNode := nodeMap[subProjectCheckoutPath]
	require.NotNil(t, subCheckoutNode, "Sub-project checkout node should exist")
	assert.Equal(t, "sub-project-checkout", subCheckoutNode.Name)
	assert.Equal(t, KindEcosystemWorktreeSubProject, subCheckoutNode.Kind,
		"Sub-project with .git directory should be classified as KindEcosystemWorktreeSubProject")
	assert.Equal(t, ecoWorktreePath, subCheckoutNode.ParentEcosystemPath)
	assert.Equal(t, ecoPath, subCheckoutNode.RootEcosystemPath,
		"RootEcosystemPath should point to the top-level ecosystem root")
}

func TestTransformToWorkspaceNodes_StandaloneProject(t *testing.T) {
	// Create a temporary filesystem structure
	tempDir := t.TempDir()
	projectPath := filepath.Join(tempDir, "standalone-project")
	require.NoError(t, os.MkdirAll(projectPath, 0755))

	result := &DiscoveryResult{
		Projects: []Project{
			{
				Name: "standalone-project",
				Path: projectPath,
				Workspaces: []DiscoveredWorkspace{
					{
						Name:              "main",
						Path:              projectPath,
						Type:              WorkspaceTypePrimary,
						ParentProjectPath: projectPath,
					},
				},
			},
		},
	}

	nodes := TransformToWorkspaceNodes(result, nil)

	require.Len(t, nodes, 1)
	node := nodes[0]
	assert.Equal(t, "standalone-project", node.Name)
	assert.Equal(t, KindStandaloneProject, node.Kind)
	assert.Empty(t, node.ParentEcosystemPath)
	assert.Empty(t, node.RootEcosystemPath)
}

func TestTransformToWorkspaceNodes_EcosystemSubProject(t *testing.T) {
	// Create a temporary filesystem structure
	tempDir := t.TempDir()
	ecoPath := filepath.Join(tempDir, "my-ecosystem")
	subProjectPath := filepath.Join(ecoPath, "sub-project")
	require.NoError(t, os.MkdirAll(subProjectPath, 0755))

	// Create .git directory (full checkout)
	gitDirPath := filepath.Join(subProjectPath, ".git")
	require.NoError(t, os.MkdirAll(gitDirPath, 0755))

	result := &DiscoveryResult{
		Ecosystems: []Ecosystem{
			{
				Name: "my-ecosystem",
				Path: ecoPath,
				Type: "User",
			},
		},
		Projects: []Project{
			{
				Name:                "sub-project",
				Path:                subProjectPath,
				ParentEcosystemPath: ecoPath,
				Workspaces: []DiscoveredWorkspace{
					{
						Name:              "main",
						Path:              subProjectPath,
						Type:              WorkspaceTypePrimary,
						ParentProjectPath: subProjectPath,
					},
				},
			},
		},
	}

	nodes := TransformToWorkspaceNodes(result, nil)

	// Find the sub-project node
	var subProjectNode *WorkspaceNode
	for _, node := range nodes {
		if node.Path == subProjectPath {
			subProjectNode = node
			break
		}
	}

	require.NotNil(t, subProjectNode)
	assert.Equal(t, "sub-project", subProjectNode.Name)
	assert.Equal(t, KindEcosystemSubProject, subProjectNode.Kind)
	assert.Equal(t, ecoPath, subProjectNode.ParentEcosystemPath)
	assert.Equal(t, ecoPath, subProjectNode.RootEcosystemPath)
}

func TestFindRootEcosystem(t *testing.T) {
	// Create a mock node map with a hierarchy
	ecoRootPath := "/path/to/ecosystem"
	ecoWorktreePath := "/path/to/ecosystem/.grove-worktrees/feature"
	subProjectPath := "/path/to/ecosystem/.grove-worktrees/feature/sub-project"

	nodeMap := map[string]*WorkspaceNode{
		ecoRootPath: {
			Name:              "ecosystem",
			Path:              ecoRootPath,
			Kind:              KindEcosystemRoot,
			RootEcosystemPath: ecoRootPath,
		},
		ecoWorktreePath: {
			Name:                "feature",
			Path:                ecoWorktreePath,
			Kind:                KindEcosystemWorktree,
			ParentEcosystemPath: ecoRootPath,
		},
		subProjectPath: {
			Name:                "sub-project",
			Path:                subProjectPath,
			Kind:                KindEcosystemWorktreeSubProjectWorktree,
			ParentEcosystemPath: ecoWorktreePath,
		},
	}

	// Test finding root from ecosystem worktree
	root := findRootEcosystem(ecoWorktreePath, nodeMap)
	assert.Equal(t, ecoRootPath, root)

	// Test finding root from sub-project (should traverse up through ecosystem worktree)
	root = findRootEcosystem(subProjectPath, nodeMap)
	assert.Equal(t, ecoRootPath, root)

	// Test with a path not in the map (should return the path itself)
	root = findRootEcosystem("/unknown/path", nodeMap)
	assert.Equal(t, "/unknown/path", root)
}

func TestTransformToWorkspaceNodes_ClonedRepos(t *testing.T) {
	tempDir := t.TempDir()
	clonedPath := filepath.Join(tempDir, "cloned-repo")

	result := &DiscoveryResult{
		Projects: []Project{
			{
				Name:        "cloned-repo",
				Path:        clonedPath,
				Type:        "Cloned",
				Version:     "v1.0.0",
				Commit:      "abc123",
				AuditStatus: "passed",
			},
		},
	}

	nodes := TransformToWorkspaceNodes(result, nil)

	require.Len(t, nodes, 1)
	node := nodes[0]
	assert.Equal(t, "cloned-repo", node.Name)
	assert.Equal(t, KindNonGroveRepo, node.Kind)
	assert.Equal(t, "v1.0.0", node.Version)
	assert.Equal(t, "abc123", node.Commit)
	assert.Equal(t, "passed", node.AuditStatus)
}
