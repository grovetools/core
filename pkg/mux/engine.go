package mux

import (
	"context"
	"time"
)

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
}
