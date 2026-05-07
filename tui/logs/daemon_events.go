package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/grovetools/core/pkg/daemon"
)

// batchStateMsg delivers a parsed daemon state event and the continuation pump.
type batchStateMsg struct {
	log newLogMsg
	ctx context.Context
	ch  <-chan daemon.StateUpdate
}

// pumpStateMsg re-arms the state stream pump after a skipped event.
type pumpStateMsg struct {
	ctx context.Context
	ch  <-chan daemon.StateUpdate
}

// pumpFirstStateUpdate reads the first StateUpdate from the stream channel.
func (m *Model) pumpFirstStateUpdate(sCtx context.Context, ch <-chan daemon.StateUpdate) tea.Msg {
	select {
	case <-sCtx.Done():
		return nil
	case update, ok := <-ch:
		if !ok {
			return nil
		}
		msg := parseStateUpdate(update)
		if msg == nil {
			return pumpStateMsg{ctx: sCtx, ch: ch}
		}
		return batchStateMsg{log: *msg, ctx: sCtx, ch: ch}
	}
}

// pumpStateStream returns a tea.Cmd that reads the next StateUpdate from the channel.
func pumpStateStream(sCtx context.Context, ch <-chan daemon.StateUpdate) tea.Cmd {
	return func() tea.Msg {
		for {
			select {
			case <-sCtx.Done():
				return nil
			case update, ok := <-ch:
				if !ok {
					return nil
				}
				msg := parseStateUpdate(update)
				if msg == nil {
					continue
				}
				return batchStateMsg{log: *msg, ctx: sCtx, ch: ch}
			}
		}
	}
}

// parseStateUpdate converts a daemon StateUpdate into a synthetic newLogMsg
// that the TUI's existing log rendering can display. The component is set to
// "daemon.<UpdateType>" and the msg field is a human-readable summary ported
// from the groved monitor logic.
func parseStateUpdate(update daemon.StateUpdate) *newLogMsg {
	level, summary, fields := classifyStateUpdate(update)
	if summary == "" {
		return nil
	}

	data := map[string]interface{}{
		"time":        time.Now().Format(time.RFC3339),
		"level":       level,
		"component":   "daemon." + update.UpdateType,
		"msg":         summary,
		"update_type": update.UpdateType,
	}
	if update.Source != "" {
		data["source"] = update.Source
	}
	for k, v := range fields {
		data[k] = v
	}

	// Serialize the full update into rawData so the JSON viewer (J key) works.
	if raw, err := json.Marshal(update); err == nil {
		var fullPayload map[string]interface{}
		if json.Unmarshal(raw, &fullPayload) == nil {
			data["_state_update"] = fullPayload
		}
	}

	return &newLogMsg{
		workspace: "daemon",
		data:      data,
	}
}

// classifyStateUpdate returns the log level, summary string, and extra fields
// for a given StateUpdate. Returns empty summary for events that should be skipped.
func classifyStateUpdate(update daemon.StateUpdate) (string, string, map[string]interface{}) {
	switch update.UpdateType {
	case "initial":
		return "info", fmt.Sprintf("Connected (%d workspaces)", len(update.Workspaces)), map[string]interface{}{
			"workspaces": len(update.Workspaces),
		}

	case "workspaces":
		source := update.Source
		if source == "" {
			source = "unknown"
		}
		fields := map[string]interface{}{
			"source":     source,
			"workspaces": len(update.Workspaces),
		}
		if update.Scanned > 0 && update.Scanned != len(update.Workspaces) {
			fields["scanned"] = update.Scanned
		}
		return "info", fmt.Sprintf("Workspace discovery (%s, %d found)", source, len(update.Workspaces)), fields

	case "sessions":
		var running, pending, flow, opencode int
		for _, s := range update.Sessions {
			switch s.Type {
			case "opencode_session":
				opencode++
			case "interactive_agent", "agent", "oneshot", "chat", "headless_agent", "shell":
				flow++
			default:
			}
			if s.Status == "running" {
				running++
			} else if s.Status == "pending_user" || s.Status == "idle" {
				pending++
			}
		}
		return "info", fmt.Sprintf("Sessions: %d total, %d running, %d pending", len(update.Sessions), running, pending), map[string]interface{}{
			"total":   len(update.Sessions),
			"running": running,
			"pending": pending,
			"flow":    flow,
		}

	case "focus":
		return "info", fmt.Sprintf("Focus update (%d workspaces)", update.Scanned), map[string]interface{}{
			"workspaces": update.Scanned,
		}

	case "config_reload":
		file := update.ConfigFile
		if file == "" {
			file = "unknown"
		}
		return "info", fmt.Sprintf("Config reload: %s", file), map[string]interface{}{
			"file": file,
		}

	case "watcher_status":
		if p, ok := update.Payload.(map[string]interface{}); ok {
			return "info", "Watcher status", p
		}
		return "info", "Watcher status", nil

	case "skill_sync":
		if p, ok := update.Payload.(map[string]interface{}); ok {
			if errStr, _ := p["error"].(string); errStr != "" {
				return "error", fmt.Sprintf("Skill sync error: %s", errStr), p
			}
			return "info", "Skill sync", p
		}
		return "info", "Skill sync", nil

	case "session":
		if p, ok := update.Payload.(map[string]interface{}); ok {
			if _, hasNativeID := p["native_id"]; hasNativeID {
				return "info", "Session confirmed", p
			}
			if status, _ := p["status"].(string); status != "" {
				return "info", fmt.Sprintf("Session status: %s", status), p
			}
			if outcome, _ := p["outcome"].(string); outcome != "" {
				return "warn", fmt.Sprintf("Session ended: %s", outcome), p
			}
			if title, _ := p["title"].(string); title != "" {
				return "info", fmt.Sprintf("Session intent: %s", title), p
			}
		}
		return "info", "Session event", nil

	case "workspaces_delta":
		return "info", fmt.Sprintf("Workspace delta (%d changes)", len(update.WorkspaceDeltas)), map[string]interface{}{
			"deltas": len(update.WorkspaceDeltas),
		}

	case "session_status":
		if p, ok := update.Payload.(map[string]interface{}); ok {
			return "info", "Session status change", p
		}
		return "info", "Session status change", nil

	case "session_end":
		if p, ok := update.Payload.(map[string]interface{}); ok {
			return "warn", "Session ended", p
		}
		return "warn", "Session ended", nil

	case "job_submitted", "job_started", "job_completed", "job_failed":
		level := "info"
		if update.UpdateType == "job_failed" {
			level = "error"
		}
		if p, ok := update.Payload.(map[string]interface{}); ok {
			return level, fmt.Sprintf("Job %s", update.UpdateType[4:]), p
		}
		return level, fmt.Sprintf("Job %s", update.UpdateType[4:]), nil

	case "note_event":
		if p, ok := update.Payload.(map[string]interface{}); ok {
			return "info", "Note event", p
		}
		return "info", "Note event", nil

	case "memory_index":
		if p, ok := update.Payload.(map[string]interface{}); ok {
			return "info", "Memory index update", p
		}
		return "info", "Memory index update", nil

	case "plans":
		return "info", "Plans update", nil

	default:
		return "info", fmt.Sprintf("Event: %s", update.UpdateType), nil
	}
}
