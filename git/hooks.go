package git

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

const hookScriptTemplate = `#!/bin/sh
# Grove git hook - {{.HookName}}
# Auto-generated, do not edit directly

GROVE_BIN="{{.GroveBinary}}"

# Check if Grove is installed
if ! command -v "$GROVE_BIN"github.com/mattsolo1/grove-core/dev/null 2>&1; then
    echo "Grove not found. Skipping {{.HookName}} hook."
    exit 0
fi

# Get git information
OLD_REF=$1
NEW_REF=$2
BRANCH_SWITCH=$3

# Only run on branch switches
if [ "$BRANCH_SWITCH" = "1" ] || [ "$OLD_REF" != "$NEW_REF" ]; then
    echo "Grove: Detected branch change"
    
    # Get old and new branch names
    OLD_BRANCH=$(git name-rev --name-only "$OLD_REF" 2>/dev/null | sed 's/remotes\/origin\///')
    NEW_BRANCH=$(git symbolic-ref --short HEAD 2>/dev/null)
    
    echo "Grove: Switching from $OLD_BRANCH to $NEW_BRANCH"
    
    # Run Grove branch change handler
    cd "$(git rev-parse --show-toplevel)"
    "$GROVE_BIN" internal branch-changed --from "$OLD_BRANCH" --to "$NEW_BRANCH"
fi
`

const preCommitHookTemplate = `#!/bin/sh
# Grove git hook - pre-commit
# Auto-generated, do not edit directly

GROVE_BIN="{{.GroveBinary}}"

# Check if Grove is installed
if ! command -v "$GROVE_BIN"github.com/mattsolo1/grove-core/dev/null 2>&1; then
    exit 0
fi

# Check if Grove is running
if "$GROVE_BIN" status --quiet 2>/dev/null; then
    echo "Grove: Services are running. Checking for issues..."
    
    # Run any pre-commit checks (e.g., linting in containers)
    # This is extensible based on grove.yml configuration
    "$GROVE_BIN" internal pre-commit-check
fi
`

// HookManager manages git hooks for Grove
type HookManager struct {
	groveBinary string
}

// Ensure it implements the interface
var _ HookProvider = (*HookManager)(nil)

// NewHookManager creates a new hook manager
func NewHookManager(groveBinary string) *HookManager {
	if groveBinary == "" {
		groveBinary = "grove"
	}
	return &HookManager{
		groveBinary: groveBinary,
	}
}

// InstallHooks installs Grove git hooks
func (m *HookManager) InstallHooks(ctx context.Context, repoPath string) error {
	hooksDir := filepath.Join(repoPath, ".git", "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return fmt.Errorf("create hooks directory: %w", err)
	}

	hooks := map[string]string{
		"post-checkout": hookScriptTemplate,
		"post-merge":    hookScriptTemplate,
		"pre-commit":    preCommitHookTemplate,
	}

	for hookName, templateContent := range hooks {
		if err := m.installHook(hooksDir, hookName, templateContent); err != nil {
			return fmt.Errorf("install %s hook: %w", hookName, err)
		}
	}

	return nil
}

// UninstallHooks removes Grove git hooks
func (m *HookManager) UninstallHooks(ctx context.Context, repoPath string) error {
	hooksDir := filepath.Join(repoPath, ".git", "hooks")

	hooks := []string{"post-checkout", "post-merge", "pre-commit"}

	for _, hookName := range hooks {
		hookPath := filepath.Join(hooksDir, hookName)

		// Check if it's a Grove hook before removing
		if m.isGroveHook(hookPath) {
			if err := os.Remove(hookPath); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove %s hook: %w", hookName, err)
			}
		}
	}

	return nil
}

// installHook installs a single git hook
func (m *HookManager) installHook(hooksDir, hookName, templateContent string) error {
	hookPath := filepath.Join(hooksDir, hookName)

	// Check if hook already exists
	if _, err := os.Stat(hookPath); err == nil {
		// Check if it's a Grove hook
		if !m.isGroveHook(hookPath) {
			// Backup existing hook
			backupPath := hookPath + ".pre-grove"
			if err := os.Rename(hookPath, backupPath); err != nil {
				return fmt.Errorf("backup existing hook: %w", err)
			}
		}
	}

	// Generate hook content
	tmpl, err := template.New(hookName).Parse(templateContent)
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	data := struct {
		HookName    string
		GroveBinary string
	}{
		HookName:    hookName,
		GroveBinary: m.groveBinary,
	}

	if err := tmpl.Execute(&buf, data); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}

	// Write hook file with executable permissions
	// #nosec G306 - Git hooks need to be executable
	if err := os.WriteFile(hookPath, buf.Bytes(), 0755); err != nil {
		return fmt.Errorf("write hook file: %w", err)
	}

	return nil
}

// isGroveHook checks if a hook file is managed by Grove
func (m *HookManager) isGroveHook(hookPath string) bool {
	content, err := os.ReadFile(hookPath)
	if err != nil {
		return false
	}
	return bytes.Contains(content, []byte("Grove git hook"))
}