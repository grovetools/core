package mux

import "os"

// The concrete agent routing targets. "auto" is not one of them: by the time an
// agent job is launched it may be running inside groved, which inherited none of
// the submitting terminal's environment and therefore cannot answer "auto".
// Every executor refuses an empty target rather than guess (flow's
// interactive_agent_executor.go / isolated_agent_executor.go), so resolution has
// to happen at the submitting process's perimeter — that is what this file is.
//
// The perimeter is wider than any one repo: every process that builds a
// models.JobSubmitRequest answers this question, and models lives here in core.
// The derivation was copy-pasted per submitter before it lived here, and the
// submitter that got missed (`flow plan retry --run`) shipped jobs that died on
// arrival. A second copy of the table below is how that happens again.
const (
	AgentTargetTmux   = "tmux"
	AgentTargetNative = "native"
	AgentTargetTuimux = "tuimux"
)

// ResolveAgentTarget derives the routing target for jobs submitted from this
// process, from the caller's own environment. Every submission path must go
// through it: flow's `plan run`, `plan resume` and `plan retry --run`, and
// grove.nvim's `chat`, all launch the same agents into the same mux, and a path
// that skips the derivation submits a job the executor can only fail.
func ResolveAgentTarget() string {
	return agentTargetFor(ActiveMux(), os.Getenv(EnvGroveTerminal) != "")
}

// ResolveAgentTargetHosted is the TUI's front door to the same derivation.
// hosted is set by the terminal panel wrapper that constructed the TUI, so it is
// authoritative and deliberately consulted instead of GROVE_TERMINAL — that
// variable is exported to every process a grove terminal spawns, including ones
// that are not hosted panes, so sniffing it would claim native routing for TUIs
// that have no pane to launch into.
func ResolveAgentTargetHosted(hosted bool) string {
	if hosted {
		return AgentTargetNative
	}
	return agentTargetFor(ActiveMux(), false)
}

// agentTargetFor is the precedence table itself, kept pure so it is testable
// without mutating process environment. tuimux outranks a grove terminal because
// a tuimux pane sets both markers (tuimux exports GROVE_TERMINAL for the editors
// it hosts) and the tuimux daemon is the one that actually owns the PTY. tmux is
// the fallback because it is the only target that works from a bare shell.
func agentTargetFor(active MuxType, groveTerminal bool) string {
	switch {
	case active == MuxTuimux:
		return AgentTargetTuimux
	case groveTerminal:
		return AgentTargetNative
	default:
		return AgentTargetTmux
	}
}
