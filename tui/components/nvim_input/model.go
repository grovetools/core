// Package nvim_input provides a multi-line text input component backed by an
// embedded Neovim instance. It wraps components/nvim.Model with a temporary
// file and a custom "grove_submit" RPC handler that emits SubmitMsg when the
// user presses <C-Enter>.
package nvim_input

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sirupsen/logrus"

	"github.com/grovetools/core/logging"
	"github.com/grovetools/core/tui/components/nvim"
	"github.com/grovetools/core/tui/theme"
)

var log *logrus.Entry

func init() {
	log = logging.NewLogger("nvim-input")
}

// SubmitMsg is emitted when the user triggers the submit action (<C-Enter>).
type SubmitMsg struct {
	Content string
}

// Model is a Bubble Tea model wrapping an embedded Neovim for multi-line input.
// It implements Focusable and TextInputActive so the pane manager can gate
// layout keys while the user is typing.
type Model struct {
	nvim     nvim.Model
	tmpFile  string // path to the temp file backing the buffer
	width    int
	height   int
	focused  bool
	escaped  bool // true after Escape in normal mode — allows Tab to cycle out
	ready    bool // true once Init has run
	err      error
	submitCh chan string // receives content from the RPC handler
}

// New creates a new NvimInputPane. The caller should defer Close().
func New() (*Model, error) {
	// Create a temp file for the neovim buffer
	tmp, err := os.CreateTemp("", "grove-input-*.md")
	if err != nil {
		return nil, fmt.Errorf("nvim_input: create temp file: %w", err)
	}
	tmp.Close()

	submitCh := make(chan string, 4)

	nv, err := nvim.New(nvim.Options{
		Width:      80,
		Height:     5,
		FileToOpen: tmp.Name(),
		UseConfig:  false,
	})
	if err != nil {
		os.Remove(tmp.Name())
		return nil, fmt.Errorf("nvim_input: create nvim: %w", err)
	}

	m := &Model{
		nvim:     nv,
		tmpFile:  tmp.Name(),
		submitCh: submitCh,
	}

	// Register the "grove_submit" RPC handler. Neovim Lua will call
	// rpcnotify(1, "grove_submit") and we read the buffer contents via RPC.
	if err := nv.RegisterHandler("grove_submit", func(args ...string) {
		// Read buffer lines via the args passed from Lua getline(1, '$')
		content := strings.Join(args, "\n")
		select {
		case submitCh <- content:
		default:
			log.Warn("grove_submit: channel full, dropping")
		}
	}); err != nil {
		nv.Close()
		os.Remove(tmp.Name())
		return nil, fmt.Errorf("nvim_input: register handler: %w", err)
	}

	return m, nil
}

// Init implements tea.Model. Injects the <C-Enter> keymap into neovim and
// starts listening for redraw + submit events.
func (m *Model) Init() tea.Cmd {
	// Inject the submit keymap: <C-Enter> in all modes calls rpcnotify
	// with the buffer contents, then clears the buffer.
	luaSetup := `
vim.keymap.set({'n', 'i', 'v'}, '<C-CR>', function()
  local lines = vim.api.nvim_buf_get_lines(0, 0, -1, false)
  local content = table.concat(lines, "\n")
  if content ~= "" then
    vim.rpcnotify(1, "grove_submit", content)
    vim.api.nvim_buf_set_lines(0, 0, -1, false, {""})
    vim.cmd("startinsert")
  end
end, {noremap = true})
vim.cmd("startinsert")
`
	go func() {
		if err := m.nvim.ExecLua(luaSetup, nil); err != nil {
			log.WithError(err).Error("failed to inject submit keymap")
		}
	}()

	m.ready = true
	return tea.Batch(
		m.nvim.Init(),
		m.waitForSubmit(),
	)
}

// waitForSubmit returns a tea.Cmd that blocks until the RPC handler fires.
func (m *Model) waitForSubmit() tea.Cmd {
	ch := m.submitCh
	return func() tea.Msg {
		content, ok := <-ch
		if !ok {
			return nil
		}
		return SubmitMsg{Content: content}
	}
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Reserve 1 line for the header
		nvimH := max(msg.Height-1, 1)
		innerMsg := tea.WindowSizeMsg{Width: msg.Width, Height: nvimH}
		updated, cmd := m.nvim.Update(innerMsg)
		m.nvim = updated.(nvim.Model)
		return m, cmd

	case tea.KeyMsg:
		if msg.Type == tea.KeyEscape && m.nvim.Mode() == "normal" {
			// In normal mode, Escape sets the escaped flag so that
			// IsTextEntryActive returns false, allowing Tab to cycle
			// focus out of this pane.
			m.escaped = true
			return m, nil
		}
		// Any non-Escape key clears the escaped flag — the user is
		// actively typing/navigating inside neovim again.
		m.escaped = false
		// Forward to inner nvim
		updated, cmd := m.nvim.Update(msg)
		m.nvim = updated.(nvim.Model)
		return m, cmd

	case SubmitMsg:
		// Re-emit to parent, then keep listening
		return m, tea.Batch(
			func() tea.Msg { return msg },
			m.waitForSubmit(),
		)

	case nvim.NvimExitMsg:
		// Neovim died — propagate
		updated, cmd := m.nvim.Update(msg)
		m.nvim = updated.(nvim.Model)
		return m, cmd
	}

	// Forward everything else to inner nvim
	updated, cmd := m.nvim.Update(msg)
	m.nvim = updated.(nvim.Model)
	return m, cmd
}

// View implements tea.Model.
func (m *Model) View() string {
	if m.err != nil {
		return fmt.Sprintf("nvim input error: %v", m.err)
	}

	t := theme.DefaultTheme

	titleStyle := lipgloss.NewStyle().Bold(true).Padding(0, 1)
	if m.focused {
		titleStyle = titleStyle.Foreground(theme.DefaultColors.Orange)
	} else {
		titleStyle = titleStyle.Foreground(theme.DefaultColors.Blue)
	}

	header := titleStyle.Render("Input")
	mode := m.nvim.FormatMode()
	if mode != "" {
		header += t.Muted.Render(fmt.Sprintf(" [%s]", mode))
	}
	if m.focused {
		header += t.Muted.Render(" (C-Enter=submit)")
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, m.nvim.View())
}

// Focus implements panes.Focusable.
func (m *Model) Focus() tea.Cmd {
	m.focused = true
	m.escaped = false
	m.nvim.SetFocused(true)
	return nil
}

// Blur implements panes.Focusable.
func (m *Model) Blur() {
	m.focused = false
	m.nvim.SetFocused(false)
}

// IsTextEntryActive implements panes.TextInputActive.
// Returns true when focused UNLESS the user pressed Escape in normal mode,
// which signals intent to cycle focus out via Tab.
func (m *Model) IsTextEntryActive() bool {
	return m.focused && !m.escaped
}

// Close cleans up the neovim process and temp file.
func (m *Model) Close() error {
	if m.submitCh != nil {
		close(m.submitCh)
		m.submitCh = nil
	}
	if err := m.nvim.Close(); err != nil {
		return err
	}
	return os.Remove(m.tmpFile)
}
