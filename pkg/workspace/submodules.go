package workspace

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

// SetupSubmodules initializes submodules, creating linked worktrees where possible.
func SetupSubmodules(ctx context.Context, worktreePath, branchName string, repos []string, ds *DiscoveryService) error {
	// If no discovery service provided, create a default one
	if ds == nil {
		logger := logrus.New()
		logger.SetOutput(os.Stderr)
		logger.SetLevel(logrus.WarnLevel)
		ds = NewDiscoveryService(logger)
	}
	cmdCheckout := exec.CommandContext(ctx, "git", "checkout", "HEAD", "--", ".")
	cmdCheckout.Dir = worktreePath
	cmdCheckout.CombinedOutput() // Ignore error

	gitmodulesPath := filepath.Join(worktreePath, ".gitmodules")
	hasGitmodules := true
	if _, err := os.Stat(gitmodulesPath); os.IsNotExist(err) {
		hasGitmodules = false
		// If no .gitmodules but repos are specified, still try to create linked worktrees
		if len(repos) == 0 {
			return nil // No submodules and no repos specified
		}
	}

	var submodulePaths map[string]string
	if hasGitmodules {
		var err error
		submodulePaths, err = parseGitmodules(gitmodulesPath)
		if err != nil {
			return setupSubmodulesStandard(ctx, worktreePath, branchName)
		}
	} else {
		// No gitmodules, but repos are specified - create synthetic paths
		submodulePaths = make(map[string]string)
		for _, repo := range repos {
			// Use repo name as both the key and the path
			submodulePaths[repo] = repo
		}
	}

	localWorkspaces, err := discoverLocalWorkspacesFromService(ctx, ds)
	if err != nil && hasGitmodules {
		return setupSubmodulesStandard(ctx, worktreePath, branchName)
	}

	repoFilter := make(map[string]bool)
	if len(repos) > 0 {
		for _, repo := range repos {
			repoFilter[repo] = true
		}
	}
	
	gitRoot := filepath.Dir(filepath.Dir(worktreePath))
	var externalSubmodules []string

	for submoduleName, submodulePath := range submodulePaths {
		if len(repoFilter) > 0 && !repoFilter[submoduleName] {
			fmt.Printf("%s: skipping (not in repos filter)\n", submoduleName)
			continue
		}
		targetPath := filepath.Join(worktreePath, submodulePath)
		mainSubmodulePath := filepath.Join(gitRoot, submodulePath)
		
		if _, err := os.Stat(filepath.Join(mainSubmodulePath, ".git")); err == nil {
			fmt.Printf("%s: creating linked worktree\n", submoduleName)
			os.MkdirAll(filepath.Dir(targetPath), 0755)
			if _, err := os.Stat(filepath.Join(targetPath, ".git")); err != nil {
				os.RemoveAll(targetPath)
				cmdWorktree := exec.CommandContext(ctx, "git", "worktree", "add", targetPath, "-B", branchName)
				cmdWorktree.Dir = mainSubmodulePath
				cmdWorktree.Run()
			}
			continue
		}
		
		if localRepoPath, hasLocal := localWorkspaces[submoduleName]; hasLocal {
			fmt.Printf("%s: creating linked worktree\n", submoduleName)
			os.MkdirAll(filepath.Dir(targetPath), 0755)
			if _, err := os.Stat(filepath.Join(targetPath, ".git")); err != nil {
				os.RemoveAll(targetPath)
				cmdWorktree := exec.CommandContext(ctx, "git", "worktree", "add", targetPath, "-B", branchName)
				cmdWorktree.Dir = localRepoPath
				cmdWorktree.Run()
			}
		} else {
			externalSubmodules = append(externalSubmodules, submodulePath)
		}
	}

	if len(externalSubmodules) > 0 {
		for _, submodulePath := range externalSubmodules {
			cmdUpdate := exec.CommandContext(ctx, "git", "submodule", "update", "--init", "--recursive", "--", submodulePath)
			cmdUpdate.Dir = worktreePath
			cmdUpdate.Run()
		}
	}
	return nil
}

func setupSubmodulesStandard(ctx context.Context, worktreePath, branchName string) error {
	cmdUpdate := exec.CommandContext(ctx, "git", "submodule", "update", "--init", "--recursive")
	cmdUpdate.Dir = worktreePath
	cmdUpdate.Run()
	return nil
}

// discoverLocalWorkspacesFromService uses the DiscoveryService to find projects and their primary workspace paths.
func discoverLocalWorkspacesFromService(ctx context.Context, ds *DiscoveryService) (map[string]string, error) {
	if ds == nil {
		return make(map[string]string), fmt.Errorf("discovery service is nil")
	}

	result, err := ds.DiscoverAll()
	if err != nil {
		return nil, fmt.Errorf("failed to discover all workspaces: %w", err)
	}

	workspaceMap := make(map[string]string)
	for _, proj := range result.Projects {
		// The primary workspace path is the project's own path.
		if proj.Path != "" {
			workspaceMap[proj.Name] = proj.Path
		}
	}
	return workspaceMap, nil
}

// DiscoverLocalWorkspaces is kept for backward compatibility with tests
// New code should use discoverLocalWorkspacesFromService directly
func DiscoverLocalWorkspaces(ctx context.Context) (map[string]string, error) {
	// Check for test environment override first
	if mockData := os.Getenv("GROVE_TEST_WORKSPACES"); mockData != "" {
		var workspaces []WorkspaceInfo
		if err := json.Unmarshal([]byte(mockData), &workspaces); err != nil {
			return make(map[string]string), nil
		}

		result := make(map[string]string)
		for _, ws := range workspaces {
			// Find the main worktree - prioritize is_main:true, but fall back to path matching
			mainPath := ""
			for _, wt := range ws.Worktrees {
				if wt.IsMain {
					mainPath = wt.Path
					break
				}
			}

			// If no is_main:true found, use path matching as fallback
			if mainPath == "" && len(ws.Worktrees) > 0 {
				// Normalize paths for comparison (handle case sensitivity)
				wsPathNorm := strings.ToLower(filepath.Clean(ws.Path))
				for _, wt := range ws.Worktrees {
					wtPathNorm := strings.ToLower(filepath.Clean(wt.Path))
					if wsPathNorm == wtPathNorm {
						mainPath = wt.Path
						break
					}
				}
				// If still no match, use the first worktree as a last resort
				if mainPath == "" {
					mainPath = ws.Worktrees[0].Path
				}
			}

			if mainPath != "" {
				result[ws.Name] = mainPath
			}
		}
		return result, nil
	}

	cmd := exec.CommandContext(ctx, "grove", "ws", "list", "--json")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return make(map[string]string), nil
	}
	var workspaces []WorkspaceInfo
	if err := json.Unmarshal(stdout.Bytes(), &workspaces); err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for _, ws := range workspaces {
		// Find the main worktree - prioritize is_main:true, but fall back to path matching
		mainPath := ""
		for _, wt := range ws.Worktrees {
			if wt.IsMain {
				mainPath = wt.Path
				break
			}
		}

		// If no is_main:true found, use path matching as fallback
		if mainPath == "" && len(ws.Worktrees) > 0 {
			// Normalize paths for comparison (handle case sensitivity)
			wsPathNorm := strings.ToLower(filepath.Clean(ws.Path))
			for _, wt := range ws.Worktrees {
				wtPathNorm := strings.ToLower(filepath.Clean(wt.Path))
				if wsPathNorm == wtPathNorm {
					mainPath = wt.Path
					break
				}
			}
			// If still no match, use the first worktree as a last resort
			if mainPath == "" {
				mainPath = ws.Worktrees[0].Path
			}
		}

		if mainPath != "" {
			result[ws.Name] = mainPath
		}
	}
	return result, nil
}

func parseGitmodules(gitmodulesPath string) (map[string]string, error) {
	file, err := os.Open(gitmodulesPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	submodules := make(map[string]string)
	var currentName string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[submodule") {
			start, end := strings.Index(line, "\""), strings.LastIndex(line, "\"")
			if start != -1 && end != -1 && start < end {
				currentName = line[start+1 : end]
			}
		} else if strings.HasPrefix(line, "path =") && currentName != "" {
			submodules[currentName] = strings.TrimSpace(strings.TrimPrefix(line, "path ="))
		}
	}
	return submodules, scanner.Err()
}