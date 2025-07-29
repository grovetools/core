package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// VersionInfo holds version information for a Grove component
type VersionInfo struct {
	Version   string
	Commit    string
	BuildDate string
	BuildArch string
}

// SetVersionTemplate sets a custom version template for a cobra command
func SetVersionTemplate(cmd *cobra.Command, info VersionInfo) {
	cmd.SetVersionTemplate(fmt.Sprintf(`{{.Name}} {{.Version}}
  Commit:    %s
  Built:     %s
  Arch:      %s
`, info.Commit, info.BuildDate, info.BuildArch))
}

// NewVersionCommand creates a standard version command
func NewVersionCommand(componentName string, info VersionInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: fmt.Sprintf("Print the version number of %s", componentName),
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("%s %s\n", componentName, info.Version)
			fmt.Printf("  Commit:    %s\n", info.Commit)
			fmt.Printf("  Built:     %s\n", info.BuildDate)
			fmt.Printf("  Arch:      %s\n", info.BuildArch)
		},
	}
}