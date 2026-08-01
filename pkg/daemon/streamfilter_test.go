package daemon

import (
	"net/url"
	"reflect"
	"testing"
)

// The zero filter is the compatibility contract: an old client's subscribe URL
// carries no parameters, must parse to the zero filter, and the zero filter
// must admit everything. If any of these three drift, filtering stops being
// additive and silently starves existing subscribers.
func TestZeroFilterIsTheFullStream(t *testing.T) {
	var f StreamFilter
	if !f.IsZero() {
		t.Fatal("zero StreamFilter should report IsZero")
	}
	if f.Encode() != "" {
		t.Fatalf("zero filter encoded to %q, want empty (URL must be unchanged)", f.Encode())
	}
	for _, typ := range []string{"initial", "session", "job_started", "workspaces_delta", "anything"} {
		if !f.AllowsType(typ) {
			t.Errorf("zero filter rejected type %q", typ)
		}
	}
	if !f.AllowsPath("/anywhere/at/all") {
		t.Error("zero filter rejected a path")
	}

	parsed := ParseStreamFilter(url.Values{})
	if !parsed.IsZero() {
		t.Fatalf("empty query parsed to %+v, want zero filter", parsed)
	}
	// A present-but-empty parameter is the same thing: ?types= is what a
	// caller that built a list and found it empty would send.
	parsed = ParseStreamFilter(url.Values{"types": {""}, "paths": {""}})
	if !parsed.IsZero() {
		t.Fatalf("empty parameters parsed to %+v, want zero filter", parsed)
	}
}

func TestAllowsType(t *testing.T) {
	f := StreamFilter{Types: []string{"session", "job_started"}}
	for _, tc := range []struct {
		typ  string
		want bool
	}{
		{"session", true},
		{"job_started", true},
		{"job_completed", false},
		{"workspaces_delta", false},
		// The snapshot is an ordinary member of the allow-list: a filter that
		// does not name it does not get it. This is the saving, so it must not
		// quietly become an exception.
		{StreamTypeInitial, false},
		{"", false},
		// Exact match only — no prefix or glob semantics leaking in.
		{"job", false},
		{"job_started_extra", false},
		{"SESSION", false},
	} {
		if got := f.AllowsType(tc.typ); got != tc.want {
			t.Errorf("AllowsType(%q) = %v, want %v", tc.typ, got, tc.want)
		}
	}

	if !(StreamFilter{Types: []string{StreamTypeInitial}}).AllowsType(StreamTypeInitial) {
		t.Error("a filter naming the snapshot should receive it")
	}
}

func TestAllowsPath(t *testing.T) {
	f := StreamFilter{Paths: []string{"/ws/alpha", "/ws/beta/"}}
	for _, tc := range []struct {
		path string
		want bool
	}{
		{"/ws/alpha", true},
		{"/ws/alpha/sub/dir", true},
		{"/ws/beta", true},         // trailing slash in the declaration
		{"/ws/beta/plans", true},   // and beneath it
		{"/ws/alphabet", false},    // prefix match must respect path boundaries
		{"/ws/alpha-other", false}, // ditto
		{"/ws/gamma", false},
		{"/ws", false},                // a parent is not inside the declared path
		{"/ws/alpha/../gamma", false}, // cleaned before comparison
		// An event with no path cannot be judged by a path filter; dropping it
		// would make the filter lossy in a way the caller never asked for.
		{"", true},
	} {
		if got := f.AllowsPath(tc.path); got != tc.want {
			t.Errorf("AllowsPath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestEncodeParseRoundTrip(t *testing.T) {
	f := StreamFilter{
		Types: []string{"session", "job_started"},
		Paths: []string{"/ws/a b", "/ws/c"},
	}
	q, err := url.ParseQuery(f.Encode())
	if err != nil {
		t.Fatalf("Encode produced an unparseable query %q: %v", f.Encode(), err)
	}
	got := ParseStreamFilter(q)
	if !reflect.DeepEqual(got, f) {
		t.Fatalf("round trip changed the filter: got %+v, want %+v", got, f)
	}
}

// An item containing a comma would split into two on the way back, so Encode
// drops it rather than corrupting the declaration. Better to under-declare a
// path (the subscriber sees more) than to invent one that matches nothing.
func TestEncodeDropsCommaBearingItems(t *testing.T) {
	f := StreamFilter{Types: []string{"session", "bad,type"}, Paths: []string{"/ws/a,b"}}
	q, err := url.ParseQuery(f.Encode())
	if err != nil {
		t.Fatalf("unparseable query: %v", err)
	}
	got := ParseStreamFilter(q)
	if !reflect.DeepEqual(got.Types, []string{"session"}) {
		t.Errorf("types = %v, want [session]", got.Types)
	}
	if len(got.Paths) != 0 {
		t.Errorf("paths = %v, want none", got.Paths)
	}
}

func TestParseTrimsAndSkipsBlanks(t *testing.T) {
	got := ParseStreamFilter(url.Values{"types": {" session , ,job_started, "}})
	want := []string{"session", "job_started"}
	if !reflect.DeepEqual(got.Types, want) {
		t.Fatalf("types = %v, want %v", got.Types, want)
	}
}

// The encoded URL has to be stable across process restarts so daemon logs and
// packet captures stay diffable; map iteration order is not.
func TestStreamFilterTypesIsSortedAndDropsFalseEntries(t *testing.T) {
	set := map[string]bool{"session": true, "job_started": true, "job_failed": true, "focus": false}
	want := []string{"job_failed", "job_started", "session"}
	for i := 0; i < 8; i++ {
		if got := StreamFilterTypes(set); !reflect.DeepEqual(got, want) {
			t.Fatalf("StreamFilterTypes = %v, want %v", got, want)
		}
	}
}
