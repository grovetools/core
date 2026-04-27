package workspace

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SetupGoWorkspaceForWorktree checks if the current project uses Go workspaces
// and if so, creates an appropriate go.work file in the worktree.
func SetupGoWorkspaceForWorktree(worktreePath, gitRoot string) error {
	goModPath := filepath.Join(gitRoot, "go.mod")
	if _, err := os.Stat(goModPath); os.IsNotExist(err) {
		return nil // Not a Go project
	}

	// Start searching from the parent of gitRoot to avoid finding the worktree's
	// own go.work file (which we're about to overwrite).
	config, err := FindRootGoWorkspace(filepath.Dir(gitRoot))
	if err != nil || config == nil {
		return nil // No go.work file found, nothing to do
	}

	content := GenerateWorktreeGoWork(config)

	worktreeGoWorkPath := filepath.Join(worktreePath, "go.work")
	return os.WriteFile(worktreeGoWorkPath, []byte(content), 0o644) //nolint:gosec // go.work file is not sensitive
}

// FindRootGoWorkspace searches for a go.work file by walking up the directory tree.
func FindRootGoWorkspace(startPath string) (*GoWorkspaceConfig, error) {
	absPath, err := filepath.Abs(startPath)
	if err != nil {
		return nil, err
	}

	currentPath := absPath
	for {
		goWorkPath := filepath.Join(currentPath, "go.work")
		if _, err := os.Stat(goWorkPath); err == nil {
			config := &GoWorkspaceConfig{
				RootGoWorkPath: goWorkPath,
				WorkspaceRoot:  currentPath,
			}
			if err := parseGoWork(goWorkPath, config); err != nil {
				return nil, err
			}
			return config, nil
		}
		parent := filepath.Dir(currentPath)
		if parent == currentPath {
			break
		}
		currentPath = parent
	}
	return nil, nil
}

type GoWorkspaceConfig struct {
	RootGoWorkPath string
	WorkspaceRoot  string
	GoVersion      string
	ModulePaths    []string
}

func parseGoWork(path string, config *GoWorkspaceConfig) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	inUseBlock := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.HasPrefix(line, "go ") {
			config.GoVersion = line
			continue
		}
		if line == "use (" {
			inUseBlock = true
			continue
		}
		if inUseBlock && line == ")" {
			inUseBlock = false
			continue
		}
		if inUseBlock {
			config.ModulePaths = append(config.ModulePaths, line)
		} else if strings.HasPrefix(line, "use ") {
			config.ModulePaths = append(config.ModulePaths, strings.TrimPrefix(line, "use "))
		}
	}
	return scanner.Err()
}

func GenerateWorktreeGoWork(config *GoWorkspaceConfig) string {
	var sb strings.Builder
	sb.WriteString(config.GoVersion + "\n\n")
	sb.WriteString("use (\n")
	sb.WriteString("\t.\n")
	for _, modulePath := range config.ModulePaths {
		sb.WriteString(fmt.Sprintf("\t%s\n", filepath.Join(config.WorkspaceRoot, modulePath)))
	}
	sb.WriteString(")\n")
	return sb.String()
}
