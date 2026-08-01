package daemon

import (
	"net/url"
	"path/filepath"
	"sort"
	"strings"
)

// StreamTypeInitial is the update_type of the snapshot frame /api/stream sends
// at subscribe time. It is not a store update type — the server synthesizes it
// — but it is matched by the same allow-list as every other type, so a filtered
// subscriber that wants the snapshot has to name it explicitly.
//
// This matters more than it looks: the snapshot is the entire enriched-workspace
// map plus the plan index, and on a mature host it is multiple megabytes — the
// overwhelming majority of all bytes a subscriber ever reads. Omitting it is the
// single largest saving a filter can make. A subscriber that omits it also gives
// up the theme and boot-phase state the snapshot carries, so a TUI that restyles
// on daemon theme changes must list "initial".
const StreamTypeInitial = "initial"

// StreamFilter declares which /api/stream events a subscriber wants delivered.
// The zero value means "everything", which is the historical behaviour and what
// an unfiltered subscribe keeps getting — the filter is purely additive.
//
// Filtering is server-side by design: a client-side drop list still pays for the
// daemon to serialize and write the event and for the client to decode it, and
// it makes every subscriber re-implement the same list. Declaring interest at
// subscribe time means the bytes are never produced.
type StreamFilter struct {
	// Types is an allow-list of update_type values. Empty means every type.
	// Matching is exact — no globs. Include StreamTypeInitial to keep the
	// subscribe-time snapshot.
	Types []string
	// Paths is an allow-list of workspace path prefixes, applied only to the
	// events that carry workspace paths (the workspace snapshot and deltas).
	// Empty means every workspace. An event with no workspace path in it is
	// unaffected: a path allow-list cannot judge a session or job event, and
	// silently dropping those would make the filter lossy in a way callers
	// cannot see.
	Paths []string
}

// IsZero reports whether the filter constrains nothing, i.e. the subscriber
// gets the full stream.
func (f StreamFilter) IsZero() bool { return len(f.Types) == 0 && len(f.Paths) == 0 }

// AllowsType reports whether updateType survives the type allow-list.
func (f StreamFilter) AllowsType(updateType string) bool {
	if len(f.Types) == 0 {
		return true
	}
	for _, t := range f.Types {
		if t == updateType {
			return true
		}
	}
	return false
}

// AllowsPath reports whether a workspace path survives the path allow-list.
// A declared path matches itself and anything beneath it.
func (f StreamFilter) AllowsPath(path string) bool {
	if len(f.Paths) == 0 {
		return true
	}
	if path == "" {
		// No path to judge — the caller decides; be permissive rather than
		// silently dropping an event whose scope we cannot determine.
		return true
	}
	path = filepath.Clean(path)
	for _, want := range f.Paths {
		want = filepath.Clean(want)
		if path == want || strings.HasPrefix(path, want+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// Encode renders the filter as query parameters for GET /api/stream (no leading
// "?"). The zero filter encodes to the empty string, so an unfiltered subscribe
// sends the exact URL it always did.
//
// List items are comma-separated. An item containing a comma is dropped rather
// than encoded, because the parser splits on commas after URL-decoding and
// would otherwise silently turn one item into two.
func (f StreamFilter) Encode() string {
	var parts []string
	if v := encodeStreamList(f.Types); v != "" {
		parts = append(parts, "types="+v)
	}
	if v := encodeStreamList(f.Paths); v != "" {
		parts = append(parts, "paths="+v)
	}
	return strings.Join(parts, "&")
}

func encodeStreamList(items []string) string {
	escaped := make([]string, 0, len(items))
	for _, it := range items {
		it = strings.TrimSpace(it)
		if it == "" || strings.Contains(it, ",") {
			continue
		}
		escaped = append(escaped, url.QueryEscape(it))
	}
	return strings.Join(escaped, ",")
}

// ParseStreamFilter reads a StreamFilter out of a request's query parameters.
// Absent or empty parameters yield the zero filter (full stream), so an old
// client's URL parses to today's behaviour.
func ParseStreamFilter(q url.Values) StreamFilter {
	return StreamFilter{
		Types: parseStreamList(q.Get("types")),
		Paths: parseStreamList(q.Get("paths")),
	}
}

func parseStreamList(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// StreamFilterTypes builds a type allow-list from a set-shaped map, the form
// TUIs already keep their client-side drop lists in. The result is sorted so
// the encoded URL is stable across runs (map iteration order is not), which
// keeps daemon logs and packet captures diffable.
func StreamFilterTypes(set map[string]bool) []string {
	types := make([]string, 0, len(set))
	for t, want := range set {
		if want {
			types = append(types, t)
		}
	}
	sort.Strings(types)
	return types
}
