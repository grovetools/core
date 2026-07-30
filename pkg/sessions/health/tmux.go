package health

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/tmux"
)

// CLITmuxProber answers WindowAlive by asking tmux. It is the reason
// tmux-hosted agents stop coming back UNKNOWN: TmuxTarget already names
// the window, we just never looked.
//
// Two shapes of attach point are understood:
//
//	"session:window" (TmuxTarget) — the session must exist AND still
//	                                carry that window (by name or index)
//	"session"        (TmuxKey)    — the session must exist
//
// A tmux error is never converted into "gone": WindowAlive returns the
// error, and Classify keeps such a session on UNKNOWN. Only a
// successful query that finds nothing is negative evidence.
type CLITmuxProber struct {
	// SocketFor returns the tmux server socket a session's window lives
	// on ("" for the default server). Injected because per-job socket
	// naming (flow's isolated agents get `flow-job-<id>`) is flow's
	// convention, not core's. Nil means "always the default server".
	SocketFor func(s *models.Session) string
}

// WindowAlive reports whether the session's tmux window still exists.
func (p *CLITmuxProber) WindowAlive(ctx context.Context, s *models.Session) (bool, error) {
	if s == nil {
		return false, fmt.Errorf("nil session")
	}
	sessionName, windowRef := splitTmuxTarget(s)
	if sessionName == "" {
		return false, fmt.Errorf("no tmux attach point recorded")
	}

	socket := ""
	if p.SocketFor != nil {
		socket = p.SocketFor(s)
	}
	client, err := newTmuxClient(socket)
	if err != nil {
		return false, err
	}

	exists, err := client.SessionExists(ctx, sessionName)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	if windowRef == "" {
		return true, nil
	}

	windows, err := client.ListWindowsDetailed(ctx, sessionName)
	if err != nil {
		return false, err
	}
	idx, isIndex := parseWindowIndex(windowRef)
	for _, w := range windows {
		if w.Name == windowRef {
			return true, nil
		}
		if isIndex && w.Index == idx {
			return true, nil
		}
	}
	return false, nil
}

func newTmuxClient(socket string) (*tmux.Client, error) {
	if socket != "" {
		return tmux.NewClientWithSocket(socket)
	}
	return tmux.NewClient()
}

// splitTmuxTarget resolves a session's attach point into a tmux session
// name and an optional window reference. TmuxTarget ("session:window")
// wins over TmuxKey (a bare session name) when both are recorded.
func splitTmuxTarget(s *models.Session) (sessionName, windowRef string) {
	if s.TmuxTarget != "" {
		if name, window, ok := strings.Cut(s.TmuxTarget, ":"); ok {
			return name, window
		}
		return s.TmuxTarget, ""
	}
	return s.TmuxKey, ""
}

func parseWindowIndex(ref string) (int, bool) {
	n, err := strconv.Atoi(ref)
	return n, err == nil
}
