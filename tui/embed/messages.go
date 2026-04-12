// Package embed defines the standard contract for grovetools TUIs that want
// to be embeddable inside a host application (such as the terminal multiplexer)
// while still being runnable as standalone CLI binaries.
//
// The contract is intentionally minimal: any tea.Model can be embeddable as long
// as it speaks the standard message types defined in this package. A host catches
// these messages to coordinate layout, focus, workspace context, and lifecycle.
// In standalone mode, StandaloneHost provides the same translations so each CLI
// binary becomes a thin shim instead of duplicating Bubble Tea boilerplate.
package embed

import (
	"github.com/grovetools/core/pkg/workspace"
)

// DoneMsg is emitted by a sub-TUI when its primary lifecycle completes.
// Sub-TUIs should return this via a tea.Cmd instead of calling tea.Quit directly,
// so the host can decide whether to close the panel, advance a workflow, or extract
// the result. Result carries any value the sub-TUI wants to surface to its caller.
type DoneMsg struct {
	Result any
	Err    error
}

// FocusMsg informs a sub-TUI that it has gained focus in the host layout.
type FocusMsg struct{}

// BlurMsg informs a sub-TUI that it has lost focus in the host layout.
type BlurMsg struct{}

// SetWorkspaceMsg informs a workspace-scoped sub-TUI to repoint at a new workspace.
type SetWorkspaceMsg struct {
	Node *workspace.WorkspaceNode
}

// EditRequestMsg is emitted by a sub-TUI when it wants the host to open a file
// in an external editor. This replaces the previous IPC anti-pattern of writing
// edit requests to /tmp files.
type EditRequestMsg struct {
	Path string
}

// EditFinishedMsg is sent by the host back to the sub-TUI once the editor closes,
// signaling that the file may have changed and the sub-TUI should refresh.
type EditFinishedMsg struct {
	Err error
}

// PreviewRequestMsg is emitted by a sub-TUI when it wants the host to preview
// a file (e.g., open it in a side pane or split) without transferring focus
// from the sub-TUI. Hosts that don't support previewing should treat this as
// a no-op rather than as an EditRequestMsg.
type PreviewRequestMsg struct {
	Path string
}

// OpenAgentSessionMsg is emitted by a sub-TUI when it wants the host to
// open or focus an interactive agent session as a new panel. SessionID
// uniquely identifies the daemon-tracked session; the host resolves it
// against its session cache to find the working directory and tmux
// target, then either focuses the existing agent panel or spawns a new
// one via the AgentPanelFactory.
//
// This is the host-routed alternative to the standalone TUI's
// "tea.Quit + tmux switch-client" behavior. Hosts that don't know about
// agent sessions should treat the message as a no-op.
type OpenAgentSessionMsg struct {
	SessionID string
}

// SwitchTabMsg requests that the host pager activate a different tab.
// Intercepted by core/tui/components/pager; no-op for hosts that don't
// use it. When TabID is non-empty the pager resolves the target by
// matching against PageWithID implementations; otherwise TabIndex is
// used as a positional fallback. Out-of-range indices are silently
// ignored.
type SwitchTabMsg struct {
	TabID    string // human-readable tab ID (e.g. "stats", "jobs")
	TabIndex int    // positional fallback, used when TabID is empty
}

// NavigateMsg requests the host navigate to a specific panel and
// optionally a specific tab within that panel. The terminal host
// intercepts this, switches panels, and forwards SwitchTabMsg to the
// newly focused panel when TabID is non-empty.
type NavigateMsg struct {
	PanelID string // panel ID (e.g. "context", "flow", "skills")
	TabID   string // tab within the panel; empty = default tab
}

// CloseRequestMsg is emitted by a sub-TUI to request closure from the host.
// Hosts may intercept this to confirm with the user before closing.
type CloseRequestMsg struct{}

// CloseConfirmMsg is sent by the host to confirm closure (or emitted by a sub-TUI
// to force closure without confirmation).
type CloseConfirmMsg struct{}

// AgentSplitAction enumerates the open/close actions for SplitAgentRequestMsg.
type AgentSplitAction int

const (
	AgentSplitOpen  AgentSplitAction = iota
	AgentSplitClose
)

// SplitEditorRequestMsg is emitted by a sub-TUI (or pane) to request that
// the host open a file in a new sibling editor pane. In standalone mode the
// host wrapper dynamically injects an embedded neovim pane; in hosted mode
// (groveterm) the host may create a native Ghostty PTY split instead.
type SplitEditorRequestMsg struct {
	Path string // file to open
	Line int    // optional line number to jump to (0 = don't jump)
}

// SplitEditorClosedMsg is sent when the editor pane created by a
// SplitEditorRequestMsg is closed (e.g. user typed :q). The host removes
// the editor pane and may notify the originator.
type SplitEditorClosedMsg struct {
	Path string
}

// SplitEditorCloseRequestMsg is emitted by a sub-TUI to request that the
// host close an ephemeral editor split that was previously opened via
// SplitEditorRequestMsg. This allows the sub-TUI to tear down the editor
// split programmatically (e.g. when the user switches to a different detail
// pane) without waiting for the editor process to exit on its own.
type SplitEditorCloseRequestMsg struct{}

// SplitAgentRequestMsg is emitted by a sub-TUI to request that the host
// split the current pane and display the native PTY agent panel for the
// given JobID alongside the emitting panel. AgentSplitClose reverses the
// split, removing the agent pane from the BSP tree (but keeping it alive
// in the rail).
type SplitAgentRequestMsg struct {
	JobID  string
	Action AgentSplitAction
}
