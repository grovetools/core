// Package logs provides an embeddable bubbletea model that tails
// grove-core log files (both per-workspace and system) and renders
// them in a list + details split view. This is the extracted form of
// the `core logs --tui` model — host applications (such as the grove
// terminal) embed it directly as a panel, while the CLI entrypoint in
// core/cmd/logs_tui.go runs it standalone via tea.NewProgram.
//
// The extraction follows the grovetools embed contract (see
// core/tui/embed): tea.Quit is never called directly (DoneMsg is
// returned instead), the model accepts SetWorkspaceMsg to repoint its
// filter, and all background goroutines are bound to a context passed
// into New() so Close() can cleanly tear them down.
package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	stdlog "log"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hpcloud/tail"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/logging"
	logskeymap "github.com/grovetools/core/pkg/keymap"
	"github.com/grovetools/core/pkg/logging/logutil"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/tui/components/help"
	"github.com/grovetools/core/tui/components/jsontree"
	"github.com/grovetools/core/tui/embed"
	"github.com/grovetools/core/tui/theme"
)

// Config is the public constructor payload for New. GetWorkspaces is
// called from the background discovery goroutine on every tick, so
// host implementations must return a snapshot that is safe to read
// concurrently with the host's own mutations (typically by copying
// under a read lock before returning).
type Config struct {
	// GetWorkspaces returns the current ecosystem workspace set. The
	// model calls it once per discovery tick (every 500ms) so newly
	// registered workspaces start being tailed without re-constructing
	// the model.
	GetWorkspaces func() []*workspace.WorkspaceNode
	// Ecosystem is set to show logs from every workspace returned by
	// GetWorkspaces simultaneously. When false, only lines from the
	// active workspace (as set via embed.SetWorkspaceMsg) are shown.
	Ecosystem bool
	// SystemOnly shows only the central system log stream.
	SystemOnly bool
	// IncludeSystem includes system-scoped entries that have no
	// explicit workspace context even when not in ecosystem mode.
	IncludeSystem bool
	// LogConfig is the initial logging visibility config. May be nil;
	// the model loads defaults and refreshes on SetWorkspaceMsg.
	LogConfig *logging.Config
	// OverrideOpts carries CLI filter overrides (--show-all,
	// --component, --also-show, --ignore-hide).
	OverrideOpts *logging.OverrideOptions
	// Follow turns on auto-scroll on new entries at construction.
	Follow bool
	// InitialWorkspacePath seeds the active-workspace filter before
	// the host has had a chance to broadcast embed.SetWorkspaceMsg.
	// When non-empty and Ecosystem is false, the model will only
	// display log lines whose WorkspacePath matches this path (plus
	// system entries, per IncludeSystem). Ignored when Ecosystem is
	// true.
	InitialWorkspacePath string
}

// paneFocus tracks which pane has focus.
type paneFocus int

const (
	listPane paneFocus = iota
	viewportPane
)

// LogScope selects which log sources the viewer tails and displays.
// Project shows only the active workspace's logs; Ecosystem shows every
// workspace plus system logs; System shows only the central daemon log
// stream. The scope is mutable at runtime via the ToggleScope keybinding.
type LogScope int

const (
	ScopeProject LogScope = iota
	ScopeEcosystem
	ScopeSystem
)

// String returns the human-readable label used in the status bar and
// status messages.
func (s LogScope) String() string {
	switch s {
	case ScopeProject:
		return "Project"
	case ScopeEcosystem:
		return "Ecosystem"
	case ScopeSystem:
		return "System"
	default:
		return "Unknown"
	}
}

// logKeyMapT is an internal alias for the shared logs keymap type.
type logKeyMapT = logskeymap.LogKeyMap

// logItem represents a single log entry.
type logItem struct {
	workspace     string
	workspacePath string
	level         string
	message       string
	component     string
	timestamp     time.Time
	rawData       map[string]interface{}
	// style hook — the item delegate pulls workspace color styling
	// from its owning model, so items themselves are plain value types.
	styleFn func(string) lipgloss.Style
}

// Title renders the compact one-line view for a log item.
func (i logItem) Title() string {
	wsStyle := i.workspaceStyle()
	levelStyle := themeLevelStyle(i.level)
	timeStyle := theme.DefaultTheme.Muted
	componentStyle := theme.DefaultTheme.Muted.Bold(true)

	return fmt.Sprintf("%s %s %s %s %s",
		wsStyle.Render(fmt.Sprintf("[%s]", i.workspace)),
		levelStyle.Render(fmt.Sprintf("[%s]", strings.ToUpper(i.level))),
		timeStyle.Render(i.timestamp.Format("2006-01-02 15:04:05")),
		componentStyle.Render(fmt.Sprintf("[%s]", i.component)),
		i.message,
	)
}

func (i logItem) Description() string { return "" }
func (i logItem) FilterValue() string { return i.component }

func (i logItem) workspaceStyle() lipgloss.Style {
	if i.styleFn != nil {
		return i.styleFn(i.workspace)
	}
	return lipgloss.NewStyle()
}

// FormatDetails returns the multi-line detail pane body for a log
// entry, including source info, categorized fields, and pretty output.
func (i logItem) FormatDetails() string {
	var lines []string

	headerStyle := theme.DefaultTheme.Header
	lines = append(lines, headerStyle.Render("Log Entry Details"))
	lines = append(lines, "")

	wsStyle := i.workspaceStyle()
	levelStyle := themeLevelStyle(i.level)
	timeStyle := theme.DefaultTheme.Muted
	componentStyle := theme.DefaultTheme.Muted.Bold(true)

	lines = append(lines, fmt.Sprintf("Workspace:  %s", wsStyle.Render(i.workspace)))
	lines = append(lines, fmt.Sprintf("Level:      %s", levelStyle.Render(strings.ToUpper(i.level))))
	lines = append(lines, fmt.Sprintf("Component:  %s", componentStyle.Render(i.component)))
	lines = append(lines, fmt.Sprintf("Time:       %s", timeStyle.Render(i.timestamp.Format("2006-01-02 15:04:05"))))
	lines = append(lines, fmt.Sprintf("Message:    %s", i.message))

	if prettyAnsi, ok := i.rawData["pretty_ansi"].(string); ok && prettyAnsi != "" {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("Output:     %s", prettyAnsi))
	}

	lines = append(lines, "")

	standardFields := map[string]bool{
		"level": true, "msg": true, "component": true, "time": true, "_verbosity": true,
		"pretty_ansi": true, "pretty_text": true,
	}

	var fileInfo, funcInfo string
	if file, ok := i.rawData["file"].(string); ok {
		fileInfo = file
	}
	if fn, ok := i.rawData["func"].(string); ok {
		funcInfo = fn
	}

	var verbosityMap map[string]int
	if verbosityRaw, exists := i.rawData["_verbosity"]; exists {
		if verbosityMapInterface, ok := verbosityRaw.(map[string]interface{}); ok {
			verbosityMap = make(map[string]int)
			for k, val := range verbosityMapInterface {
				if intVal, ok := val.(float64); ok {
					verbosityMap[k] = int(intVal)
				}
			}
		}
	}

	fieldStyle := theme.DefaultTheme.Muted
	fileStyle := theme.DefaultTheme.Muted
	borderStyle := theme.DefaultTheme.Muted

	if fileInfo != "" || funcInfo != "" {
		lines = append(lines, borderStyle.Render("┌─ Source:"))
		if fileInfo != "" {
			lines = append(lines, fileStyle.Render(fmt.Sprintf("│ %s %s", theme.IconArchive, fileInfo)))
		}
		if funcInfo != "" {
			lines = append(lines, fileStyle.Render(fmt.Sprintf("│ %s %s", theme.IconShell, funcInfo)))
		}
	}

	fieldsByLevel := map[int][]string{
		0: {}, 1: {}, 2: {}, 3: {},
	}

	for k, value := range i.rawData {
		if !standardFields[k] && k != "file" && k != "func" {
			var formattedValue string
			switch v := value.(type) {
			case map[string]interface{}, []interface{}:
				jsonBytes, err := json.MarshalIndent(v, "", "  ")
				if err == nil {
					formattedValue = "\n" + string(jsonBytes)
				} else {
					formattedValue = fmt.Sprintf("%v", v)
				}
			case string:
				formattedValue = v
			case float64:
				if v == float64(int64(v)) {
					formattedValue = fmt.Sprintf("%.0f", v)
				} else {
					formattedValue = fmt.Sprintf("%.2f", v)
				}
			case bool:
				formattedValue = fmt.Sprintf("%t", v)
			default:
				formattedValue = fmt.Sprintf("%v", v)
			}

			verbosityLevel := 0
			if verbosityMap != nil {
				if level, exists := verbosityMap[k]; exists {
					verbosityLevel = level
				}
			}

			if verbosityLevel < 4 {
				fieldsByLevel[verbosityLevel] = append(fieldsByLevel[verbosityLevel], fmt.Sprintf("%-20s %s", k+":", formattedValue))
			}
		}
	}

	hasFields := false
	for _, fields := range fieldsByLevel {
		if len(fields) > 0 {
			hasFields = true
			break
		}
	}

	if hasFields {
		if fileInfo != "" || funcInfo != "" {
			lines = append(lines, borderStyle.Render("├─ Fields:"))
		} else {
			lines = append(lines, borderStyle.Render("┌─ Fields:"))
		}

		for level := 0; level < 4; level++ {
			if fields := fieldsByLevel[level]; len(fields) > 0 {
				sort.Strings(fields)
				for idx, field := range fields {
					hasMoreFields := false
					for checkLevel := level + 1; checkLevel < 4; checkLevel++ {
						if len(fieldsByLevel[checkLevel]) > 0 {
							hasMoreFields = true
							break
						}
					}
					isLast := (level == 3 || len(fieldsByLevel[level+1]) == 0) && idx == len(fields)-1
					if isLast && !hasMoreFields {
						lines = append(lines, fieldStyle.Render(fmt.Sprintf("└─ %s", field)))
					} else {
						lines = append(lines, fieldStyle.Render(fmt.Sprintf("├─ %s", field)))
					}
				}
			}
		}
	}

	return strings.Join(lines, "\n")
}

// themeLevelStyle returns theme-based styling for log levels.
func themeLevelStyle(level string) lipgloss.Style {
	switch strings.ToLower(level) {
	case "info":
		return theme.DefaultTheme.Success
	case "warning", "warn":
		return theme.DefaultTheme.Warning
	case "error", "fatal", "panic":
		return theme.DefaultTheme.Error
	case "debug", "trace":
		return theme.DefaultTheme.Muted
	default:
		return lipgloss.NewStyle()
	}
}

// itemDelegate renders log items with cursor/visual highlighting.
type itemDelegate struct {
	model *Model
}

func (d itemDelegate) Height() int                               { return 1 }
func (d itemDelegate) Spacing() int                              { return 0 }
func (d itemDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd { return nil }

func (d itemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i, ok := item.(logItem)
	if !ok {
		return
	}
	str := i.Title()

	isVisuallySelected := false
	if d.model != nil && d.model.visualMode {
		minIdx := d.model.visualStart
		maxIdx := d.model.visualEnd
		if minIdx > maxIdx {
			minIdx, maxIdx = maxIdx, minIdx
		}
		isVisuallySelected = index >= minIdx && index <= maxIdx
	}

	isSelected := index == m.Index()
	isFocused := d.model == nil || d.model.focus == listPane

	if isVisuallySelected {
		str = theme.DefaultTheme.VisualSelection.Render(str)
	} else if isSelected && isFocused {
		str = theme.DefaultTheme.Selected.Render(str)
	} else if isSelected && !isFocused {
		str = theme.DefaultTheme.SelectedUnfocused.Render(str)
	}

	fmt.Fprint(w, str)
}

// Model is the extracted logs TUI bubbletea model. It satisfies
// tea.Model and the grovetools embed contract.
type Model struct {
	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	cfg    Config

	// Active filter (set via embed.SetWorkspaceMsg). When cfg.Ecosystem
	// is true the filter is ignored and all entries are shown.
	activeWorkspacePath string

	// Data: master `items` is bounded at 10,000 entries; `visible`
	// holds the subset currently displayed in `list` per the active
	// filter. This avoids re-filtering on every append and keeps the
	// O(N) rebuild confined to filter-change events.
	items   []logItem
	visible []list.Item

	// UI
	list           list.Model
	keys           logKeyMapT
	spinner        spinner.Model
	viewport       viewport.Model
	help           help.Model
	width          int
	height         int
	followMode     bool
	filtersEnabled bool
	filteredCount  int
	ready          bool
	focus          paneFocus
	visualMode     bool
	visualStart    int
	visualEnd      int
	statusMessage  string
	jsonTree       jsontree.Model
	jsonView       bool
	lastGotoG      time.Time

	// Filter config
	logConfig     *logging.Config
	overrideOpts  *logging.OverrideOptions
	activeScope   LogScope
	includeSystem bool

	// wsCtx is a sub-context of ctx that bounds the lifetime of the
	// per-workspace tailing goroutines. When SetWorkspaceMsg rotates
	// the active workspace, we cancel the old wsCtx (halting every
	// tailer bound to it), reset tailedFiles, and allocate a fresh
	// child context. The parent ctx stays alive for the whole panel
	// lifetime so the discovery loop itself keeps running across
	// switches — only the per-file tailers recycle. wsCtxMu guards
	// reads from the discovery goroutine against Update-side
	// mutations on the bubbletea event loop.
	wsCtx    context.Context
	wsCancel context.CancelFunc
	wsCtxMu  sync.Mutex

	// Channel and tailing state — instance-owned (formerly globals)
	logChan       chan logutil.TailedLine
	tailedFiles   map[string]bool
	tailedFilesMu sync.Mutex

	// Workspace coloring (formerly module-level globals)
	workspaceColorMap   map[string]lipgloss.Style
	workspaceColorIndex int
	colorMu             sync.Mutex
}

// New constructs a Model bound to ctx. The caller MUST eventually
// either cancel ctx or call Close() on the returned model to stop
// the background tailing goroutines.
func New(ctx context.Context, cfg Config) *Model {
	ctx, cancel := context.WithCancel(ctx)

	keys := func() logskeymap.LogKeyMap {
		c, _ := config.LoadDefault()
		return logskeymap.NewLogKeyMap(c)
	}()

	logCfg := cfg.LogConfig
	if logCfg == nil {
		defaultCfg := logging.GetDefaultLoggingConfig()
		if c, err := config.LoadDefault(); err == nil {
			_ = c.UnmarshalExtension("logging", &defaultCfg)
		}
		logCfg = &defaultCfg
	}

	l := list.New([]list.Item{}, itemDelegate{}, 0, 0)
	l.Title = "Grove Logs"
	l.SetShowStatusBar(false)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	l.SetShowPagination(true)
	l.InfiniteScrolling = false
	l.DisableQuitKeybindings()
	l.Styles.PaginationStyle = theme.DefaultTheme.Muted.PaddingLeft(2)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = theme.DefaultTheme.Highlight

	m := &Model{
		ctx:                 ctx,
		cancel:              cancel,
		cfg:                 cfg,
		activeWorkspacePath: cfg.InitialWorkspacePath,
		list:                l,
		keys:                keys,
		spinner:             sp,
		help:                help.New(keys),
		followMode:          cfg.Follow,
		filtersEnabled:      false,
		logChan:             make(chan logutil.TailedLine, 100),
		logConfig:           logCfg,
		overrideOpts:        cfg.OverrideOpts,
		includeSystem:       cfg.IncludeSystem,
		tailedFiles:         make(map[string]bool),
		workspaceColorMap:   make(map[string]lipgloss.Style),
	}
	switch {
	case cfg.SystemOnly:
		m.activeScope = ScopeSystem
	case cfg.Ecosystem:
		m.activeScope = ScopeEcosystem
	default:
		m.activeScope = ScopeProject
	}
	// Initialize the per-workspace sub-context that tailing goroutines
	// bind to. Rotated on every workspace switch via SetWorkspaceMsg.
	m.wsCtx, m.wsCancel = context.WithCancel(ctx)
	m.list.SetDelegate(itemDelegate{model: m})
	return m
}

// currentWorkspaceCtx returns a snapshot of the active wsCtx. Called
// from the discovery goroutine on each tick so it can pass the
// current child context into newly-spawned tailers. Holding wsCtxMu
// briefly is fine — all rotations happen on the bubbletea event loop.
func (m *Model) currentWorkspaceCtx() context.Context {
	m.wsCtxMu.Lock()
	defer m.wsCtxMu.Unlock()
	return m.wsCtx
}

// rotateWorkspaceCtx cancels the active wsCtx (halting every tailer
// bound to it) and allocates a fresh child of m.ctx. Called from
// Update when SetWorkspaceMsg arrives with a different workspace
// path. Returns the new context so the caller can pass it directly
// into subsequent work without re-acquiring the lock.
func (m *Model) rotateWorkspaceCtx() context.Context {
	m.wsCtxMu.Lock()
	defer m.wsCtxMu.Unlock()
	if m.wsCancel != nil {
		m.wsCancel()
	}
	m.wsCtx, m.wsCancel = context.WithCancel(m.ctx)
	return m.wsCtx
}

// resolveWorkspaceName returns a display name for the given workspace
// path by scanning cfg.GetWorkspaces. Falls back to the basename of
// the path if no match is found (e.g. during startup before the host
// has populated the enrichment cache).
func (m *Model) resolveWorkspaceName(path string) string {
	if path == "" {
		return ""
	}
	if m.cfg.GetWorkspaces != nil {
		for _, ws := range m.cfg.GetWorkspaces() {
			if ws != nil && ws.Path == path {
				return ws.Name
			}
		}
	}
	return filepath.Base(path)
}

// Close cancels the model's context, unblocking all tailing goroutines
// and the waitForLogs cmd. Safe to call multiple times.
func (m *Model) Close() error {
	if m.cancel != nil {
		m.cancel()
	}
	return nil
}

// Init kicks off the background discovery/tailing loop and arms the
// first channel-wait, spinner, and ticker commands. Discovery runs off
// the bubbletea event loop so the first paint is instant even with a
// large seed workspace set.
func (m *Model) Init() tea.Cmd {
	go m.discoveryLoop()
	return tea.Batch(
		m.spinner.Tick,
		m.waitForLogs(),
		tick(),
	)
}

// tick emits a plain tickMsg every 100ms so the View re-renders and
// any time-sensitive state (spinner, follow-mode autoscroll) stays
// fluid even without a steady stream of new log messages.
func tick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// Messages
type newLogMsg struct {
	workspace     string
	workspacePath string
	data          map[string]interface{}
}
type (
	tickMsg        time.Time
	clearStatusMsg struct{}
)

// discoveryLoop runs every 500ms, calling cfg.GetWorkspaces() to
// re-scan each known workspace's .grove/logs directory plus the
// central system logs dir. Any newly-seen file kicks off a new
// per-file tailing goroutine via startTailing; existing files are
// skipped via m.tailedFiles. All goroutines bind to m.ctx so cancel
// cleanly tears down the whole tree.
func (m *Model) discoveryLoop() {
	// Initial pass immediately so entries show up on first paint.
	m.discoverAndTailFiles()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.discoverAndTailFiles()
		}
	}
}

// discoverAndTailFiles rescans the filesystem for log files that
// match the current configuration and hands any newly-seen file to
// startTailing. Runs on every 500ms discovery tick plus once
// synchronously on first Init.
//
// The workspace-selection logic has three modes:
//
//  1. Ecosystem mode (cfg.Ecosystem == true): tail every workspace
//     returned by GetWorkspaces simultaneously. Used by
//     `core logs --ecosystem --tui` from the CLI.
//
//  2. Single-workspace embed mode (Ecosystem false,
//     activeWorkspacePath != ""): tail ONLY the .grove/logs directory
//     of the currently active workspace. This is the default for the
//     grove terminal log drawer — cross-workspace logs are almost
//     never wanted from the panel, so spinning up tailers for every
//     workspace in the ecosystem is wasteful. Switching workspaces
//     via SetWorkspaceMsg cancels the old wsCtx and this function
//     re-globs under the new path on the next tick.
//
//  3. CLI fallback mode (Ecosystem false, activeWorkspacePath empty):
//     tail every workspace in GetWorkspaces(). Preserves the
//     pre-extraction CLI behavior where `core logs --tui` (with
//     optional `-w` filter) tails multiple explicitly-named
//     workspaces without any filtering layer above.
//
// In every mode we use FindLatestLogFile (now lexically sorted by
// ISO date) rather than `filepath.Glob("*.log")` so only TODAY's
// file is tailed — this prevents stale `workspace-2025-*.log`
// files from getting replayed from offset 0 when the panel opens.
func (m *Model) discoverAndTailFiles() {
	wsCtx := m.currentWorkspaceCtx()
	if wsCtx.Err() != nil {
		return
	}

	type dirEntry struct {
		dir    string
		wsName string
		wsPath string
	}
	var dirs []dirEntry

	switch m.activeScope {
	case ScopeSystem:
		// Skip workspace discovery — system logs tailed below.
	case ScopeProject:
		if m.activeWorkspacePath != "" {
			// Single-workspace embed mode.
			dirs = append(dirs, dirEntry{
				dir:    filepath.Join(m.activeWorkspacePath, ".grove", "logs"),
				wsName: m.resolveWorkspaceName(m.activeWorkspacePath),
				wsPath: m.activeWorkspacePath,
			})
		} else {
			// CLI fallback: no active workspace path — tail everything
			// from GetWorkspaces so `core logs --tui` preserves its
			// pre-extraction multi-workspace behavior.
			var workspaces []*workspace.WorkspaceNode
			if m.cfg.GetWorkspaces != nil {
				workspaces = m.cfg.GetWorkspaces()
			}
			for _, ws := range workspaces {
				if ws == nil {
					continue
				}
				dirs = append(dirs, dirEntry{
					dir:    filepath.Join(ws.Path, ".grove", "logs"),
					wsName: ws.Name,
					wsPath: ws.Path,
				})
			}
		}
	case ScopeEcosystem:
		var workspaces []*workspace.WorkspaceNode
		if m.cfg.GetWorkspaces != nil {
			workspaces = m.cfg.GetWorkspaces()
		}
		for _, ws := range workspaces {
			if ws == nil {
				continue
			}
			dirs = append(dirs, dirEntry{
				dir:    filepath.Join(ws.Path, ".grove", "logs"),
				wsName: ws.Name,
				wsPath: ws.Path,
			})
		}
	}

	for _, d := range dirs {
		latest, err := logutil.FindLatestLogFile(d.dir)
		if err != nil {
			continue
		}
		m.startTailing(wsCtx, latest, d.wsName, d.wsPath)
	}

	// System logs: tail whenever the scope requires them, or the caller
	// explicitly opted in via IncludeSystem.
	if m.activeScope == ScopeSystem || m.activeScope == ScopeEcosystem || m.includeSystem {
		systemLogsDir := logutil.GetSystemLogsDir()
		if latest, err := logutil.FindLatestLogFile(systemLogsDir); err == nil {
			m.startTailing(wsCtx, latest, "system", "")
		}
	}
}

// startTailing spawns a goroutine that tails a single log file and
// funnels new lines into m.logChan. The goroutine is bound to the
// caller-supplied wsCtx — not m.ctx — so canceling that context on
// a workspace switch halts the tailer before the panel starts
// feeding a stale file into the active workspace's view.
//
// The initial SeekInfo is now {Offset: 0, Whence: SeekEnd}: we do
// NOT replay the historical contents of the file. The embedded
// drawer is meant for "what's happening now" — users who want
// backlog use `core logs --tui` or `core logs -f` from the CLI.
// This eliminates the bulk of the stale-log problem the user saw
// with old workspace-2025-*.log files.
func (m *Model) startTailing(wsCtx context.Context, path, wsName, wsPath string) {
	m.tailedFilesMu.Lock()
	if m.tailedFiles[path] {
		m.tailedFilesMu.Unlock()
		return
	}
	m.tailedFiles[path] = true
	m.tailedFilesMu.Unlock()

	go func() {
		cfg := tail.Config{
			Follow: true,
			ReOpen: true,
			// Stream only new lines — never replay historical content.
			Location: &tail.SeekInfo{Offset: 0, Whence: io.SeekEnd},
			Logger:   stdlog.New(io.Discard, "", 0),
		}
		t, err := tail.TailFile(path, cfg)
		if err != nil {
			return
		}
		defer t.Cleanup()
		for {
			select {
			case <-wsCtx.Done():
				_ = t.Stop()
				return
			case line, ok := <-t.Lines:
				if !ok {
					return
				}
				if line.Err != nil {
					continue
				}
				select {
				case <-wsCtx.Done():
					_ = t.Stop()
					return
				case m.logChan <- logutil.TailedLine{Workspace: wsName, WorkspacePath: wsPath, Line: line.Text}:
				}
			}
		}
	}()
}

// waitForLogs blocks on the next entry from the tail channel or the
// context cancellation, whichever comes first. This is the key fix
// that prevents the bubbletea runtime from parking forever on a
// channel that the tailers have silently stopped feeding — on Close
// we cancel m.ctx, which unblocks the select and exits the cmd
// cleanly.
func (m *Model) waitForLogs() tea.Cmd {
	return func() tea.Msg {
		for {
			select {
			case <-m.ctx.Done():
				return nil
			case line, ok := <-m.logChan:
				if !ok {
					return nil
				}
				var rawEntry map[string]interface{}
				if err := json.Unmarshal([]byte(line.Line), &rawEntry); err != nil {
					// Skip non-JSON lines and try again.
					continue
				}
				return newLogMsg{
					workspace:     line.Workspace,
					workspacePath: line.WorkspacePath,
					data:          rawEntry,
				}
			}
		}
	}
}

// workspaceStyleFor returns a consistent lipgloss style for the given
// workspace display name, cached so multiple renders of the same
// workspace pick the same color.
func (m *Model) workspaceStyleFor(ws string) lipgloss.Style {
	m.colorMu.Lock()
	defer m.colorMu.Unlock()
	if style, ok := m.workspaceColorMap[ws]; ok {
		return style
	}
	color := theme.DefaultTheme.AccentColors[m.workspaceColorIndex%len(theme.DefaultTheme.AccentColors)]
	style := lipgloss.NewStyle().Foreground(color).Bold(true)
	m.workspaceColorMap[ws] = style
	m.workspaceColorIndex++
	return style
}

// matchesActiveFilter returns true when the given item should be
// visible under the current active scope.
func (m *Model) matchesActiveFilter(it logItem) bool {
	switch m.activeScope {
	case ScopeEcosystem:
		return true
	case ScopeSystem:
		return it.workspacePath == ""
	case ScopeProject:
		if it.workspacePath == "" {
			return m.includeSystem || m.activeWorkspacePath == ""
		}
		if m.activeWorkspacePath == "" {
			return true
		}
		return it.workspacePath == m.activeWorkspacePath
	}
	return false
}

// rebuildVisible recomputes m.visible from m.items under the current
// filter. Called on filter-change events (SetWorkspaceMsg). The list
// model is updated with the new items slice at the end.
func (m *Model) rebuildVisible() {
	m.visible = m.visible[:0]
	for _, it := range m.items {
		if m.matchesActiveFilter(it) {
			m.visible = append(m.visible, it)
		}
	}
	m.list.SetItems(m.visible)
}

// clearStatusMessageAfter emits a clearStatusMsg after d elapses so
// transient status-bar notifications (copy confirmations, toggle
// acks) clear themselves without an extra user action.
func (m *Model) clearStatusMessageAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return clearStatusMsg{}
	})
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func (m *Model) getSelectedContent() string {
	minIdx := m.visualStart
	maxIdx := m.visualEnd
	if minIdx > maxIdx {
		minIdx, maxIdx = maxIdx, minIdx
	}

	visibleItems := m.list.VisibleItems()

	var logs []map[string]interface{}
	for i := minIdx; i <= maxIdx && i < len(visibleItems); i++ {
		if item, ok := visibleItems[i].(logItem); ok {
			logEntry := make(map[string]interface{})
			for k, v := range item.rawData {
				logEntry[k] = v
			}
			logEntry["workspace"] = item.workspace
			logs = append(logs, logEntry)
		}
	}

	jsonBytes, err := json.MarshalIndent(logs, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error formatting JSON: %v", err)
	}
	return string(jsonBytes)
}

func (m *Model) copyToClipboard(content string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		} else {
			return fmt.Errorf("no clipboard utility found (install xclip or xsel)")
		}
	case "windows":
		cmd = exec.Command("cmd", "/c", "clip")
	default:
		return fmt.Errorf("unsupported platform")
	}
	cmd.Stdin = strings.NewReader(content)
	return cmd.Run()
}

// doneCmd returns a tea.Cmd that delivers embed.DoneMsg. This replaces
// every tea.Quit site in the original CLI model — when embedded, the
// host catches DoneMsg to close the panel; when running standalone,
// the CLI shim in core/cmd/logs_tui.go intercepts DoneMsg and returns
// tea.Quit.
func doneCmd() tea.Cmd {
	return func() tea.Msg { return embed.DoneMsg{} }
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Embed contract messages
	switch msg := msg.(type) {
	case embed.SetWorkspaceMsg:
		newPath := ""
		if msg.Node != nil {
			newPath = msg.Node.Path
		}
		if newPath == m.activeWorkspacePath {
			// Same workspace — no-op. Avoids churning tailers when the
			// host re-broadcasts SetWorkspaceMsg on every session
			// activation.
			return m, nil
		}

		// Cancel every tailer bound to the previous workspace so the
		// goroutines exit before we start new ones. The parent ctx
		// stays alive so the discovery loop keeps running — only the
		// per-file tailers rotate.
		m.rotateWorkspaceCtx()

		// Reset the tailedFiles ledger so the next discovery tick
		// re-adds the new workspace's files. Stale entries would
		// otherwise make startTailing skip them.
		m.tailedFilesMu.Lock()
		m.tailedFiles = make(map[string]bool)
		m.tailedFilesMu.Unlock()

		// Drop buffered log entries from the old workspace — they're
		// no longer relevant to the view and letting them linger just
		// confuses the `m/n` position counter. If we later want to
		// keep a cross-workspace backlog we'd swap this for a
		// per-workspace item cache, but the user's explicit ask is
		// "almost never need cross-workspace logs", so just flush.
		m.items = nil
		m.visible = m.visible[:0]
		m.list.SetItems(m.visible)

		m.activeWorkspacePath = newPath

		// Reload logging config from the new workspace path so
		// per-workspace [logging] rules take effect.
		if msg.Node != nil {
			if cfg, err := config.LoadFrom(msg.Node.Path); err == nil && cfg != nil {
				logCfg := logging.GetDefaultLoggingConfig()
				_ = cfg.UnmarshalExtension("logging", &logCfg)
				m.logConfig = &logCfg
			}
		}
		return m, nil

	case embed.FocusMsg:
		// No-op: tailing runs independently of focus so the backlog
		// stays warm across focus cycles.
		return m, nil

	case embed.BlurMsg:
		return m, nil
	}

	// Handle jsontree.BackMsg to exit JSON view
	if _, ok := msg.(jsontree.BackMsg); ok {
		m.jsonView = false
		return m, nil
	}

	// If help is showing, handle ESC to close it
	if m.help.ShowAll {
		if kmsg, ok := msg.(tea.KeyMsg); ok {
			if key.Matches(kmsg, m.keys.Base.Quit) {
				return m, doneCmd()
			}
			if key.Matches(kmsg, m.keys.Clear) || kmsg.String() == "esc" {
				m.help.Toggle()
				return m, nil
			}
		}
		return m, nil
	}

	// If in JSON view, delegate updates to the JSON tree component
	if m.jsonView {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			if key.Matches(msg, m.keys.Base.Quit) {
				return m, doneCmd()
			}
			if key.Matches(msg, m.keys.Base.Help) {
				m.help.Toggle()
				return m, nil
			}
			if key.Matches(msg, m.keys.SwitchFocus) {
				if m.focus == listPane {
					m.focus = viewportPane
					m.jsonTree.SetSize(m.width-4, m.height-3)
				} else {
					m.focus = listPane
					listHeight := m.height / 2
					viewportHeight := m.height - listHeight - 3
					m.jsonTree.SetSize(m.width-4, viewportHeight)
				}
				return m, nil
			}
		case tea.WindowSizeMsg:
			if m.focus == viewportPane {
				m.jsonTree.SetSize(msg.Width-4, m.height-3)
			} else {
				listHeight := m.height / 2
				viewportHeight := m.height - listHeight - 3
				m.jsonTree.SetSize(msg.Width-4, viewportHeight)
			}
		}
		newTree, cmd := m.jsonTree.Update(msg)
		m.jsonTree = newTree.(jsontree.Model)
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.list.FilterState() == list.Filtering {
			switch {
			case key.Matches(msg, m.keys.Base.Quit):
				return m, doneCmd()
			case key.Matches(msg, m.keys.Clear):
				m.list.ResetFilter()
				return m, nil
			}
		} else {
			if key.Matches(msg, m.keys.GotoTop) {
				if time.Since(m.lastGotoG) < 500*time.Millisecond {
					m.list.Select(0)
					m.lastGotoG = time.Time{}
					return m, nil
				}
				m.lastGotoG = time.Now()
				return m, nil
			}

			switch {
			case key.Matches(msg, m.keys.Base.Quit):
				return m, doneCmd()

			case key.Matches(msg, m.keys.Base.Help):
				m.help.Toggle()
				return m, nil

			case key.Matches(msg, m.keys.SwitchFocus) || key.Matches(msg, m.keys.Expand):
				if m.focus == listPane {
					m.focus = viewportPane
					m.viewport.Height = m.height - 3
				} else {
					m.focus = listPane
					listHeight := m.height / 2
					m.viewport.Height = m.height - listHeight - 3
				}
				return m, nil
			}

			if m.focus == viewportPane {
				if key.Matches(msg, m.keys.Base.Left) {
					currentIndex := m.list.Index()
					if currentIndex > 0 {
						m.list.Select(currentIndex - 1)
						if selectedItem := m.list.SelectedItem(); selectedItem != nil {
							if li, ok := selectedItem.(logItem); ok {
								m.viewport.SetContent(li.FormatDetails())
								m.viewport.GotoTop()
							}
						}
					}
					return m, nil
				}

				if key.Matches(msg, m.keys.Base.Right) {
					currentIndex := m.list.Index()
					visibleItems := len(m.list.VisibleItems())
					if currentIndex < visibleItems-1 {
						m.list.Select(currentIndex + 1)
						if selectedItem := m.list.SelectedItem(); selectedItem != nil {
							if li, ok := selectedItem.(logItem); ok {
								m.viewport.SetContent(li.FormatDetails())
								m.viewport.GotoTop()
							}
						}
					}
					return m, nil
				}

				if key.Matches(msg, m.keys.Clear) || msg.String() == "esc" {
					m.focus = listPane
					listHeight := m.height / 2
					m.viewport.Height = m.height - listHeight - 3
					return m, nil
				}
				if key.Matches(msg, m.keys.ViewJSON) {
					if selectedItem := m.list.SelectedItem(); selectedItem != nil {
						if li, ok := selectedItem.(logItem); ok {
							var jsonData interface{}
							for _, v := range li.rawData {
								switch v.(type) {
								case map[string]interface{}, []interface{}:
									jsonData = v
								}
								if jsonData != nil {
									break
								}
							}
							if jsonData != nil {
								m.jsonTree = jsontree.New(jsonData)
								m.jsonTree.SetSize(m.width-4, m.height-3)
								m.jsonView = true
							} else {
								m.statusMessage = "No JSON data in this log entry"
							}
						}
					}
					return m, nil
				}
				var cmd tea.Cmd
				m.viewport, cmd = m.viewport.Update(msg)
				return m, cmd
			}

			// List pane focused
			switch {
			case key.Matches(msg, m.keys.VisualModeStart):
				if !m.visualMode {
					m.visualMode = true
					m.visualStart = m.list.Index()
					m.visualEnd = m.list.Index()
				} else {
					m.visualMode = false
				}
				m.list.SetDelegate(itemDelegate{model: m})
				m.list.SetItems(m.list.Items())
				return m, nil

			case key.Matches(msg, m.keys.Yank):
				if m.visualMode {
					content := m.getSelectedContent()
					if err := m.copyToClipboard(content); err == nil {
						lineCount := absInt(m.visualEnd-m.visualStart) + 1
						m.statusMessage = fmt.Sprintf("Copied %d log entries as JSON", lineCount)
					} else {
						m.statusMessage = fmt.Sprintf("Copy failed: %v", err)
					}
					m.visualMode = false
					m.list.SetDelegate(itemDelegate{model: m})
					return m, m.clearStatusMessageAfter(2 * time.Second)
				}
				return m, nil

			case key.Matches(msg, m.keys.Clear):
				if m.visualMode {
					m.visualMode = false
					m.statusMessage = ""
					m.list.SetDelegate(itemDelegate{model: m})
					return m, nil
				}

			case key.Matches(msg, m.keys.GotoEnd):
				m.list.Select(len(m.visible) - 1)
				return m, nil

			case key.Matches(msg, m.keys.HalfUp):
				visibleHeight := m.height - 4
				halfPage := visibleHeight / 2
				currentIndex := m.list.Index()
				newIndex := currentIndex - halfPage
				if newIndex < 0 {
					newIndex = 0
				}
				m.list.Select(newIndex)
				return m, nil

			case key.Matches(msg, m.keys.HalfDown):
				visibleHeight := m.height - 4
				halfPage := visibleHeight / 2
				currentIndex := m.list.Index()
				newIndex := currentIndex + halfPage
				if newIndex >= len(m.visible) {
					newIndex = len(m.visible) - 1
				}
				m.list.Select(newIndex)
				return m, nil

			case key.Matches(msg, m.keys.Search):
				// Fall through to list.Update so it can start filtering.

			case key.Matches(msg, m.keys.ToggleFollow):
				m.followMode = !m.followMode
				if m.followMode {
					m.statusMessage = "Follow mode enabled"
				} else {
					m.statusMessage = "Follow mode disabled"
				}
				return m, m.clearStatusMessageAfter(2 * time.Second)

			case key.Matches(msg, m.keys.ToggleFilters):
				m.filtersEnabled = !m.filtersEnabled
				if m.filtersEnabled {
					m.statusMessage = "Filters enabled"
				} else {
					m.statusMessage = "Filters disabled (showing all)"
				}
				return m, m.clearStatusMessageAfter(2 * time.Second)

			case key.Matches(msg, m.keys.ToggleScope):
				// Cycle: Project -> Ecosystem -> System -> Project.
				switch m.activeScope {
				case ScopeProject:
					m.activeScope = ScopeEcosystem
				case ScopeEcosystem:
					m.activeScope = ScopeSystem
				case ScopeSystem:
					m.activeScope = ScopeProject
				}
				m.statusMessage = fmt.Sprintf("Scope: %s", m.activeScope)

				// Cancel current tailers, reset the ledger, and drop
				// accumulated items so the new scope starts fresh rather
				// than leaving stale entries in the view.
				m.rotateWorkspaceCtx()
				m.tailedFilesMu.Lock()
				m.tailedFiles = make(map[string]bool)
				m.tailedFilesMu.Unlock()
				m.items = nil
				m.visible = m.visible[:0]
				m.list.SetItems(m.visible)

				// Discover synchronously so the panel doesn't sit empty
				// for a full discovery tick after the keypress.
				m.discoverAndTailFiles()
				return m, m.clearStatusMessageAfter(2 * time.Second)

			case key.Matches(msg, m.keys.ViewJSON):
				if selectedItem := m.list.SelectedItem(); selectedItem != nil {
					if li, ok := selectedItem.(logItem); ok {
						var jsonData interface{}
						for _, v := range li.rawData {
							switch v.(type) {
							case map[string]interface{}, []interface{}:
								jsonData = v
							}
							if jsonData != nil {
								break
							}
						}
						if jsonData != nil {
							m.jsonTree = jsontree.New(jsonData)
							listHeight := m.height / 2
							viewportHeight := m.height - listHeight - 3
							m.jsonTree.SetSize(m.width-4, viewportHeight)
							m.jsonView = true
						} else {
							m.statusMessage = "No JSON data in this log entry"
							return m, m.clearStatusMessageAfter(2 * time.Second)
						}
					}
				}
				return m, nil
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		m.help.SetSize(msg.Width, msg.Height)

		// Drawer-geometry fix: when the available height is too small
		// to comfortably split list + details, allocate everything to
		// the list pane and hide the details pane entirely.
		if m.height < 15 {
			m.list.SetSize(msg.Width, m.height-1)
			viewportWidth := msg.Width - 12
			if viewportWidth < 1 {
				viewportWidth = 1
			}
			if !m.ready {
				m.viewport = viewport.New(viewportWidth, 1)
				m.ready = true
			} else {
				m.viewport.Width = viewportWidth
				m.viewport.Height = 1
			}
			return m, nil
		}

		listHeight := m.height / 2
		viewportHeight := m.height - listHeight - 3

		m.list.SetSize(msg.Width, listHeight)

		viewportWidth := msg.Width - 12
		if !m.ready {
			m.viewport = viewport.New(viewportWidth, viewportHeight)
			m.viewport.YPosition = listHeight + 1
			m.ready = true
		} else {
			m.viewport.Width = viewportWidth
			m.viewport.Height = viewportHeight
		}

		if selectedItem := m.list.SelectedItem(); selectedItem != nil {
			if li, ok := selectedItem.(logItem); ok {
				m.viewport.SetContent(li.FormatDetails())
			}
		}

		return m, nil

	case newLogMsg:
		level, _ := msg.data["level"].(string)
		message, _ := msg.data["msg"].(string)
		component, _ := msg.data["component"].(string)
		timeStr, _ := msg.data["time"].(string)

		// Drop entries that the active scope would never display. We
		// keep them out of m.items (not just m.visible) to avoid piling
		// up unbounded memory under long sessions. matchesActiveFilter
		// handles the final visibility decision further down.
		if msg.workspace == "system" {
			wsContext, _ := msg.data["workspace"].(string)
			if wsContext == "" && m.activeScope == ScopeProject && !m.includeSystem {
				return m, m.waitForLogs()
			}
		} else if m.activeScope == ScopeSystem {
			return m, m.waitForLogs()
		}

		// Component visibility filter
		if m.filtersEnabled && m.logConfig != nil {
			visibilityResult := logging.GetComponentVisibility(component, m.logConfig, m.overrideOpts)
			if !visibilityResult.Visible {
				m.filteredCount++
				return m, m.waitForLogs()
			}
		}

		var logTime time.Time
		if parsedTime, err := time.Parse(time.RFC3339, timeStr); err == nil {
			logTime = parsedTime
		}

		newItem := logItem{
			workspace:     msg.workspace,
			workspacePath: msg.workspacePath,
			level:         level,
			message:       message,
			component:     component,
			timestamp:     logTime,
			rawData:       msg.data,
			styleFn:       m.workspaceStyleFor,
		}

		// Append to master slice in timestamp order.
		i := sort.Search(len(m.items), func(j int) bool {
			return m.items[j].timestamp.After(newItem.timestamp)
		})
		if i == len(m.items) {
			m.items = append(m.items, newItem)
		} else {
			m.items = append(m.items, logItem{})
			copy(m.items[i+1:], m.items[i:])
			m.items[i] = newItem
		}

		// Enforce 10,000 cap, dropping oldest 1000 when we overflow so
		// we don't reallocate on every insert.
		if len(m.items) > 11000 {
			m.items = m.items[len(m.items)-10000:]
			// Rebuild visible slice because the drop may have evicted
			// currently-visible items.
			m.rebuildVisible()
		}

		// Two-slice approach: only append to visible if it matches.
		if m.matchesActiveFilter(newItem) {
			// If we inserted in the middle of items, we must rebuild
			// visible to maintain order. For the common append-at-end
			// case we can just append.
			if i == len(m.items)-1 {
				m.visible = append(m.visible, newItem)
				m.list.SetItems(m.visible)
			} else {
				m.rebuildVisible()
			}

			if m.followMode {
				m.list.Select(len(m.visible) - 1)
				if selectedItem := m.list.SelectedItem(); selectedItem != nil {
					if li, ok := selectedItem.(logItem); ok {
						m.viewport.SetContent(li.FormatDetails())
						m.viewport.GotoTop()
					}
				}
			}
		}

		return m, m.waitForLogs()

	case tickMsg:
		return m, tick()

	case clearStatusMsg:
		m.statusMessage = ""
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Delegate to inner list.
	prevIndex := m.list.Index()
	newListModel, cmd := m.list.Update(msg)
	m.list = newListModel
	cmds = append(cmds, cmd)

	if m.visualMode && m.list.Index() != prevIndex {
		m.visualEnd = m.list.Index()
		m.list.SetDelegate(itemDelegate{model: m})
	}

	if m.list.Index() != prevIndex {
		if selectedItem := m.list.SelectedItem(); selectedItem != nil {
			if li, ok := selectedItem.(logItem); ok {
				m.viewport.SetContent(li.FormatDetails())
				m.viewport.GotoTop()
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) View() string {
	if m.help.ShowAll {
		return m.help.View()
	}

	if !m.ready {
		return "Initializing..."
	}

	statusStyle := theme.DefaultTheme.Muted

	followIndicator := " [Follow:OFF]"
	if m.followMode {
		followIndicator = " [Follow:ON]"
	}

	filtersIndicator := " [Filters:OFF]"
	if m.filtersEnabled {
		filtersIndicator = " [Filters:ON]"
	}

	filteredCountIndicator := ""
	if m.filteredCount > 0 {
		filteredCountIndicator = fmt.Sprintf(" [%d hidden]", m.filteredCount)
	}

	filterIndicator := ""
	searchStyle := theme.DefaultTheme.Warning
	if m.list.FilterState() == list.Filtering {
		filterTerm := m.list.FilterValue()
		if filterTerm == "" {
			filterIndicator = " [SEARCHING: type to filter]"
		} else {
			filterIndicator = fmt.Sprintf(" [SEARCHING: %s]", searchStyle.Render(filterTerm))
		}
	} else if m.list.FilterState() == list.FilterApplied {
		filterTerm := m.list.FilterValue()
		filterIndicator = fmt.Sprintf(" [FILTERED: %s]", searchStyle.Render(filterTerm))
	}

	visibleCount := len(m.list.VisibleItems())
	currentIndex := m.list.Index()
	if currentIndex < 0 {
		currentIndex = 0
	}

	var position string
	if visibleCount == 0 {
		position = "0/0"
	} else {
		position = fmt.Sprintf("%d/%d", currentIndex+1, visibleCount)
		if m.list.FilterState() != list.Unfiltered && visibleCount < len(m.visible) {
			position = fmt.Sprintf("%d/%d (of %d)", currentIndex+1, visibleCount, len(m.visible))
		}
	}

	scopeIndicator := fmt.Sprintf(" [Scope: %s]", m.activeScope)

	modeIndicator := ""
	if m.jsonView {
		modeIndicator = " [JSON VIEW - esc to exit]"
	} else if m.focus == viewportPane {
		modeIndicator = " [SCROLLING - tab to return]"
	} else if m.visualMode {
		modeIndicator = " [VISUAL]"
	} else if m.statusMessage != "" {
		modeIndicator = fmt.Sprintf(" [%s]", m.statusMessage)
	}

	status := statusStyle.Render(fmt.Sprintf(" Logs: %s%s%s%s%s%s%s | ? for help | q to quit",
		position, scopeIndicator, followIndicator, filtersIndicator, filteredCountIndicator, filterIndicator, modeIndicator))

	// Drawer-geometry fix: at very small heights we show only the list
	// pane plus the status bar. This keeps rendering sensible even in a
	// 10-row horizontal drawer inside the grove terminal.
	if m.height < 15 {
		var listView string
		func() {
			defer func() {
				if r := recover(); r != nil {
					listView = fmt.Sprintf("Error rendering list: %v", r)
				}
			}()
			listView = m.list.View()
		}()
		return lipgloss.JoinVertical(lipgloss.Left, listView, status)
	}

	if m.focus == viewportPane {
		detailsStyle := theme.DefaultTheme.DetailsBox.
			Padding(0, 2).
			BorderForeground(theme.DefaultTheme.Highlight.GetForeground())

		var detailsContent string
		if m.jsonView {
			detailsContent = m.jsonTree.View()
		} else {
			detailsContent = m.viewport.View()
		}

		detailsView := detailsStyle.Render(detailsContent)

		return lipgloss.JoinVertical(
			lipgloss.Left,
			detailsView,
			status,
		)
	}

	var listView string
	func() {
		defer func() {
			if r := recover(); r != nil {
				listView = fmt.Sprintf("Error rendering list: %v", r)
			}
		}()
		listView = m.list.View()
	}()

	detailsStyle := theme.DefaultTheme.DetailsBox.
		Padding(0, 2).
		MarginLeft(1).
		Width(m.width - 3)

	var detailsContent string
	if m.jsonView {
		detailsContent = m.jsonTree.View()
	} else {
		detailsContent = m.viewport.View()
	}

	detailsView := detailsStyle.Render(detailsContent)

	return lipgloss.JoinVertical(
		lipgloss.Left,
		listView,
		detailsView,
		status,
	)
}
