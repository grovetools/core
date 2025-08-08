package conventional

import (
	"fmt"
	"strings"
	"time"
)

// Generate creates a markdown changelog from a list of commits for a new version.
func Generate(newVersion string, commits []*Commit) string {
	var builder strings.Builder

	// Header
	builder.WriteString(fmt.Sprintf("## %s (%s)\n\n", newVersion, time.Now().Format("2006-01-02")))

	// Group commits by type
	groupedCommits := make(map[string][]*Commit)
	for _, commit := range commits {
		groupedCommits[commit.Type] = append(groupedCommits[commit.Type], commit)
	}

	// Define the order of sections
	sectionOrder := map[string]string{
		"feat":     "Features",
		"fix":      "Bug Fixes",
		"perf":     "Performance Improvements",
		"refactor": "Code Refactoring",
		"docs":     "Documentation",
		"style":    "Styles",
		"test":     "Tests",
		"build":    "Build System",
		"ci":       "Continuous Integration",
		"chore":    "Chores",
	}

	// Write BREAKING CHANGES section first
	breakingChanges := []*Commit{}
	for _, commit := range commits {
		if commit.IsBreaking {
			breakingChanges = append(breakingChanges, commit)
		}
	}
	if len(breakingChanges) > 0 {
		builder.WriteString("### 💥 BREAKING CHANGES\n\n")
		for _, commit := range breakingChanges {
			builder.WriteString(formatCommitLine(commit))
		}
		builder.WriteString("\n")
	}

	// Write other sections
	for typeKey, title := range sectionOrder {
		if commits, ok := groupedCommits[typeKey]; ok && len(commits) > 0 {
			builder.WriteString(fmt.Sprintf("### %s\n\n", title))
			for _, commit := range commits {
				// Don't list breaking changes twice
				if !commit.IsBreaking {
					builder.WriteString(formatCommitLine(commit))
				}
			}
			builder.WriteString("\n")
		}
	}

	return builder.String()
}

func formatCommitLine(c *Commit) string {
	scope := ""
	if c.Scope != "" {
		scope = fmt.Sprintf("**%s:** ", c.Scope)
	}
	return fmt.Sprintf("* %s%s\n", scope, c.Subject)
}