package daemon

import (
	"encoding/json"

	"github.com/grovetools/core/pkg/models"
)

// UpdateTypeForgeState is the SSE update_type the daemon's forge poller
// broadcasts when its cache changes. The payload is a
// models.ForgeStatePayload carrying only the repos that changed on that sweep.
const UpdateTypeForgeState = "forge_state"

// ParseForgeState extracts a forge payload from a StateUpdate, mirroring
// ParseThemeChanged: it accepts both an already-typed payload (in-process
// clients, tests) and the generic map a JSON-decoded SSE frame produces, so
// bespoke streams — treemux's HUD, nav — share one decode path.
//
// A frame with no repos still decodes successfully and reports ok. That is
// deliberate: the caller decides what an empty change-set means, and the one
// thing it never means is "there are no pull requests" (the stream is lossy by
// design — see models.ForgeStatePayload).
func ParseForgeState(update StateUpdate) (*models.ForgeStatePayload, bool) {
	if update.UpdateType != UpdateTypeForgeState {
		return nil, false
	}
	switch p := update.Payload.(type) {
	case *models.ForgeStatePayload:
		if p == nil {
			return nil, false
		}
		return p, true
	case models.ForgeStatePayload:
		return &p, true
	}
	data, err := json.Marshal(update.Payload)
	if err != nil {
		return nil, false
	}
	var p models.ForgeStatePayload
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, false
	}
	return &p, true
}
