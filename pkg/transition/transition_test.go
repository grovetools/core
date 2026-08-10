package transition

import (
	"bytes"
	"strings"
	"testing"
)

func TestValidatePositiveEvidence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		evidence Evidence
	}{
		{
			name: "positive count",
			evidence: Evidence{
				Action: "migrate",
				Counts: []Count{{Name: "notebooks", Value: 2}},
			},
		},
		{
			name: "resolved root",
			evidence: Evidence{
				Action: "mount",
				ResolvedRoots: []ResolvedRoot{{
					Name: "code", Declared: "~/Code", Resolved: "/home/alice/Code",
				}},
			},
		},
		{
			name: "accepted server echo",
			evidence: Evidence{
				Action:       "subscribe",
				ServerBacked: true,
				ServerEcho: &ServerEcho{
					Accepted: true, Response: "subscription active", RequestID: "req-7",
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := tt.evidence.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestValidateZeroEvidenceRequiresReason(t *testing.T) {
	t.Parallel()

	withoutReason := Evidence{
		Action: "migrate",
		Counts: []Count{{Name: "notebooks", Value: 0}},
	}
	if err := withoutReason.FinishSuccess(); err == nil || !strings.Contains(err.Error(), "zero evidence") {
		t.Fatalf("FinishSuccess() error = %v, want zero evidence error", err)
	}

	withoutReason.Reason = "already migrated"
	if err := withoutReason.FinishSuccess(); err != nil {
		t.Fatalf("FinishSuccess() with reason error = %v", err)
	}

	withoutReason.Reason = " \t "
	if err := withoutReason.FinishSuccess(); err == nil {
		t.Fatal("FinishSuccess() accepted whitespace-only reason")
	}
}

func TestValidateServerBackedRequiresAcceptedEcho(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		echo *ServerEcho
		want string
	}{
		{name: "missing", want: "requires an accepted server echo"},
		{name: "rejected", echo: &ServerEcho{Response: "rejected"}, want: "does not show acceptance"},
		{name: "request is not response", echo: &ServerEcho{Accepted: true}, want: "response is required"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := Evidence{Action: "subscribe", ServerBacked: true, ServerEcho: tt.echo, Reason: "no changes"}
			if err := e.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestValidateRejectsMalformedEvidence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		evidence Evidence
		want     string
	}{
		{name: "action", evidence: Evidence{Reason: "none"}, want: "action is required"},
		{name: "count name", evidence: Evidence{Action: "x", Counts: []Count{{Value: 1}}}, want: "name is required"},
		{name: "negative count", evidence: Evidence{Action: "x", Counts: []Count{{Name: "items", Value: -1}}, Reason: "none"}, want: "must not be negative"},
		{name: "duplicate count", evidence: Evidence{Action: "x", Counts: []Count{{Name: "items", Value: 1}, {Name: "items", Value: 2}}}, want: "duplicated"},
		{name: "root declared", evidence: Evidence{Action: "x", ResolvedRoots: []ResolvedRoot{{Name: "code", Resolved: "/code"}}}, want: "declared path is required"},
		{name: "root resolved", evidence: Evidence{Action: "x", ResolvedRoots: []ResolvedRoot{{Name: "code", Declared: "~/code"}}}, want: "resolved path is required"},
		{name: "duplicate root", evidence: Evidence{Action: "x", ResolvedRoots: []ResolvedRoot{{Name: "code", Declared: "a", Resolved: "b"}, {Name: "code", Declared: "c", Resolved: "d"}}}, want: "duplicated"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := tt.evidence.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestRenderHumanStableOrderingAndRootDisplay(t *testing.T) {
	t.Parallel()

	e := Evidence{
		Action: "migrate",
		Counts: []Count{
			{Name: "roots", Value: 1},
			{Name: "notebooks", Value: 2},
		},
		ResolvedRoots: []ResolvedRoot{
			{Name: "work", Declared: "$HOME/work", Resolved: "/home/alice/work"},
			{Name: "code", Declared: "~/Code", Resolved: "/home/alice/Code"},
		},
		ServerEcho: &ServerEcho{Accepted: true, Response: "applied", RequestID: "42"},
	}
	want := "transition: migrate\n" +
		"counts:\n" +
		"  notebooks: 2\n" +
		"  roots: 1\n" +
		"resolved roots:\n" +
		"  code: ~/Code -> /home/alice/Code\n" +
		"  work: $HOME/work -> /home/alice/work\n" +
		"server accepted: applied (request 42)\n"

	var got bytes.Buffer
	if err := RenderHuman(&got, e); err != nil {
		t.Fatalf("RenderHuman() error = %v", err)
	}
	if got.String() != want {
		t.Fatalf("RenderHuman() =\n%s\nwant:\n%s", got.String(), want)
	}

	// Rendering must neither depend on nor mutate caller slice order.
	if e.Counts[0].Name != "roots" || e.ResolvedRoots[0].Name != "work" {
		t.Fatal("RenderHuman mutated caller-owned slices")
	}
}

func TestRenderJSONStableOrdering(t *testing.T) {
	t.Parallel()

	e := Evidence{
		Action: "migrate",
		Counts: []Count{{Name: "z", Value: 3}, {Name: "a", Value: 1}},
		ResolvedRoots: []ResolvedRoot{
			{Name: "z", Declared: "~/z", Resolved: "/home/a/z"},
			{Name: "a", Declared: "~/a", Resolved: "/home/a/a"},
		},
	}
	want := "{\n" +
		"  \"action\": \"migrate\",\n" +
		"  \"counts\": [\n" +
		"    {\n" +
		"      \"name\": \"a\",\n" +
		"      \"value\": 1\n" +
		"    },\n" +
		"    {\n" +
		"      \"name\": \"z\",\n" +
		"      \"value\": 3\n" +
		"    }\n" +
		"  ],\n" +
		"  \"resolved_roots\": [\n" +
		"    {\n" +
		"      \"name\": \"a\",\n" +
		"      \"declared\": \"~/a\",\n" +
		"      \"resolved\": \"/home/a/a\"\n" +
		"    },\n" +
		"    {\n" +
		"      \"name\": \"z\",\n" +
		"      \"declared\": \"~/z\",\n" +
		"      \"resolved\": \"/home/a/z\"\n" +
		"    }\n" +
		"  ]\n" +
		"}\n"

	var first, second bytes.Buffer
	if err := RenderJSON(&first, e); err != nil {
		t.Fatalf("RenderJSON() error = %v", err)
	}
	if err := RenderJSON(&second, e); err != nil {
		t.Fatalf("RenderJSON() second error = %v", err)
	}
	if first.String() != want || second.String() != first.String() {
		t.Fatalf("RenderJSON() =\n%s\nwant:\n%s", first.String(), want)
	}
}

func TestRenderersRefuseInvalidSuccess(t *testing.T) {
	t.Parallel()

	e := Evidence{Action: "migrate"}
	if err := RenderHuman(&bytes.Buffer{}, e); err == nil {
		t.Fatal("RenderHuman accepted evidence-free success")
	}
	if err := RenderJSON(&bytes.Buffer{}, e); err == nil {
		t.Fatal("RenderJSON accepted evidence-free success")
	}
}
