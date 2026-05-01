package keybind

import (
	"context"
	"os"
	"path/filepath"

	"github.com/grovetools/tuimux"
	"github.com/pelletier/go-toml/v2"
)

type tuimuxFileConfig struct {
	Keys struct {
		Leader      string                                `toml:"leader"`
		LeaderBinds map[string]string                     `toml:"leader_binds"`
		Global      map[string]tuimux.GlobalBindingConfig `toml:"global"`
	} `toml:"keys"`
}

// TuimuxCollector collects keybindings from the tuimux config file.
type TuimuxCollector struct {
	configPath string
}

// NewTuimuxCollector creates a collector that reads ~/.config/tuimux/config.toml.
func NewTuimuxCollector() *TuimuxCollector {
	home, _ := os.UserHomeDir()
	return &TuimuxCollector{
		configPath: filepath.Join(home, ".config", "tuimux", "config.toml"),
	}
}

func (c *TuimuxCollector) Name() string {
	return "tuimux"
}

func (c *TuimuxCollector) Layer() Layer {
	return LayerTuimuxGlobal
}

func (c *TuimuxCollector) Collect(ctx context.Context) ([]Binding, error) {
	data, err := os.ReadFile(c.configPath)
	if err != nil {
		return nil, nil
	}

	var cfg tuimuxFileConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, nil
	}

	var bindings []Binding

	for key, gcfg := range cfg.Keys.Global {
		style := gcfg.Style
		if style == "" {
			style = "popup"
		}
		action := gcfg.Command
		if action == "" {
			action = style
		}

		provenance := ProvenanceUserConfig
		if isGroveCommand(action) {
			provenance = ProvenanceGrove
		}

		bindings = append(bindings, Binding{
			Key:        Normalize(key, "tuimux"),
			RawKey:     key,
			Layer:      LayerTuimuxGlobal,
			Source:     "tuimux",
			Action:     action,
			Provenance: provenance,
			ConfigFile: c.configPath,
		})
	}

	for action, key := range cfg.Keys.LeaderBinds {
		bindings = append(bindings, Binding{
			Key:        Normalize(key, "tuimux"),
			RawKey:     key,
			Layer:      LayerTuimuxLeader,
			Source:     "tuimux-leader",
			Action:     action,
			Provenance: ProvenanceUserConfig,
			ConfigFile: c.configPath,
		})
	}

	return bindings, nil
}

func isGroveCommand(action string) bool {
	groveCommands := []string{"grove", "flow", "nav", "nb", "cx", "tend", "hooks", "core", "console"}
	for _, cmd := range groveCommands {
		if len(action) >= len(cmd) && action[:len(cmd)] == cmd {
			if len(action) == len(cmd) || action[len(cmd)] == ' ' {
				return true
			}
		}
	}
	return false
}
