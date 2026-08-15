package sidecar

import (
	"encoding/json"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/grovetools/core/panelkit/panelproto"
)

// The viewport-split frames are the first pair that lets an OUT-OF-PROCESS
// panel ask the host for a second pane. These tests pin the runtime's half:
// that the two host→app frames decode into events, that they survive the trip
// to a tea.Msg, and that the capability which gates them is one a
// full-featured panel gets by default.

func frameOf(t *testing.T, typ string, payload any) panelproto.Frame {
	t.Helper()
	f := panelproto.Frame{Type: typ}
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal %s payload: %v", typ, err)
		}
		f.Payload = raw
	}
	return f
}

func TestDecodeViewportKeyFrame(t *testing.T) {
	c := &Client{}
	ev, ok := c.decode(frameOf(t, panelproto.TypeViewportKey, panelproto.ViewportKey{Key: "ctrl+d"}))
	if !ok {
		t.Fatal("a viewport_key frame was dropped")
	}
	got, isKey := ev.(ViewportKeyEvent)
	if !isKey {
		t.Fatalf("event = %T, want ViewportKeyEvent", ev)
	}
	if got.Key != "ctrl+d" {
		t.Errorf("Key = %q, want %q — the spelling is the one the claim was made in", got.Key, "ctrl+d")
	}
}

func TestDecodeViewportClosedFrame(t *testing.T) {
	c := &Client{}
	ev, ok := c.decode(frameOf(t, panelproto.TypeViewportClosed, nil))
	if !ok {
		t.Fatal("a viewport_closed frame was dropped")
	}
	if _, isClosed := ev.(ViewportClosedEvent); !isClosed {
		t.Fatalf("event = %T, want ViewportClosedEvent", ev)
	}
}

// A malformed payload is dropped rather than fatal, the same as every other
// frame this runtime decodes: a host that garbles one key must not take the
// panel's control plane down with it.
func TestDecodeMalformedViewportKeyIsDropped(t *testing.T) {
	c := &Client{}
	f := panelproto.Frame{Type: panelproto.TypeViewportKey, Payload: json.RawMessage(`"not an object"`)}
	if ev, ok := c.decode(f); ok {
		t.Errorf("a malformed viewport_key decoded to %#v, want it dropped", ev)
	}
}

func TestViewportEventsBecomeMessages(t *testing.T) {
	msg := ToMsg(ViewportKeyEvent{Key: "j"})
	key, ok := msg.(ViewportKeyMsg)
	if !ok {
		t.Fatalf("ToMsg(ViewportKeyEvent) = %T, want ViewportKeyMsg", msg)
	}
	if key.Key != "j" {
		t.Errorf("Key = %q, want %q", key.Key, "j")
	}
	if _, ok := ToMsg(ViewportClosedEvent{}).(ViewportClosedMsg); !ok {
		t.Error("ToMsg(ViewportClosedEvent) did not produce a ViewportClosedMsg")
	}
	// A sanity check that the new cases did not shadow the existing ones.
	if _, ok := ToMsg(CloseEvent{}).(tea.QuitMsg); !ok {
		t.Error("CloseEvent stopped becoming a quit")
	}
}

// A panel that opens a viewport but never declared `split` would never hear a
// forwarded key or learn that its pane went away, which is why the capability
// is in the default set rather than something each panel must remember.
func TestDefaultCapabilitiesIncludeSplit(t *testing.T) {
	for _, c := range DefaultCapabilities {
		if c == panelproto.CapSplit {
			return
		}
	}
	t.Errorf("DefaultCapabilities = %v, want it to include %q", DefaultCapabilities, panelproto.CapSplit)
}

// The three app→host sends are no-ops with no host, like every other Client
// send — a panel must be able to call them in a hostless run without guarding
// each one.
func TestSplitSendsAreNoOpsWithNoHost(t *testing.T) {
	c := &Client{}
	if err := c.SplitViewport(panelproto.SplitViewport{Title: "x"}); err != nil {
		t.Errorf("SplitViewport with no host: %v", err)
	}
	if err := c.UpdateSplitViewport(panelproto.SplitViewportUpdate{Content: "x"}); err != nil {
		t.Errorf("UpdateSplitViewport with no host: %v", err)
	}
	if err := c.CloseSplitViewport(); err != nil {
		t.Errorf("CloseSplitViewport with no host: %v", err)
	}
}
