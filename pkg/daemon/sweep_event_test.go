package daemon

import (
	"encoding/json"
	"testing"

	"github.com/grovetools/core/pkg/models"
)

// The real SSE path: the frame has been through JSON, so Payload arrives as a
// generic map.
func TestParseSweepProgressFromGenericPayload(t *testing.T) {
	data, err := json.Marshal(models.GitSweepProgress{
		Reason: "boot", Tier: "cold",
		TierDone: 8, TierTotal: 613, Done: 68, Total: 681,
		ElapsedMS: 45_000, WorkMS: 4_500,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var generic map[string]any
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	p, ok := ParseSweepProgress(StateUpdate{UpdateType: UpdateTypeSweepProgress, Payload: generic})
	if !ok {
		t.Fatal("ParseSweepProgress refused a generic payload")
	}
	if p.Done != 68 || p.Total != 681 || p.Tier != "cold" {
		t.Fatalf("decoded %+v", p)
	}
	// Elapsed far exceeding work is the paced tail, not a stall — a consumer
	// must be able to tell them apart.
	if p.WorkMS >= p.ElapsedMS {
		t.Fatalf("work %d / elapsed %d: the pacing gap did not survive decoding", p.WorkMS, p.ElapsedMS)
	}
}

func TestParseSweepProgressAcceptsAllThreeEventsAndRejectsOthers(t *testing.T) {
	payload := &models.GitSweepProgress{Reason: "refresh", Total: 3}
	for _, typ := range SweepUpdateTypes() {
		if _, ok := ParseSweepProgress(StateUpdate{UpdateType: typ, Payload: payload}); !ok {
			t.Errorf("%s did not parse", typ)
		}
	}
	if _, ok := ParseSweepProgress(StateUpdate{UpdateType: "workspaces_delta", Payload: payload}); ok {
		t.Error("ParseSweepProgress claimed an unrelated update type")
	}
	if _, ok := ParseSweepProgress(StateUpdate{UpdateType: UpdateTypeSweepStarted}); ok {
		t.Error("a nil payload parsed as sweep progress")
	}
}
