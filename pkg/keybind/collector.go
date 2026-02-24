package keybind

import (
	"context"
)

// Collector gathers key bindings from a specific source.
type Collector interface {
	// Name returns the collector's identifier (e.g., "fish", "tmux-root").
	Name() string
	// Layer returns the layer this collector operates on.
	Layer() Layer
	// Collect gathers bindings from the source.
	Collect(ctx context.Context) ([]Binding, error)
}

// CollectorFunc adapts a function to the Collector interface.
type CollectorFunc struct {
	name    string
	layer   Layer
	collect func(ctx context.Context) ([]Binding, error)
}

// NewCollectorFunc creates a new function-based collector.
func NewCollectorFunc(name string, layer Layer, collect func(ctx context.Context) ([]Binding, error)) *CollectorFunc {
	return &CollectorFunc{
		name:    name,
		layer:   layer,
		collect: collect,
	}
}

func (c *CollectorFunc) Name() string {
	return c.name
}

func (c *CollectorFunc) Layer() Layer {
	return c.layer
}

func (c *CollectorFunc) Collect(ctx context.Context) ([]Binding, error) {
	return c.collect(ctx)
}

// MultiCollector wraps multiple collectors into one.
type MultiCollector struct {
	collectors []Collector
}

// NewMultiCollector creates a collector that runs multiple collectors.
func NewMultiCollector(collectors ...Collector) *MultiCollector {
	return &MultiCollector{collectors: collectors}
}

// Add adds a collector to the multi-collector.
func (m *MultiCollector) Add(c Collector) {
	m.collectors = append(m.collectors, c)
}

// Collectors returns all wrapped collectors.
func (m *MultiCollector) Collectors() []Collector {
	return m.collectors
}

// CollectorResult holds the result of a single collector's run.
type CollectorResult struct {
	Collector Collector
	Bindings  []Binding
	Error     error
}
