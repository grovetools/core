package embed

// OutlineOffer describes an agent transcript a hosted panel can hand its host
// to outline: enough identity to name the pin (JobID, Title) and enough source
// to build it (TranscriptPath, Provider, the marker directories). The host
// prefers its own live view of the agent when it has one — a daemon session
// under the same JobID — and falls back to reading TranscriptPath, which is
// how a finished or archived job's outline is served after the agent is gone.
type OutlineOffer struct {
	JobID            string
	Title            string
	Provider         string
	TranscriptPath   string
	WorkingDirectory string
	JobFilePath      string
	PlanDirectory    string
}

// OutlineOfferer is implemented by hosted panels that can answer "what would
// an outline of the thing under your cursor be?" — the host's pin-outline
// chord asks the focused panel this when the panel is not itself an agent
// pane. ok=false means the current selection has nothing to outline (no job
// under the cursor, or no transcript recorded for it).
type OutlineOfferer interface {
	OutlineOffer() (OutlineOffer, bool)
}
