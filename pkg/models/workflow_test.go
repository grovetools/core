package models

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestWorkflowEventJSONRoundTrip(t *testing.T) {
	ts := time.Date(2026, 6, 10, 17, 7, 14, 0, time.UTC)

	t.Run("full event", func(t *testing.T) {
		ev := WorkflowEvent{
			Kind:            WorkflowAgentCompleted,
			JobID:           "20260610-174747-workflow-subagent-deep-integration",
			ClaudeSessionID: "6c1e876f-0000-0000-0000-000000000000",
			RunID:           "wf_d2a7bbf5-710",
			AgentID:         "ad48c96",
			AgentType:       "workflow-subagent",
			TranscriptPath:  "/home/u/.claude/projects/slug/6c1e876f/subagents/workflows/wf_d2a7bbf5-710/agent-ad48c96.jsonl",
			Prompt:          "do the thing",
			Phase:           "Phase 1",
			ResultSummary:   "ok",
			LastMessage:     "done.",
			Timestamp:       ts,
			Source:          WorkflowSourceHooks,
		}

		data, err := json.Marshal(ev)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		var got WorkflowEvent
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !reflect.DeepEqual(ev, got) {
			t.Errorf("round-trip mismatch:\n  in:  %+v\n  out: %+v", ev, got)
		}
	})

	t.Run("minimal ad-hoc agent event omits enrichment fields", func(t *testing.T) {
		// An Agent-tool spawn has no RunID and (at start) no enrichment.
		ev := WorkflowEvent{
			Kind:            WorkflowAgentStarted,
			JobID:           "job-1",
			ClaudeSessionID: "sess-1",
			AgentID:         "a1",
			Timestamp:       ts,
			Source:          WorkflowSourceHooks,
		}

		data, err := json.Marshal(ev)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}

		for _, key := range []string{
			"run_id", "agent_type", "transcript_path", "prompt",
			"phase", "result_summary", "last_message",
		} {
			if strings.Contains(string(data), `"`+key+`"`) {
				t.Errorf("expected %q to be omitted from minimal event, got: %s", key, data)
			}
		}

		var got WorkflowEvent
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if !reflect.DeepEqual(ev, got) {
			t.Errorf("round-trip mismatch:\n  in:  %+v\n  out: %+v", ev, got)
		}
	})

	t.Run("kind constants", func(t *testing.T) {
		want := map[WorkflowKind]string{
			WorkflowRunDiscovered:  "run_discovered",
			WorkflowAgentStarted:   "agent_started",
			WorkflowAgentCompleted: "agent_completed",
			WorkflowRunStale:       "run_stale",
		}
		for k, s := range want {
			if string(k) != s {
				t.Errorf("kind %v = %q, want %q", k, string(k), s)
			}
		}
	})
}

func TestWorkflowRunStateJSONRoundTrip(t *testing.T) {
	ts := time.Date(2026, 6, 10, 17, 9, 0, 0, time.UTC)
	state := WorkflowRunState{
		RunID:           "wf_d2a7bbf5-710",
		JobID:           "job-1",
		ClaudeSessionID: "sess-1",
		Name:            "p0-hook-probe",
		Phases:          []string{"Phase 1", "Phase 2"},
		Agents: map[string]*Subagent{
			"a1": {
				ID:              "a1",
				ParentSessionID: "sess-1",
				StartedAt:       ts.Add(-time.Minute),
				CompletedAt:     ts,
				Status:          "completed",
				Success:         true,
			},
		},
		StartedCount:   2,
		CompletedCount: 1,
		Stale:          false,
		UpdatedAt:      ts,
	}

	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got WorkflowRunState
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(state, got) {
		t.Errorf("round-trip mismatch:\n  in:  %+v\n  out: %+v", state, got)
	}
}
