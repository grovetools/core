package logs

import (
	"context"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
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
