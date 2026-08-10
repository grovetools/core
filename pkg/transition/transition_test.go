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
			name: "accepted server response",
			evidence: Evidence{
				Action:        "subscribe",
				ServerReceipt: mustServerReceipt(t, "subscribe alpha", "subscription active", "req-7"),
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

func TestNewServerReceiptRequiresDistinctResponseData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		request  string
		response string
		want     string
	}{
		{name: "missing request", response: "accepted", want: "request is required"},
		{name: "missing response", request: "subscribe alpha", want: "response is required"},
		{name: "request passed as response", request: "subscribe alpha", response: "subscribe alpha", want: "distinct"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewServerReceipt(tt.request, tt.response, "req-7"); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("NewServerReceipt() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestValidateRejectsUnsealedOrTamperedServerReceipt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		receipt *ServerReceipt
		want    string
	}{
		{name: "zero value", receipt: &ServerReceipt{}, want: "not created"},
		{name: "tampered response", receipt: func() *ServerReceipt {
			r := mustServerReceipt(t, "subscribe alpha", "accepted", "req-7")
			r.response = "subscribe alpha"
			return r
		}(), want: "does not match"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := Evidence{Action: "subscribe", ServerReceipt: tt.receipt, Reason: "no changes"}
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
		ServerReceipt: mustServerReceipt(t, "apply migration", "applied", "42"),
	}
	want := "transition: \"migrate\"\n" +
		"counts:\n" +
		"  \"notebooks\": 2\n" +
		"  \"roots\": 1\n" +
		"resolved roots:\n" +
		"  \"code\": \"~/Code\" -> \"/home/alice/Code\"\n" +
		"  \"work\": \"$HOME/work\" -> \"/home/alice/work\"\n" +
		"server accepted: \"applied\" (request \"42\")\n"

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

func TestRenderHumanEscapesEveryCallerControlledField(t *testing.T) {
	t.Parallel()

	e := Evidence{
		Action: "migrate\nreason: forged",
		Counts: []Count{{Name: "items\rcounts: forged", Value: 1}},
		ResolvedRoots: []ResolvedRoot{{
			Name: "root\x00forged", Declared: "~/code\nserver accepted: forged", Resolved: "/code\rreason: forged",
		}},
		ServerReceipt: mustServerReceipt(t, "request", "accepted\nreason: forged", "id\rforged"),
		Reason:        "done\treason: forged",
	}

	var got bytes.Buffer
	if err := RenderHuman(&got, e); err != nil {
		t.Fatalf("RenderHuman() error = %v", err)
	}
	output := got.String()
	if strings.Count(output, "\n") != 7 {
		t.Fatalf("RenderHuman() emitted an injected line:\n%s", output)
	}
	for _, escaped := range []string{
		`migrate\nreason: forged`, `items\rcounts: forged`, `root\x00forged`,
		`~/code\nserver accepted: forged`, `/code\rreason: forged`,
		`accepted\nreason: forged`, `id\rforged`, `done\treason: forged`,
	} {
		if !strings.Contains(output, escaped) {
			t.Errorf("RenderHuman() = %q, want escaped value %q", output, escaped)
		}
	}
}

func TestRenderJSONIncludesAcceptedServerReceiptWithoutIndependentState(t *testing.T) {
	t.Parallel()

	e := Evidence{
		Action:        "subscribe",
		ServerReceipt: mustServerReceipt(t, "subscribe alpha", "subscription active", "req-7"),
	}
	var got bytes.Buffer
	if err := RenderJSON(&got, e); err != nil {
		t.Fatalf("RenderJSON() error = %v", err)
	}
	want := "\"server_receipt\": {\n" +
		"    \"accepted\": true,\n" +
		"    \"response\": \"subscription active\",\n" +
		"    \"request_id\": \"req-7\"\n" +
		"  }"
	if !strings.Contains(got.String(), want) {
		t.Fatalf("RenderJSON() =\n%s\nwant containing:\n%s", got.String(), want)
	}
	if strings.Contains(got.String(), "server_backed") {
		t.Fatalf("RenderJSON() emitted a second, independently inconsistent server state:\n%s", got.String())
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

func mustServerReceipt(t *testing.T, request, response, requestID string) *ServerReceipt {
	t.Helper()
	receipt, err := NewServerReceipt(request, response, requestID)
	if err != nil {
		t.Fatalf("NewServerReceipt() error = %v", err)
	}
	return receipt
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
