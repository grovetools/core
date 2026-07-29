package daemon

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"
)

// serveStreamWithFeatures serves /api/stream on a unix socket, echoing back
// the query it was called with (as the source field) so a test can prove the
// parameters reached the daemon, and advertising the given feature header.
// An empty features string models a daemon that predates stream hardening.
func serveStreamWithFeatures(t *testing.T, sockPath, features string, ring int) {
	t.Helper()
	ul, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen unix %s: %v", sockPath, err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if features != "" {
			w.Header().Set(StreamFeaturesHeader, features)
			w.Header().Set(StreamRingHeader, fmt.Sprint(ring))
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "data: {\"update_type\":\"test-marker\",\"seq\":7,\"source\":%q}\n\n", r.URL.RawQuery)
		if fl, ok := w.(http.Flusher); ok {
			fl.Flush()
		}
		<-r.Context().Done()
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(ul)
	t.Cleanup(func() { srv.Close(); ul.Close() })
}

func streamOnce(t *testing.T, sockPath string, opts StreamOptions) (StateUpdate, StreamCapabilities) {
	t.Helper()
	client, err := NewRemoteClient(sockPath)
	if err != nil {
		t.Fatalf("NewRemoteClient: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	ch, caps, err := client.StreamStateWithOptions(ctx, opts)
	if err != nil {
		t.Fatalf("StreamStateWithOptions: %v", err)
	}
	select {
	case u := <-ch:
		return u, caps
	case <-time.After(3 * time.Second):
		t.Fatal("no update received")
	}
	return StateUpdate{}, caps
}

func TestStreamOptionsReachTheDaemon(t *testing.T) {
	sock := shortTempSocket(t)
	serveStreamWithFeatures(t, sock, streamFeatureGapAll, 1024)

	update, caps := streamOnce(t, sock, StreamOptions{
		Resume: true,
		Since:  42,
		Types:  []string{"job_*", " ", "note_event"},
	})

	query, err := url.ParseQuery(update.Source)
	if err != nil {
		t.Fatalf("parse echoed query %q: %v", update.Source, err)
	}
	if got := query.Get("since"); got != "42" {
		t.Errorf("since = %q, want 42", got)
	}
	if got := query.Get("types"); got != "job_*,note_event" {
		t.Errorf("types = %q, want job_*,note_event (blank entries dropped)", got)
	}
	if !caps.Sequenced || !caps.Replay || !caps.TypeFilter {
		t.Errorf("capabilities = %+v, want all three", caps)
	}
	if caps.RingSize != 1024 {
		t.Errorf("RingSize = %d, want 1024", caps.RingSize)
	}
	if update.Seq != 7 {
		t.Errorf("Seq = %d, want 7", update.Seq)
	}
}

// Resume=false must not send since= at all, so an old daemon (and a reader of
// the daemon log) sees exactly the historical request.
func TestStreamOptionsOmitAnUnrequestedCursor(t *testing.T) {
	sock := shortTempSocket(t)
	serveStreamWithFeatures(t, sock, streamFeatureGapAll, 1024)

	update, _ := streamOnce(t, sock, StreamOptions{Types: []string{"job_*"}})
	query, _ := url.ParseQuery(update.Source)
	if _, present := query["since"]; present {
		t.Errorf("since was sent without Resume: %q", update.Source)
	}
}

// The zero options must produce a bare request — StreamState's exact behavior.
func TestZeroStreamOptionsSendNoQuery(t *testing.T) {
	if got := (StreamOptions{}).query(); got != "" {
		t.Errorf("zero StreamOptions rendered %q, want an empty query", got)
	}
	// Resume with Since 0 is meaningful: "replay everything you still hold".
	if got := (StreamOptions{Resume: true}).query(); got != "since=0" {
		t.Errorf("Resume-from-zero rendered %q, want since=0", got)
	}
}

// Backward compatibility: a daemon that predates the hardening answers 200 and
// ignores the parameters. The client must surface that honestly rather than
// pretend the stream is filtered and sequenced.
func TestOldDaemonReportsNoCapabilities(t *testing.T) {
	sock := shortTempSocket(t)
	serveStreamWithFeatures(t, sock, "", 0)

	_, caps := streamOnce(t, sock, StreamOptions{Resume: true, Since: 5, Types: []string{"job_*"}})
	if caps.Supported() {
		t.Fatalf("capabilities = %+v, want the zero value for a pre-hardening daemon", caps)
	}
}

// A daemon that gained sequencing but not filtering must be described exactly,
// not rounded up.
func TestPartialCapabilityAdvertisement(t *testing.T) {
	caps := parseStreamCapabilities("seq, since", "256")
	if !caps.Sequenced || !caps.Replay {
		t.Errorf("caps = %+v, want Sequenced and Replay", caps)
	}
	if caps.TypeFilter {
		t.Error("TypeFilter was inferred from an advertisement that did not include it")
	}
	if caps.RingSize != 256 {
		t.Errorf("RingSize = %d, want 256", caps.RingSize)
	}
	if !caps.Supported() {
		t.Error("Supported() = false despite two features")
	}
}

func TestParseStreamGap(t *testing.T) {
	// The wire shape: payload decoded from JSON into a generic map.
	update := StateUpdate{
		UpdateType: UpdateTypeStreamGap,
		Payload: map[string]any{
			"reason":    StreamGapTooOld,
			"since":     float64(3),
			"oldest":    float64(100),
			"current":   float64(1123),
			"ring_size": float64(1024),
		},
	}
	gap, ok := ParseStreamGap(update)
	if !ok {
		t.Fatal("ParseStreamGap returned false for a stream_gap frame")
	}
	if gap.Reason != StreamGapTooOld || gap.Oldest != 100 || gap.Current != 1123 || gap.RingSize != 1024 {
		t.Fatalf("gap = %+v", gap)
	}
	if gap.Restarted() {
		t.Error("a too_old gap reported Restarted()")
	}

	// A typed payload (in-process producers) decodes without a round trip.
	typed := StateUpdate{UpdateType: UpdateTypeStreamGap, Payload: &StreamGap{Reason: StreamGapReset}}
	gap, ok = ParseStreamGap(typed)
	if !ok || !gap.Restarted() {
		t.Fatalf("typed gap = %+v, ok = %v; want a reset", gap, ok)
	}

	// Anything else is not a gap.
	for _, other := range []StateUpdate{
		{UpdateType: "job_completed"},
		{UpdateType: UpdateTypeStreamGap, Payload: map[string]any{}},
		{UpdateType: UpdateTypeStreamGap, Payload: "not an object"},
	} {
		if _, ok := ParseStreamGap(other); ok {
			t.Errorf("ParseStreamGap accepted %+v", other)
		}
	}
}
