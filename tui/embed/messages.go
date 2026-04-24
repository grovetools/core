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
//
// Convention for embedded TUIs: never call os.Getwd() to resolve "the
// user's current workspace." Inside long-lived hosts like treemux, the
// process cwd is pinned to the host's launch directory and does not
// follow in-process workspace switches. Instead, store msg.Node.Path on
// the Model when this message arrives and use that tracked field for any
// workspace-relative path resolution. Seed the field at construction
// time from a config option (e.g. ActiveWorkspacePath) so the first
// render is correct even before the host broadcasts its first switch.
// Fall back to os.Getwd() only for standalone CLI invocations where no
// host will ever send SetWorkspaceMsg.
type SetWorkspaceMsg struct {
	Node *workspace.WorkspaceNode
}

// EditRequestMsg is emitted by a sub-TUI when it wants the host to open a file
// in an external editor. This replaces the previous IPC anti-pattern of writing
// edit requests to /tmp files.
type EditRequestMsg struct {
	Path string
}

// InlineEditRequestMsg is emitted by a sub-TUI when it wants the host to
// open a file in an editor *in place* — replacing the emitting panel's BSP
// node with an ephemeral EditorPanel. When the editor exits, the host swaps
// the original panel back. This avoids opening a new window/tab in the rail.
type InlineEditRequestMsg struct {
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
	AgentSplitOpen AgentSplitAction = iota
	AgentSplitClose
)

// SplitEditorRequestMsg is emitted by a sub-TUI (or pane) to request that
// the host open a file in a new sibling editor pane. In standalone mode the
// host wrapper dynamically injects an embedded neovim pane; in hosted mode
// (groveterm) the host may create a native Ghostty PTY split instead.
type SplitEditorRequestMsg struct {
	Path  string  // file to open
	Line  int     // optional line number to jump to (0 = don't jump)
	Ratio float64 // split ratio for the origin pane (0 = default 0.5)
	Focus bool    // if true, focus the editor; if false, keep focus on origin
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

// SplitViewportRequestMsg is emitted by a sub-TUI to request that the host
// split the current pane and open a generic scrollable text viewport alongside
// it. The viewport renders pre-styled content and supports auto-scroll (tail
// follow) for streaming use cases like live logs.
type SplitViewportRequestMsg struct {
	PanelID    string  // logical ID for the viewport (e.g. "logs")
	Title      string  // header line displayed above the viewport content
	Content    string  // initial content to render
	AutoScroll bool    // true = start in follow-tail mode
	Ratio      float64 // split ratio for the origin pane (0 = default 0.5)
	Focus      bool    // true = steal focus to the viewport; false = keep focus on originator
}

// SplitViewportCloseRequestMsg is emitted by a sub-TUI to request that the
// host close a viewport BSP split that was previously opened via
// SplitViewportRequestMsg. Used when the originator switches detail modes.
type SplitViewportCloseRequestMsg struct{}

// SplitViewportClosedMsg is sent by the host back to the sub-TUI when the
// viewport panel is closed (user pressed q, or Leader x, or host closed it
// programmatically). The originator should demote its internal state.
type SplitViewportClosedMsg struct{}

// UpdateViewportContentMsg is emitted by a sub-TUI to push new content to
// an active viewport panel. When Append is true the content is appended to
// existing lines; when false the viewport content is fully replaced.
type UpdateViewportContentMsg struct {
	Content string
	Append  bool
}

// UpdateViewportTitleMsg is emitted by a sub-TUI to dynamically change the
// title displayed in the viewport panel header. Used when switching detail
// types (e.g. logs → frontmatter) without recreating the BSP split.
type UpdateViewportTitleMsg struct {
	Title string
}

// SplitContextRequestMsg is emitted by a sub-TUI to request that the host
// split the current pane and open the cx context panel alongside it.
type SplitContextRequestMsg struct {
	WorkDir   string
	RulesFile string
	Ratio     float64 // split ratio (0 = default 0.5)
	Focus     bool    // true = steal focus; false = keep focus on originator
}

// SplitContextCloseRequestMsg is emitted by a sub-TUI to request that the
// host close a context panel BSP split.
type SplitContextCloseRequestMsg struct{}

// SplitContextClosedMsg is sent by the host back to the sub-TUI when the
// context panel is closed.
type SplitContextClosedMsg struct{}

// UpdateContextScopeMsg is emitted by a sub-TUI to dynamically change the
// rules file scoped in its active context panel sibling.
type UpdateContextScopeMsg struct {
	RulesFile string
}

// SplitMemoryRequestMsg is emitted by a sub-TUI to request that the host
// split the current pane and open the memory search panel alongside it.
// Query seeds the search input; if empty the panel opens with a blank search.
type SplitMemoryRequestMsg struct {
	Query string  // initial search query to seed
	Ratio float64 // split ratio for the origin pane (0 = default 0.5)
	Focus bool    // true = steal focus; false = keep focus on originator
}

// SplitMemoryCloseRequestMsg is emitted by a sub-TUI to request that the
// host close a memory panel BSP split.
type SplitMemoryCloseRequestMsg struct{}

// SplitMemoryClosedMsg is sent by the host back to the sub-TUI when the
// memory panel is closed.
type SplitMemoryClosedMsg struct{}

// AppendContextRuleMsg is emitted by a sub-TUI (e.g. the memory panel)
// when the user wants to add a file to the originating panel's context
// rules. The host routes it to the BSP sibling of the emitting panel.
type AppendContextRuleMsg struct {
	Path string // absolute file path to add as a context rule
}

// SplitAgentRequestMsg is emitted by a sub-TUI to request that the host
// split the current pane and display the native PTY agent panel for the
// given JobID alongside the emitting panel. AgentSplitClose reverses the
// split, removing the agent pane from the BSP tree (but keeping it alive
// in the rail).
type SplitAgentRequestMsg struct {
	JobID  string
	Action AgentSplitAction
	Ratio  float64 // split ratio for the origin pane (0 = default 0.5)
}
