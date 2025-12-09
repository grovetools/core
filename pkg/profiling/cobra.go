package profiling

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"runtime/pprof"

	"github.com/spf13/cobra"
)

// CobraProfiler encapsulates profiling state and flag management for Cobra apps.
type CobraProfiler struct {
	cpuProfileFile *os.File
	cpuProfilePath string
	memProfilePath string
	timing         bool
}

// NewCobraProfiler creates a new profiler for Cobra integration.
func NewCobraProfiler() *CobraProfiler {
	return &CobraProfiler{}
}

// AddFlags adds the profiling flags to the given Cobra command.
func (p *CobraProfiler) AddFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&p.cpuProfilePath, "cpu-profile", "", "Write CPU profile to file")
	cmd.PersistentFlags().StringVar(&p.memProfilePath, "mem-profile", "", "Write memory profile to file")
	cmd.PersistentFlags().BoolVar(&p.timing, "timing", false, "Print hierarchical timing summary on exit")
}

// PreRun is intended to be used as a Cobra PersistentPreRunE hook.
// It initializes profiling based on the flags provided.
func (p *CobraProfiler) PreRun(cmd *cobra.Command, args []string) error {
	if p.timing {
		Enable()
	}

	if p.cpuProfilePath != "" {
		f, err := os.Create(p.cpuProfilePath)
		if err != nil {
			return fmt.Errorf("could not create CPU profile: %w", err)
		}
		p.cpuProfileFile = f
		if err := pprof.StartCPUProfile(p.cpuProfileFile); err != nil {
			return fmt.Errorf("could not start CPU profile: %w", err)
		}
	}
	return nil
}

// PostRun is intended to be used as a Cobra PersistentPostRun hook.
// It finalizes profiling, writing files and printing summaries.
func (p *CobraProfiler) PostRun(cmd *cobra.Command, args []string) {
	if p.cpuProfileFile != nil {
		pprof.StopCPUProfile()
		p.cpuProfileFile.Close()
		fmt.Printf("CPU profile written to %s\n", p.cpuProfilePath)
	}

	if p.memProfilePath != "" {
		f, err := os.Create(p.memProfilePath)
		if err != nil {
			log.Printf("could not create memory profile: %v", err)
			return
		}
		defer f.Close()
		runtime.GC() // get up-to-date statistics
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Printf("could not write memory profile: %v", err)
		}
		fmt.Printf("Memory profile written to %s\n", p.memProfilePath)
	}

	if p.timing {
		Summarize(os.Stderr)
	}
}
