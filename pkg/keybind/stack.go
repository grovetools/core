package keybind

import (
	"context"
	"sync"
)

// BuildStack runs all collectors concurrently and aggregates results into a Stack.
func BuildStack(ctx context.Context, collectors ...Collector) (*Stack, error) {
	stack := NewStack()

	if len(collectors) == 0 {
		return stack, nil
	}

	// Run collectors concurrently
	var wg sync.WaitGroup
	results := make(chan CollectorResult, len(collectors))

	for _, c := range collectors {
		wg.Add(1)
		go func(collector Collector) {
			defer wg.Done()
			bindings, err := collector.Collect(ctx)
			results <- CollectorResult{
				Collector: collector,
				Bindings:  bindings,
				Error:     err,
			}
		}(c)
	}

	// Close results channel when all collectors are done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Aggregate results
	var errs []error
	for result := range results {
		if result.Error != nil {
			errs = append(errs, result.Error)
			continue
		}
		for _, b := range result.Bindings {
			stack.AddBinding(b)
		}
	}

	// Return first error if any (could enhance to return multi-error)
	if len(errs) > 0 {
		return stack, errs[0]
	}

	return stack, nil
}

// BuildStackWithOptions allows customizing stack building behavior.
type BuildStackOptions struct {
	// ContinueOnError continues collecting even if some collectors fail.
	ContinueOnError bool
	// Layers limits collection to specific layers.
	Layers []Layer
}

// BuildStackWithOpts builds a stack with options.
func BuildStackWithOpts(ctx context.Context, opts BuildStackOptions, collectors ...Collector) (*Stack, []error) {
	stack := NewStack()

	if len(collectors) == 0 {
		return stack, nil
	}

	// Filter collectors by layer if specified
	filteredCollectors := collectors
	if len(opts.Layers) > 0 {
		layerSet := make(map[Layer]bool)
		for _, l := range opts.Layers {
			layerSet[l] = true
		}
		filteredCollectors = make([]Collector, 0)
		for _, c := range collectors {
			if layerSet[c.Layer()] {
				filteredCollectors = append(filteredCollectors, c)
			}
		}
	}

	if len(filteredCollectors) == 0 {
		return stack, nil
	}

	// Run collectors concurrently
	var wg sync.WaitGroup
	results := make(chan CollectorResult, len(filteredCollectors))

	for _, c := range filteredCollectors {
		wg.Add(1)
		go func(collector Collector) {
			defer wg.Done()
			bindings, err := collector.Collect(ctx)
			results <- CollectorResult{
				Collector: collector,
				Bindings:  bindings,
				Error:     err,
			}
		}(c)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	// Aggregate results
	var errs []error
	for result := range results {
		if result.Error != nil {
			errs = append(errs, result.Error)
			if !opts.ContinueOnError {
				continue
			}
		}
		for _, b := range result.Bindings {
			stack.AddBinding(b)
		}
	}

	return stack, errs
}

// DefaultCollectors returns the standard set of collectors for the current environment.
// This is a convenience function that creates collectors for all available layers.
func DefaultCollectors(ctx context.Context) []Collector {
	var collectors []Collector

	// Add collectors based on what's available in the environment
	// These are added in the order they should be checked (L0 to L6)

	// L0: macOS collector (if on macOS)
	if macos := NewMacOSCollector(); macos != nil {
		collectors = append(collectors, macos)
	}

	// L2: Shell collector (detect current shell)
	shell := DetectShell()
	switch shell {
	case "fish":
		collectors = append(collectors, NewFishCollector())
	case "zsh":
		collectors = append(collectors, NewZshCollector())
	case "bash":
		collectors = append(collectors, NewBashCollector())
	}

	// L3-L5: Tmux collectors (if tmux is available)
	if IsTmuxAvailable() {
		collectors = append(collectors, NewTmuxRootCollector())
		collectors = append(collectors, NewTmuxPrefixCollector())
		collectors = append(collectors, NewTmuxCustomCollector())
	}

	// L5: Grove collector (reads from config)
	collectors = append(collectors, NewGroveCollector())

	// L6: Neovim collector (if nvim is available)
	if IsNeovimAvailable() {
		collectors = append(collectors, NewNeovimCollector())
	}

	return collectors
}

// IsTmuxAvailable checks if tmux is available and we're in a tmux session.
func IsTmuxAvailable() bool {
	// Implementation will check for tmux command and TMUX env var
	// Placeholder for now, actual implementation in tmux.go
	return tmuxAvailable
}

// tmuxAvailable is set by tmux.go during init
var tmuxAvailable = false
