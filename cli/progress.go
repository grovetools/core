package cli

import (
	"fmt"
	"sync"
	"time"
)

// ProgressReporter reports concurrent operation progress
type ProgressReporter struct {
	mu       sync.Mutex
	statuses map[string]string
	start    time.Time
}

// NewProgressReporter creates a new progress reporter
func NewProgressReporter() *ProgressReporter {
	return &ProgressReporter{
		statuses: make(map[string]string),
		start:    time.Now(),
	}
}

// Update updates the status of a service
func (p *ProgressReporter) Update(service, status string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.statuses[service] = status
	p.render()
}

// render displays the current progress
func (p *ProgressReporter) render() {
	// Clear previous output (simplified for example)
	fmt.Print("\033[H\033[2J")

	elapsed := time.Since(p.start).Round(time.Second)
	fmt.Printf("Grove operation in progress... [%s]\n\n", elapsed)

	for service, status := range p.statuses {
		symbol := "[.]"
		switch status {
		case "completed":
			symbol = "[*]"
		case "failed":
			symbol = "[x]"
		case "starting":
			symbol = "[~]"
		}

		fmt.Printf("%s %s: %s\n", symbol, service, status)
	}
}

// Done marks the operation as complete
func (p *ProgressReporter) Done() {
	p.mu.Lock()
	defer p.mu.Unlock()

	elapsed := time.Since(p.start).Round(time.Millisecond)
	fmt.Printf("\nOperation completed in %s\n", elapsed)
}