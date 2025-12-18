package logviewer

import (
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// StreamWriter implements io.Writer and sends complete log lines as messages to a TUI program.
// It buffers partial lines until a newline is encountered, ensuring that log lines are not split
// in the middle when streaming output.
type StreamWriter struct {
	program           *tea.Program
	workspace         string
	buffer            strings.Builder
	mu                sync.Mutex
	NoWorkspacePrefix bool // If true, LogLineMsg will have NoPrefix set
}

// NewStreamWriter creates a new StreamWriter that sends log lines to the given TUI program.
// The workspace parameter is used to tag log lines with their source.
func NewStreamWriter(program *tea.Program, workspace string) *StreamWriter {
	return &StreamWriter{
		program:   program,
		workspace: workspace,
	}
}

// Write implements io.Writer. It buffers incoming data and sends complete lines
// (terminated by newline) as LogLineMsg to the TUI program.
func (w *StreamWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Write new data to the buffer
	w.buffer.Write(p)

	// Process all complete lines from the buffer
	content := w.buffer.String()
	lines := strings.Split(content, "\n")

	// Keep the last incomplete line in the buffer
	if len(lines) > 0 {
		w.buffer.Reset()
		lastLine := lines[len(lines)-1]
		w.buffer.WriteString(lastLine)

		// Send all complete lines (all but the last)
		for i := 0; i < len(lines)-1; i++ {
			if w.program != nil {
				w.program.Send(LogLineMsg{
					Workspace: w.workspace,
					Line:      lines[i],
					NoPrefix:  w.NoWorkspacePrefix,
				})
			}
		}
	}

	return len(p), nil
}
