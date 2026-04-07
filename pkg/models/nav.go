package models

// NavConfig represents the static portion of nav configuration that lives in
// the grove config files (e.g. grove.toml, keys.toml). Unlike NavSessionsFile,
// which is dynamic state mutated at runtime, NavConfig is loaded from the
// declarative config and exposed via the daemon so non-nav clients (terminal,
// studio) can resolve group prefix transitions without re-implementing the
// nav config loader.
type NavConfig struct {
	// Groups maps the group name to its static config. The "default" group
	// uses the top-level nav.prefix and is included here so callers do not
	// need a separate special case.
	Groups map[string]NavGroupConfig `json:"groups"`
}

// NavGroupConfig is the per-group static configuration exposed to clients.
// It currently carries only the prefix, which is what the leader-chord
// state machine needs to detect group prefix transitions.
type NavGroupConfig struct {
	Prefix string `json:"prefix"`
}

// NavSessionsFile represents the sessions file stored in ~/.local/state/grove/nav/sessions.yml.
// This is the dynamic binding state that the daemon and nav CLI share.
type NavSessionsFile struct {
	Sessions          map[string]NavSessionConfig `yaml:"sessions" json:"sessions"`
	LockedKeys        []string                    `yaml:"locked_keys,omitempty" json:"locked_keys,omitempty"`
	Groups            map[string]NavGroupState    `yaml:"groups,omitempty" json:"groups,omitempty"`
	LastAccessedGroup string                      `yaml:"last_accessed_group,omitempty" json:"last_accessed_group,omitempty"`
}

// NavGroupState holds the dynamic session state for a workspace group.
type NavGroupState struct {
	Sessions   map[string]NavSessionConfig `yaml:"sessions" json:"sessions"`
	LockedKeys []string                    `yaml:"locked_keys,omitempty" json:"locked_keys,omitempty"`
}

// NavSessionConfig defines the configuration for a single session mapped to a key.
// Supports both shorthand (o = "/path") and full table (o = { path = "/path" }) formats.
type NavSessionConfig struct {
	Path string `yaml:"path" toml:"path" json:"path"`
}

// UnmarshalTOML implements custom unmarshaling to support shorthand string format.
// Accepts both: o = "/path" and o = { path = "/path" }
func (t *NavSessionConfig) UnmarshalTOML(data interface{}) error {
	switch v := data.(type) {
	case string:
		t.Path = v
		return nil
	case map[string]interface{}:
		if path, ok := v["path"].(string); ok {
			t.Path = path
		}
		return nil
	default:
		return nil
	}
}

// UnmarshalYAML implements custom unmarshaling to support shorthand string format.
// Accepts both: o: "/path" and o: { path: "/path" }
func (t *NavSessionConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Try string first (shorthand)
	var path string
	if err := unmarshal(&path); err == nil {
		t.Path = path
		return nil
	}

	// Fall back to struct (full format)
	type plain NavSessionConfig
	return unmarshal((*plain)(t))
}
