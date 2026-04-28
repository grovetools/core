package embed

import (
	tea "github.com/charmbracelet/bubbletea"
	tuimux_embed "github.com/grovetools/tuimux/embed"
)

type StandaloneHost = tuimux_embed.StandaloneHost

var NewStandaloneHost = tuimux_embed.NewStandaloneHost

func RunStandalone(m tea.Model, opts ...tea.ProgramOption) (any, error) {
	return tuimux_embed.RunStandalone(m, opts...)
}
