package daemonstream

import (
	"context"
	"encoding/json"

	tea "github.com/charmbracelet/bubbletea"
	grovelogging "github.com/grovetools/core/logging"
	"github.com/grovetools/core/pkg/daemon"
	"github.com/grovetools/core/tui/theme"
)

// AttachAgentPaneMsg is produced when the daemon broadcasts an attach_agent_pane SSE event.
type AttachAgentPaneMsg struct {
	JobID     string            `json:"job_id"`
	PlanName  string            `json:"plan_name"`
	JobTitle  string            `json:"job_title"`
	PtyID     string            `json:"pty_id"`
	WorkDir   string            `json:"work_dir"`
	Env       map[string]string `json:"env,omitempty"`
	AutoSplit bool              `json:"auto_split"`
}

// ThemeChangedMsg is produced when the daemon broadcasts a theme_changed SSE
// event, or when an initial snapshot after (re)connect carries the current
// theme. By the time an embedding TUI receives it, HandleUpdate has already
// re-themed the process via theme.SetTheme (a no-op when GROVE_THEME pins the
// theme), so the TUI only needs to rebuild any cached styles and repaint.
type ThemeChangedMsg struct {
	Payload daemon.ThemeChangedPayload
}

// StreamReadyMsg signals that the SSE subscription is established.
type StreamReadyMsg struct {
	Ch <-chan daemon.StateUpdate
	// Caps reports what the connected daemon honored of the requested
	// StreamOptions. The zero value means a daemon that predates stream
	// hardening: no sequence numbers, no replay, no server-side filtering —
	// so a TUI that filtered server-side must filter locally too.
	Caps daemon.StreamCapabilities
}

// StreamGapMsg is produced when the daemon reports it could not honor a
// resume cursor. The subscription stays live; the consumer must reconcile
// its own derived state (re-fetch what it cares about) rather than assume
// the events it missed simply did not happen.
type StreamGapMsg struct {
	Gap daemon.StreamGap
}

// StreamErrorMsg signals an SSE stream error or closure.
type StreamErrorMsg struct {
	Err error
}

// StateMsg carries a single SSE update from the daemon.
type StateMsg struct {
	Update daemon.StateUpdate
}

// StartStreamCmd opens the daemon SSE subscription.
func StartStreamCmd(daemonClient daemon.Client) tea.Cmd {
	return StartStreamCmdWithOptions(daemonClient, daemon.StreamOptions{})
}

// StartStreamCmdWithOptions opens the subscription with a resume cursor and/or
// a server-side type filter. The resulting StreamReadyMsg carries the
// daemon's actual capabilities: a TUI that narrowed the stream with Types must
// check Caps.TypeFilter and keep its local switch, because an older daemon
// answers with the full firehose.
func StartStreamCmdWithOptions(daemonClient daemon.Client, opts daemon.StreamOptions) tea.Cmd {
	ulog := grovelogging.NewUnifiedLogger("daemonstream")
	return func() tea.Msg {
		if daemonClient == nil || !daemonClient.IsRunning() {
			ulog.Debug("Daemon not running, skipping SSE stream").StructuredOnly().Log(context.Background())
			return nil
		}

		ctx := context.Background()
		ch, caps, err := daemonClient.StreamStateWithOptions(ctx, opts)
		if err != nil {
			ulog.Warn("Failed to connect daemon SSE stream").
				Field("error", err.Error()).StructuredOnly().Log(ctx)
			return StreamErrorMsg{Err: err}
		}

		ulog.Info("Connected to daemon SSE stream").
			Field("sequenced", caps.Sequenced).
			Field("replay", caps.Replay).
			Field("type_filter", caps.TypeFilter).
			StructuredOnly().Log(ctx)
		return StreamReadyMsg{Ch: ch, Caps: caps}
	}
}

// WaitForNextMsg blocks on the SSE channel for the next update.
func WaitForNextMsg(ch <-chan daemon.StateUpdate) tea.Cmd {
	return func() tea.Msg {
		if ch == nil {
			return nil
		}
		update, ok := <-ch
		if !ok {
			return StreamErrorMsg{Err: nil}
		}
		return StateMsg{Update: update}
	}
}

// HandleUpdate processes an SSE update and returns a tea.Cmd if it contains
// an attach_agent_pane event, a theme change, or a stream gap.
func HandleUpdate(update daemon.StateUpdate) tea.Cmd {
	if payload, ok := daemon.ParseThemeChanged(update); ok {
		return handleThemeChanged(payload)
	}

	if gap, ok := daemon.ParseStreamGap(update); ok {
		return handleStreamGap(gap)
	}

	if update.UpdateType != "attach_agent_pane" {
		return nil
	}

	ulog := grovelogging.NewUnifiedLogger("daemonstream")

	data, err := json.Marshal(update.Payload)
	if err != nil {
		return nil
	}
	var msg AttachAgentPaneMsg
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil
	}

	ulog.Info("Received attach_agent_pane event").
		Field("job_id", msg.JobID).
		Field("pty_id", msg.PtyID).
		StructuredOnly().Log(context.Background())

	return func() tea.Msg { return msg }
}

// handleStreamGap surfaces a resume gap so the embedding TUI can reconcile.
// It deliberately does NOT tear the subscription down: the stream stays live
// and correct from here on, and the daemon has already re-sent the initial
// snapshot. What is lost is the individual events in between.
func handleStreamGap(gap *daemon.StreamGap) tea.Cmd {
	grovelogging.NewUnifiedLogger("daemonstream").
		Info("Daemon stream reported a resume gap; reconciling").
		Field("reason", gap.Reason).
		Field("since", gap.Since).
		Field("oldest", gap.Oldest).
		Field("current", gap.Current).
		StructuredOnly().Log(context.Background())

	msg := StreamGapMsg{Gap: *gap}
	return func() tea.Msg { return msg }
}

// handleThemeChanged applies a daemon theme change to the running process and
// surfaces a ThemeChangedMsg so the embedding TUI can restyle. SetTheme
// resolves aliases and family names itself and self-no-ops when GROVE_THEME
// pins the theme for this process.
func handleThemeChanged(payload *daemon.ThemeChangedPayload) tea.Cmd {
	ulog := grovelogging.NewUnifiedLogger("daemonstream")

	if err := theme.SetTheme(payload.Name); err != nil {
		ulog.Warn("Ignoring theme change for unknown theme").
			Field("theme", payload.Name).
			Field("error", err.Error()).
			StructuredOnly().Log(context.Background())
		return nil
	}

	ulog.Info("Applied daemon theme change").
		Field("theme", payload.Name).
		Field("family", payload.Family).
		StructuredOnly().Log(context.Background())

	msg := ThemeChangedMsg{Payload: *payload}
	return func() tea.Msg { return msg }
}
