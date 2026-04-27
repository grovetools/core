package panedemo

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/grovetools/core/tui/components/help"
	"github.com/grovetools/core/tui/components/nvim"
	nvim_input "github.com/grovetools/core/tui/components/nvim_input"
	"github.com/grovetools/core/tui/embed"
	"github.com/grovetools/core/tui/panes"
	"github.com/grovetools/core/tui/theme"
)

// appKeyMap extends the pane manager keymap with app-level bindings.
type appKeyMap struct {
	panes.KeyMap
	Quit          key.Binding
	Help          key.Binding
	TogglePreview key.Binding
}

func newAppKeyMap(km panes.KeyMap) appKeyMap {
	return appKeyMap{
		KeyMap: km,
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		TogglePreview: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "preview"),
		),
	}
}

// ShortHelp returns bindings for the compact help bar.
func (km appKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		km.CycleNext,
		km.ToggleFullscreen,
		km.TogglePinned,
		km.ToggleDirection,
		km.ResizeGrow,
		km.ResizeShrink,
		km.TogglePreview,
		km.Quit,
	}
}

// FullHelp returns bindings for the full help overlay.
func (km appKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{km.CycleNext, km.CyclePrev, km.ToggleFullscreen, km.TogglePinned, km.ToggleDirection},
		{km.ResizeGrow, km.ResizeShrink, km.TogglePreview, km.Quit, km.Help},
	}
}

// Model is the top-level bubbletea model for the pane demo.
type Model struct {
	manager    panes.Manager
	help       help.Model
	keys       appKeyMap
	width      int
	height     int
	nvimInput  *nvim_input.Model // keep reference for cleanup
	editorPath string            // path of the currently open editor split (empty = none)
}

// New creates a new pane demo Model. If km is non-nil, it overrides
// the default pane manager keybindings (useful when embedding inside
// a host that reserves Tab for its own navigation).
func New(km *panes.KeyMap) Model {
	baseKM := panes.DefaultKeyMap()
	if km != nil {
		baseKM = *km
	}
	akm := newAppKeyMap(baseKM)

	// Create NvimInputPane for the bottom input area
	ni, err := nvim_input.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: nvim input unavailable (%v), falling back to text input\n", err)
		// Fall back to the old text input pane
		mgr := panes.New(
			panes.Pane{ID: "list", Model: newListPane(), Fixed: 30, MinSize: 20},
			panes.Pane{ID: "logs", Model: newLogPane(), Flex: 2, MinSize: 20},
			panes.Pane{ID: "input", Model: newInputPane(), Fixed: 5, MinSize: 3},
			panes.Pane{ID: "preview", Model: newPreviewPane(), Flex: 1, MinSize: 15, Hidden: true},
		)
		mgr.KeyMap = akm.KeyMap
		return Model{manager: mgr, help: help.New(akm), keys: akm}
	}

	mgr := panes.New(
		panes.Pane{ID: "list", Model: newListPane(), Fixed: 30, MinSize: 20},
		panes.Pane{ID: "logs", Model: newLogPane(), Flex: 2, MinSize: 20},
		panes.Pane{ID: "input", Model: ni, Fixed: 6, MinSize: 4},
		panes.Pane{ID: "preview", Model: newPreviewPane(), Flex: 1, MinSize: 15, Hidden: true},
	)
	mgr.KeyMap = akm.KeyMap

	h := help.New(akm)

	return Model{
		manager:   mgr,
		help:      h,
		keys:      akm,
		nvimInput: ni,
	}
}

func (a Model) Init() tea.Cmd {
	// Initialize all child pane models (e.g., log pane ticker, nvim input).
	var cmds []tea.Cmd
	for i := range a.manager.Panes {
		if cmd := a.manager.Panes[i].Model.Init(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	// Focus the first pane
	if p := a.manager.ActivePane(); p != nil {
		if f, ok := p.Model.(panes.Focusable); ok {
			if cmd := f.Focus(); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}
	return tea.Batch(cmds...)
}

func (a Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height

		// Help gets 1 line at the bottom, status bar gets 1 line
		footerHeight := 2
		managerMsg := tea.WindowSizeMsg{
			Width:  msg.Width,
			Height: msg.Height - footerHeight,
		}

		var cmd tea.Cmd
		a.manager, cmd = a.manager.Update(managerMsg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

		a.help.Width = msg.Width
		a.help.Height = msg.Height
		return a, tea.Batch(cmds...)

	case embed.SplitEditorRequestMsg:
		return a.openEditorSplit(msg.Path)

	case nvim.NvimExitMsg:
		// If the exiting nvim is the editor split, remove it
		if a.editorPath != "" && msg.FilePath == a.editorPath {
			return a.closeEditorSplit()
		}
		// Otherwise forward to manager (could be the input pane's nvim)
		var cmd tea.Cmd
		a.manager, cmd = a.manager.Update(msg)
		return a, cmd

	case nvim_input.SubmitMsg:
		// Log the submitted content to the logs pane
		logMsg := itemSelectedMsg{
			Title: "User Input",
			Desc:  msg.Content,
		}
		return a, panes.SendCmd("logs", logMsg)

	case tea.KeyMsg:
		// Help overlay intercepts when open
		if a.help.ShowAll {
			var cmd tea.Cmd
			a.help, cmd = a.help.Update(msg)
			return a, cmd
		}

		// App-level keys (only when text input not active)
		if !a.manager.IsTextInputActive() {
			switch {
			case key.Matches(msg, a.keys.Quit):
				a.Close()
				return a, tea.Quit
			case key.Matches(msg, a.keys.Help):
				a.help.Toggle()
				return a, nil
			case key.Matches(msg, a.keys.TogglePreview):
				var cmd tea.Cmd
				a.manager, cmd = a.manager.SetHidden("preview", !a.manager.IsHidden("preview"))
				return a, cmd
			}
		}
	}

	// Forward to manager
	var cmd tea.Cmd
	a.manager, cmd = a.manager.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	return a, tea.Batch(cmds...)
}

// openEditorSplit dynamically adds an nvim editor pane for the given file.
func (a Model) openEditorSplit(path string) (tea.Model, tea.Cmd) {
	if a.editorPath != "" {
		// Already have an editor open — ignore
		return a, nil
	}

	// Create a scratch file if it doesn't exist
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte("# "+path+"\n"), 0o644); err != nil { //nolint:gosec // demo scratch file
			return a, nil
		}
	}

	nvimModel, err := nvim.New(nvim.Options{
		Width:      40,
		Height:     20,
		FileToOpen: path,
		UseConfig:  false,
	})
	if err != nil {
		return a, nil
	}

	a.editorPath = path

	// Insert editor pane after "logs" (index 1), before "input"
	editorPane := panes.Pane{ID: "editor", Model: &nvimModel, Flex: 2, MinSize: 20}
	newPanes := make([]panes.Pane, 0, len(a.manager.Panes)+1)
	for _, p := range a.manager.Panes {
		newPanes = append(newPanes, p)
		if p.ID == "logs" {
			newPanes = append(newPanes, editorPane)
		}
	}
	a.manager.Panes = newPanes

	// Focus the editor pane
	for i, p := range a.manager.Panes {
		if p.ID == "editor" {
			// Blur current
			if f, ok := a.manager.Panes[a.manager.ActivePaneIdx].Model.(panes.Focusable); ok {
				f.Blur()
			}
			a.manager.ActivePaneIdx = i
			break
		}
	}

	// Initialize and redistribute
	var cmds []tea.Cmd
	if cmd := nvimModel.Init(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	// Focus the editor
	nvimModel.SetFocused(true)

	// Redistribute sizes
	sizeMsg := tea.WindowSizeMsg{Width: a.manager.Width, Height: a.manager.Height}
	var cmd tea.Cmd
	a.manager, cmd = a.manager.Update(sizeMsg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	return a, tea.Batch(cmds...)
}

// closeEditorSplit removes the editor pane and restores focus.
func (a Model) closeEditorSplit() (tea.Model, tea.Cmd) {
	a.editorPath = ""

	// Remove the "editor" pane
	newPanes := make([]panes.Pane, 0, len(a.manager.Panes)-1)
	for _, p := range a.manager.Panes {
		if p.ID == "editor" {
			// Close the nvim model if possible
			if closer, ok := p.Model.(interface{ Close() error }); ok {
				closer.Close()
			}
			continue
		}
		newPanes = append(newPanes, p)
	}
	a.manager.Panes = newPanes

	// Fix active pane index
	if a.manager.ActivePaneIdx >= len(a.manager.Panes) {
		a.manager.ActivePaneIdx = 0
	}

	// Focus the new active pane
	if f, ok := a.manager.Panes[a.manager.ActivePaneIdx].Model.(panes.Focusable); ok {
		f.Focus()
	}

	// Redistribute
	sizeMsg := tea.WindowSizeMsg{Width: a.manager.Width, Height: a.manager.Height}
	var cmd tea.Cmd
	a.manager, cmd = a.manager.Update(sizeMsg)

	// Emit SplitEditorClosedMsg so any listener knows
	closedCmd := func() tea.Msg {
		return embed.SplitEditorClosedMsg{Path: a.editorPath}
	}

	return a, tea.Batch(cmd, closedCmd)
}

// Close releases resources held by the model.
func (a *Model) Close() {
	if a.nvimInput != nil {
		a.nvimInput.Close()
	}
}

func (a Model) View() string {
	if a.help.ShowAll {
		return a.help.View()
	}

	t := theme.DefaultTheme

	// Manager view
	body := a.manager.View()

	// Status bar: show active pane + direction + fullscreen state
	active := a.manager.ActivePane()
	activeName := ""
	if active != nil {
		activeName = active.ID
	}

	dirLabel := "horizontal"
	if a.manager.Direction == panes.DirectionVertical {
		dirLabel = "vertical"
	}

	zoomLabel := ""
	if a.manager.FullscreenIdx >= 0 {
		if a.manager.PinnedMode {
			zoomLabel = t.Warning.Render(" [PINNED]")
		} else {
			zoomLabel = t.Warning.Render(" [ZOOM]")
		}
	}

	editorLabel := ""
	if a.editorPath != "" {
		editorLabel = t.Highlight.Render("  │  Editor: ") + t.Warning.Render(a.editorPath)
	}

	statusBar := t.Muted.Render("  Active: ") +
		t.Highlight.Render(activeName) +
		t.Muted.Render("  │  Split: ") +
		t.Highlight.Render(dirLabel) +
		zoomLabel +
		editorLabel

	statusLine := lipgloss.NewStyle().
		Width(a.width).
		Render(statusBar)

	// Help bar
	helpBar := a.help.View()

	return lipgloss.JoinVertical(lipgloss.Left, body, statusLine, helpBar)
}
