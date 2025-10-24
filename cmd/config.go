package cmd

import (
	"fmt"
	"os"

	"github.com/mattsolo1/grove-core/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func NewConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config-layers",
		Short: "Display the layered configuration for the current context",
		Long: `Shows how the final configuration is built by merging layers:
1. Global config (~/.config/grove/grove.yml)
2. Ecosystem config (parent grove.yml with workspaces, if in an ecosystem)
3. Project config (grove.yml)
4. Override files (grove.override.yml)
This is useful for debugging configuration issues.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get current directory: %w", err)
			}

			layered, err := config.LoadLayered(cwd)
			if err != nil {
				return fmt.Errorf("failed to load layered config: %w", err)
			}

			printLayer := func(title string, path string, cfg *config.Config) {
				if cfg == nil {
					return
				}
				fmt.Printf("--- # %s\n", title)
				if path != "" {
					fmt.Printf("# Source: %s\n", path)
				}
				data, _ := yaml.Marshal(cfg)
				fmt.Println(string(data))
			}

			printLayer("GLOBAL CONFIG", layered.FilePaths[config.SourceGlobal], layered.Global)
			printLayer("ECOSYSTEM CONFIG", layered.FilePaths[config.SourceEcosystem], layered.Ecosystem)
			printLayer("PROJECT CONFIG", layered.FilePaths[config.SourceProject], layered.Project)
			for _, override := range layered.Overrides {
				printLayer("OVERRIDE CONFIG", override.Path, override.Config)
			}
			printLayer("FINAL MERGED CONFIG", "", layered.Final)

			return nil
		},
	}
	return cmd
}
