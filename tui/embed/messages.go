// Package embed defines the standard contract for grovetools TUIs that want
// to be embeddable inside a host application (such as the terminal multiplexer)
// while still being runnable as standalone CLI binaries.
//
// Canonical definitions now live in github.com/grovetools/tuimux/embed.
// This package re-exports them via Go type aliases for backward compatibility.
package embed

import tuimux_embed "github.com/grovetools/tuimux/embed"

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
	AppendContextRuleMsg         = tuimux_embed.AppendContextRuleMsg
	SplitAgentRequestMsg         = tuimux_embed.SplitAgentRequestMsg
)

const (
	AgentSplitOpen  = tuimux_embed.AgentSplitOpen
	AgentSplitClose = tuimux_embed.AgentSplitClose
)
