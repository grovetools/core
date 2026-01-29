package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/grovetools/core/pkg/paths"
	"github.com/spf13/cobra"
)

// PathsOutput represents the XDG-compliant paths used by Grove.
type PathsOutput struct {
	ConfigDir string `json:"config_dir"`
	DataDir   string `json:"data_dir"`
	StateDir  string `json:"state_dir"`
	CacheDir  string `json:"cache_dir"`
	BinDir    string `json:"bin_dir"`
}

func NewPathsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "paths",
		Short: "Print the XDG-compliant paths used by Grove",
		Long: `Print the XDG-compliant paths used by Grove.

This command outputs the paths in JSON format by default, making it easy
to parse from scripts and other tools.

The paths follow the XDG Base Directory Specification:
- config_dir: Configuration files (grove.yml)
- data_dir: Persistent data (binaries, plugins, notebooks)
- state_dir: Runtime state (databases, logs, sessions)
- cache_dir: Temporary/regenerable data
- bin_dir: Grove binaries (subdirectory of data_dir)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			output := PathsOutput{
				ConfigDir: paths.ConfigDir(),
				DataDir:   paths.DataDir(),
				StateDir:  paths.StateDir(),
				CacheDir:  paths.CacheDir(),
				BinDir:    paths.BinDir(),
			}

			jsonData, err := json.MarshalIndent(output, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal paths to JSON: %w", err)
			}
			fmt.Println(string(jsonData))
			return nil
		},
	}

	return cmd
}
