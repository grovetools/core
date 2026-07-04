package logs

import (
	"testing"

	"github.com/charmbracelet/bubbles/list"
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
