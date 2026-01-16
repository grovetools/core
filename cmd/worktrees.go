package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/grovetools/core/cli"
	"github.com/grovetools/core/git"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/spf13/cobra"
)

// Styles for worktree display
var (
	worktreeHeaderStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("99")).
		MarginTop(1).
		MarginBottom(0)

	worktreePathStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))

	worktreeBranchStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("33")).
		Bold(true)

	worktreeCleanStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("34"))

	worktreeDirtyStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("208"))

	worktreeErrorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("196"))

	worktreeBoxStyle = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		MarginLeft(2)
)

// NewWorktreesCmd creates the `worktrees` command
func NewWorktreesCmd() *cobra.Command {
	cmd := cli.NewStandardCommand(
		"worktrees",
		"Manage git worktrees across workspaces",
	)
	cmd.Long = `Manage and view git worktrees across all workspaces in the ecosystem.`

	cmd.AddCommand(newWorktreesListCmd())

	return cmd
}

func newWorktreesListCmd() *cobra.Command {
	cmd := cli.NewStandardCommand(
		"list",
		"Show git worktrees for all workspaces",
	)
	cmd.Long = `Display git worktrees for each workspace in the ecosystem with their status.
Only shows workspaces that have additional worktrees beyond the main one.`

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		logger := cli.GetLogger(cmd)

		// Discover all projects
		projects, err := workspace.GetProjects(logger)
		if err != nil {
			return fmt.Errorf("failed to discover workspaces: %w", err)
		}

		// Collect worktrees for each project
		type worktreeResult struct {
			name      string
			worktrees []git.WorktreeWithStatus
		}

		var results []worktreeResult
		for _, project := range projects {
			worktrees, err := git.ListWorktreesWithStatus(project.Path)
			if err != nil {
				logger.WithError(err).WithField("path", project.Path).Warn("Failed to list worktrees")
				continue
			}
			// Only include if there are additional worktrees
			if len(worktrees) > 1 {
				results = append(results, worktreeResult{
					name:      project.Name,
					worktrees: worktrees,
				})
			}
		}

		if len(results) == 0 {
			fmt.Println("No workspaces have additional worktrees.")
			return nil
		}

		// Sort by name
		sort.Slice(results, func(i, j int) bool {
			return results[i].name < results[j].name
		})

		// Display results
		for _, result := range results {
			header := worktreeHeaderStyle.Render(result.name)
			fmt.Println(header)

			var lines []string
			for _, wt := range result.worktrees {
				line := formatWorktreeLine(wt)
				lines = append(lines, line)
			}

			content := strings.Join(lines, "\n")
			boxed := worktreeBoxStyle.Render(content)
			fmt.Println(boxed)
		}

		return nil
	}

	return cmd
}

func formatWorktreeLine(wt git.WorktreeWithStatus) string {
	cwd, _ := os.Getwd()
	relPath, err := filepath.Rel(cwd, wt.Path)
	if err != nil {
		relPath = wt.Path
	}

	pathStr := worktreePathStyle.Render(relPath)

	branchStr := worktreeBranchStyle.Render(wt.Branch)
	if wt.Branch == "" {
		branchStr = worktreeBranchStyle.Render("(no branch)")
	}

	var statusStr string
	if wt.Status != nil {
		if wt.Status.IsDirty {
			counts := []string{}
			if wt.Status.StagedCount > 0 {
				counts = append(counts, fmt.Sprintf("S:%d", wt.Status.StagedCount))
			}
			if wt.Status.ModifiedCount > 0 {
				counts = append(counts, fmt.Sprintf("M:%d", wt.Status.ModifiedCount))
			}
			if wt.Status.UntrackedCount > 0 {
				counts = append(counts, fmt.Sprintf("?:%d", wt.Status.UntrackedCount))
			}
			statusStr = worktreeDirtyStyle.Render(fmt.Sprintf("● %s", strings.Join(counts, " ")))
		} else {
			statusStr = worktreeCleanStyle.Render("Clean")
		}
	} else {
		statusStr = worktreeErrorStyle.Render("Unknown")
	}

	return fmt.Sprintf("%-50s %-20s %s", pathStr, branchStr, statusStr)
}
