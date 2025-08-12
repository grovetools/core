package models

// RepoHookConfig represents the structure of .canopy.yaml
type RepoHookConfig struct {
	Hooks struct {
		OnStop []HookCommand `yaml:"on_stop"`
	} `yaml:"hooks"`
}

// HookCommand defines a command to be executed for a hook.
type HookCommand struct {
	Name    string `yaml:"name"`
	Command string `yaml:"command"`
	RunIf   string `yaml:"run_if,omitempty"` // "always" or "changes"
}
