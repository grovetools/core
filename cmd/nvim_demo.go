package cmd

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattsolo1/grove-core/config"
	"github.com/mattsolo1/grove-core/logging"
	"github.com/mattsolo1/grove-core/tui/components/nvim"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var log *logrus.Entry

func init() {
	log = logging.NewLogger("nvim-demo")
}

// focusState represents which pane has focus
type focusState int

const (
	focusFileList focusState = iota
	focusNvim
)

// fileItem implements list.Item for the file selector
type fileItem struct {
	path  string
	name  string
	isDir bool
}

func (f fileItem) FilterValue() string { return f.name }
func (f fileItem) Title() string {
	if f.isDir {
		return "[D] " + f.name
	}
	return "[F] " + f.name
}
func (f fileItem) Description() string { return f.path }

// nvimDemoModel is the parent Bubble Tea model that manages both the file list and nvim component.
type nvimDemoModel struct {
	fileList    list.Model
	nvimModel   nvim.Model
	focus       focusState
	width       int
	height      int
	currentFile string
	err         error
}

// loadFileList loads files from the current directory
func loadFileList(dir string) []list.Item {
	var items []list.Item

	// Add parent directory
	if dir != "." {
		items = append(items, fileItem{
			path:  filepath.Dir(dir),
			name:  "..",
			isDir: true,
		})
	}

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == dir {
			return nil
		}

		// Skip hidden files and directories
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Only show top-level items (don't recurse)
		if filepath.Dir(path) != dir {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		items = append(items, fileItem{
			path:  path,
			name:  d.Name(),
			isDir: d.IsDir(),
		})

		return nil
	})

	if err != nil {
		return []list.Item{fileItem{name: "Error loading files", path: ""}}
	}

	return items
}

func initialNvimDemoModel(useConfig bool, initialFile string) (nvimDemoModel, error) {
	// Create file list
	items := loadFileList(".")
	delegate := list.NewDefaultDelegate()
	fileList := list.New(items, delegate, 30, 20)
	fileList.Title = "Files"
	fileList.SetShowStatusBar(false)
	fileList.SetFilteringEnabled(true)

	// Create nvim component
	nvimOpts := nvim.Options{
		Width:      80,
		Height:     24,
		FileToOpen: initialFile,
		UseConfig:  useConfig,
	}
	nvimModel, err := nvim.New(nvimOpts)
	if err != nil {
		return nvimDemoModel{}, fmt.Errorf("failed to create nvim component: %w", err)
	}

	return nvimDemoModel{
		fileList:    fileList,
		nvimModel:   nvimModel,
		focus:       focusFileList,
		width:       80,
		height:      24,
		currentFile: initialFile,
	}, nil
}

func (m nvimDemoModel) Init() tea.Cmd {
	return m.nvimModel.Init()
}

func (m nvimDemoModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Global shortcuts
		if msg.Type == tea.KeyCtrlC {
			m.nvimModel.Close()
			return m, tea.Quit
		}

		// Ctrl+B toggles focus between file list and nvim
		if msg.Type == tea.KeyCtrlB {
			if m.focus == focusFileList {
				m.focus = focusNvim
				m.nvimModel.SetFocused(true)
			} else {
				m.focus = focusFileList
				m.nvimModel.SetFocused(false)
			}
			return m, nil
		}

		// Handle keys based on focus
		if m.focus == focusFileList {
			// Enter opens a file or navigates into a directory
			if msg.Type == tea.KeyEnter {
				if item, ok := m.fileList.SelectedItem().(fileItem); ok {
					if item.isDir {
						// Navigate into directory
						absPath, _ := filepath.Abs(item.path)
						newItems := loadFileList(absPath)
						m.fileList.SetItems(newItems)
						m.fileList.Title = "Files: " + absPath
					} else {
						// Open file in nvim
						m.currentFile = item.path
						m.nvimModel.OpenFile(item.path)
						m.focus = focusNvim
						m.nvimModel.SetFocused(true)
					}
				}
				return m, nil
			}

			// Update file list
			m.fileList, cmd = m.fileList.Update(msg)
			cmds = append(cmds, cmd)
		} else {
			// Forward keys to Neovim component
			var updatedModel tea.Model
			updatedModel, cmd = m.nvimModel.Update(msg)
			m.nvimModel = updatedModel.(nvim.Model)
			cmds = append(cmds, cmd)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// File list takes 30% of width, nvim takes the rest
		fileListWidth := m.width * 30 / 100
		nvimWidth := m.width - fileListWidth - 2 // -2 for border
		nvimHeight := m.height - 3                // -3 for status and help

		m.fileList.SetSize(fileListWidth, nvimHeight)
		cmd = m.nvimModel.SetSize(nvimWidth, nvimHeight)
		cmds = append(cmds, cmd)

	case error:
		m.err = msg
		return m, tea.Quit

	default:
		// Forward other messages to nvim component
		var updatedModel tea.Model
		updatedModel, cmd = m.nvimModel.Update(msg)
		m.nvimModel = updatedModel.(nvim.Model)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m nvimDemoModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("An error occurred: %v\n", m.err)
	}

	// Define styles
	var (
		focusedBorder = lipgloss.RoundedBorder()
		normalBorder  = lipgloss.Border{}

		focusedStyle = lipgloss.NewStyle().
				Border(focusedBorder).
				BorderForeground(lipgloss.Color("62"))

		normalStyle = lipgloss.NewStyle().
				Border(normalBorder).
				BorderForeground(lipgloss.Color("240"))
	)

	// Render file list
	fileListStyle := normalStyle
	if m.focus == focusFileList {
		fileListStyle = focusedStyle
	}
	fileListView := fileListStyle.Render(m.fileList.View())

	// Render nvim view
	nvimView := m.nvimModel.View()

	// Combine both views side by side
	mainView := lipgloss.JoinHorizontal(lipgloss.Top, fileListView, nvimView)

	// Status line
	statusText := fmt.Sprintf(" Ctrl+B: Switch Focus | Ctrl+C: Quit")
	if m.currentFile != "" {
		statusText = fmt.Sprintf(" File: %s | Mode: %s | %s",
			m.currentFile,
			strings.ToUpper(m.nvimModel.Mode()),
			"Ctrl+B: Switch Focus | Ctrl+C: Quit")
	}

	var focusIndicator string
	if m.focus == focusFileList {
		focusIndicator = "[FILES]"
	} else {
		focusIndicator = "[NVIM]"
	}

	statusStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("62")).
		Foreground(lipgloss.Color("230")).
		Width(m.width).
		Padding(0, 1)

	statusLine := statusStyle.Render(focusIndicator + statusText)

	// Combine main view and status line
	return lipgloss.JoinVertical(lipgloss.Left, mainView, statusLine)
}

// NewNvimDemoCmd creates the nvim-demo command.
func NewNvimDemoCmd() *cobra.Command {
	var fileToOpen string

	cmd := &cobra.Command{
		Use:   "nvim-demo [file]",
		Short: "Demo of embedded Neovim component",
		Long: `Demonstrates the reusable Neovim component with a file browser.

The demo shows a side-by-side view with a file list on the left and an
embedded Neovim editor on the right. Press Ctrl+B to switch focus between
the file list and the editor.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get file from args if provided
			if len(args) > 0 {
				fileToOpen = args[0]
			}

			// Load configuration
			cfg, err := config.LoadDefault()
			if err != nil {
				log.WithError(err).Warn("Failed to load config, using defaults")
				cfg = &config.Config{}
			}

			// Extract editor configuration
			useConfig := false
			if cfg.TUI != nil && cfg.TUI.NvimEmbed != nil {
				useConfig = cfg.TUI.NvimEmbed.UserConfig
			}

			// Initialize the model
			m, err := initialNvimDemoModel(useConfig, fileToOpen)
			if err != nil {
				return fmt.Errorf("failed to initialize nvim demo: %w", err)
			}

			// Start the Bubble Tea program
			p := tea.NewProgram(m, tea.WithAltScreen())
			if _, err := p.Run(); err != nil {
				return fmt.Errorf("error running nvim demo: %w", err)
			}

			return nil
		},
	}

	return cmd
}
