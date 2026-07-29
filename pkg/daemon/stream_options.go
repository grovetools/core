package daemon

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
)

// The hardened /api/stream contract, client side.
//
// The daemon's state stream used to be an unfiltered, uncursored firehose:
// every subscriber decoded every frame, and a subscriber whose buffer filled
// (or that reconnected) had no way to learn what it missed. Since the
// hardening, frames carry a monotonic sequence, ?since= replays a bounded
// in-memory ring, and ?types= filters server-side.
//
// ALL OF IT IS OPTIONAL, IN BOTH DIRECTIONS. A caller that keeps using
// StreamState gets the historical behavior. A caller that opts in may still be
// talking to a daemon that predates the feature — an older groved ignores
// unknown query parameters and emits no sequence numbers, so it answers 200
// with a plain firehose rather than 404. That is why capability detection here
// reads a RESPONSE HEADER instead of relying on the IsEndpointNotFound
// convention that covers wholly-new endpoints: the endpoint is not new, its
// contract is. Callers MUST check StreamCapabilities before trusting Seq, and
// must be prepared to receive frames they asked to filter out.
const (
	// StreamFeaturesHeader lists the features the connected daemon supports.
	StreamFeaturesHeader = "X-Grove-Stream-Features"
	// StreamRingHeader carries the daemon's replay-ring bound, in updates.
	StreamRingHeader = "X-Grove-Stream-Ring"

	// UpdateTypeStreamGap is the control frame the daemon emits when a ?since=
	// cursor could not be honored exactly. It is never suppressed by ?types=.
	UpdateTypeStreamGap = "stream_gap"
)

// Stream feature tokens as they appear in StreamFeaturesHeader.
const (
	streamFeatureSeq    = "seq"
	streamFeatureSince  = "since"
	streamFeatureTypes  = "types"
	streamFeatureGapAll = "seq,since,types"
)

// StreamOptions configures a state subscription. The zero value is the
// historical firehose-from-now behavior.
type StreamOptions struct {
	// Since resumes after a sequence number the caller already processed.
	// Only consulted when Resume is set, so a caller can legitimately ask to
	// replay from the very beginning of the daemon's ring with Since == 0.
	Since uint64
	// Resume opts into replay. Without it, Since is ignored and the stream
	// starts from now (plus the usual initial snapshot).
	Resume bool
	// Types filters server-side by update_type glob — "job_*", "note_event".
	// Empty means everything. A daemon that does not support filtering sends
	// everything regardless, so a caller that MUST NOT see other types should
	// re-check locally (see StreamCapabilities.TypeFilter).
	Types []string
}

// query renders the options as a URL query string ("" when nothing is set).
func (o StreamOptions) query() string {
	values := url.Values{}
	if o.Resume {
		values.Set("since", strconv.FormatUint(o.Since, 10))
	}
	if types := o.cleanTypes(); len(types) > 0 {
		values.Set("types", strings.Join(types, ","))
	}
	return values.Encode()
}

func (o StreamOptions) cleanTypes() []string {
	out := make([]string, 0, len(o.Types))
	for _, t := range o.Types {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// StreamCapabilities reports what the daemon on the other end of a
// subscription actually supports. A zero value means a pre-hardening daemon:
// no sequence numbers, no replay, no server-side filtering.
type StreamCapabilities struct {
	// Sequenced means frames carry a usable Seq.
	Sequenced bool
	// Replay means ?since= was honored.
	Replay bool
	// TypeFilter means ?types= was honored server-side.
	TypeFilter bool
	// RingSize is the daemon's replay bound in updates (0 when unknown). A
	// client that wants gap-free resumption should reconnect well inside it.
	RingSize int
}

// Supported reports whether the daemon implements the hardened contract at all.
func (c StreamCapabilities) Supported() bool { return c.Sequenced || c.Replay || c.TypeFilter }

// parseStreamCapabilities reads the advertisement headers off an SSE response.
func parseStreamCapabilities(features, ring string) StreamCapabilities {
	var caps StreamCapabilities
	for _, token := range strings.Split(features, ",") {
		switch strings.TrimSpace(token) {
		case streamFeatureSeq:
			caps.Sequenced = true
		case streamFeatureSince:
			caps.Replay = true
		case streamFeatureTypes:
			caps.TypeFilter = true
		}
	}
	if n, err := strconv.Atoi(strings.TrimSpace(ring)); err == nil && n > 0 {
		caps.RingSize = n
	}
	return caps
}

// StreamGap is the payload of a stream_gap control frame: the daemon could not
// honor a ?since= cursor exactly, and the caller must snapshot-reconcile
// rather than assume it has a continuous history.
//
// The daemon helps: after a gap it re-sends the "initial" snapshot frame, so a
// consumer whose state is derived purely from the snapshot needs no extra
// fetch. A consumer that reacts to individual lifecycle events (a job finished,
// a note changed) has genuinely lost those and must re-read whatever it cares
// about via the REST endpoints.
type StreamGap struct {
	// Reason is StreamGapTooOld or StreamGapReset.
	Reason string `json:"reason"`
	// Since is the cursor that was sent.
	Since uint64 `json:"since"`
	// Oldest is the lowest sequence the daemon could still have replayed.
	Oldest uint64 `json:"oldest"`
	// Current is the daemon's sequence when the gap was detected.
	Current uint64 `json:"current"`
	// RingSize is the daemon's replay bound in updates.
	RingSize int `json:"ring_size"`
}

const (
	// StreamGapTooOld means the daemon's ring had already evicted the updates
	// the cursor asked for: the client was away too long, or fell too far
	// behind. Reconnect sooner, or filter harder.
	StreamGapTooOld = "too_old"
	// StreamGapReset means the cursor is AHEAD of the daemon — sequences
	// restart at 1 with each daemon process, so this is what a daemon restart
	// looks like from the client side. Discard the cursor.
	StreamGapReset = "reset"
)

// Restarted reports whether the gap is a daemon restart rather than an
// overflow. Both require reconciling; only this one invalidates the cursor.
func (g StreamGap) Restarted() bool { return g.Reason == StreamGapReset }

// ParseStreamGap extracts the gap payload from a stream_gap frame, mirroring
// ParseThemeChanged's shape so consumers have one decode idiom for control
// frames.
func ParseStreamGap(update StateUpdate) (*StreamGap, bool) {
	if update.UpdateType != UpdateTypeStreamGap {
		return nil, false
	}
	if g, ok := update.Payload.(*StreamGap); ok && g != nil {
		return g, true
	}
	data, err := json.Marshal(update.Payload)
	if err != nil {
		return nil, false
	}
	var g StreamGap
	if err := json.Unmarshal(data, &g); err != nil || g.Reason == "" {
		return nil, false
	}
	return &g, true
}
