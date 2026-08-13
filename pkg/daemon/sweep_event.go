package daemon

import (
	"encoding/json"

	"github.com/grovetools/core/pkg/models"
)

// The SSE update_types the daemon's tier-ordered git sweep broadcasts. The
// payload of all three is a models.GitSweepProgress.
//
// A consumer that wants only these — a progress bar, an Inspector page — should
// declare them on the subscription (StreamFilter.Types) rather than filtering a
// firehose client-side: the sweep runs concurrently with the workspace deltas
// it produces, which are the largest frames the daemon sends.
const (
	UpdateTypeSweepStarted   = "sweep_started"
	UpdateTypeSweepProgress  = "sweep_progress"
	UpdateTypeSweepCompleted = "sweep_completed"
)

// SweepUpdateTypes is the roster of sweep event types, for callers building a
// StreamFilter.
func SweepUpdateTypes() []string {
	return []string{UpdateTypeSweepStarted, UpdateTypeSweepProgress, UpdateTypeSweepCompleted}
}

// ParseSweepProgress extracts a sweep payload from a StateUpdate, mirroring
// ParseForgeState: it accepts an already-typed payload (in-process clients,
// tests) and the generic map a JSON-decoded SSE frame produces.
//
// The update type is returned to the caller by way of update.UpdateType — the
// three sweep events share one payload shape deliberately, so a consumer can
// render every frame through the same path and only branch on start/finish
// where it actually matters.
func ParseSweepProgress(update StateUpdate) (*models.GitSweepProgress, bool) {
	switch update.UpdateType {
	case UpdateTypeSweepStarted, UpdateTypeSweepProgress, UpdateTypeSweepCompleted:
	default:
		return nil, false
	}
	if update.Payload == nil {
		// A sweep frame with no payload carries no position; decoding it would
		// hand the caller a zero-valued "0 of 0 done" that renders as a
		// finished sweep.
		return nil, false
	}
	switch p := update.Payload.(type) {
	case *models.GitSweepProgress:
		if p == nil {
			return nil, false
		}
		return p, true
	case models.GitSweepProgress:
		return &p, true
	}
	data, err := json.Marshal(update.Payload)
	if err != nil {
		return nil, false
	}
	var p models.GitSweepProgress
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, false
	}
	return &p, true
}
