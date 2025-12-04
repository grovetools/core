package tmux

type LaunchOptions struct {
	SessionName      string
	WorkingDirectory string
	WindowName       string
	WindowIndex      int // Window index to position the window at (-1 = no specific index)
	Panes            []PaneOptions
}

type PaneOptions struct {
	Command          string
	WorkingDirectory string
	SendKeys         string
	Env              map[string]string // Environment variables to set in the pane
}

// Window holds detailed information about a tmux window.
type Window struct {
	ID       string `json:"id"`
	Index    int    `json:"index"`
	Name     string `json:"name"`
	IsActive bool   `json:"is_active"`
	Command  string `json:"command"` // Active pane's command
	PID      int    `json:"pid"`     // Active pane's PID
}

// NewWindowOptions provides detailed options for creating a new window.
type NewWindowOptions struct {
	Target     string
	WindowName string
	Command    string
	WorkingDir string
	Env        []string // Environment variables in "KEY=VALUE" format
}