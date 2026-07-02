// Package logs provides an embeddable bubbletea model that streams
// grove-core log entries from the daemon's aggregated SSE endpoint
// and renders them in a list + details split view. This is the
// extracted form of the `core logs --tui` model — host applications
// (such as the grove terminal) embed it directly as a panel, while
// the CLI entrypoint in core/cmd/logs_tui.go runs it standalone via
// tea.NewProgram.
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
	"os/exec"
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

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/logging"
	"github.com/grovetools/core/pkg/daemon"
	logskeymap "github.com/grovetools/core/pkg/keymap"
	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/tui/components/help"
	"github.com/grovetools/core/tui/components/jsontree"
	"github.com/grovetools/core/tui/embed"
	tuikeymap "github.com/grovetools/core/tui/keymap"
	"github.com/grovetools/core/tui/theme"
)

// Config is the public constructor payload for New.
type Config struct {
	// DaemonClient is used to connect to the daemon's aggregated log stream.
	DaemonClient daemon.Client
	// InitialScope sets the starting scope (e.g. "workspace", "ecosystem", "all", "system").
	InitialScope string
	// IncludeSystem includes system-scoped entries alongside the active scope.
	IncludeSystem bool
	// LogConfig is the initial logging visibility config. May be nil.
	LogConfig *logging.Config
	// OverrideOpts carries CLI filter overrides (--show-all, --component, --also-show, --ignore-hide).
	OverrideOpts *logging.OverrideOptions
	// Follow turns on auto-scroll on new entries at construction.
	Follow bool
	// InitialWorkspacePath seeds the active-workspace filter before
	// the host has had a chance to broadcast embed.SetWorkspaceMsg.
	InitialWorkspacePath string
	// Replay is the number of historical lines the daemon should replay on connect.
	Replay int
	// Compact suppresses the detail split pane and focus-switching keys,
	// rendering only the streaming log list.
	Compact bool
	// InitialLevel sets the starting minimum log level (e.g. "debug", "info", "warn", "error").
	// Empty string defaults to "info".
	InitialLevel string
	// EventsOnly starts the viewer in events-only mode: only entries
	// carrying a non-empty `event` field or at warn level and above are
	// shown. Toggleable at runtime with the ToggleEvents key ("E").
	EventsOnly bool
}

// paneFocus tracks which pane has focus.
type paneFocus int

const (
	listPane paneFocus = iota
	viewportPane
)

// LogScope selects which log sources the viewer displays.
type LogScope int

const (
	ScopeProject LogScope = iota
	ScopeEcosystem
	ScopeAll
	ScopeSystem
	ScopeDaemon
)

// String returns the human-readable label used in the status bar.
func (s LogScope) String() string {
	switch s {
	case ScopeProject:
		return "Workspace"
	case ScopeEcosystem:
		return "Ecosystem"
	case ScopeAll:
		return "All"
	case ScopeSystem:
		return "System"
	case ScopeDaemon:
		return "Daemon"
	default:
		return "Unknown"
	}
}

// scopeToParam converts the internal LogScope to the daemon API string.
func (s LogScope) scopeToParam() string {
	switch s {
	case ScopeProject:
		return "workspace"
	case ScopeEcosystem:
		return "ecosystem"
	case ScopeAll:
		return "all"
	case ScopeSystem:
		return "system"
	case ScopeDaemon:
		return "daemon"
	default:
		return "workspace"
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
	styleFn       func(string) lipgloss.Style
}

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

// FormatDetails returns the multi-line detail pane body for a log entry.
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

	// Active filter (set via embed.SetWorkspaceMsg).
	activeWorkspacePath string

	// Data: items holds all entries received from the daemon stream;
	// visible holds the subset matching component filters.
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
	eventsOnly     bool
	filteredCount  int
	unseenAlerts   int
	ready          bool
	focus          paneFocus
	visualMode     bool
	visualStart    int
	visualEnd      int
	statusMessage  string
	jsonTree       jsontree.Model
	jsonView       bool
	sequence       *tuikeymap.SequenceState

	// Compact mode: list-only, no detail viewport or focus switching.
	compact bool

	// Component picker overlay
	showComponentPicker bool
	hiddenComponents    map[string]bool
	pickerItems         []string // sorted component names
	pickerCursor        int

	// Filter config
	logConfig     *logging.Config
	overrideOpts  *logging.OverrideOptions
	activeScope   LogScope
	includeSystem bool
	minLevel      int // 0=debug, 1=info, 2=warn, 3=error

	// Stream lifecycle: streamCtx bounds the active SSE connection.
	// On filter changes we cancel it and reconnect with new params.
	streamCtx    context.Context
	streamCancel context.CancelFunc
	streamCtxMu  sync.Mutex

	// Workspace coloring
	workspaceColorMap   map[string]lipgloss.Style
	workspaceColorIndex int
	colorMu             sync.Mutex
}

// New constructs a Model bound to ctx. The caller MUST eventually
// either cancel ctx or call Close() on the returned model to stop
// the background stream.
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

	replay := cfg.Replay
	if replay == 0 {
		replay = 100
	}
	// Store replay in config for connectToDaemon
	cfg.Replay = replay

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
		eventsOnly:          cfg.EventsOnly,
		logConfig:           logCfg,
		overrideOpts:        cfg.OverrideOpts,
		includeSystem:       cfg.IncludeSystem,
		workspaceColorMap:   make(map[string]lipgloss.Style),
		minLevel:            parseLevelConfig(cfg.InitialLevel),
		hiddenComponents:    make(map[string]bool),
		compact:             cfg.Compact,
		sequence:            tuikeymap.NewSequenceState(),
	}

	// Resolve initial scope
	switch cfg.InitialScope {
	case "ecosystem":
		m.activeScope = ScopeEcosystem
	case "all":
		m.activeScope = ScopeAll
	case "system":
		m.activeScope = ScopeSystem
	case "daemon":
		m.activeScope = ScopeDaemon
	default:
		m.activeScope = ScopeProject
	}

	m.list.SetDelegate(itemDelegate{model: m})
	return m
}

// Close cancels the model's context, unblocking the stream and any
// pending commands. Safe to call multiple times.
func (m *Model) Close() error {
	if m.cancel != nil {
		m.cancel()
	}
	return nil
}

// Init kicks off the daemon stream connection and arms the spinner
// and ticker commands.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.connectToDaemon(),
		tick(),
	)
}

// tick emits a plain tickMsg every 100ms for UI refresh.
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
	streamErrMsg   struct{ err error }
)

// parseLevelConfig converts a level string from Config.InitialLevel to the
// numeric minLevel value. Returns 1 (INFO) for empty or unrecognized input.
func parseLevelConfig(s string) int {
	switch strings.ToLower(s) {
	case "debug":
		return 0
	case "info":
		return 1
	case "warn", "warning":
		return 2
	case "error":
		return 3
	default:
		return 1
	}
}

// levelToParam converts the numeric minLevel to the daemon API string.
func levelToParam(minLevel int) string {
	switch minLevel {
	case 0:
		return "debug"
	case 1:
		return "info"
	case 2:
		return "warn"
	case 3:
		return "error"
	default:
		return "debug"
	}
}

// connectToDaemon cancels any existing stream, builds new LogStreamOptions
// from the current UI state, and starts pumping the daemon's SSE channel.
func (m *Model) connectToDaemon() tea.Cmd {
	m.streamCtxMu.Lock()
	if m.streamCancel != nil {
		m.streamCancel()
	}
	sCtx, sCancel := context.WithCancel(m.ctx)
	m.streamCtx = sCtx
	m.streamCancel = sCancel
	m.streamCtxMu.Unlock()

	opts := models.LogStreamOptions{
		Scope:     m.activeScope.scopeToParam(),
		Workspace: m.activeWorkspacePath,
		Level:     levelToParam(m.minLevel),
		System:    m.includeSystem,
		Replay:    m.cfg.Replay,
	}

	client := m.cfg.DaemonClient
	if client == nil {
		return func() tea.Msg {
			return streamErrMsg{err: fmt.Errorf("no daemon client configured")}
		}
	}

	if m.activeScope == ScopeDaemon {
		return func() tea.Msg {
			ch, err := client.StreamState(sCtx)
			if err != nil {
				return streamErrMsg{err: err}
			}
			return m.pumpFirstStateUpdate(sCtx, ch)
		}
	}

	return func() tea.Msg {
		ch, err := client.StreamLogs(sCtx, opts)
		if err != nil {
			return streamErrMsg{err: err}
		}
		return m.pumpFirstLine(sCtx, ch)
	}
}

// pumpFirstLine reads the first line from the stream channel. This is
// separated from pumpStream so the initial connectToDaemon tea.Cmd
// can block until the first message arrives (or the stream closes).
func (m *Model) pumpFirstLine(sCtx context.Context, ch <-chan models.LogStreamLine) tea.Msg {
	select {
	case <-sCtx.Done():
		return nil
	case line, ok := <-ch:
		if !ok {
			return nil
		}
		msg := parseStreamLine(line)
		if msg == nil {
			return pumpStreamMsg{ctx: sCtx, ch: ch}
		}
		return batchLogMsg{log: *msg, ctx: sCtx, ch: ch}
	}
}

// pumpStreamMsg is a tea.Msg that re-arms the stream pump for the next line.
type pumpStreamMsg struct {
	ctx context.Context
	ch  <-chan models.LogStreamLine
}

// batchLogMsg delivers both a parsed log line and the continuation pump.
type batchLogMsg struct {
	log newLogMsg
	ctx context.Context
	ch  <-chan models.LogStreamLine
}

// pumpStream returns a tea.Cmd that reads the next line from the channel.
func pumpStream(sCtx context.Context, ch <-chan models.LogStreamLine) tea.Cmd {
	return func() tea.Msg {
		for {
			select {
			case <-sCtx.Done():
				return nil
			case line, ok := <-ch:
				if !ok {
					return nil
				}
				msg := parseStreamLine(line)
				if msg == nil {
					continue
				}
				return batchLogMsg{log: *msg, ctx: sCtx, ch: ch}
			}
		}
	}
}

// parseStreamLine parses JSON from a LogStreamLine into a newLogMsg.
func parseStreamLine(line models.LogStreamLine) *newLogMsg {
	var rawEntry map[string]interface{}
	if err := json.Unmarshal([]byte(line.Line), &rawEntry); err != nil {
		return nil
	}
	return &newLogMsg{
		workspace:     line.Workspace,
		workspacePath: line.WorkspacePath,
		data:          rawEntry,
	}
}

// workspaceStyleFor returns a consistent lipgloss style for the given
// workspace display name.
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

// levelRank maps log level strings to numeric ranks for filtering.
func levelRank(level string) int {
	switch strings.ToLower(level) {
	case "error", "fatal", "panic":
		return 3
	case "warn", "warning":
		return 2
	case "info":
		return 1
	case "debug", "trace":
		return 0
	default:
		return -1
	}
}

var levelLabels = [4]string{"DEBUG", "INFO", "WARN", "ERROR"}

// rebuildVisible recomputes m.visible from m.items under the current
// component filter. Level/scope filtering is done server-side by the
// daemon; only component visibility filtering happens client-side.
func (m *Model) rebuildVisible() {
	m.visible = m.visible[:0]
	for _, it := range m.items {
		if m.matchesComponentFilter(it) && m.matchesEventsFilter(it) {
			m.visible = append(m.visible, it)
		}
	}
	m.list.SetItems(m.visible)
}

// matchesComponentFilter returns true when the item passes the client-side
// component visibility filter. Level and scope filtering is handled by the
// daemon, so this only checks component visibility.
func (m *Model) matchesComponentFilter(it logItem) bool {
	if m.hiddenComponents[it.component] {
		return false
	}
	if !m.filtersEnabled || m.logConfig == nil {
		return true
	}
	visibilityResult := logging.GetComponentVisibility(it.component, m.logConfig, m.overrideOpts)
	return visibilityResult.Visible
}

// matchesEventsFilter returns true when the item passes the events-only
// filter: entries carrying a non-empty structured `event` field (lifecycle
// events such as job.created, plan.finished, note.updated) or at warn level
// and above. It applies regardless of filtersEnabled, which gates only the
// component visibility config. Daemon-scope entries are synthesized by
// classifyStateUpdate and already represent curated events, so they are
// always passed through.
func (m *Model) matchesEventsFilter(it logItem) bool {
	if !m.eventsOnly {
		return true
	}
	if m.activeScope == ScopeDaemon {
		return true
	}
	if ev, ok := it.rawData["event"].(string); ok && ev != "" {
		return true
	}
	return levelRank(it.level) >= 2
}

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

func (m *Model) openComponentPicker() {
	counts := make(map[string]int)
	for _, item := range m.items {
		if item.component != "" {
			counts[item.component]++
		}
	}
	m.pickerItems = make([]string, 0, len(counts))
	for name := range counts {
		m.pickerItems = append(m.pickerItems, name)
	}
	sort.Strings(m.pickerItems)
	m.pickerCursor = 0
	m.showComponentPicker = true
}

func (m *Model) componentPickerView() string {
	counts := make(map[string]int)
	for _, item := range m.items {
		if item.component != "" {
			counts[item.component]++
		}
	}

	titleStyle := theme.DefaultTheme.Header
	lines := []string{titleStyle.Render("Component Filter") + "  (space: toggle, a: all, n: none, esc: close)", ""}

	hiddenCount := 0
	for _, name := range m.pickerItems {
		check := "✓"
		style := lipgloss.NewStyle()
		if m.hiddenComponents[name] {
			check = " "
			style = theme.DefaultTheme.Muted
			hiddenCount++
		}
		cursor := "  "
		if m.pickerCursor < len(m.pickerItems) && m.pickerItems[m.pickerCursor] == name {
			cursor = "> "
		}
		line := fmt.Sprintf("%s[%s] %-40s %d events", cursor, check, name, counts[name])
		lines = append(lines, style.Render(line))
	}

	if hiddenCount > 0 {
		lines = append(lines, "", theme.DefaultTheme.Warning.Render(fmt.Sprintf("  %d component(s) hidden", hiddenCount)))
	}

	return strings.Join(lines, "\n")
}

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
			return m, nil
		}

		m.activeWorkspacePath = newPath
		m.items = nil
		m.visible = m.visible[:0]
		m.list.SetItems(m.visible)

		// Reload logging config from the new workspace path.
		if msg.Node != nil {
			if cfg, err := config.LoadFrom(msg.Node.Path); err == nil && cfg != nil {
				logCfg := logging.GetDefaultLoggingConfig()
				_ = cfg.UnmarshalExtension("logging", &logCfg)
				m.logConfig = &logCfg
			}
		}
		return m, m.connectToDaemon()

	case embed.FocusMsg:
		m.unseenAlerts = 0
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

	// If component picker is showing, handle its input
	if m.showComponentPicker {
		if kmsg, ok := msg.(tea.KeyMsg); ok {
			if key.Matches(kmsg, m.keys.Base.Quit) {
				return m, doneCmd()
			}
			switch kmsg.String() {
			case "esc", "C":
				m.showComponentPicker = false
				return m, nil
			case "j", "down":
				if m.pickerCursor < len(m.pickerItems)-1 {
					m.pickerCursor++
				}
				return m, nil
			case "k", "up":
				if m.pickerCursor > 0 {
					m.pickerCursor--
				}
				return m, nil
			case " ", "enter":
				if m.pickerCursor < len(m.pickerItems) {
					name := m.pickerItems[m.pickerCursor]
					m.hiddenComponents[name] = !m.hiddenComponents[name]
					if !m.hiddenComponents[name] {
						delete(m.hiddenComponents, name)
					}
					m.rebuildVisible()
				}
				return m, nil
			case "a":
				for k := range m.hiddenComponents {
					delete(m.hiddenComponents, k)
				}
				m.rebuildVisible()
				return m, nil
			case "n":
				for _, name := range m.pickerItems {
					m.hiddenComponents[name] = true
				}
				m.rebuildVisible()
				return m, nil
			}
		}
		return m, nil
	}

	// If in JSON view, delegate updates to the JSON tree component
	if m.jsonView && !m.compact {
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
			// Route multi-key sequences (gg) through the shared sequence
			// state so GotoTop's binding can truthfully declare "gg".
			seqResult, _ := m.sequence.Process(msg, m.keys.GotoTop)
			switch seqResult {
			case tuikeymap.SequenceMatch:
				m.sequence.Clear()
				m.list.Select(0)
				return m, nil
			case tuikeymap.SequencePending:
				// First "g" of a potential "gg" — wait for more input.
				return m, nil
			}
			m.sequence.Clear()

			switch {
			case key.Matches(msg, m.keys.Base.Quit):
				return m, doneCmd()

			case key.Matches(msg, m.keys.Base.Help):
				m.help.Toggle()
				return m, nil

			case key.Matches(msg, m.keys.SwitchFocus) || key.Matches(msg, m.keys.Expand):
				if m.compact {
					return m, nil
				}
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
				// Single item yank: copy selected item's JSON
				if selectedItem := m.list.SelectedItem(); selectedItem != nil {
					if li, ok := selectedItem.(logItem); ok {
						jsonBytes, err := json.MarshalIndent(li.rawData, "", "  ")
						if err == nil {
							if clipErr := m.copyToClipboard(string(jsonBytes)); clipErr == nil {
								m.statusMessage = "Copied log entry JSON"
							} else {
								m.statusMessage = fmt.Sprintf("Copy failed: %v", clipErr)
							}
						}
						return m, m.clearStatusMessageAfter(2 * time.Second)
					}
				}
				return m, nil

			case key.Matches(msg, m.keys.CopyRawText):
				if selectedItem := m.list.SelectedItem(); selectedItem != nil {
					if li, ok := selectedItem.(logItem); ok {
						rawText := fmt.Sprintf("[%s] [%s] %s [%s] %s",
							li.workspace, strings.ToUpper(li.level),
							li.timestamp.Format("2006-01-02 15:04:05"),
							li.component, li.message)
						if err := m.copyToClipboard(rawText); err == nil {
							m.statusMessage = "Copied log line text"
						} else {
							m.statusMessage = fmt.Sprintf("Copy failed: %v", err)
						}
						return m, m.clearStatusMessageAfter(2 * time.Second)
					}
				}
				return m, nil

			case key.Matches(msg, m.keys.ClearBuffer):
				m.items = nil
				m.visible = m.visible[:0]
				m.list.SetItems(nil)
				m.statusMessage = "Buffer cleared"
				return m, m.clearStatusMessageAfter(2 * time.Second)

			case key.Matches(msg, m.keys.OpenEditor):
				if selectedItem := m.list.SelectedItem(); selectedItem != nil {
					if li, ok := selectedItem.(logItem); ok {
						if filePath, ok := li.rawData["file"].(string); ok && filePath != "" {
							line := 0
							if parts := strings.SplitN(filePath, ":", 2); len(parts) == 2 {
								filePath = parts[0]
								fmt.Sscanf(parts[1], "%d", &line)
							}
							return m, func() tea.Msg {
								return embed.SplitEditorRequestMsg{Path: filePath, Line: line, Focus: true}
							}
						}
						m.statusMessage = "No file path in this log entry"
						return m, m.clearStatusMessageAfter(2 * time.Second)
					}
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
				m.rebuildVisible()
				return m, m.clearStatusMessageAfter(2 * time.Second)

			case key.Matches(msg, m.keys.ToggleEvents):
				m.eventsOnly = !m.eventsOnly
				if m.eventsOnly {
					m.statusMessage = "Events only: showing events + warn/error"
				} else {
					m.statusMessage = "Events only: off"
				}
				m.rebuildVisible()
				return m, m.clearStatusMessageAfter(2 * time.Second)

			case key.Matches(msg, m.keys.ToggleScope):
				switch m.activeScope {
				case ScopeProject:
					m.activeScope = ScopeEcosystem
				case ScopeEcosystem:
					m.activeScope = ScopeAll
				case ScopeAll:
					m.activeScope = ScopeSystem
				case ScopeSystem:
					m.activeScope = ScopeDaemon
				case ScopeDaemon:
					m.activeScope = ScopeProject
				}
				m.statusMessage = fmt.Sprintf("Scope: %s", m.activeScope)
				m.items = nil
				m.visible = m.visible[:0]
				m.list.SetItems(m.visible)
				return m, tea.Batch(m.connectToDaemon(), m.clearStatusMessageAfter(2*time.Second))

			case key.Matches(msg, m.keys.ToggleSystem):
				m.includeSystem = !m.includeSystem
				if m.includeSystem {
					m.statusMessage = "System logs: included"
				} else {
					m.statusMessage = "System logs: excluded"
				}
				m.items = nil
				m.visible = m.visible[:0]
				m.list.SetItems(m.visible)
				return m, tea.Batch(m.connectToDaemon(), m.clearStatusMessageAfter(2*time.Second))

			case key.Matches(msg, m.keys.CycleLevel):
				m.minLevel = (m.minLevel + 1) % 4
				m.statusMessage = fmt.Sprintf("Level filter: %s+", levelLabels[m.minLevel])
				m.items = nil
				m.visible = m.visible[:0]
				m.list.SetItems(m.visible)
				return m, tea.Batch(m.connectToDaemon(), m.clearStatusMessageAfter(2*time.Second))

			case key.Matches(msg, m.keys.ComponentSummary):
				m.openComponentPicker()
				return m, nil

			case key.Matches(msg, m.keys.ViewJSON) && !m.compact:
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

		if m.compact || m.height < 15 {
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

	case batchLogMsg:
		// Process the log line and re-arm the stream pump.
		cmd := m.handleNewLog(msg.log)
		return m, tea.Batch(cmd, pumpStream(msg.ctx, msg.ch))

	case pumpStreamMsg:
		// Non-JSON line was skipped; re-arm the pump.
		return m, pumpStream(msg.ctx, msg.ch)

	case batchStateMsg:
		cmd := m.handleNewLog(msg.log)
		return m, tea.Batch(cmd, pumpStateStream(msg.ctx, msg.ch))

	case pumpStateMsg:
		return m, pumpStateStream(msg.ctx, msg.ch)

	case streamErrMsg:
		m.statusMessage = fmt.Sprintf("Stream error: %v", msg.err)
		return m, m.clearStatusMessageAfter(5 * time.Second)

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

// handleNewLog processes a single newLogMsg and returns any follow-up commands.
func (m *Model) handleNewLog(msg newLogMsg) tea.Cmd {
	level, _ := msg.data["level"].(string)
	message, _ := msg.data["msg"].(string)
	component, _ := msg.data["component"].(string)
	timeStr, _ := msg.data["time"].(string)

	// Count warn- and error-level arrivals regardless of filters/visibility;
	// the counter is cleared when the panel regains focus. Warn is included
	// so advisory records (e.g. config schema warnings) can drive the host's
	// attention affordance, not just hard errors.
	if levelRank(level) >= 2 {
		m.unseenAlerts++
	}

	// Component visibility filter (client-side only)
	if m.filtersEnabled && m.logConfig != nil {
		visibilityResult := logging.GetComponentVisibility(component, m.logConfig, m.overrideOpts)
		if !visibilityResult.Visible {
			m.filteredCount++
			return nil
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

	// Enforce 10,000 cap.
	if len(m.items) > 11000 {
		m.items = m.items[len(m.items)-10000:]
		m.rebuildVisible()
	}

	// Append to visible (daemon already filtered by scope/level).
	if i == len(m.items)-1 {
		if m.matchesEventsFilter(newItem) {
			m.visible = append(m.visible, newItem)
			m.list.SetItems(m.visible)
		}
	} else {
		m.rebuildVisible()
	}

	if m.followMode && len(m.visible) > 0 {
		m.list.Select(len(m.visible) - 1)
		if selectedItem := m.list.SelectedItem(); selectedItem != nil {
			if li, ok := selectedItem.(logItem); ok {
				m.viewport.SetContent(li.FormatDetails())
				m.viewport.GotoTop()
			}
		}
	}

	return nil
}

// UnseenAlerts returns the number of warn- and error-level records that
// arrived since the panel was last focused; the count is cleared on
// embed.FocusMsg.
func (m *Model) UnseenAlerts() int {
	return m.unseenAlerts
}

func (m *Model) View() string {
	if m.help.ShowAll {
		return m.help.View()
	}

	if m.showComponentPicker {
		return m.componentPickerView()
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
	hiddenCompCount := len(m.hiddenComponents)
	if hiddenCompCount > 0 {
		filteredCountIndicator = fmt.Sprintf(" [hiding: %d components]", hiddenCompCount)
	} else if m.filteredCount > 0 {
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

	systemIndicator := ""
	if m.includeSystem && m.activeScope != ScopeSystem && m.activeScope != ScopeDaemon {
		systemIndicator = " [+System]"
	}

	levelIndicator := fmt.Sprintf(" [Level: %s+]", levelLabels[m.minLevel])

	eventsIndicator := ""
	if m.eventsOnly {
		eventsIndicator = " [Events]"
	}

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

	status := statusStyle.Render(fmt.Sprintf(" Logs: %s%s%s%s%s%s%s%s%s%s | ? for help | q to quit",
		position, scopeIndicator, systemIndicator, levelIndicator, eventsIndicator, followIndicator, filtersIndicator, filteredCountIndicator, filterIndicator, modeIndicator))

	if m.compact || m.height < 15 {
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
