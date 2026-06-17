package models

import "time"

// Workflow / subagent activity types.
//
// Three related schemas exist on purpose — they serve different layers and
// must not be collapsed:
//
//   - WorkflowEvent (this file): the EPHEMERAL WIRE PROTOCOL. Hooks (and the
//     daemon's journal watcher) publish these to groved via
//     daemon.Client.PublishWorkflowEvent → POST /api/workflows/event. Events
//     are deltas; they are merged/deduped by (RunID, AgentID) on the daemon
//     and are not themselves a durable record.
//   - Subagent (tool.go): the DURABLE PER-AGENT record, surfaced on
//     Session.Subagents and inside WorkflowRunState.Agents. The daemon folds
//     WorkflowEvents into it: agent_started → StartedAt; agent_completed →
//     CompletedAt/Status/ResultSummary.
//   - Event (event.go): the generic LOCAL JSONL FALLBACK format that grove
//     hooks append to events.jsonl when no daemon is reachable. It carries an
//     opaque Data payload and is parsed best-effort.
//
// Field-shape provenance: the Claude Code hook payloads these events are
// built from were empirically probed on CC v2.1.172 (see the hook-probe
// results note, 2026-06-10). Notably:
//   - AgentType discriminates spawn sources: workflow-spawned agents carry
//     "workflow-subagent"; Agent-tool spawns carry the subagent type name
//     (e.g. "Explore").
//   - RunID is only attributable at SubagentStop (from the wf_<runId> dir
//     embedded in agent_transcript_path, or background_tasks[].id) or from
//     the journal. It is empty for ad-hoc Agent-tool spawns — run
//     attribution is enrichment, not a precondition.
//   - Rich fields (LastMessage, TranscriptPath, …) are not guaranteed on
//     every payload variant; every enrichment field here is omitempty and
//     consumers must tolerate absence.

// WorkflowKind identifies the lifecycle transition a WorkflowEvent describes.
type WorkflowKind string

const (
	// WorkflowRunDiscovered announces a new workflow run (journal-sourced).
	WorkflowRunDiscovered WorkflowKind = "run_discovered"
	// WorkflowAgentStarted announces a subagent spawn (SubagentStart hook or
	// journal "started" entry). For Agent-tool spawns RunID is empty.
	WorkflowAgentStarted WorkflowKind = "agent_started"
	// WorkflowAgentCompleted announces a subagent finishing (SubagentStop
	// hook or journal result entry).
	WorkflowAgentCompleted WorkflowKind = "agent_completed"
	// WorkflowRunStale marks a run whose session died or whose journal went
	// quiet past the staleness threshold.
	WorkflowRunStale WorkflowKind = "run_stale"
)

// WorkflowEvent source values.
const (
	// WorkflowSourceHooks marks events produced by grove hooks from Claude
	// Code hook payloads (lifecycle authority: timestamps, transcript paths).
	WorkflowSourceHooks = "hooks"
	// WorkflowSourceJournal marks events produced by tailing the session's
	// workflow journal (attribution/enrichment: run ids, phases, results).
	WorkflowSourceJournal = "journal"
)

// WorkflowEvent is the ephemeral wire protocol for subagent/workflow
// lifecycle deltas flowing from producers (hooks, journal watcher) to the
// daemon. See the package comment block above for how it relates to the
// durable Subagent record and the local-fallback Event.
type WorkflowEvent struct {
	Kind            WorkflowKind `json:"kind"`
	JobID           string       `json:"job_id"`
	ClaudeSessionID string       `json:"claude_session_id"`
	// RunID is the workflow run id (wf_… directory name). Empty for ad-hoc
	// Agent-tool spawns, which have no workflow run.
	RunID   string `json:"run_id,omitempty"`
	AgentID string `json:"agent_id"`
	// AgentType discriminates spawn sources: "workflow-subagent" for
	// workflow-spawned agents, or the subagent type name (e.g. "Explore")
	// for Agent-tool spawns. Optional on older CC payload variants.
	AgentType string `json:"agent_type,omitempty"`
	// Name is the descriptive agent name, populated from agent-<id>.meta.json
	// "description" field for simple/ad-hoc subagents, or from the static
	// script "label:" for workflow subagents. Best-effort enrichment.
	Name string `json:"name,omitempty"`
	// TranscriptPath is the per-agent transcript jsonl path
	// (agent_transcript_path; only available at SubagentStop).
	TranscriptPath string `json:"transcript_path,omitempty"`
	// Prompt is the spawn prompt (Agent-tool spawns only, via PreToolUse
	// tool_input.prompt; workflow spawn prompts require journal enrichment).
	Prompt string `json:"prompt,omitempty"`
	// Phase is the workflow phase title (journal enrichment only).
	Phase string `json:"phase,omitempty"`
	// WorkflowName is the workflow name (from background_tasks[].name at
	// SubagentStop, or the journal's run header). Optional enrichment.
	WorkflowName string `json:"workflow_name,omitempty"`
	// ResultSummary is a short structured-result summary (journal enrichment).
	ResultSummary string `json:"result_summary,omitempty"`
	// LastMessage is the agent's final assistant text
	// (last_assistant_message at SubagentStop; not guaranteed).
	LastMessage string    `json:"last_message,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
	// Source is WorkflowSourceHooks or WorkflowSourceJournal.
	Source string `json:"source"`
}

// WorkflowRunState is the aggregated snapshot of a single workflow run as
// served by GET /api/workflows. The daemon folds WorkflowEvents into this
// shape, deduping by (RunID, AgentID); Agents reuses the durable Subagent
// record (keyed by agent id) so the same per-agent state also populates
// Session.Subagents with zero extra schema.
type WorkflowRunState struct {
	RunID           string `json:"run_id"`
	JobID           string `json:"job_id"`
	ClaudeSessionID string `json:"claude_session_id"`
	// Name is the workflow name (from background_tasks[].name or journal).
	Name string `json:"name,omitempty"`
	// Phases lists phase titles in journal order (journal enrichment).
	Phases []string `json:"phases,omitempty"`
	// Agents maps agent id → durable per-agent record. Event mapping:
	// agent_started → StartedAt; agent_completed → CompletedAt, Status,
	// ResultSummary.
	Agents         map[string]*Subagent `json:"agents,omitempty"`
	StartedCount   int                  `json:"started_count"`
	CompletedCount int                  `json:"completed_count"`
	// Stale is set when the owning session died or the journal went quiet
	// past the staleness threshold (never decided by the PID reaper).
	Stale     bool      `json:"stale"`
	UpdatedAt time.Time `json:"updated_at"`
}

// WorkflowSnapshot is the GET /api/workflows response: aggregated run state
// keyed by run ID, plus run-less subagents (ad-hoc Agent-tool spawns and
// not-yet-attributed workflow agents) keyed by session key (job ID when
// stamped, else the claude session ID). Consumers reconcile against this
// snapshot since the workflow_* SSE broadcast is lossy-by-design.
type WorkflowSnapshot struct {
	Runs  map[string]*WorkflowRunState    `json:"runs"`
	Adhoc map[string]map[string]*Subagent `json:"adhoc,omitempty"`
}
