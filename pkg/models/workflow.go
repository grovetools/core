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
	// WorkflowRunCompleted marks a run that reached a clean terminal state:
	// the owning session ended (the real terminal trigger) with every started
	// agent having a recorded result. Distinct from RunStale, which is
	// reserved for session-ended-with-stragglers.
	WorkflowRunCompleted WorkflowKind = "run_completed"
	// WorkflowChildrenSnapshot carries a point-in-time live-background-child
	// count (LiveChildren) for a session, forwarded from the hook SubagentStop
	// background_tasks/session_crons snapshot. Unlike the other kinds it is
	// NOT a per-agent lifecycle delta and mints no agent/run rows: the daemon
	// only writes ev.LiveChildren onto the owning session and never persists
	// the event (no boot replay). When hook-sourced it also carries
	// LiveBashChildren — the authoritative live set of background *bash* jobs
	// at that turn boundary — which the daemon reconciles into its bash-child
	// bookkeeping (start new, clear absent) so "N bg" and the indented line
	// cover bash too (F6).
	WorkflowChildrenSnapshot WorkflowKind = "children_snapshot"
	// WorkflowBashStarted announces a background bash job spawn, forwarded from
	// the PostToolUse hook for a Bash tool whose tool_response carries a
	// backgroundTaskId. AgentID is that id; Name is the command (the render
	// title). This is the ONLY liveness signal for a session with NO subagent
	// (which never fires SubagentStop), so bash there is visible from spawn and
	// cleared only by the TTL floor — the accepted F6 reliability bound. It is
	// never persisted (bash is ephemeral; a replayed start would fake liveness).
	WorkflowBashStarted WorkflowKind = "bash_started"
)

// BashChildRef identifies one live background bash job in a children snapshot:
// its harness id (background_tasks[].id == the PostToolUse backgroundTaskId)
// and command (render title). Carried on WorkflowChildrenSnapshot events built
// from a hook SubagentStop, whose background_tasks[] view is authoritative.
type BashChildRef struct {
	ID      string `json:"id"`
	Command string `json:"command,omitempty"`
}

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
	LastMessage string `json:"last_message,omitempty"`
	// LiveChildren is the live-background-child count carried by a
	// WorkflowChildrenSnapshot event (only meaningful for that kind). It is
	// omitempty: a zero-count snapshot serializes without the field and decodes
	// to 0 on the daemon, which assigns session.LiveChildren unconditionally so
	// a zero snapshot correctly clears idle-busy.
	LiveChildren int       `json:"live_children,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
	// Source is WorkflowSourceHooks or WorkflowSourceJournal.
	Source string `json:"source"`
	// LiveBashChildren is the authoritative set of live background bash jobs a
	// session owns at a turn boundary, set by hooks on a WorkflowChildrenSnapshot
	// event from the SubagentStop background_tasks[] type=="shell" entries. The
	// daemon treats a hook-sourced snapshot as authoritative: it starts/refreshes
	// each listed bash child and marks any tracked bash child absent from the list
	// completed. nil on daemon-derived snapshots (which never touch bash state).
	LiveBashChildren []BashChildRef `json:"live_bash_children,omitempty"`
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
	Stale bool `json:"stale"`
	// Completed is set when the run reached a clean terminal state: the
	// owning session ended with every started agent having a result. Distinct
	// from Stale (session ended with stragglers).
	Completed bool      `json:"completed"`
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
