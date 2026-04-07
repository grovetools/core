package embed

import (
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
)

// StandaloneHost wraps an embeddable tea.Model so it can be run as a standalone
// CLI binary. It catches the standard embed messages and translates them into
// the appropriate standalone behaviors:
//
//   - DoneMsg captures Result/Err and quits the program.
//   - CloseRequestMsg / CloseConfirmMsg quit the program.
//   - EditRequestMsg suspends the TUI via tea.ExecProcess, runs $EDITOR on the
//     requested path, and resumes the TUI with an EditFinishedMsg dispatched
//     back to the wrapped model.
//
// All other messages are forwarded to the wrapped model unchanged.
type StandaloneHost struct {
	Model  tea.Model
	Result any
	Err    error
}

// NewStandaloneHost wraps the given model in a StandaloneHost.
func NewStandaloneHost(m tea.Model) StandaloneHost {
	return StandaloneHost{Model: m}
}

func (h StandaloneHost) Init() tea.Cmd {
	return h.Model.Init()
}

func (h StandaloneHost) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case DoneMsg:
		h.Result = msg.Result
		h.Err = msg.Err
		return h, tea.Quit

	case CloseRequestMsg, CloseConfirmMsg:
		return h, tea.Quit

	case EditRequestMsg:
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vi"
		}
		cmd := exec.Command(editor, msg.Path)
		return h, tea.ExecProcess(cmd, func(err error) tea.Msg {
			return EditFinishedMsg{Err: err}
		})
	}

	var cmd tea.Cmd
	h.Model, cmd = h.Model.Update(msg)
	return h, cmd
}

func (h StandaloneHost) View() string {
	return h.Model.View()
}

// RunStandalone is a convenience wrapper for CLI binaries launching an
// embeddable sub-TUI. It wraps the model in a StandaloneHost, runs the program,
// and returns the captured DoneMsg payload (Result, Err) from the final model.
//
// If the bubbletea program itself fails, that error is returned with a nil
// result. Otherwise the result and any error reported via DoneMsg are returned.
func RunStandalone(m tea.Model, opts ...tea.ProgramOption) (any, error) {
	host := NewStandaloneHost(m)
	p := tea.NewProgram(host, opts...)

	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	if finalHost, ok := finalModel.(StandaloneHost); ok {
		return finalHost.Result, finalHost.Err
	}
	return nil, nil
}
