package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/grovetools/core/tui/components/panedemo"
)

func main() {
	// nil keymap = use default Tab/Shift+Tab bindings
	p := tea.NewProgram(panedemo.New(nil), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
