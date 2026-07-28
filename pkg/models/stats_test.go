package models

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
	"time"
)

func statsFixture() SystemStats {
	return SystemStats{
		SampledAt: time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC),
		Runtime: RuntimeStats{
			Goroutines:     412,
			HeapAlloc:      128 << 20,
			HeapSys:        256 << 20,
			NumGC:          184,
			GCPauseTotalMS: 1200.5,
			GoMemLimit:     2147483648,
			UptimeMS:       3600000,
		},
		Self: SelfStats{
			PID:    27462,
			CPUPct: 41.0,
			RSSKB:  727040,
			Procs:  63,
			Top:    &ProcStat{PID: 999, Comm: "git", CPUPct: 12.5, RSSKB: 40960},
			Children: []ProcStat{
				{PID: 999, Comm: "git", CPUPct: 12.5, RSSKB: 40960},
				{PID: 1000, Comm: "gopls", CPUPct: 3.0, RSSKB: 900000},
			},
		},
		Counters: map[string]float64{},
		Warnings: []HealthWarning{},
	}
}

// keysOf returns the sorted JSON object keys under path in doc.
func keysOf(t *testing.T, obj map[string]json.RawMessage) []string {
	t.Helper()
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func subObject(t *testing.T, obj map[string]json.RawMessage, key string) map[string]json.RawMessage {
	t.Helper()
	raw, ok := obj[key]
	if !ok {
		t.Fatalf("missing key %q", key)
	}
	var sub map[string]json.RawMessage
	if err := json.Unmarshal(raw, &sub); err != nil {
		t.Fatalf("key %q is not an object: %v", key, err)
	}
	return sub
}

// TestSystemStatsJSONFieldNames pins the snake_case wire contract: these
// names are consumed by curl/agents and the groved CLI, so renames are
// breaking changes.
func TestSystemStatsJSONFieldNames(t *testing.T) {
	data, err := json.Marshal(statsFixture())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	wantTop := []string{"counters", "runtime", "sampled_at", "self", "warnings"}
	if got := keysOf(t, top); !reflect.DeepEqual(got, wantTop) {
		t.Errorf("top-level keys = %v, want %v", got, wantTop)
	}

	wantRuntime := []string{"gc_pause_total_ms", "gomemlimit", "goroutines", "heap_alloc", "heap_sys", "num_gc", "uptime_ms"}
	if got := keysOf(t, subObject(t, top, "runtime")); !reflect.DeepEqual(got, wantRuntime) {
		t.Errorf("runtime keys = %v, want %v", got, wantRuntime)
	}

	wantSelf := []string{"children", "cpu_pct", "pid", "procs", "rss_kb", "top"}
	if got := keysOf(t, subObject(t, top, "self")); !reflect.DeepEqual(got, wantSelf) {
		t.Errorf("self keys = %v, want %v", got, wantSelf)
	}

	wantProc := []string{"comm", "cpu_pct", "pid", "rss_kb"}
	if got := keysOf(t, subObject(t, subObject(t, top, "self"), "top")); !reflect.DeepEqual(got, wantProc) {
		t.Errorf("self.top keys = %v, want %v", got, wantProc)
	}

	// Reserved fields render as empty containers, not null.
	if string(top["counters"]) != "{}" {
		t.Errorf("counters = %s, want {}", top["counters"])
	}
	if string(top["warnings"]) != "[]" {
		t.Errorf("warnings = %s, want []", top["warnings"])
	}
}

// TestHealthWarningJSONFieldNames pins the R3-reserved warning shape (doc 50).
func TestHealthWarningJSONFieldNames(t *testing.T) {
	w := HealthWarning{
		Path:      "watcher/scans",
		Condition: "scan backlog > 100",
		Offender:  "hash-object",
		Since:     time.Date(2026, 7, 27, 11, 0, 0, 0, time.UTC),
	}
	data, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := []string{"condition", "offender", "path", "since"}
	if got := keysOf(t, obj); !reflect.DeepEqual(got, want) {
		t.Errorf("warning keys = %v, want %v", got, want)
	}
}

// TestSystemStatsRoundTrip proves lossless encode/decode of every field.
func TestSystemStatsRoundTrip(t *testing.T) {
	in := statsFixture()
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out SystemStats
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round trip mismatch:\n in: %+v\nout: %+v", in, out)
	}
}
