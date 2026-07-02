package daemon

import (
	"encoding/json"
	"testing"
)

func TestParseThemeChanged_FromGenericPayload(t *testing.T) {
	// Simulate the SSE client path: the payload arrives as
	// map[string]interface{} after generic JSON decoding.
	wire := []byte(`{
		"update_type": "theme_changed",
		"source": "config",
		"payload": {
			"name": "kanagawa",
			"family": "kanagawa",
			"mode": "hex",
			"dark": {"name": "kanagawa-dark", "appearance": "dark", "bg": "#1f1f28"},
			"light": {"name": "kanagawa-light", "appearance": "light", "bg": "#f2ecbc"}
		}
	}`)
	var update StateUpdate
	if err := json.Unmarshal(wire, &update); err != nil {
		t.Fatalf("unmarshal wire update: %v", err)
	}

	payload, ok := ParseThemeChanged(update)
	if !ok {
		t.Fatal("expected theme_changed payload to parse")
	}
	if payload.Name != "kanagawa" || payload.Family != "kanagawa" || payload.Mode != "hex" {
		t.Errorf("unexpected header: %+v", payload)
	}
	if payload.Dark == nil || payload.Dark.Bg != "#1f1f28" {
		t.Errorf("unexpected dark palette: %+v", payload.Dark)
	}
	if payload.Light == nil || payload.Light.Bg != "#f2ecbc" {
		t.Errorf("unexpected light palette: %+v", payload.Light)
	}
}

func TestParseThemeChanged_FromTypedPayload(t *testing.T) {
	update := StateUpdate{
		UpdateType: UpdateTypeThemeChanged,
		Payload:    &ThemeChangedPayload{Name: "terminal", Mode: "ansi"},
	}
	payload, ok := ParseThemeChanged(update)
	if !ok || payload.Name != "terminal" || payload.Mode != "ansi" {
		t.Fatalf("expected typed payload passthrough, got %v %v", payload, ok)
	}
}

func TestParseThemeChanged_FromInitialSnapshot(t *testing.T) {
	update := StateUpdate{
		UpdateType: "initial",
		Theme:      &ThemeChangedPayload{Name: "gruvbox", Family: "gruvbox", Mode: "hex"},
	}
	payload, ok := ParseThemeChanged(update)
	if !ok || payload.Name != "gruvbox" {
		t.Fatalf("expected initial snapshot theme to parse, got %v %v", payload, ok)
	}
}

func TestParseThemeChanged_Negative(t *testing.T) {
	cases := []StateUpdate{
		{UpdateType: "config_reload", ConfigFile: "grove.toml"},
		{UpdateType: "initial"}, // no theme stamped
		{UpdateType: UpdateTypeThemeChanged, Payload: nil},
		{UpdateType: UpdateTypeThemeChanged, Payload: map[string]interface{}{"family": "x"}}, // missing name
		{UpdateType: UpdateTypeThemeChanged, Payload: (*ThemeChangedPayload)(nil)},
	}
	for i, update := range cases {
		if _, ok := ParseThemeChanged(update); ok {
			t.Errorf("case %d: expected parse to fail for %+v", i, update)
		}
	}
}
