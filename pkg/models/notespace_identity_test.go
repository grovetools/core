package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPlanBundleAndNoteEventUseIDKeyedNotespaceFields(t *testing.T) {
	values := []any{
		PlanBundle{NotespaceID: "id-1", NotespaceName: "display", PlanName: "plan"},
		NoteEvent{Event: NoteEventMoved, NotespaceID: "id-1", NotespaceName: "display", PrevNotespaceID: "id-0", PrevNotespaceName: "old-display", Path: "plans/x.md"},
		DocumentPathInfo{ID: "doc", NotespaceID: "id-1", NotespaceName: "display"},
	}
	for _, value := range values {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), `"notespace_id"`) {
			t.Fatalf("notespace id missing from %T: %s", value, data)
		}
		if strings.Contains(string(data), `"workspace"`) || strings.Contains(string(data), `"prev_workspace"`) {
			t.Fatalf("legacy name-keyed identity leaked from %T: %s", value, data)
		}
	}
}
