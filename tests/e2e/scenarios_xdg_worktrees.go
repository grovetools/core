package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/grovetools/tend/pkg/fs"
	"github.com/grovetools/tend/pkg/git"
	"github.com/grovetools/tend/pkg/harness"

	"github.com/grovetools/core/pkg/workspace"
)

// samePath compares two filesystem paths after resolving symlinks and
// normalizing case (macOS /var -> /private/var, case-insensitive FS). Discovery
// preserves raw paths while the sandbox home may sit behind symlinks, so a naive
// string compare is unreliable.
func samePath(a, b string) bool {
	if a == "" || b == "" {
		return a == b
	}
	ra, err := filepath.EvalSymlinks(a)
	if err != nil {
		ra = a
	}
	rb, err := filepath.EvalSymlinks(b)
	if err != nil {
		rb = b
	}
	return strings.EqualFold(ra, rb)
}

// XDGWorktreeDiscoveryScenario verifies that an ecosystem worktree living under
// the XDG data dir (paths.WorktreesDir()/<DirIdentifier>/<name>) — NOT the
// legacy <eco>/.grove-worktrees layout — is discovered, classified with the
// correct kinds, and carries the node-identity contract: every parent link
// points at the ORIGINAL checkouts, never the XDG container.
func XDGWorktreeDiscoveryScenario() *harness.Scenario {
	var ecoDir, subADir, xdgWtPath, wtSubAPath string

	return &harness.Scenario{
		Name:        "xdg-worktree-discovery",
		Description: "Discovers an XDG-located ecosystem worktree and verifies kinds + parent links point at original checkouts.",
		Tags:        []string{"core", "workspace", "xdg", "discovery"},
		Steps: []harness.Step{
			{
				Name: "Setup grove config and ecosystem with an XDG worktree",
				Func: func(ctx *harness.Context) error {
					homeDir := ctx.HomeDir()
					workDir := filepath.Join(homeDir, "work")
					if err := fs.CreateDir(workDir); err != nil {
						return err
					}

					// Register ~/work as a grove; disable cx repo discovery so the
					// scan never reaches outside the sandbox.
					groveYML := `groves:
  work:
    path: ~/work
    enabled: true
context:
  repos_dir: ""
`
					if err := fs.WriteString(filepath.Join(homeDir, ".config", "grove", "grove.yml"), groveYML); err != nil {
						return err
					}

					// Ecosystem root (real git repo).
					ecoDir = filepath.Join(workDir, "my-eco")
					if err := fs.WriteString(filepath.Join(ecoDir, "grove.yml"),
						"version: '1.0'\nname: my-eco\nworkspaces: ['*']\n"); err != nil {
						return err
					}
					ecoRepo, err := git.SetupTestRepo(ecoDir)
					if err != nil {
						return err
					}
					if err := ecoRepo.AddCommit("initial commit"); err != nil {
						return err
					}

					// Ecosystem sub-project (separate git repo).
					subADir = filepath.Join(ecoDir, "sub-a")
					if err := fs.WriteString(filepath.Join(subADir, "grove.yml"),
						"version: '1.0'\nname: sub-a\n"); err != nil {
						return err
					}
					subARepo, err := git.SetupTestRepo(subADir)
					if err != nil {
						return err
					}
					if err := subARepo.AddCommit("initial commit"); err != nil {
						return err
					}

					// XDG ecosystem worktree under the SANDBOXED data dir. The
					// identifier matches what core computes for ecoDir, so
					// discovery scans the same base.
					id := workspace.DirIdentifier(ecoDir)
					xdgWtPath = filepath.Join(ctx.DataDir(), "grove", "worktrees", id, "wt1")
					if err := ecoRepo.CreateWorktree(xdgWtPath, "wt1"); err != nil {
						return fmt.Errorf("creating XDG ecosystem worktree: %w", err)
					}

					// Linked sub-project worktree inside the XDG ecosystem worktree.
					wtSubAPath = filepath.Join(xdgWtPath, "sub-a")
					if err := subARepo.CreateWorktree(wtSubAPath, "wt1"); err != nil {
						return fmt.Errorf("creating linked sub-project worktree: %w", err)
					}
					return nil
				},
			},
			{
				Name: "Discover and verify XDG worktree kinds + parent links",
				Func: func(ctx *harness.Context) error {
					cmd := ctx.Command("core", "ws", "--json")
					result := cmd.Run()
					ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
					if result.Error != nil {
						return fmt.Errorf("discovery failed: %w\nstderr: %s", result.Error, result.Stderr)
					}

					var nodes []*workspace.WorkspaceNode
					if err := json.Unmarshal([]byte(result.Stdout), &nodes); err != nil {
						return fmt.Errorf("failed to unmarshal nodes: %w\nstdout: %s", err, result.Stdout)
					}

					var ecoNode, wtNode, wtSubNode *workspace.WorkspaceNode
					for _, n := range nodes {
						switch {
						case n.Kind == workspace.KindEcosystemRoot && samePath(n.Path, ecoDir):
							ecoNode = n
						case n.Kind == workspace.KindEcosystemWorktree && n.Name == "wt1":
							wtNode = n
						case n.Kind == workspace.KindEcosystemWorktreeSubProjectWorktree && samePath(n.Path, wtSubAPath):
							wtSubNode = n
						}
					}

					if ecoNode == nil {
						return fmt.Errorf("ecosystem root my-eco not discovered\nnodes: %s", result.Stdout)
					}

					// The XDG ecosystem worktree: correct kind, real basename, and
					// all three parent links point at the ORIGINAL eco checkout.
					if wtNode == nil {
						return fmt.Errorf("XDG ecosystem worktree wt1 not discovered\nnodes: %s", result.Stdout)
					}
					if !samePath(wtNode.Path, xdgWtPath) {
						return fmt.Errorf("worktree Path = %q, want XDG path %q", wtNode.Path, xdgWtPath)
					}
					if !samePath(wtNode.ParentProjectPath, ecoDir) {
						return fmt.Errorf("worktree ParentProjectPath = %q, want original checkout %q", wtNode.ParentProjectPath, ecoDir)
					}
					if !samePath(wtNode.ParentEcosystemPath, ecoDir) {
						return fmt.Errorf("worktree ParentEcosystemPath = %q, want original checkout %q", wtNode.ParentEcosystemPath, ecoDir)
					}
					if !samePath(wtNode.RootEcosystemPath, ecoDir) {
						return fmt.Errorf("worktree RootEcosystemPath = %q, want original checkout %q", wtNode.RootEcosystemPath, ecoDir)
					}

					// The linked sub-project worktree inside the XDG worktree:
					// ParentEcosystem is the worktree, Root is the original eco,
					// ParentProject is the original sub-project checkout.
					if wtSubNode == nil {
						return fmt.Errorf("linked sub-project worktree inside XDG worktree not discovered\nnodes: %s", result.Stdout)
					}
					if !samePath(wtSubNode.ParentEcosystemPath, xdgWtPath) {
						return fmt.Errorf("sub-project worktree ParentEcosystemPath = %q, want XDG worktree %q", wtSubNode.ParentEcosystemPath, xdgWtPath)
					}
					if !samePath(wtSubNode.RootEcosystemPath, ecoDir) {
						return fmt.Errorf("sub-project worktree RootEcosystemPath = %q, want original eco %q", wtSubNode.RootEcosystemPath, ecoDir)
					}
					if !samePath(wtSubNode.ParentProjectPath, subADir) {
						return fmt.Errorf("sub-project worktree ParentProjectPath = %q, want original sub checkout %q", wtSubNode.ParentProjectPath, subADir)
					}
					return nil
				},
			},
		},
	}
}

// XDGWorktreeNotebookInheritanceScenario reproduces the bug class where a project
// checked out in an XDG worktree
// (~/.local/share/grove/worktrees/<eco>-<hash>/<repo>) resolved to the WRONG
// notebook — the global default ("nb") — instead of its ORIGIN grove's notebook.
//
// Setup: grove "proj" (path ~/work) maps to notebook "projnb"; the default
// notebook is "nb". A repo lives under the grove, and an XDG-located worktree is
// created from it. Because the worktree's own Path is outside every grove path,
// the old assignNotebookName (which matched ONLY node.Path) fell through to the
// default "nb". The fix falls back to the worktree's ORIGIN repo via
// GetGroupingKey(), so the worktree inherits "projnb".
//
// Assertion: the worktree node's notebook_name (from `core ws --json` and
// `core ws cwd --json` run from the worktree) is "projnb", NOT "nb". This FAILS
// against the old behavior and PASSES with the fix.
func XDGWorktreeNotebookInheritanceScenario() *harness.Scenario {
	var projDir, xdgWtPath string

	return &harness.Scenario{
		Name:        "xdg-worktree-notebook-inheritance",
		Description: "An XDG-located worktree inherits its origin grove's notebook (projnb), not the global default (nb).",
		Tags:        []string{"core", "workspace", "notebooks", "xdg", "regression"},
		Steps: []harness.Step{
			{
				Name: "Setup grove mapped to a non-default notebook + XDG worktree",
				Func: func(ctx *harness.Context) error {
					homeDir := ctx.HomeDir()
					workDir := filepath.Join(homeDir, "work")
					if err := fs.CreateDir(workDir); err != nil {
						return err
					}

					// Grove "proj" (~/work) -> notebook "projnb". Default notebook
					// is "nb". The notebook DEFINITIONS exist so the resolved name
					// is meaningful, and cx repo discovery is disabled so the scan
					// stays inside the sandbox.
					groveYML := `groves:
  proj:
    path: ~/work
    enabled: true
    notebook: projnb
notebooks:
  rules:
    default: nb
  definitions:
    nb:
      root_dir: ~/notebooks/nb
    projnb:
      root_dir: ~/notebooks/projnb
context:
  repos_dir: ""
`
					if err := fs.WriteString(filepath.Join(homeDir, ".config", "grove", "grove.yml"), groveYML); err != nil {
						return err
					}

					// The origin repo (main checkout) lives UNDER the grove path.
					projDir = filepath.Join(workDir, "proj-repo")
					if err := fs.WriteString(filepath.Join(projDir, "grove.yml"),
						"version: '1.0'\nname: proj-repo\n"); err != nil {
						return err
					}
					repo, err := git.SetupTestRepo(projDir)
					if err != nil {
						return err
					}
					if err := repo.AddCommit("initial commit"); err != nil {
						return err
					}

					// XDG worktree under the sandboxed data dir — its Path lives
					// OUTSIDE every grove path, which is what triggered the bug.
					id := workspace.DirIdentifier(projDir)
					xdgWtPath = filepath.Join(ctx.DataDir(), "grove", "worktrees", id, "feature-x")
					if err := repo.CreateWorktree(xdgWtPath, "feature-x"); err != nil {
						return fmt.Errorf("creating XDG worktree: %w", err)
					}
					return nil
				},
			},
			{
				Name: "Origin checkout resolves to its grove notebook (projnb)",
				Func: func(ctx *harness.Context) error {
					coreBinary, err := FindProjectBinary()
					if err != nil {
						return err
					}
					cmd := ctx.Command(coreBinary, "ws", "cwd", "--json").Dir(projDir)
					result := cmd.Run()
					ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
					if result.Error != nil {
						return fmt.Errorf("`core ws cwd` failed in origin: %w\nstderr: %s", result.Error, result.Stderr)
					}
					var node workspace.WorkspaceNode
					if err := json.Unmarshal([]byte(result.Stdout), &node); err != nil {
						return fmt.Errorf("failed to unmarshal origin node: %w\nstdout: %s", err, result.Stdout)
					}
					if node.NotebookName != "projnb" {
						return fmt.Errorf("origin checkout notebook_name = %q, want %q", node.NotebookName, "projnb")
					}
					return nil
				},
			},
			{
				Name: "XDG worktree inherits origin grove notebook (projnb), not default (nb)",
				Func: func(ctx *harness.Context) error {
					// Resolve from the worktree context, mirroring how the daemon/TUI
					// resolves a node for a checked-out worktree.
					coreBinary, err := FindProjectBinary()
					if err != nil {
						return err
					}
					cmd := ctx.Command(coreBinary, "ws", "cwd", "--json").Dir(xdgWtPath)
					result := cmd.Run()
					ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
					if result.Error != nil {
						return fmt.Errorf("`core ws cwd` failed in XDG worktree: %w\nstderr: %s", result.Error, result.Stderr)
					}

					var node workspace.WorkspaceNode
					if err := json.Unmarshal([]byte(result.Stdout), &node); err != nil {
						return fmt.Errorf("failed to unmarshal worktree node: %w\nstdout: %s", err, result.Stdout)
					}

					// The crux of the regression: the worktree must inherit its
					// origin grove's notebook, NOT fall back to the default.
					if node.NotebookName == "nb" {
						return fmt.Errorf("XDG worktree fell back to the DEFAULT notebook %q; want origin grove notebook %q (this is the regressed behavior)", "nb", "projnb")
					}
					if node.NotebookName != "projnb" {
						return fmt.Errorf("XDG worktree notebook_name = %q, want %q", node.NotebookName, "projnb")
					}

					// Sanity: confirm we actually resolved the worktree node and its
					// origin link points back at the checkout under the grove.
					if !samePath(node.ParentProjectPath, projDir) {
						return fmt.Errorf("XDG worktree ParentProjectPath = %q, want origin checkout %q", node.ParentProjectPath, projDir)
					}
					return nil
				},
			},
			{
				Name: "Full discovery also tags the worktree with projnb",
				Func: func(ctx *harness.Context) error {
					coreBinary, err := FindProjectBinary()
					if err != nil {
						return err
					}
					cmd := ctx.Command(coreBinary, "ws", "--json")
					result := cmd.Run()
					ctx.ShowCommandOutput(cmd.String(), result.Stdout, result.Stderr)
					if result.Error != nil {
						return fmt.Errorf("discovery failed: %w\nstderr: %s", result.Error, result.Stderr)
					}

					var nodes []*workspace.WorkspaceNode
					if err := json.Unmarshal([]byte(result.Stdout), &nodes); err != nil {
						return fmt.Errorf("failed to unmarshal nodes: %w\nstdout: %s", err, result.Stdout)
					}

					var wtNode *workspace.WorkspaceNode
					for _, n := range nodes {
						if n.Kind == workspace.KindStandaloneProjectWorktree && samePath(n.Path, xdgWtPath) {
							wtNode = n
							break
						}
					}
					if wtNode == nil {
						return fmt.Errorf("XDG worktree feature-x not discovered\nnodes: %s", result.Stdout)
					}
					if wtNode.NotebookName != "projnb" {
						return fmt.Errorf("discovered XDG worktree notebook_name = %q, want %q\nnodes: %s", wtNode.NotebookName, "projnb", result.Stdout)
					}
					return nil
				},
			},
		},
	}
}

// XDGZombieWorktreeScenario is the XDG variant of the legacy zombie-worktree
// log scenario: a long-running logger initialized inside an XDG worktree must
// (a) route logs to the XDG state dir (never recreate worktree-local logs), and
// (b) once the worktree is deleted, recognize it as a zombie so the directory is
// never resurrected. ExplicitOnly + slow because it builds and runs a real
// background process.
func XDGZombieWorktreeScenario() *harness.Scenario {
	var projDir, xdgWtPath, captureFile string
	var bgProcess *exec.Cmd
	var cancel context.CancelFunc

	return &harness.Scenario{
		Name:         "xdg-zombie-worktree-log-recreation",
		Description:  "Deleted XDG worktree is detected as zombie and not resurrected by a long-running logger.",
		Tags:         []string{"core", "logging", "worktree", "xdg", "regression", "slow"},
		ExplicitOnly: true,
		Steps: []harness.Step{
			{
				Name: "Setup project and XDG worktree",
				Func: func(ctx *harness.Context) error {
					projDir = filepath.Join(ctx.HomeDir(), "work", "xdg-zombie-proj")
					groveYML := `name: xdg-zombie-proj
version: "1.0"
logging:
  file:
    enabled: true
`
					if err := fs.WriteString(filepath.Join(projDir, "grove.yml"), groveYML); err != nil {
						return err
					}
					repo, err := git.SetupTestRepo(projDir)
					if err != nil {
						return err
					}
					if err := repo.AddCommit("initial commit"); err != nil {
						return err
					}

					// XDG worktree under the sandboxed data dir.
					id := workspace.DirIdentifier(projDir)
					xdgWtPath = filepath.Join(ctx.DataDir(), "grove", "worktrees", id, "zombie-feature")
					return repo.CreateWorktree(xdgWtPath, "zombie-feature")
				},
			},
			{
				Name: "Start background logger inside the XDG worktree",
				Func: func(ctx *harness.Context) error {
					// The program logs in a loop and, on every iteration, records
					// whether core now considers its worktree a zombie. Stdout is
					// captured to a file the assertions read.
					program := `
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/grovetools/core/logging"
	"github.com/grovetools/core/pkg/workspace"
)

func main() {
	workDir := os.Args[1]
	if err := os.Chdir(workDir); err != nil {
		fmt.Fprintf(os.Stderr, "chdir %s: %v\n", workDir, err)
		os.Exit(1)
	}
	// Initialize the logger ONCE to model a long-running process holding a
	// single logger instance (the original resurrection trigger).
	log := logging.NewLogger("xdg-zombie-logger")
	for {
		if workspace.IsZombieWorktree(workDir) {
			fmt.Println("ZOMBIE_DETECTED")
		}
		log.Info("Background logger is still active.")
		time.Sleep(300 * time.Millisecond)
	}
}
`
					tmpDir := ctx.NewDir("xdg-bg-process")
					programPath := filepath.Join(tmpDir, "main.go")
					if err := fs.WriteString(programPath, program); err != nil {
						return fmt.Errorf("writing background program: %w", err)
					}

					captureFile = filepath.Join(tmpDir, "stdout.txt")
					out, err := os.Create(captureFile)
					if err != nil {
						return err
					}

					var processCtx context.Context
					processCtx, cancel = context.WithCancel(context.Background())
					bgProcess = exec.CommandContext(processCtx, "go", "run", programPath, xdgWtPath)
					// Point only the grove XDG vars at the sandbox so core's
					// DataDir()/StateDir() and the worktree-base check resolve to
					// ctx.DataDir()/ctx.StateDir(). We deliberately keep the host
					// HOME (and GOCACHE/GOMODCACHE) so `go run` can still build —
					// clobbering HOME breaks module/cache resolution. GROVE_HOME is
					// cleared because it beats XDG_DATA_HOME in core's resolution.
					bgProcess.Env = append(os.Environ(),
						"GROVE_HOME=",
						fmt.Sprintf("XDG_DATA_HOME=%s", ctx.DataDir()),
						fmt.Sprintf("XDG_STATE_HOME=%s", ctx.StateDir()),
					)
					bgProcess.Stdout = out
					bgProcess.Stderr = os.Stderr
					if err := bgProcess.Start(); err != nil {
						return fmt.Errorf("starting background process: %w", err)
					}

					// Give `go run` time to build + emit the first log line.
					time.Sleep(5 * time.Second)

					logGlob := filepath.Join(ctx.StateDir(), "grove", "logs", "workspaces", "xdg-zombie-proj", "zombie-feature", "*.log")
					logFiles, err := filepath.Glob(logGlob)
					if err != nil || len(logFiles) == 0 {
						return fmt.Errorf("logger did not create the initial XDG-state log file (glob %s)", logGlob)
					}
					// Logs must NOT be written inside the worktree itself.
					if _, err := os.Stat(filepath.Join(xdgWtPath, ".grove", "logs")); err == nil {
						return fmt.Errorf("logger wrote worktree-local logs; expected XDG state redirection")
					}
					return nil
				},
			},
			{
				Name: "Delete the XDG worktree directory",
				Func: func(ctx *harness.Context) error {
					return os.RemoveAll(xdgWtPath)
				},
			},
			{
				Name: "Verify worktree is detected as zombie and not resurrected",
				Func: func(ctx *harness.Context) error {
					// Let the logger run several more iterations against the now
					// deleted worktree.
					time.Sleep(3 * time.Second)

					// Core assertion: the deleted worktree directory is NOT
					// recreated by the still-running logger.
					if _, err := os.Stat(xdgWtPath); !os.IsNotExist(err) {
						content, _ := os.ReadDir(xdgWtPath)
						return fmt.Errorf("XDG worktree directory must not be recreated by the logger; contents: %v", content)
					}

					// The logger observed the zombie state (IsZombieWorktree==true
					// for the deleted XDG worktree).
					captured, err := fs.ReadString(captureFile)
					if err != nil {
						return fmt.Errorf("reading capture file: %w", err)
					}
					if !strings.Contains(captured, "ZOMBIE_DETECTED") {
						return fmt.Errorf("expected logger to detect the deleted XDG worktree as a zombie; captured:\n%s", captured)
					}
					return nil
				},
			},
			{
				Name: "Verify logs remain in the XDG state dir",
				Func: func(ctx *harness.Context) error {
					logFiles, err := filepath.Glob(filepath.Join(ctx.StateDir(), "grove", "logs", "workspaces", "xdg-zombie-proj", "zombie-feature", "*.log"))
					if err != nil || len(logFiles) == 0 {
						return fmt.Errorf("XDG-state log file missing after worktree deletion")
					}
					logContent, err := fs.ReadString(logFiles[0])
					if err != nil {
						return fmt.Errorf("reading redirected log: %w", err)
					}
					if !contains(logContent, "[xdg-zombie-logger]") && !contains(logContent, `"component":"xdg-zombie-logger"`) {
						return fmt.Errorf("redirected log missing background logger output; got: %s", logContent)
					}
					return nil
				},
			},
		},
		Teardown: []harness.Step{
			{
				Name: "Stop background process",
				Func: func(ctx *harness.Context) error {
					if cancel != nil {
						cancel()
					}
					if bgProcess != nil {
						_ = bgProcess.Wait()
					}
					return nil
				},
			},
		},
	}
}
