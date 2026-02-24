package keybind

import (
	"context"
	"fmt"
	"strings"

	"github.com/grovetools/core/config"
)

// GroveCollector collects key bindings from grove.toml configuration.
type GroveCollector struct {
	cfg *config.Config
}

// NewGroveCollector creates a collector that reads from grove.toml.
func NewGroveCollector() *GroveCollector {
	return &GroveCollector{}
}

// NewGroveCollectorWithConfig creates a collector with a pre-loaded config.
func NewGroveCollectorWithConfig(cfg *config.Config) *GroveCollector {
	return &GroveCollector{cfg: cfg}
}

func (c *GroveCollector) Name() string {
	return "grove"
}

func (c *GroveCollector) Layer() Layer {
	// Grove bindings are typically in custom tmux tables
	return LayerTmuxCustomTable
}

func (c *GroveCollector) Collect(ctx context.Context) ([]Binding, error) {
	cfg := c.cfg
	if cfg == nil {
		var err error
		cfg, err = config.LoadDefault()
		if err != nil {
			return nil, nil // Config not available, return empty
		}
	}

	var bindings []Binding

	// Extract keys extension from config
	var keysExt KeysExtension
	if err := cfg.UnmarshalExtension("keys", &keysExt); err != nil {
		return nil, fmt.Errorf("failed to unmarshal keys extension: %w", err)
	}

	// Collect tmux popup bindings
	popupBindings := c.collectPopups(keysExt)
	bindings = append(bindings, popupBindings...)

	// Collect nav bindings
	navBindings := c.collectNav(keysExt)
	bindings = append(bindings, navBindings...)

	return bindings, nil
}

// collectPopups extracts tmux popup bindings from config.
func (c *GroveCollector) collectPopups(keysExt KeysExtension) []Binding {
	var bindings []Binding

	prefix := keysExt.Tmux.Prefix
	tableName := "grove-popups" // default table name

	// Determine the effective table based on prefix
	if prefix == "" {
		// Direct root bindings (no prefix)
		for action, popupCfg := range keysExt.Tmux.Popups {
			rawKeys := parseStringOrSlice(popupCfg.Key)
			for _, rawKey := range rawKeys {
				normalizedKey := Normalize(rawKey, "grove")
				bindings = append(bindings, Binding{
					Key:         normalizedKey,
					RawKey:      rawKey,
					Layer:       LayerTmuxRoot,
					Source:      "grove",
					Action:      c.getPopupCommand(action, popupCfg),
					Description: action,
					Provenance:  ProvenanceGrove,
					ConfigFile:  "grove.toml [keys.tmux.popups]",
				})
			}
		}
	} else {
		// Prefix-based bindings (custom table)
		for action, popupCfg := range keysExt.Tmux.Popups {
			rawKeys := parseStringOrSlice(popupCfg.Key)
			for _, rawKey := range rawKeys {
				normalizedKey := Normalize(rawKey, "grove")
				bindings = append(bindings, Binding{
					Key:         normalizedKey,
					RawKey:      rawKey,
					Layer:       LayerTmuxCustomTable,
					Source:      "grove",
					Action:      c.getPopupCommand(action, popupCfg),
					Description: action,
					Provenance:  ProvenanceGrove,
					ConfigFile:  "grove.toml [keys.tmux.popups]",
					TableName:   tableName,
				})
			}
		}

		// Also add the prefix key itself to root
		normalizedPrefix := Normalize(prefix, "grove")
		bindings = append(bindings, Binding{
			Key:         normalizedPrefix,
			RawKey:      prefix,
			Layer:       LayerTmuxRoot,
			Source:      "grove",
			Action:      fmt.Sprintf("switch-client -T %s", tableName),
			Description: "Enter grove popups table",
			Provenance:  ProvenanceGrove,
			ConfigFile:  "grove.toml [keys.tmux.prefix]",
		})
	}

	return bindings
}

// collectNav extracts nav-related bindings from config.
func (c *GroveCollector) collectNav(keysExt KeysExtension) []Binding {
	var bindings []Binding

	// Available keys for nav panes
	for _, rawKey := range keysExt.Nav.AvailableKeys {
		normalizedKey := Normalize(rawKey, "grove")
		bindings = append(bindings, Binding{
			Key:         normalizedKey,
			RawKey:      rawKey,
			Layer:       LayerTmuxRoot, // Nav pane keys are typically in root
			Source:      "grove-nav",
			Action:      "select-pane (nav)",
			Description: "Nav pane selection",
			Provenance:  ProvenanceGrove,
			ConfigFile:  "grove.toml [keys.nav.available_keys]",
		})
	}

	return bindings
}

// getPopupCommand returns the tmux command for a popup action.
func (c *GroveCollector) getPopupCommand(action string, popupCfg TmuxPopupConfig) string {
	if popupCfg.Command != "" {
		return popupCfg.Command
	}
	// Map known action names to commands
	if cmd, ok := TmuxCommandMap[action]; ok {
		return cmd
	}
	return action
}

// KeysExtension represents the [keys] block in grove.toml.
type KeysExtension struct {
	Tmux struct {
		Prefix string                     `yaml:"prefix,omitempty" toml:"prefix,omitempty"`
		Popups map[string]TmuxPopupConfig `yaml:"popups" toml:"popups"`
	} `yaml:"tmux" toml:"tmux"`
	Nav struct {
		Prefix        string   `yaml:"prefix,omitempty" toml:"prefix,omitempty"`
		AvailableKeys []string `yaml:"available_keys" toml:"available_keys"`
	} `yaml:"nav" toml:"nav"`
}

// TmuxPopupConfig defines the behavior of a tmux popup binding.
type TmuxPopupConfig struct {
	Key            interface{} `yaml:"key" toml:"key"`
	Command        string      `yaml:"command" toml:"command"`
	Style          string      `yaml:"style,omitempty" toml:"style,omitempty"`
	ExitOnComplete bool        `yaml:"exit_on_complete,omitempty" toml:"exit_on_complete,omitempty"`
}

// TmuxCommandMap maps action names to tmux commands.
var TmuxCommandMap = map[string]string{
	"flow_status":      "flow tmux status",
	"nb_tui":           "nb tmux tui",
	"session_switcher": "nav sz",
	"editor":           "core editor",
	"diffview":         "nav diffview",
	"nav_key_manager":  "nav km",
	"nav_history":      "nav history",
	"nav_windows":      "nav windows",
	"hooks_sessions":   "hooks sessions browse",
	"tend_sessions":    "tend sessions",
	"cx_view":          "cx view",
	"cx_stats":         "cx stats",
	"cx_list":          "cx list",
	"cx_edit":          "cx rules edit",
	"console":          "console",
}

// parseStringOrSlice converts interface{} to []string.
func parseStringOrSlice(val interface{}) []string {
	switch v := val.(type) {
	case string:
		return []string{v}
	case []interface{}:
		var res []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				res = append(res, s)
			}
		}
		return res
	case []string:
		return v
	default:
		return nil
	}
}

// GetGrovePrefix returns the configured grove prefix key from config.
func GetGrovePrefix(cfg *config.Config) string {
	var keysExt KeysExtension
	if err := cfg.UnmarshalExtension("keys", &keysExt); err != nil {
		return "C-g" // default
	}
	if keysExt.Tmux.Prefix != "" {
		return keysExt.Tmux.Prefix
	}
	return "C-g"
}

// GetGroveTableName returns the name of the grove popups table.
func GetGroveTableName() string {
	return "grove-popups"
}

// IsGroveBinding checks if a binding is managed by Grove.
func IsGroveBinding(b Binding) bool {
	return b.Provenance == ProvenanceGrove ||
		strings.Contains(b.Source, "grove") ||
		strings.Contains(b.Action, "grove") ||
		strings.Contains(b.Action, "flow ") ||
		strings.Contains(b.Action, "nb ") ||
		strings.Contains(b.Action, "nav ") ||
		strings.Contains(b.Action, "cx ") ||
		strings.Contains(b.Action, "tend ")
}
