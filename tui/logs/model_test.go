package logs

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/grovetools/core/logging"
	"github.com/grovetools/core/pkg/models"
)

func eventsFilterFixtures() (eventInfo, plainInfo, plainDebug, warnItem, errItem logItem) {
	eventInfo = logItem{level: "info", rawData: map[string]interface{}{"event": "job.created"}}
	plainInfo = logItem{level: "info", rawData: map[string]interface{}{}}
	plainDebug = logItem{level: "debug", rawData: map[string]interface{}{"event": ""}}
	warnItem = logItem{level: "warning", rawData: map[string]interface{}{}}
	errItem = logItem{level: "error", rawData: nil}
	return
}

func TestMatchesEventsFilterDisabled(t *testing.T) {
	eventInfo, plainInfo, plainDebug, warnItem, errItem := eventsFilterFixtures()
	m := &Model{eventsOnly: false}
	for _, it := range []logItem{eventInfo, plainInfo, plainDebug, warnItem, errItem} {
		if !m.matchesEventsFilter(it) {
			t.Errorf("eventsOnly off: item %+v should pass", it)
		}
	}
}

func TestMatchesEventsFilterEnabled(t *testing.T) {
	eventInfo, plainInfo, plainDebug, warnItem, errItem := eventsFilterFixtures()
	// filtersEnabled stays false (as constructed by New); EventsOnly must
	// apply regardless of that flag.
	m := &Model{eventsOnly: true, filtersEnabled: false}

	if !m.matchesEventsFilter(eventInfo) {
		t.Error("event-tagged info entry should pass")
	}
	if m.matchesEventsFilter(plainInfo) {
		t.Error("plain info entry should be filtered")
	}
	if m.matchesEventsFilter(plainDebug) {
		t.Error("debug entry with empty event field should be filtered")
	}
	if !m.matchesEventsFilter(warnItem) {
		t.Error("warn entry should always pass")
	}
	if !m.matchesEventsFilter(errItem) {
		t.Error("error entry should always pass")
	}
}

func TestMatchesEventsFilterDaemonScope(t *testing.T) {
	_, plainInfo, _, _, _ := eventsFilterFixtures()
	// Daemon-scope entries are curated by classifyStateUpdate and must not
	// be filtered even when they carry no event field at info level.
	m := &Model{eventsOnly: true, activeScope: ScopeDaemon}
	if !m.matchesEventsFilter(plainInfo) {
		t.Error("Daemon-scope info entry should pass the events filter")
	}
}

func TestComponentOverrideEnablesFiltersAndKeepsHiddenEntriesBuffered(t *testing.T) {
	m := New(context.Background(), Config{
		OverrideOpts: &logging.OverrideOptions{ShowOnly: []string{"wanted"}},
	})
	defer m.Close()

	if !m.filtersEnabled {
		t.Fatal("--component/ShowOnly did not enable component filters")
	}
	m.handleNewLog(newLogMsg{data: map[string]interface{}{"level": "info", "component": "other", "msg": "hidden"}})
	m.handleNewLog(newLogMsg{data: map[string]interface{}{"level": "info", "component": "wanted", "msg": "shown"}})
	if got := len(m.items); got != 2 {
		t.Fatalf("received buffer has %d entries, want 2", got)
	}
	if got := len(m.visible); got != 1 {
		t.Fatalf("visible rows = %d, want 1", got)
	}

	m.filtersEnabled = false
	m.rebuildVisible()
	if got := len(m.visible); got != 2 {
		t.Fatalf("disabling filters did not reveal buffered entry: %d rows", got)
	}
}

func TestVisibilityCountsAndPickerHidingAreTruthful(t *testing.T) {
	m := New(context.Background(), Config{EventsOnly: true, InitialLevel: "info"})
	defer m.Close()
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 20})
	m.hiddenComponents["muted"] = true
	m.items = []logItem{
		{level: "debug", component: "keep", rawData: map[string]interface{}{"event": "job.created"}, repeatCount: 1},
		{level: "info", component: "keep", rawData: map[string]interface{}{}, repeatCount: 1},
		{level: "warn", component: "muted", rawData: map[string]interface{}{}, repeatCount: 1},
		{level: "error", component: "keep", rawData: map[string]interface{}{}, repeatCount: 2},
	}
	m.rebuildVisible()

	shown, received, byLevel, byEvents, byComponent := m.visibilityCounts()
	if shown != 2 || received != 5 || byLevel != 1 || byEvents != 1 || byComponent != 1 {
		t.Fatalf("counts = shown:%d received:%d level:%d events:%d component:%d", shown, received, byLevel, byEvents, byComponent)
	}
	view := m.frameView()
	for _, want := range []string{
		"2/5 shown/received",
		"hidden: level 1, events 1, component 1",
		"Filters:OFF",
		"hiding: 1 component",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("status missing %q: %q", want, view)
		}
	}
}

func TestLevelCyclePreservesBufferAndFiltersLocally(t *testing.T) {
	m := New(context.Background(), Config{InitialLevel: "info"})
	defer m.Close()
	for _, level := range []string{"debug", "info", "warn", "error"} {
		m.handleNewLog(newLogMsg{data: map[string]interface{}{"level": level, "msg": level}})
	}
	if len(m.items) != 4 || len(m.visible) != 3 {
		t.Fatalf("initial items/visible = %d/%d, want 4/3", len(m.items), len(m.visible))
	}

	generation := m.streamGeneration
	for _, wantVisible := range []int{2, 1, 4} { // warn, error, then debug
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
		if len(m.items) != 4 || len(m.visible) != wantVisible {
			t.Fatalf("cycled level items/visible = %d/%d, want 4/%d", len(m.items), len(m.visible), wantVisible)
		}
		if m.streamGeneration != generation {
			t.Fatalf("level cycle reconnected stream: generation %d -> %d", generation, m.streamGeneration)
		}
	}
}

func TestHelpOverlayDelegatesNavigationAndQuestionMarkCloses(t *testing.T) {
	m := New(context.Background(), Config{})
	defer m.Close()
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 8})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if !m.help.ShowAll {
		t.Fatal("? did not open help")
	}
	before := m.help.View()
	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	after := m.help.View()
	if after == before {
		t.Fatal("pgdown was swallowed instead of scrolling the help viewport")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if m.help.ShowAll {
		t.Fatal("? did not close help")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.help.ShowAll {
		t.Fatal("esc did not close help")
	}
}

func TestRebuildVisibleAppliesEventsFilter(t *testing.T) {
	eventInfo, plainInfo, _, warnItem, _ := eventsFilterFixtures()
	m := &Model{
		eventsOnly:       true,
		filtersEnabled:   false,
		hiddenComponents: map[string]bool{},
		list:             list.New([]list.Item{}, itemDelegate{}, 0, 0),
	}
	m.items = []logItem{eventInfo, plainInfo, warnItem}
	m.rebuildVisible()
	if len(m.visible) != 2 {
		t.Fatalf("expected 2 visible items (event + warn), got %d", len(m.visible))
	}

	m.eventsOnly = false
	m.rebuildVisible()
	if len(m.visible) != 3 {
		t.Fatalf("expected 3 visible items with eventsOnly off, got %d", len(m.visible))
	}
}

// TestUnseenAlertsCountsWarnAndError locks in the alert counter's level
// threshold: warn and error arrivals increment it (so advisory records like
// config schema warnings can drive host attention affordances), info/debug
// do not, and embed.FocusMsg-style clearing is exposed via UnseenAlerts.
func TestUnseenAlertsCountsWarnAndError(t *testing.T) {
	m := &Model{}
	m.list = list.New(nil, itemDelegate{model: m}, 0, 0)

	for _, level := range []string{"debug", "info", "warning", "warn", "error"} {
		m.handleNewLog(newLogMsg{data: map[string]interface{}{"level": level, "msg": "x"}})
	}
	if got := m.UnseenAlerts(); got != 3 {
		t.Fatalf("UnseenAlerts = %d, want 3 (warning + warn + error)", got)
	}

	m.unseenAlerts = 0
	if got := m.UnseenAlerts(); got != 0 {
		t.Fatalf("UnseenAlerts after clear = %d, want 0", got)
	}
}

// spinnerGateModel builds the minimum Model the spinner gate needs: a real
// spinner (so frame advancement is observable) and a real list (so a
// regression that delegates ticks to it would run rather than panic).
func spinnerGateModel() *Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	m := &Model{ctx: context.Background(), spinner: sp}
	m.list = list.New(nil, itemDelegate{model: m}, 0, 0)
	return m
}

// TestSpinnerTickIdleArmsNothing is the idle-CPU regression: with no connect
// in flight a spinner tick must die where it lands. A nil command means no
// timer is left armed, which is the entire cost this gate removes — the
// ungated version re-armed at 10fps forever and fell through to a full
// bubbles-list update plus a View on every one of those ticks.
func TestSpinnerTickIdleArmsNothing(t *testing.T) {
	m := spinnerGateModel()
	before := m.spinner.View()

	_, cmd := m.Update(m.spinner.Tick())

	if cmd != nil {
		t.Error("idle spinner tick returned a command; the tick must not re-arm while nothing is loading")
	}
	if got := m.spinner.View(); got != before {
		t.Errorf("idle spinner advanced a frame: %q -> %q", before, got)
	}
}

// TestSpinnerTickAnimatesWhileConnecting is the other half: the gate must not
// break the spinner during a real load.
func TestSpinnerTickAnimatesWhileConnecting(t *testing.T) {
	m := spinnerGateModel()
	m.connecting = true
	before := m.spinner.View()

	_, cmd := m.Update(m.spinner.Tick())

	if cmd == nil {
		t.Error("spinner tick did not re-arm while a connect was in flight")
	}
	if got := m.spinner.View(); got == before {
		t.Errorf("spinner frame did not advance during a connect (still %q)", got)
	}
}

// TestConnectArmsSpinnerAndStreamTrafficStopsIt walks the window end to end:
// issuing a connect opens it, and the first message off the stream closes it.
func TestConnectArmsSpinnerAndStreamTrafficStopsIt(t *testing.T) {
	streamMsgs := map[string]any{
		"batchLogMsg":   batchLogMsg{log: newLogMsg{data: map[string]interface{}{"level": "info", "msg": "x"}}},
		"pumpStreamMsg": pumpStreamMsg{},
		"batchStateMsg": batchStateMsg{log: newLogMsg{data: map[string]interface{}{"level": "info", "msg": "x"}}},
		"pumpStateMsg":  pumpStateMsg{},
		"streamErrMsg":  streamErrMsg{err: context.Canceled},
		// The backstop for a stream that connects and then stays silent,
		// so a quiet host can never pin the spinner on forever.
		"stopSpinnerMsg": stopSpinnerMsg{},
	}

	for name, msg := range streamMsgs {
		t.Run(name, func(t *testing.T) {
			m := spinnerGateModel()

			// DaemonClient is nil, so connectToDaemon short-circuits to a
			// streamErrMsg command — but it must still have opened the
			// spinner's window before doing so.
			m.connectToDaemon()
			if !m.connecting {
				t.Fatal("connectToDaemon did not mark the model as connecting")
			}

			// connectToDaemon advances the stream generation; stamp synthetic
			// traffic as belonging to that active connection.
			switch typed := msg.(type) {
			case batchLogMsg:
				typed.generation = m.streamGeneration
				msg = typed
			case pumpStreamMsg:
				typed.generation = m.streamGeneration
				msg = typed
			case batchStateMsg:
				typed.generation = m.streamGeneration
				msg = typed
			case pumpStateMsg:
				typed.generation = m.streamGeneration
				msg = typed
			case streamErrMsg:
				typed.generation = m.streamGeneration
				msg = typed
			case stopSpinnerMsg:
				typed.generation = m.streamGeneration
				msg = typed
			}
			m.Update(msg)

			if m.connecting {
				t.Errorf("%s left the model connecting; the spinner would keep ticking", name)
			}
			if _, cmd := m.Update(m.spinner.Tick()); cmd != nil {
				t.Errorf("spinner still re-armed after %s", name)
			}
		})
	}
}

func TestConsecutiveRepeatsCollapseAndExpand(t *testing.T) {
	m := New(context.Background(), Config{})
	defer m.Close()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	base := time.Now().UTC()
	for n := 0; n < 3; n++ {
		m.handleNewLog(newLogMsg{
			workspace: "ws", workspacePath: "/ws",
			data: map[string]interface{}{
				"level": "warn", "component": "watcher", "msg": "retry",
				"error": "connection refused\nstack", "time": base.Add(time.Duration(n) * time.Second).Format(time.RFC3339),
			},
		})
	}
	if len(m.items) != 1 || m.items[0].repeatCount != 3 {
		t.Fatalf("repeat groups = %#v, want one group with count 3", m.items)
	}
	if got := m.items[0].Title(); !strings.Contains(got, "×3") {
		t.Fatalf("collapsed title %q does not render ×3", got)
	}
	m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if got := len(m.visible); got != 3 {
		t.Fatalf("expanded visible rows = %d, want 3", got)
	}

	m.handleNewLog(newLogMsg{workspace: "ws", workspacePath: "/ws", data: map[string]interface{}{
		"level": "warn", "component": "watcher", "msg": "retry", "error": "timeout", "time": base.Add(4 * time.Second).Format(time.RFC3339),
	}})
	if len(m.items) != 2 {
		t.Fatalf("different error prefix collapsed unexpectedly: %d groups", len(m.items))
	}
}

func TestWarnTitleAndSearchIncludeSanitizedError(t *testing.T) {
	it := logItem{
		level: "warn", message: "failed\x1b[2J", component: "sync",
		rawData: map[string]interface{}{"error": "permission denied\nsecret stack"},
	}
	if got := it.Title(); !strings.Contains(got, "permission denied") || strings.Contains(got, "\x1b") {
		t.Fatalf("warn title did not append and sanitize error: %q", got)
	}
	if got := it.FilterValue(); !strings.Contains(got, "failed") || !strings.Contains(got, "permission denied") {
		t.Fatalf("FilterValue %q does not include message + error", got)
	}
}

func TestLargeFieldFoldSanitizeWrapAndExpandedView(t *testing.T) {
	payload := "\x1b[2J" + strings.Repeat("abcdefghij", 400) + "\x00"
	it := logItem{rawData: map[string]interface{}{"payload": payload}}
	folded := it.formatDetails(40, false)
	if strings.Contains(folded, "\x1b") || strings.Contains(folded, "\x00") {
		t.Fatalf("folded field retained terminal controls: %q", folded)
	}
	if !strings.Contains(folded, "folded:") || !strings.Contains(folded, "enter to expand") {
		t.Fatalf("fold marker absent: %q", folded)
	}
	for _, line := range strings.Split(folded, "\n") {
		if len([]rune(line)) > 40 {
			t.Fatalf("detail line exceeds pane width: %d: %q", len([]rune(line)), line)
		}
	}
	expanded := it.formatDetails(40, true)
	if strings.Contains(expanded, "folded:") || len(expanded) <= len(folded) {
		t.Fatalf("expanded field did not reveal folded content (folded=%d expanded=%d)", len(folded), len(expanded))
	}
}

func TestEnterOpensScrollableExpandedFieldView(t *testing.T) {
	m := New(context.Background(), Config{})
	defer m.Close()
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	payload := strings.Repeat("expanded-value-", 300)
	m.handleNewLog(newLogMsg{data: map[string]interface{}{
		"level": "info", "component": "test", "msg": "large", "payload": payload,
	}})

	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.fieldView || m.focus != viewportPane {
		t.Fatalf("enter did not open expanded field viewport: fieldView=%v focus=%v", m.fieldView, m.focus)
	}
	if got := m.viewport.View(); strings.Contains(got, "folded:") {
		t.Fatalf("expanded viewport still shows folded marker: %q", got)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.fieldView || m.focus != listPane {
		t.Fatalf("escape did not return from expanded fields: fieldView=%v focus=%v", m.fieldView, m.focus)
	}
}

func TestDisconnectedAndReconnectStatusAreVisible(t *testing.T) {
	m := New(context.Background(), Config{})
	defer m.Close()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	if got := m.frameView(); !strings.Contains(got, "Disconnected") {
		t.Fatalf("disconnected status is not visible: %q", got)
	}
	m.reconnecting = true
	m.reconnectBackoff = 2 * time.Second
	if got := m.frameView(); !strings.Contains(got, "reconnecting in 2s") {
		t.Fatalf("reconnecting status is not visible: %q", got)
	}
}

func TestClosedStreamStaysVisiblyDisconnectedDuringFailedReconnect(t *testing.T) {
	m := New(context.Background(), Config{})
	defer m.Close()
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	m.streamConnected = true

	closed := make(chan models.LogStreamLine)
	close(closed)
	closedMsg := pumpStream(m.ctx, closed, m.streamGeneration)()
	if _, ok := closedMsg.(streamErrMsg); !ok {
		t.Fatalf("closed stream produced %T, want streamErrMsg", closedMsg)
	}
	_, retryTimer := m.Update(closedMsg)
	if retryTimer == nil || !strings.Contains(m.frameView(), "Disconnected; reconnecting in") {
		t.Fatalf("closed stream did not visibly enter backoff: %q", m.frameView())
	}

	// Deliver the timer message directly rather than sleeping. A nil client
	// makes this retry fail deterministically after connectToDaemon has put the
	// model into the otherwise-quiet in-flight retry state.
	generation := m.streamGeneration
	_, failedConnect := m.Update(streamReconnectMsg{generation: generation})
	if failedConnect == nil {
		t.Fatal("reconnect attempt did not return a connect command")
	}
	if got := m.frameView(); !strings.Contains(got, "Disconnected") {
		t.Fatalf("in-flight reconnect hid the outage: %q", got)
	}
	failedMsg := failedConnect()
	if _, ok := failedMsg.(streamErrMsg); !ok {
		t.Fatalf("failed reconnect produced %T, want streamErrMsg", failedMsg)
	}
	m.Update(failedMsg)
	if got := m.frameView(); !strings.Contains(got, "Disconnected; reconnecting in") {
		t.Fatalf("failed reconnect did not remain visibly disconnected: %q", got)
	}
}

func TestStreamReconnectBackoffAndStaleGeneration(t *testing.T) {
	m := spinnerGateModel()
	m.streamGeneration = 7
	_, cmd := m.Update(streamErrMsg{err: context.Canceled, generation: 7})
	if cmd == nil || !m.reconnecting || m.reconnectBackoff != initialReconnectBackoff {
		t.Fatalf("first disconnect state = reconnecting:%v backoff:%s cmd:%v", m.reconnecting, m.reconnectBackoff, cmd != nil)
	}
	m.Update(streamErrMsg{err: context.Canceled, generation: 7})
	if m.reconnectBackoff != 2*initialReconnectBackoff {
		t.Fatalf("second backoff = %s, want %s", m.reconnectBackoff, 2*initialReconnectBackoff)
	}
	m.Update(streamErrMsg{err: context.Canceled, generation: 6})
	if m.reconnectBackoff != 2*initialReconnectBackoff {
		t.Fatalf("stale generation changed backoff to %s", m.reconnectBackoff)
	}
	m.markStreamConnected()
	if m.reconnecting || m.reconnectBackoff != 0 || !m.streamConnected {
		t.Fatalf("traffic did not reset reconnect state")
	}
}
