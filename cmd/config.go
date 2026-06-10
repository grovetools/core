package cmd

import (
	"fmt"
	"os"
	"regexp"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/grovetools/core/config"
)

// secretKeyPattern matches config keys whose values must never be printed in
// plaintext by config-layers.
var secretKeyPattern = regexp.MustCompile(`(?i)api_key|apikey|token|secret|password`)

// redactSecretString masks a secret value, keeping the last 4 characters as a
// hint when the value is long enough to not leak meaningful entropy.
func redactSecretString(s string) string {
	if len(s) >= 12 {
		return "****" + s[len(s)-4:]
	}
	return "[redacted]"
}

// redactSecrets walks a decoded YAML/JSON-like tree and masks every value
// whose key matches secretKeyPattern. It mutates and returns the node.
func redactSecrets(node interface{}) interface{} {
	switch v := node.(type) {
	case map[string]interface{}:
		for k, val := range v {
			if secretKeyPattern.MatchString(k) {
				if s, ok := val.(string); ok {
					if s != "" {
						v[k] = redactSecretString(s)
					}
				} else if val != nil {
					v[k] = "[redacted]"
				}
				continue
			}
			v[k] = redactSecrets(val)
		}
		return v
	case []interface{}:
		for i, item := range v {
			v[i] = redactSecrets(item)
		}
		return v
	default:
		return node
	}
}

// marshalRedacted renders a config layer as YAML with secret values masked.
func marshalRedacted(cfg *config.Config) ([]byte, error) {
	raw, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	var tree interface{}
	if err := yaml.Unmarshal(raw, &tree); err != nil {
		return nil, err
	}
	return yaml.Marshal(redactSecrets(tree))
}

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

			printLayer := func(title, path string, cfg *config.Config) {
				if cfg == nil {
					return
				}
				fmt.Printf("--- # %s\n", title)
				if path != "" {
					fmt.Printf("# Source: %s\n", path)
				}
				data, err := marshalRedacted(cfg)
				if err != nil {
					fmt.Printf("# Error rendering layer: %v\n", err)
					return
				}
				fmt.Println(string(data))
			}

			printLayer("GLOBAL CONFIG", layered.FilePaths[config.SourceGlobal], layered.Global)
			if layered.GlobalOverride != nil {
				printLayer("GLOBAL OVERRIDE CONFIG", layered.FilePaths[config.SourceGlobalOverride], layered.GlobalOverride.Config)
			}
			printLayer("ECOSYSTEM CONFIG", layered.FilePaths[config.SourceEcosystem], layered.Ecosystem)
			printLayer("PROJECT NOTEBOOK CONFIG", layered.FilePaths[config.SourceProjectNotebook], layered.ProjectNotebook)
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
