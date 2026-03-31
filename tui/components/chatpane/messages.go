package chatpane

// InputSubmittedMsg is sent when the user presses Enter with non-empty input.
type InputSubmittedMsg struct {
	Text string
}
