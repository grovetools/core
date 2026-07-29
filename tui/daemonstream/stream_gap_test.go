package daemonstream

import (
	"testing"

	"github.com/grovetools/core/pkg/daemon"
)

// A stream_gap frame must surface to the embedding TUI as a StreamGapMsg, not
// be swallowed as an unrecognized update type — a TUI that never hears about
// the gap keeps rendering state it silently stopped receiving deltas for.
func TestHandleUpdateSurfacesStreamGap(t *testing.T) {
	cmd := HandleUpdate(daemon.StateUpdate{
		UpdateType: daemon.UpdateTypeStreamGap,
		Payload: map[string]any{
			"reason":    daemon.StreamGapReset,
			"since":     float64(900),
			"oldest":    float64(1),
			"current":   float64(12),
			"ring_size": float64(1024),
		},
	})
	if cmd == nil {
		t.Fatal("HandleUpdate returned no cmd for a stream_gap frame")
	}
	msg, ok := cmd().(StreamGapMsg)
	if !ok {
		t.Fatalf("cmd produced %T, want StreamGapMsg", cmd())
	}
	if !msg.Gap.Restarted() {
		t.Errorf("gap = %+v, want a restart", msg.Gap)
	}
	if msg.Gap.Since != 900 || msg.Gap.Current != 12 {
		t.Errorf("gap bookkeeping = %+v, want Since 900 / Current 12", msg.Gap)
	}
}

// Ordinary frames must keep their existing behavior.
func TestHandleUpdateIgnoresUnrelatedFrames(t *testing.T) {
	if cmd := HandleUpdate(daemon.StateUpdate{UpdateType: "job_completed"}); cmd != nil {
		t.Fatal("HandleUpdate produced a cmd for an unrelated update type")
	}
}
