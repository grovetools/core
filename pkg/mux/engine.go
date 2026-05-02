package mux

import (
	"context"
	"time"
)

// LaunchOptions describes a session+panes to create.
type LaunchOptions struct {
	SessionName      string
	WorkingDirectory string
	WindowName       string
	WindowIndex      int // Window index to position the window at (-1 = no specific index)
	Panes            []PaneOptions
}

// PaneOptions describes a single pane within a launch.
type PaneOptions struct {
	Command          string
	WorkingDirectory string
	SendKeys         string
	Env              map[string]string
}

// WindowInfo holds detailed information about a window.
type WindowInfo struct {
	ID       string
	Index    int
	Name     string
	IsActive bool
	Command  string
	PID      int
}

type SessionOption func(*sessionConfig)

type sessionConfig struct {
	WorkDir string
}

func WithWorkDir(dir string) SessionOption {
	return func(c *sessionConfig) {
		c.WorkDir = dir
	}
}

func applySessionOptions(opts []SessionOption) sessionConfig {
	var cfg sessionConfig
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}

type MuxEngine interface {
	CreateSession(ctx context.Context, name string, opts ...SessionOption) error
	KillSession(ctx context.Context, name string) error
	ListSessions(ctx context.Context) ([]SessionInfo, error)
	SessionExists(ctx context.Context, name string) (bool, error)
	StartServer(ctx context.Context, name string, opts ...SessionOption) error
	KillServer(ctx context.Context, name string) error
	ListServers(ctx context.Context) ([]ServerInfo, error)
	SendKeys(ctx context.Context, target string, keys ...string) error
	CapturePane(ctx context.Context, target string) (string, error)
	WaitForIdle(ctx context.Context, target string, timeout time.Duration) error
	WaitForText(ctx context.Context, target string, pattern string, timeout time.Duration) (string, error)
	Run(ctx context.Context, target string, command string, timeout time.Duration) (string, error)
	SplitWindow(ctx context.Context, target string, horizontal bool) (string, error)
	ListPanes(ctx context.Context, sessionName string) ([]PaneInfo, error)
	NewWindow(ctx context.Context, sessionName string, windowName string, workDir string, detached bool) error
	GetSessionPID(ctx context.Context, sessionName string) (int, error)
	SwitchSession(ctx context.Context, name string, cwd string) error
	GetPanePID(ctx context.Context, target string) (int, error)
	GetCurrentSession(ctx context.Context) (string, error)
	SelectWindow(ctx context.Context, target string) error
	KillWindow(ctx context.Context, target string) error
	ListWindows(ctx context.Context, session string) ([]WindowInfo, error)
	PaneExists(ctx context.Context, target string) (bool, error)
	GetPaneCommand(ctx context.Context, target string) (string, error)
	GetSessionPath(ctx context.Context, session string) (string, error)
	WaitForSessionClose(ctx context.Context, session string, interval time.Duration) error
	GetCursorPosition(ctx context.Context, target string) (int, int, error)
	Launch(ctx context.Context, opts LaunchOptions) error
}

type MuxTUIEngine interface {
	MuxEngine
	OpenInEditorWindow(ctx context.Context, editorCmd, filePath, windowName string, windowIndex int, reset bool) error
	FocusOrRunCommandInWindow(ctx context.Context, cmd, windowName string, windowIndex int) error
	ClosePopup(ctx context.Context) error
	IsPopup(ctx context.Context) (bool, error)
}

type SessionInfo struct {
	Name        string
	ClientCount int
}

type ServerInfo struct {
	Name        string
	ClientCount int
}

type PaneInfo struct {
	ID                string
	Active            bool
	Idle              bool
	ForegroundProcess string
	Cwd               string
	PID               int
}
