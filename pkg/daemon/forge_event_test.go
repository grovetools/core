package daemon

import (
	"encoding/json"
	"testing"

	"github.com/grovetools/core/pkg/models"
)

// TestParseForgeStateFromGenericPayload covers the real SSE path: the frame has
// been through JSON, so Payload arrives as a generic map.
func TestParseForgeStateFromGenericPayload(t *testing.T) {
	data, err := json.Marshal(models.ForgeStatePayload{
		Repos: []models.ForgeRepoState{{
			Provider: "github",
			Repo:     "github.com/grovetools/nav",
			State:    models.ForgeStateStale,
		}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var generic map[string]any
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	p, ok := ParseForgeState(StateUpdate{UpdateType: UpdateTypeForgeState, Payload: generic})
	if !ok {
		t.Fatal("ParseForgeState refused a generic payload")
	}
	if len(p.Repos) != 1 || p.Repos[0].State != models.ForgeStateStale {
		t.Fatalf("decoded %+v, want one stale repo", p.Repos)
	}
}

// TestParseForgeStateFromTypedPayload covers in-process clients and tests,
// which put the struct on the update directly.
func TestParseForgeStateFromTypedPayload(t *testing.T) {
	payload := &models.ForgeStatePayload{Repos: []models.ForgeRepoState{{Repo: "r"}}}
	got, ok := ParseForgeState(StateUpdate{UpdateType: UpdateTypeForgeState, Payload: payload})
	if !ok || got != payload {
		t.Fatalf("typed payload not passed through: got %v ok=%v", got, ok)
	}

	byValue, ok := ParseForgeState(StateUpdate{UpdateType: UpdateTypeForgeState, Payload: *payload})
	if !ok || len(byValue.Repos) != 1 {
		t.Fatalf("value payload not decoded: got %v ok=%v", byValue, ok)
	}
}

// TestParseForgeStateEmptyFrameIsStillAFrame: a sweep where nothing changed
// still decodes. The caller decides what that means — and it never means "there
// are no pull requests".
func TestParseForgeStateEmptyFrameIsStillAFrame(t *testing.T) {
	p, ok := ParseForgeState(StateUpdate{UpdateType: UpdateTypeForgeState, Payload: map[string]any{}})
	if !ok {
		t.Fatal("an empty forge_state frame should still decode")
	}
	if len(p.Repos) != 0 {
		t.Fatalf("expected no repos, got %+v", p.Repos)
	}
}

// TestParseForgeStateNegative keeps the helper from claiming frames it does not
// own, so a caller can use its bool as an early-return gate.
func TestParseForgeStateNegative(t *testing.T) {
	for _, ut := range []string{"", "initial", "workspaces_delta", UpdateTypeThemeChanged} {
		if _, ok := ParseForgeState(StateUpdate{UpdateType: ut}); ok {
			t.Errorf("ParseForgeState claimed a %q frame", ut)
		}
	}
	if _, ok := ParseForgeState(StateUpdate{UpdateType: UpdateTypeForgeState, Payload: (*models.ForgeStatePayload)(nil)}); ok {
		t.Error("a nil typed payload should be refused")
	}
}
