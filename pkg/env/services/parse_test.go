package services

import "testing"

func TestParseAndSort_OrderThenName(t *testing.T) {
	config := map[string]interface{}{
		"services": map[string]interface{}{
			"zeta":  map[string]interface{}{"command": "z", "order": int64(10)},
			"alpha": map[string]interface{}{"command": "a", "order": float64(20)},
			"beta":  map[string]interface{}{"command": "b"}, // default order 100
			"gamma": map[string]interface{}{"command": "g", "order": int64(10)},
		},
	}
	got := ParseAndSort(config)
	if len(got) != 4 {
		t.Fatalf("got %d entries, want 4", len(got))
	}
	want := []string{"gamma", "zeta", "alpha", "beta"}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("pos %d: got %q, want %q", i, got[i].Name, name)
		}
	}
}

func TestParseAndSort_NilOrMalformed(t *testing.T) {
	if r := ParseAndSort(nil); r != nil {
		t.Errorf("nil config: got %v, want nil", r)
	}
	if r := ParseAndSort(map[string]interface{}{"services": "not-a-map"}); r != nil {
		t.Errorf("malformed: got %v, want nil", r)
	}
	if r := ParseAndSort(map[string]interface{}{}); r != nil {
		t.Errorf("missing key: got %v, want nil", r)
	}
}

func TestParseAndSort_PopulatesEntryFields(t *testing.T) {
	config := map[string]interface{}{
		"services": map[string]interface{}{
			"api": map[string]interface{}{
				"type":        "",
				"command":     "run api",
				"working_dir": "services/api",
				"port_env":    "API_PORT",
				"route":       "api",
				"env":         map[string]interface{}{"FOO": "bar"},
			},
		},
	}
	entries := ParseAndSort(config)
	if len(entries) != 1 {
		t.Fatalf("got %d entries", len(entries))
	}
	e := entries[0]
	if e.Command != "run api" || e.WorkingDir != "services/api" || e.PortEnv != "API_PORT" || e.Route != "api" {
		t.Errorf("entry fields wrong: %+v", e)
	}
	if v, _ := e.Env["FOO"].(string); v != "bar" {
		t.Errorf("env FOO = %v, want bar", e.Env["FOO"])
	}
	if e.Raw == nil {
		t.Errorf("Raw not populated")
	}
}

func TestParseLifecycle(t *testing.T) {
	if ParseLifecycle(nil) != nil {
		t.Error("nil input should return nil")
	}
	if ParseLifecycle(map[string]interface{}{"post_start": ""}) != nil {
		t.Error("empty post_start should return nil")
	}
	lc := ParseLifecycle(map[string]interface{}{"post_start": "./seed.sh"})
	if lc == nil || lc.PostStart != "./seed.sh" || lc.PostStartMode != "always" {
		t.Errorf("default mode: got %+v", lc)
	}
	lc = ParseLifecycle(map[string]interface{}{"post_start": "./seed.sh", "post_start_mode": "once"})
	if lc.PostStartMode != "once" {
		t.Errorf("mode=once: got %q", lc.PostStartMode)
	}
}

func TestParseHealthCheck(t *testing.T) {
	if ParseHealthCheck(nil) != nil {
		t.Error("nil input should return nil")
	}
	hc := ParseHealthCheck(map[string]interface{}{"type": "tcp"})
	if hc == nil || hc.Type != "tcp" || hc.TimeoutSeconds != 30 {
		t.Errorf("default timeout: got %+v", hc)
	}
	hc = ParseHealthCheck(map[string]interface{}{"type": "tcp", "timeout_seconds": int64(60)})
	if hc.TimeoutSeconds != 60 {
		t.Errorf("int64 timeout: got %d", hc.TimeoutSeconds)
	}
	hc = ParseHealthCheck(map[string]interface{}{"type": "tcp", "timeout_seconds": float64(45)})
	if hc.TimeoutSeconds != 45 {
		t.Errorf("float64 timeout: got %d", hc.TimeoutSeconds)
	}
}
