package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/grovetools/core/config"
)

// xdgFixture is a full two-layout workspace tree:
//
//	<root>/work/my-eco                 ecosystem root (grove.yml + .git dir)
//	<root>/work/my-eco/sub-a           ecosystem sub-project
//	<root>/work/standalone             standalone project
//	WorktreesDir()/<id(my-eco)>/wt1    XDG ecosystem worktree (.git file)
//	  wt1/sub-a                        linked sub-project worktree (.git file)
//	  wt1/sub-b                        sub-project full checkout (.git dir)
//	WorktreesDir()/<id(standalone)>/fix-1  XDG standalone worktree (.git file)
type xdgFixture struct {
	ecoDir          string
	subADir         string
	standaloneDir   string
	ecoWtPath       string
	wtSubAPath      string
	wtSubBPath      string
	standaloneWtVal string
}

func writeGroveYML(t *testing.T, dir, fileName string, cfg config.Config) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	b, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, fileName), b, 0o644))
}

func writeWorktreeGitFile(t *testing.T, wtPath, ownerGitDir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(wtPath, 0o755))
	content := "gitdir: " + ownerGitDir + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(wtPath, ".git"), []byte(content), 0o644))
}

func setupXDGFixture(t *testing.T) *xdgFixture {
	t.Helper()
	sandboxXDG(t)

	rootDir := t.TempDir()

	// Global config registering <root>/work as a grove; cx repo discovery
	// disabled so the test never touches real user repos.
	globalConfigDir := filepath.Join(rootDir, "home", ".config", "grove")
	emptyStr := ""
	writeGroveYML(t, globalConfigDir, "grove.yml", config.Config{
		SearchPaths: map[string]config.SearchPathConfig{
			"work": {Path: filepath.Join(rootDir, "work"), Enabled: true},
		},
		Context: &config.ContextConfig{ReposDir: &emptyStr},
	})
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(rootDir, "home", ".config"))
	t.Setenv("HOME", filepath.Join(rootDir, "home"))
	t.Setenv("GROVE_CONFIG_OVERLAY", filepath.Join(globalConfigDir, "grove.yml"))

	f := &xdgFixture{}

	// Ecosystem root + sub-project.
	f.ecoDir = filepath.Join(rootDir, "work", "my-eco")
	writeGroveYML(t, f.ecoDir, "grove.yml", config.Config{Name: "my-eco", Workspaces: []string{"*"}})
	require.NoError(t, os.MkdirAll(filepath.Join(f.ecoDir, ".git"), 0o755))
	f.subADir = filepath.Join(f.ecoDir, "sub-a")
	writeGroveYML(t, f.subADir, "grove.yml", config.Config{Name: "sub-a"})
	require.NoError(t, os.MkdirAll(filepath.Join(f.subADir, ".git"), 0o755))

	// Standalone project.
	f.standaloneDir = filepath.Join(rootDir, "work", "standalone")
	writeGroveYML(t, f.standaloneDir, ".grove.yml", config.Config{Name: "standalone"})
	require.NoError(t, os.MkdirAll(filepath.Join(f.standaloneDir, ".git"), 0o755))

	// XDG ecosystem worktree of my-eco.
	f.ecoWtPath = ResolveNewWorktreePath(f.ecoDir, "wt1", true)
	writeWorktreeGitFile(t, f.ecoWtPath, filepath.Join(f.ecoDir, ".git", "worktrees", "wt1"))
	writeGroveYML(t, f.ecoWtPath, "grove.yml", config.Config{Name: "my-eco", Workspaces: []string{"*"}})

	// Linked sub-project worktree inside the XDG ecosystem worktree.
	f.wtSubAPath = filepath.Join(f.ecoWtPath, "sub-a")
	writeWorktreeGitFile(t, f.wtSubAPath, filepath.Join(f.subADir, ".git", "worktrees", "sub-a"))
	writeGroveYML(t, f.wtSubAPath, "grove.yml", config.Config{Name: "sub-a"})

	// Full-checkout sub-project inside the XDG ecosystem worktree.
	f.wtSubBPath = filepath.Join(f.ecoWtPath, "sub-b")
	writeGroveYML(t, f.wtSubBPath, "grove.yml", config.Config{Name: "sub-b"})
	require.NoError(t, os.MkdirAll(filepath.Join(f.wtSubBPath, ".git"), 0o755))

	// XDG worktree of the standalone project.
	f.standaloneWtVal = ResolveNewWorktreePath(f.standaloneDir, "fix-1", true)
	writeWorktreeGitFile(t, f.standaloneWtVal, filepath.Join(f.standaloneDir, ".git", "worktrees", "fix-1"))
	writeGroveYML(t, f.standaloneWtVal, ".grove.yml", config.Config{Name: "standalone"})

	return f
}

func discoverFixture(t *testing.T) (*xdgFixture, *DiscoveryResult, []*WorkspaceNode) {
	t.Helper()
	f := setupXDGFixture(t)

	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)
	result, err := NewDiscoveryService(logger).DiscoverAll()
	require.NoError(t, err)
	nodes := TransformToWorkspaceNodes(result, nil)
	return f, result, nodes
}

// TestDiscoverAll_XDGEcosystemWorktrees verifies the dedicated XDG
// enumeration phase: ecosystem worktrees outside the walked groves are
// discovered with explicit provenance, their sub-projects are enumerated,
// and the linking pass connects everything to the original checkouts.
func TestDiscoverAll_XDGEcosystemWorktrees(t *testing.T) {
	f, result, nodes := discoverFixture(t)

	projectsByPath := make(map[string]Project)
	for _, p := range result.Projects {
		projectsByPath[p.Path] = p
	}

	wt, ok := projectsByPath[f.ecoWtPath]
	require.True(t, ok, "XDG ecosystem worktree should be discovered; projects: %v", result.Projects)
	assert.Equal(t, "wt1", wt.Name)
	assert.Equal(t, f.ecoDir, wt.ParentEcosystemPath)
	assert.Equal(t, f.ecoDir, wt.WorktreeOwnerPath, "provenance: owner root")
	assert.Equal(t, filepath.Dir(f.ecoWtPath), wt.WorktreeSourceBase, "provenance: source base")

	subA, ok := projectsByPath[f.wtSubAPath]
	require.True(t, ok, "sub-project inside XDG worktree should be discovered")
	assert.Equal(t, f.ecoWtPath, subA.ParentEcosystemPath, "linking pass should attach sub-projects to the XDG worktree")
	assert.Empty(t, subA.WorktreeOwnerPath, "sub-projects are not ecosystem worktrees")

	_, ok = projectsByPath[f.wtSubBPath]
	require.True(t, ok, "full-checkout sub-project inside XDG worktree should be discovered")

	// The standalone project's XDG worktree shows up as a workspace entry
	// via the WorktreeBases probing in processProject.
	standalone, ok := projectsByPath[f.standaloneDir]
	require.True(t, ok)
	foundWt := false
	for _, ws := range standalone.Workspaces {
		if ws.Path == f.standaloneWtVal && ws.Type == WorkspaceTypeWorktree {
			foundWt = true
			assert.Equal(t, f.standaloneDir, ws.ParentProjectPath)
		}
	}
	assert.True(t, foundWt, "standalone XDG worktree should be enumerated as a workspace")

	// Transformed kinds and the node identity contract: parents point at
	// ORIGINAL checkouts, never the XDG container.
	nodeMap := make(map[string]*WorkspaceNode)
	for _, n := range nodes {
		nodeMap[n.Path] = n
	}

	wtNode := nodeMap[f.ecoWtPath]
	require.NotNil(t, wtNode)
	assert.Equal(t, KindEcosystemWorktree, wtNode.Kind)
	assert.Equal(t, "wt1", wtNode.Name)
	assert.Equal(t, f.ecoDir, wtNode.ParentProjectPath)
	assert.Equal(t, f.ecoDir, wtNode.ParentEcosystemPath)
	assert.Equal(t, f.ecoDir, wtNode.RootEcosystemPath)
	require.NoError(t, wtNode.Validate())

	subANode := nodeMap[f.wtSubAPath]
	require.NotNil(t, subANode)
	assert.Equal(t, KindEcosystemWorktreeSubProjectWorktree, subANode.Kind)
	assert.Equal(t, f.ecoWtPath, subANode.ParentEcosystemPath)
	assert.Equal(t, f.ecoDir, subANode.RootEcosystemPath)
	assert.Equal(t, f.subADir, subANode.ParentProjectPath)

	subBNode := nodeMap[f.wtSubBPath]
	require.NotNil(t, subBNode)
	assert.Equal(t, KindEcosystemWorktreeSubProject, subBNode.Kind)
	assert.Equal(t, f.ecoWtPath, subBNode.ParentEcosystemPath)
	assert.Equal(t, f.ecoDir, subBNode.RootEcosystemPath)

	saNode := nodeMap[f.standaloneWtVal]
	require.NotNil(t, saNode)
	assert.Equal(t, KindStandaloneProjectWorktree, saNode.Kind)
	assert.Equal(t, f.standaloneDir, saNode.ParentProjectPath)
}

// TestTransform_XDGProvenance verifies provenance-based classification
// without any filesystem layout cues: an ecosystem worktree at an arbitrary
// XDG-style path classifies via WorktreeOwnerPath, not path shape.
func TestTransform_XDGProvenance(t *testing.T) {
	sandboxXDG(t)

	ecoPath := "/checkouts/my-eco"
	xdgWtPath := "/data/grove/worktrees/my-eco-abcd1234/wt1"
	result := &DiscoveryResult{
		Ecosystems: []Ecosystem{{Name: "my-eco", Path: ecoPath, Type: "User"}},
		Projects: []Project{
			{
				Name:                "wt1",
				Path:                xdgWtPath,
				ParentEcosystemPath: ecoPath,
				WorktreeSourceBase:  filepath.Dir(xdgWtPath),
				WorktreeOwnerPath:   ecoPath,
				Workspaces: []DiscoveredWorkspace{
					{Name: "wt1", Path: xdgWtPath, Type: WorkspaceTypePrimary, ParentProjectPath: xdgWtPath},
				},
			},
			{
				Name:                "sub-a",
				Path:                xdgWtPath + "/sub-a",
				ParentEcosystemPath: xdgWtPath,
				Workspaces: []DiscoveredWorkspace{
					{Name: "main", Path: xdgWtPath + "/sub-a", Type: WorkspaceTypePrimary, ParentProjectPath: xdgWtPath + "/sub-a"},
				},
			},
		},
	}

	nodes := TransformToWorkspaceNodes(result, nil)
	nodeMap := make(map[string]*WorkspaceNode)
	for _, n := range nodes {
		nodeMap[n.Path] = n
	}

	wtNode := nodeMap[xdgWtPath]
	require.NotNil(t, wtNode)
	assert.Equal(t, KindEcosystemWorktree, wtNode.Kind)
	assert.Equal(t, ecoPath, wtNode.ParentProjectPath)
	assert.Equal(t, ecoPath, wtNode.RootEcosystemPath)

	// The sub-project classifies as inside-an-ecosystem-worktree because
	// its PARENT project carries worktree provenance (no .git file exists
	// at these fake paths, so the checkout variant is chosen).
	subNode := nodeMap[xdgWtPath+"/sub-a"]
	require.NotNil(t, subNode)
	assert.Equal(t, KindEcosystemWorktreeSubProject, subNode.Kind)
	assert.Equal(t, ecoPath, subNode.RootEcosystemPath)
}

// TestGetProjectByPath_XDG verifies the upward lookup classifies XDG
// worktrees identically to legacy ones, deriving parents from the worktree
// owner instead of the directory shape.
func TestGetProjectByPath_XDG(t *testing.T) {
	f := setupXDGFixture(t)

	t.Run("ecosystem worktree", func(t *testing.T) {
		node, err := GetProjectByPath(f.ecoWtPath)
		require.NoError(t, err)
		assert.Equal(t, "wt1", node.Name)
		assert.Equal(t, KindEcosystemWorktree, node.Kind)
		assert.Equal(t, normalizePath(t, f.ecoDir), normalizePath(t, node.ParentProjectPath))
		assert.Equal(t, normalizePath(t, f.ecoDir), normalizePath(t, node.ParentEcosystemPath))
		assert.Equal(t, normalizePath(t, f.ecoDir), normalizePath(t, node.RootEcosystemPath))
		require.NoError(t, node.Validate())
	})

	t.Run("linked sub-project worktree inside XDG worktree", func(t *testing.T) {
		node, err := GetProjectByPath(f.wtSubAPath)
		require.NoError(t, err)
		assert.Equal(t, "sub-a", node.Name)
		assert.Equal(t, KindEcosystemWorktreeSubProjectWorktree, node.Kind)
		assert.Equal(t, normalizePath(t, f.ecoWtPath), normalizePath(t, node.ParentEcosystemPath))
		assert.Equal(t, normalizePath(t, f.ecoDir), normalizePath(t, node.RootEcosystemPath))
		assert.Equal(t, normalizePath(t, f.subADir), normalizePath(t, node.ParentProjectPath))
	})

	t.Run("sub-project checkout inside XDG worktree", func(t *testing.T) {
		node, err := GetProjectByPath(f.wtSubBPath)
		require.NoError(t, err)
		assert.Equal(t, "sub-b", node.Name)
		assert.Equal(t, KindEcosystemWorktreeSubProject, node.Kind)
		assert.Equal(t, normalizePath(t, f.ecoWtPath), normalizePath(t, node.ParentEcosystemPath))
		assert.Equal(t, normalizePath(t, f.ecoDir), normalizePath(t, node.RootEcosystemPath))
	})

	t.Run("standalone project worktree", func(t *testing.T) {
		node, err := GetProjectByPath(f.standaloneWtVal)
		require.NoError(t, err)
		assert.Equal(t, "fix-1", node.Name)
		assert.Equal(t, KindStandaloneProjectWorktree, node.Kind)
		assert.Equal(t, normalizePath(t, f.standaloneDir), normalizePath(t, node.ParentProjectPath))
		assert.Empty(t, node.ParentEcosystemPath)
	})

	t.Run("deep path inside XDG worktree sub-project", func(t *testing.T) {
		deep := filepath.Join(f.wtSubAPath, "src", "pkg")
		require.NoError(t, os.MkdirAll(deep, 0o755))
		node, err := GetProjectByPath(deep)
		require.NoError(t, err)
		assert.Equal(t, "sub-a", node.Name)
		assert.Equal(t, KindEcosystemWorktreeSubProjectWorktree, node.Kind)
	})
}

// TestClassificationConsistency_XDG is the contract test from the plan:
// discovery and GetProjectByPath must assign IDENTICAL kinds (and
// equivalent parent links) to the same XDG worktree directory — daemon
// scope and UIs must never disagree about what a directory is.
func TestClassificationConsistency_XDG(t *testing.T) {
	f, _, nodes := discoverFixture(t)

	discovered := make(map[string]*WorkspaceNode)
	for _, n := range nodes {
		discovered[normalizePath(t, n.Path)] = n
	}

	for _, dir := range []string{f.ecoWtPath, f.wtSubAPath, f.wtSubBPath, f.standaloneWtVal} {
		discoveredNode, ok := discovered[normalizePath(t, dir)]
		require.True(t, ok, "discovery should produce a node for %s", dir)

		lookupNode, err := GetProjectByPath(dir)
		require.NoError(t, err, "lookup should classify %s", dir)

		assert.Equal(t, discoveredNode.Kind, lookupNode.Kind, "kind divergence for %s", dir)
		assert.Equal(t, normalizePath(t, discoveredNode.ParentEcosystemPath), normalizePath(t, lookupNode.ParentEcosystemPath), "ParentEcosystemPath divergence for %s", dir)
		assert.Equal(t, normalizePath(t, discoveredNode.RootEcosystemPath), normalizePath(t, lookupNode.RootEcosystemPath), "RootEcosystemPath divergence for %s", dir)
		assert.Equal(t, normalizePath(t, discoveredNode.ParentProjectPath), normalizePath(t, lookupNode.ParentProjectPath), "ParentProjectPath divergence for %s", dir)

		// Validate: in particular, never a KindEcosystemWorktree with empty
		// ParentProjectPath.
		require.NoError(t, discoveredNode.Validate())
		require.NoError(t, lookupNode.Validate())
	}
}

// TestProvider_FindByWorktree_XDG verifies worktree resolution against the
// discovered node index for XDG-located worktrees.
func TestProvider_FindByWorktree_XDG(t *testing.T) {
	f, result, _ := discoverFixture(t)

	provider := NewProvider(result)

	ecoNode := provider.FindByPath(f.ecoDir)
	require.NotNil(t, ecoNode)
	require.Equal(t, KindEcosystemRoot, ecoNode.Kind)

	// Ecosystem root + worktree name → the XDG ecosystem worktree node.
	wtNode := provider.FindByWorktree(ecoNode, "wt1")
	require.NotNil(t, wtNode, "FindByWorktree should resolve the XDG ecosystem worktree")
	assert.Equal(t, KindEcosystemWorktree, wtNode.Kind)
	assert.Equal(t, f.ecoWtPath, wtNode.Path)

	// Sub-project + ecosystem worktree name → the linked sub-project
	// worktree inside the XDG worktree.
	subANode := provider.FindByPath(f.subADir)
	require.NotNil(t, subANode)
	subInWt := provider.FindByWorktree(subANode, "wt1")
	require.NotNil(t, subInWt, "FindByWorktree should resolve sub-project inside the XDG worktree")
	assert.Equal(t, f.wtSubAPath, subInWt.Path)
}

// TestIdentifier_XDGNodes verifies identifiers derive from names and
// original-checkout parents, unchanged by XDG-located Paths.
func TestIdentifier_XDGNodes(t *testing.T) {
	sandboxXDG(t)

	ecoWt := &WorkspaceNode{
		Name:                "wt1",
		Path:                ResolveNewWorktreePath("/p/my-eco", "wt1", true),
		Kind:                KindEcosystemWorktree,
		ParentProjectPath:   "/p/my-eco",
		ParentEcosystemPath: "/p/my-eco",
		RootEcosystemPath:   "/p/my-eco",
	}
	assert.Equal(t, "my-eco:wt1", ecoWt.Identifier(":"))

	sub := &WorkspaceNode{
		Name:                "sub-a",
		Path:                filepath.Join(ecoWt.Path, "sub-a"),
		Kind:                KindEcosystemWorktreeSubProject,
		ParentEcosystemPath: ecoWt.Path,
		RootEcosystemPath:   "/p/my-eco",
	}
	assert.Equal(t, "my-eco:wt1:sub-a", sub.Identifier(":"))

	subWt := &WorkspaceNode{
		Name:                "sub-a",
		Path:                filepath.Join(ecoWt.Path, "sub-a"),
		Kind:                KindEcosystemWorktreeSubProjectWorktree,
		ParentProjectPath:   "/p/my-eco/sub-a",
		ParentEcosystemPath: ecoWt.Path,
		RootEcosystemPath:   "/p/my-eco",
	}
	assert.Equal(t, "my-eco:wt1:sub-a", subWt.Identifier(":"))

	standaloneWt := &WorkspaceNode{
		Name:              "fix-1",
		Path:              ResolveNewWorktreePath("/p/repo", "fix-1", true),
		Kind:              KindStandaloneProjectWorktree,
		ParentProjectPath: "/p/repo",
	}
	assert.Equal(t, "repo:fix-1", standaloneWt.Identifier(":"))
}

// TestNotebookLocator_XDGEcosystemWorktree pins the KindEcosystemWorktree
// branch of getContextNodeForPath for XDG-located nodes: notebook paths
// render from the ROOT ecosystem name, not the XDG container path.
func TestNotebookLocator_XDGEcosystemWorktree(t *testing.T) {
	sandboxXDG(t)

	cfg := &config.Config{
		Notebooks: &config.NotebooksConfig{
			Definitions: map[string]*config.Notebook{
				"nb": {
					RootDir:           "~/Code/nb",
					PlansPathTemplate: "workspaces/{{ .Workspace.Name }}/plans",
				},
			},
			Rules: &config.NotebookRules{Default: "nb"},
		},
	}
	locator := NewNotebookLocator(cfg)

	node := &WorkspaceNode{
		Name:                "wt1",
		Path:                ResolveNewWorktreePath("/p/my-eco", "wt1", true),
		Kind:                KindEcosystemWorktree,
		ParentProjectPath:   "/p/my-eco",
		ParentEcosystemPath: "/p/my-eco",
		RootEcosystemPath:   "/p/my-eco",
		NotebookName:        "nb",
	}
	plansDir, err := locator.GetPlansDir(node)
	require.NoError(t, err)
	assert.Contains(t, plansDir, filepath.Join("Code", "nb", "workspaces", "my-eco", "plans"),
		"XDG ecosystem worktree should render notebook paths from the root ecosystem name")
}

// TestResolveScope_XDGEcosystemWorktree pins the scope contract: an XDG
// ecosystem worktree is its own daemon scope (it resolves to itself, not
// to the parent ecosystem).
func TestResolveScope_XDGEcosystemWorktree(t *testing.T) {
	f := setupXDGFixture(t)

	scope := ResolveScope(f.ecoWtPath)
	assert.Equal(t, normalizePath(t, f.ecoWtPath), normalizePath(t, scope),
		"XDG ecosystem worktree should be its own scope")
}

// TestResolveScope_AnchorInvariant asserts that anchoring a container to a
// sub-project does not change the scope boundary: the container is still its
// own daemon scope, regardless of where it nests in the hierarchy.
func TestResolveScope_AnchorInvariant(t *testing.T) {
	sandboxXDG(t)

	subProjectPath := "/p/my-eco/sub-a"
	containerPath := ResolveNewWorktreePath(subProjectPath, "anchored-wt", true)

	result := &DiscoveryResult{
		Ecosystems: []Ecosystem{{Name: "my-eco", Path: "/p/my-eco", Type: "User"}},
		Projects: []Project{
			{
				Name:                "anchored-wt",
				Path:                containerPath,
				ParentEcosystemPath: "/p/my-eco",
				WorktreeSourceBase:  filepath.Dir(containerPath),
				WorktreeOwnerPath:   subProjectPath,
				Workspaces: []DiscoveredWorkspace{
					{Name: "anchored-wt", Path: containerPath, Type: WorkspaceTypePrimary, ParentProjectPath: containerPath},
				},
			},
		},
	}

	nodes := TransformToWorkspaceNodes(result, nil)
	nodeMap := make(map[string]*WorkspaceNode)
	for _, n := range nodes {
		nodeMap[n.Path] = n
	}

	containerNode := nodeMap[containerPath]
	require.NotNil(t, containerNode, "anchored container must appear as a node")
	assert.Equal(t, KindEcosystemWorktree, containerNode.Kind)
	assert.Equal(t, subProjectPath, containerNode.ParentProjectPath,
		"parent must be the anchor sub-project, not the ecosystem root")

	// Scope must return the container itself — anchoring changes nesting, not isolation.
	// Use the node directly (no live FS): scope.go's GetProjectByPath will fall back to
	// git.GetGitRoot if discovery cannot classify containerPath, so we verify the
	// invariant via the node's Kind instead.
	assert.True(t, containerNode.IsEcosystem(),
		"EcosystemWorktree is its own scope boundary regardless of anchor")
}

// TestDiscoverAll_AnchoredContainer verifies that a unified container placed
// under a sub-project's XDG base (via --anchor) is discovered as an
// EcosystemWorktree owned by that sub-project, not as a raw worktree leaf.
func TestDiscoverAll_AnchoredContainer(t *testing.T) {
	sandboxXDG(t)

	rootDir := t.TempDir()

	// Global config.
	globalConfigDir := filepath.Join(rootDir, "home", ".config", "grove")
	emptyStr := ""
	writeGroveYML(t, globalConfigDir, "grove.yml", config.Config{
		SearchPaths: map[string]config.SearchPathConfig{
			"work": {Path: filepath.Join(rootDir, "work"), Enabled: true},
		},
		Context: &config.ContextConfig{ReposDir: &emptyStr},
	})
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(rootDir, "home", ".config"))
	t.Setenv("HOME", filepath.Join(rootDir, "home"))
	t.Setenv("GROVE_CONFIG_OVERLAY", filepath.Join(globalConfigDir, "grove.yml"))

	// Ecosystem root.
	ecoDir := filepath.Join(rootDir, "work", "my-eco")
	writeGroveYML(t, ecoDir, "grove.yml", config.Config{Name: "my-eco", Workspaces: []string{"*"}})
	require.NoError(t, os.MkdirAll(filepath.Join(ecoDir, ".git"), 0o755))

	// Sub-project inside the ecosystem.
	subDir := filepath.Join(ecoDir, "sub-a")
	writeGroveYML(t, subDir, "grove.yml", config.Config{Name: "sub-a"})
	require.NoError(t, os.MkdirAll(filepath.Join(subDir, ".git"), 0o755))

	// Unified container anchored to sub-a: placed in sub-a's XDG base.
	anchoredWt := ResolveNewWorktreePath(subDir, "anchored-feature", true)
	writeWorktreeGitFile(t, anchoredWt, filepath.Join(ecoDir, ".git", "worktrees", "anchored-feature"))
	writeGroveYML(t, anchoredWt, "grove.yml", config.Config{Name: "my-eco", Workspaces: []string{"*"}})

	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)
	result, err := NewDiscoveryService(logger).DiscoverAll()
	require.NoError(t, err)

	projectsByPath := make(map[string]Project)
	for _, p := range result.Projects {
		projectsByPath[p.Path] = p
	}

	wt, ok := projectsByPath[anchoredWt]
	require.True(t, ok, "anchored container must be discovered; projects: %v", result.Projects)
	assert.Equal(t, subDir, wt.WorktreeOwnerPath,
		"owner must be the anchor sub-project")

	// Transformation: the container must appear as KindEcosystemWorktree with
	// ParentProjectPath pointing at the sub-project.
	nodes := TransformToWorkspaceNodes(result, nil)
	nodeMap := make(map[string]*WorkspaceNode)
	for _, n := range nodes {
		nodeMap[n.Path] = n
	}

	wtNode := nodeMap[anchoredWt]
	require.NotNil(t, wtNode, "anchored container must produce a node")
	assert.Equal(t, KindEcosystemWorktree, wtNode.Kind)
	assert.Equal(t, subDir, wtNode.ParentProjectPath,
		"container must nest under the anchor sub-project, not the ecosystem root")
	require.NoError(t, wtNode.Validate())
}
