package daemonstream

import (
	"context"
	"encoding/json"

	tea "github.com/charmbracelet/bubbletea"
	grovelogging "github.com/grovetools/core/logging"
	"github.com/grovetools/core/pkg/daemon"
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

// StreamReadyMsg signals that the SSE subscription is established.
type StreamReadyMsg struct {
	Ch <-chan daemon.StateUpdate
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
	ulog := grovelogging.NewUnifiedLogger("daemonstream")
	return func() tea.Msg {
		if daemonClient == nil || !daemonClient.IsRunning() {
			ulog.Debug("Daemon not running, skipping SSE stream").StructuredOnly().Log(context.Background())
			return nil
		}

		ctx := context.Background()
		ch, err := daemonClient.StreamState(ctx)
		if err != nil {
			ulog.Warn("Failed to connect daemon SSE stream").
				Field("error", err.Error()).StructuredOnly().Log(ctx)
			return StreamErrorMsg{Err: err}
		}

		ulog.Info("Connected to daemon SSE stream").StructuredOnly().Log(ctx)
		return StreamReadyMsg{Ch: ch}
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
// an attach_agent_pane event.
func HandleUpdate(update daemon.StateUpdate) tea.Cmd {
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
