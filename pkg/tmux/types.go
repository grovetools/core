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
}

// Window holds detailed information about a tmux window.
type Window struct {
	ID       string `json:"id"`
	Index    int    `json:"index"`
	Name     string `json:"name"`
	IsActive bool   `json:"is_active"`
	Command  string `json:"command"` // Active pane's command
}