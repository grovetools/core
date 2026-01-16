package nvim

import (
	"fmt"
	"os"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/grovetools/core/logging"
	"github.com/neovim/go-client/nvim"
	"github.com/sirupsen/logrus"
)

var log *logrus.Entry
var debugFile *os.File

func init() {
	log = logging.NewLogger("nvim-component")
	// Also write debug info to a file we can check
	var err error
	debugFile, err = os.Create("/tmp/nvim-component-debug.log")
	if err == nil {
		debugFile.WriteString("=== Nvim Component Debug Log ===\n")
	}
}

func debugLog(msg string) {
	if debugFile != nil {
		debugFile.WriteString(msg + "\n")
		debugFile.Sync()
	}
}

func debugLogf(format string, args ...interface{}) {
	debugLog(fmt.Sprintf(format, args...))
}

// nvimCell represents a single character cell in the Neovim grid.
type nvimCell struct {
	text string
	hlID int
}

// Options holds configuration for creating a new Neovim component.
type Options struct {
	Width      int    // Initial width of the Neovim grid
	Height     int    // Initial height of the Neovim grid
	FileToOpen string // Optional path to a file to open on startup
	UseConfig  bool   // If true, loads the user's default Neovim config
}

// Model is the Bubble Tea model for the Neovim component.
// This is a reusable component that can be embedded in any TUI application.
type Model struct {
	v                      *nvim.Nvim
	redraws                chan [][]interface{}
	grid                   [][]nvimCell
	hlDefs                 map[int]lipgloss.Style
	hlMutex                sync.RWMutex
	needDefaultColors      bool // true if queried Normal was empty
	defaultColorsProcessed bool // true after first default_colors_set
	nvimWidth              int
	nvimHeight             int
	cursorRow              int
	cursorCol              int
	mode                   string
	err                    error
	uiAttached             bool
	useConfig              bool
	focused                bool // true if this component currently has focus
}

// New creates and initializes a new Neovim component.
func New(opts Options) (Model, error) {
	redrawCh := make(chan [][]interface{}, 100)

	// Build nvim arguments
	nvimArgs := []string{"--embed"}
	if !opts.UseConfig {
		// Default to --clean to prevent loading user config
		nvimArgs = append(nvimArgs, "--clean")
	}

	v, err := nvim.NewChildProcess(nvim.ChildProcessArgs(nvimArgs...))
	if err != nil {
		return Model{}, fmt.Errorf("failed to start nvim child process: %w", err)
	}

	// The redraw handler runs in a separate goroutine and sends events to our channel.
	handler := func(updates ...[]interface{}) {
		select {
		case redrawCh <- updates:
		default:
			// Channel is full, drop the event. This can happen on rapid resizing.
		}
	}

	if err := v.RegisterHandler("redraw", handler); err != nil {
		v.Close()
		return Model{}, fmt.Errorf("failed to register redraw handler: %w", err)
	}

	// Set default dimensions if not provided
	if opts.Width == 0 {
		opts.Width = 80
	}
	if opts.Height == 0 {
		opts.Height = 24
	}

	// Attach to the UI with linegrid extensions
	if err := v.AttachUI(opts.Width, opts.Height, map[string]interface{}{
		"ext_linegrid": true,
	}); err != nil {
		v.Close()
		return Model{}, fmt.Errorf("failed to attach nvim UI: %w", err)
	}

	// Enable RGB colors immediately after attaching
	if err := v.SetUIOption("rgb", true); err != nil {
		debugLog(fmt.Sprintf("ERROR: SetUIOption rgb failed: %v", err))
	} else {
		debugLog("SUCCESS: RGB UI option enabled")
	}

	// Query default colors explicitly
	var hlNormal map[string]interface{}
	defaultStyle := lipgloss.NewStyle()
	normalWasEmpty := true
	if err := v.Call("nvim_get_hl_by_name", &hlNormal, "Normal", true); err != nil {
		debugLog(fmt.Sprintf("WARN: Could not query Normal highlight: %v", err))
	} else {
		debugLog(fmt.Sprintf("Queried Normal highlight: %+v", hlNormal))
		// Extract foreground and background colors (handle both int64 and uint64)
		var fgVal, bgVal int64
		if fg, ok := hlNormal["foreground"].(int64); ok {
			fgVal = fg
		} else if fg, ok := hlNormal["foreground"].(uint64); ok {
			fgVal = int64(fg)
		}
		if fgVal != 0 && fgVal != -1 {
			defaultStyle = defaultStyle.Foreground(lipgloss.Color(fmt.Sprintf("#%06x", fgVal)))
			debugLog(fmt.Sprintf("Set default fg from Normal: #%06x", fgVal))
			normalWasEmpty = false
		}

		if bg, ok := hlNormal["background"].(int64); ok {
			bgVal = bg
		} else if bg, ok := hlNormal["background"].(uint64); ok {
			bgVal = int64(bg)
		}
		if bgVal != 0 && bgVal != -1 {
			defaultStyle = defaultStyle.Background(lipgloss.Color(fmt.Sprintf("#%06x", bgVal)))
			debugLog(fmt.Sprintf("Set default bg from Normal: #%06x", bgVal))
			normalWasEmpty = false
		}
	}

	if normalWasEmpty {
		debugLog("Normal highlight was empty - will use default_colors_set event")
	}

	// Initialize hlDefs with default style at index 0
	hlDefs := make(map[int]lipgloss.Style)
	hlDefs[0] = defaultStyle
	debugLog(fmt.Sprintf("Initialized hlDefs[0] with default style"))

	m := Model{
		v:                 v,
		redraws:           redrawCh,
		hlDefs:            hlDefs,
		needDefaultColors: normalWasEmpty,
		nvimWidth:         opts.Width,
		nvimHeight:        opts.Height,
		uiAttached:        true,
		useConfig:         opts.UseConfig,
		focused:           false,
	}

	// Open the file if specified
	if opts.FileToOpen != "" {
		go v.Command(fmt.Sprintf("edit %s", opts.FileToOpen))
	}

	return m, nil
}

// Init implements tea.Model. It starts listening for redraw events.
func (m Model) Init() tea.Cmd {
	return m.waitForRedraw()
}

// Update implements tea.Model. It handles key messages and redraw events.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Forward all keys to Neovim - parent model should handle focus/quit
		go m.v.Input(keyToNvim(msg))

	case redrawMsg:
		// Handle the redraw events from Neovim.
		m.handleRedraw(msg)
		// Continue listening for the next event.
		cmds = append(cmds, m.waitForRedraw())

	case tea.WindowSizeMsg:
		m.nvimWidth = msg.Width
		m.nvimHeight = msg.Height

		// Inform Neovim of the new size
		go m.v.TryResizeUI(m.nvimWidth, m.nvimHeight)

	case error:
		m.err = msg
		return m, tea.Quit
	}

	return m, tea.Batch(cmds...)
}

// View implements tea.Model. It renders the Neovim grid.
func (m Model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Neovim error: %v", m.err)
	}

	// Render nvim grid
	var nvimBuilder strings.Builder

	for r := 0; r < m.nvimHeight; r++ {
		if r < len(m.grid) {
			for c := 0; c < m.nvimWidth; c++ {
				if c < len(m.grid[r]) {
					cell := m.grid[r][c]
					m.hlMutex.RLock()
					style, ok := m.hlDefs[cell.hlID]
					m.hlMutex.RUnlock()
					if !ok {
						style = lipgloss.NewStyle()
					}
					// Render cursor - only when focused
					if r == m.cursorRow && c == m.cursorCol && m.focused {
						style = style.Reverse(true)
					}
					nvimBuilder.WriteString(style.Render(cell.text))
				}
			}
		}
		if r < m.nvimHeight-1 {
			nvimBuilder.WriteString("\n")
		}
	}

	return nvimBuilder.String()
}

// SetSize updates the size of the Neovim component.
func (m *Model) SetSize(width, height int) tea.Cmd {
	m.nvimWidth = width
	m.nvimHeight = height
	go m.v.TryResizeUI(width, height)
	return nil
}

// SetFocused sets whether this component has focus (for cursor rendering).
func (m *Model) SetFocused(focused bool) {
	m.focused = focused
}

// OpenFile opens a file in the Neovim instance.
func (m *Model) OpenFile(filepath string) {
	go m.v.Command(fmt.Sprintf("edit %s", filepath))
}

// Save saves the current buffer and waits for completion.
func (m *Model) Save() error {
	if m.v != nil {
		return m.v.Command("write")
	}
	return nil
}

// Close closes the Neovim process.
func (m *Model) Close() error {
	if m.v != nil {
		return m.v.Close()
	}
	return nil
}

// Mode returns the current Neovim mode (normal, insert, visual, etc.)
func (m Model) Mode() string {
	return m.mode
}

// CursorPosition returns the current cursor row and column.
func (m Model) CursorPosition() (int, int) {
	return m.cursorRow, m.cursorCol
}
