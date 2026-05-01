package mux

import (
	"fmt"

	"github.com/grovetools/core/pkg/workspace"
)

// GenerateSessionName creates a unique session name for the given working directory.
// It resolves the project using the workspace model (notebook-aware) and formats
// it with underscores as delimiters.
func GenerateSessionName(workDir string) (string, error) {
	projInfo, err := resolveProjectForSessionNaming(workDir)
	if err != nil {
		return "", fmt.Errorf("failed to get project info for session naming: %w", err)
	}
	return projInfo.Identifier("_"), nil
}

func resolveProjectForSessionNaming(workDir string) (*workspace.WorkspaceNode, error) {
	if project, notebookRoot, _ := workspace.GetProjectFromNotebookPath(workDir); notebookRoot != "" && project != nil {
		return project, nil
	}
	return workspace.GetProjectByPath(workDir)
}
