package models

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
