package profiling

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

// Stopper is an interface for stopping a timed span.
type Stopper interface {
	Stop()
}

// span represents a single timed operation in the hierarchy.
type span struct {
	name      string
	start     time.Time
	duration  time.Duration
	children  []*span
	mu        sync.Mutex
	profiler *Profiler
}

// Stop completes the timing for this span.
func (s *span) Stop() {
	s.duration = time.Since(s.start)
	s.profiler.endSpan(s)
}

// Profiler manages a profiling session with nested timing spans.
type Profiler struct {
	enabled   bool
	mu        sync.Mutex
	root      *span
	spanStack []*span
}

var defaultProfiler = &Profiler{}

// Enable turns on the global profiler.
func Enable() {
	defaultProfiler.mu.Lock()
	defer defaultProfiler.mu.Unlock()

	if defaultProfiler.enabled {
		return
	}

	defaultProfiler.enabled = true
	defaultProfiler.root = &span{name: "root", start: time.Now(), profiler: defaultProfiler}
	defaultProfiler.spanStack = []*span{defaultProfiler.root}
}

// Start begins a new timed span with the given name.
// It returns a Stopper which must be used to end the span, typically via defer.
func Start(name string) Stopper {
	if !defaultProfiler.enabled {
		return noopStopper{}
	}
	return defaultProfiler.startSpan(name)
}

// Summarize prints a formatted, hierarchical summary of all timed spans to the writer.
func Summarize(w io.Writer) {
	defaultProfiler.mu.Lock()
	defer defaultProfiler.mu.Unlock()

	if !defaultProfiler.enabled || defaultProfiler.root == nil {
		return
	}

	// Ensure the root span is stopped to calculate total duration
	if defaultProfiler.root.duration == 0 {
		defaultProfiler.root.duration = time.Since(defaultProfiler.root.start)
	}

	fmt.Fprintln(w, "\n--- Timing Profile ---")
	printSpan(w, defaultProfiler.root, 0, defaultProfiler.root.duration)
	fmt.Fprintln(w, "--------------------")
}

func (p *Profiler) startSpan(name string) Stopper {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.enabled {
		return noopStopper{}
	}

	parent := p.spanStack[len(p.spanStack)-1]
	newSpan := &span{name: name, start: time.Now(), profiler: p}

	parent.mu.Lock()
	parent.children = append(parent.children, newSpan)
	parent.mu.Unlock()

	p.spanStack = append(p.spanStack, newSpan)
	return newSpan
}

func (p *Profiler) endSpan(s *span) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.enabled || len(p.spanStack) <= 1 {
		return
	}

	// Pop from stack
	p.spanStack = p.spanStack[:len(p.spanStack)-1]
}

// printSpan is a recursive helper to print the span tree.
func printSpan(w io.Writer, s *span, depth int, totalDuration time.Duration) {
	indent := strings.Repeat("  ", depth)
	percentage := 0.0
	if totalDuration > 0 {
		percentage = (float64(s.duration) / float64(totalDuration)) * 100
	}

	// Don't print the root span itself, just its children
	if s.name != "root" {
		fmt.Fprintf(w, "%s- %s (%v, %.1f%%)\n", indent, s.name, s.duration.Round(time.Microsecond*100), percentage)
	}

	// Sort children by start time to maintain call order
	sort.Slice(s.children, func(i, j int) bool {
		return s.children[i].start.Before(s.children[j].start)
	})

	for _, child := range s.children {
		printSpan(w, child, depth+1, totalDuration)
	}
}

// noopStopper is used when the profiler is disabled.
type noopStopper struct{}

func (s noopStopper) Stop() {}
