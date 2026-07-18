// Package embed defines the standard contract for grovetools TUIs that want
// to be embeddable inside a host application (such as the terminal multiplexer)
// while still being runnable as standalone CLI binaries.
//
// Canonical definitions now live in github.com/grovetools/tuimux/embed.
// This package re-exports them via Go type aliases for backward compatibility.
package embed

import (
	"github.com/grovetools/core/config"
	tuimux_embed "github.com/grovetools/tuimux/embed"
)

type (
	DoneMsg                      = tuimux_embed.DoneMsg
	FocusMsg                     = tuimux_embed.FocusMsg
	BlurMsg                      = tuimux_embed.BlurMsg
	SetWorkspaceMsg              = tuimux_embed.SetWorkspaceMsg
	EditRequestMsg               = tuimux_embed.EditRequestMsg
	InlineEditRequestMsg         = tuimux_embed.InlineEditRequestMsg
	EditFinishedMsg              = tuimux_embed.EditFinishedMsg
	PreviewRequestMsg            = tuimux_embed.PreviewRequestMsg
	OpenAgentSessionMsg          = tuimux_embed.OpenAgentSessionMsg
	SwitchTabMsg                 = tuimux_embed.SwitchTabMsg
	NavigateMsg                  = tuimux_embed.NavigateMsg
	CloseRequestMsg              = tuimux_embed.CloseRequestMsg
	CloseConfirmMsg              = tuimux_embed.CloseConfirmMsg
	AgentSplitAction             = tuimux_embed.AgentSplitAction
	SplitEditorRequestMsg        = tuimux_embed.SplitEditorRequestMsg
	SplitEditorClosedMsg         = tuimux_embed.SplitEditorClosedMsg
	SplitEditorCloseRequestMsg   = tuimux_embed.SplitEditorCloseRequestMsg
	SplitReviewRequestMsg        = tuimux_embed.SplitReviewRequestMsg
	SplitReviewClosedMsg         = tuimux_embed.SplitReviewClosedMsg
	SplitReviewCloseRequestMsg   = tuimux_embed.SplitReviewCloseRequestMsg
	SplitViewportRequestMsg      = tuimux_embed.SplitViewportRequestMsg
	SplitViewportCloseRequestMsg = tuimux_embed.SplitViewportCloseRequestMsg
	SplitViewportClosedMsg       = tuimux_embed.SplitViewportClosedMsg
	UpdateViewportContentMsg     = tuimux_embed.UpdateViewportContentMsg
	UpdateViewportTitleMsg       = tuimux_embed.UpdateViewportTitleMsg
	SplitContextRequestMsg       = tuimux_embed.SplitContextRequestMsg
	SplitContextCloseRequestMsg  = tuimux_embed.SplitContextCloseRequestMsg
	SplitContextClosedMsg        = tuimux_embed.SplitContextClosedMsg
	UpdateContextScopeMsg        = tuimux_embed.UpdateContextScopeMsg
	SplitMemoryRequestMsg        = tuimux_embed.SplitMemoryRequestMsg
	SplitMemoryCloseRequestMsg   = tuimux_embed.SplitMemoryCloseRequestMsg
	SplitMemoryClosedMsg         = tuimux_embed.SplitMemoryClosedMsg
	WorkspacesUpdatedMsg         = tuimux_embed.WorkspacesUpdatedMsg
	NavBindingsUpdatedMsg        = tuimux_embed.NavBindingsUpdatedMsg
	AppendContextRuleMsg         = tuimux_embed.AppendContextRuleMsg
	SplitAgentRequestMsg         = tuimux_embed.SplitAgentRequestMsg
)

// SwitchWorkspaceRequestMsg is emitted by embedded TUIs to request a workspace switch.
type SwitchWorkspaceRequestMsg struct {
	Path       string
	FocusPanel string
}

// SettingAppliedMsg is emitted by the embedded grove config TUI after a
// curated setting has been persisted to the global config layer and the
// layered config reloaded. Hosts (treemux) switch on Domain to hot-apply
// the change via their existing setters; standalone CLIs simply ignore it.
//
// Domain names the live-apply seam, not the TOML key (one domain may cover
// several keys, e.g. "focus" covers all of [tui.focus]). Config is the
// freshly merged layered.Final so handlers read the new effective values
// without re-loading. Defined here (not re-exported from tuimux/embed)
// because it carries a core config type — same pattern as
// SwitchWorkspaceRequestMsg above.
type SettingAppliedMsg struct {
	Domain string
	Config *config.Config
}

// Live-apply domains for SettingAppliedMsg.Domain. The emitting side (grove
// curated Setting.ApplyDomain) and the treemux handler both reference these
// so the contract cannot drift. Startup-only settings (e.g. [tui]
// hide_splash_on_startup) have no domain and emit nothing.
const (
	SettingDomainFocus             = "focus"              // [tui.focus] style/colors/thickness
	SettingDomainLeaderKey         = "leader_key"         // [tui] leader_key
	SettingDomainActionKey         = "action_key"         // [tui] action_key
	SettingDomainVimPaneNav        = "vim_pane_nav"       // [tui] vim_control_hjkl_pane_nav
	SettingDomainDrawerOrientation = "drawer_orientation" // [tui] drawer_orientation
	SettingDomainDrawerExpanded    = "drawer_expanded"    // [tui] drawer_expanded
	SettingDomainSidebarExpanded   = "sidebar_expanded"   // [tui] sidebar_expanded
	SettingDomainIcons             = "icons"              // [tui] icons (live apply lands with theme.SetIcons)
)

// KeyCaptureMsg is emitted by an embedded TUI's key-capture widget (the
// grove config Keys page) to arm (Active true) or disarm (Active false) the
// host's capture-next-keystroke mode. While armed the host must deliver the
// next raw tea.KeyMsg to the focused panel BEFORE its own chord/global key
// routing (tuimux's leader/action interception happens at the top of Update,
// so without this the currently-bound chord could never be re-captured — see
// the A2 correction in plans/treemux-splash/30-design-appearance-keys.md).
// The host mode is single-shot: it self-clears after forwarding one key, so
// a widget re-arms per captured chord. Standalone CLIs have no handler —
// the message is inert and the widget captures keys through the normal
// bubbletea delivery path. Defined here (not in tuimux/embed) alongside
// SettingAppliedMsg, its emitting sibling on the curated-config channel.
type KeyCaptureMsg struct {
	Active bool
}

const (
	AgentSplitOpen  = tuimux_embed.AgentSplitOpen
	AgentSplitClose = tuimux_embed.AgentSplitClose
)
