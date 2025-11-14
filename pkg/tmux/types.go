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