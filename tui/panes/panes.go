// Package panes provides a lightweight pane manager for grovetools TUIs.
//
// Canonical definitions now live in github.com/grovetools/tuimux/panes.
// This package re-exports them via Go type aliases for backward compatibility.
package panes

import tuimux_panes "github.com/grovetools/tuimux/panes"

type Direction = tuimux_panes.Direction

const (
	DirectionHorizontal = tuimux_panes.DirectionHorizontal
	DirectionVertical   = tuimux_panes.DirectionVertical
)

type (
	Pane            = tuimux_panes.Pane
	Focusable       = tuimux_panes.Focusable
	TextInputActive = tuimux_panes.TextInputActive
	StatusProvider  = tuimux_panes.StatusProvider
	Manager         = tuimux_panes.Manager
	KeyMap          = tuimux_panes.KeyMap
	TargetedMsg     = tuimux_panes.TargetedMsg
	BroadcastMsg    = tuimux_panes.BroadcastMsg
)

var (
	New           = tuimux_panes.New
	DefaultKeyMap = tuimux_panes.DefaultKeyMap
	SendCmd       = tuimux_panes.SendCmd
	BroadcastCmd  = tuimux_panes.BroadcastCmd
)
