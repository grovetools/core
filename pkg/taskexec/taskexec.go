// Package taskexec runs a single build/task command with the process-group
// semantics shared by grove's orchestrator and the daemon's machine-wide
// build queue: the command runs in its own process group, cancellation
// SIGTERMs the whole group (so make's children get a chance to run their
// cleanup), and combined stdout/stderr is streamed line-by-line to a
// callback while also being accumulated for the final result.
//
// The behavior here was lifted verbatim from grove/pkg/orchestrator's
// runner and must not drift: grove's local worker pool and the daemon's
// build queue both execute through this package so a job behaves
// identically whichever side runs it.
package taskexec

import (
	"bufio"
	"bytes"
	"context"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Options configures a single task command execution.
type Options struct {
	// Command is the explicit command to run. Empty means `make <Verb>`.
	Command []string
	// Verb is the make target used when Command is empty.
	Verb string
	// Dir is the working directory for the process.
	Dir string
	// Env is the complete environment for the process. It is used as-is,
	// not merged with the current process environment.
	Env []string
	// OnOutput, if non-nil, is invoked for every line of combined
	// stdout/stderr as it is produced. It is called from a dedicated
	// goroutine; blocking here applies backpressure to the process pipes.
	OnOutput func(line string)
}

// Task is a single prepared task command. Create with New, then Start and
// Wait — mirroring exec.Cmd so callers can distinguish "never started"
// from "ran and failed".
type Task struct {
	cmd       *exec.Cmd
	outputBuf bytes.Buffer
	streamWg  sync.WaitGroup
}

// New prepares (but does not start) a task command bound to ctx. When ctx
// is cancelled the entire process group receives SIGTERM; if the process
// still hasn't exited after 10s, exec's WaitDelay force-kills it.
func New(ctx context.Context, opts Options) *Task {
	var cmd *exec.Cmd
	if len(opts.Command) == 0 {
		cmd = exec.CommandContext(ctx, "make", opts.Verb)
	} else {
		cmd = exec.CommandContext(ctx, opts.Command[0], opts.Command[1:]...) //nolint:gosec // commands from trusted build config
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		// Kill the entire process group so the command's children (tend
		// run -p, etc.) also receive SIGTERM and can run their cleanup.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	cmd.WaitDelay = 10 * time.Second
	cmd.Dir = opts.Dir
	cmd.Env = opts.Env

	t := &Task{cmd: cmd}

	stdoutPipe, _ := cmd.StdoutPipe()
	cmd.Stderr = cmd.Stdout

	scanner := bufio.NewScanner(stdoutPipe)
	t.streamWg.Add(1)
	go func() {
		defer t.streamWg.Done()
		for scanner.Scan() {
			line := scanner.Text()
			t.outputBuf.WriteString(line + "\n")
			if opts.OnOutput != nil {
				opts.OnOutput(line)
			}
		}
	}()

	return t
}

// Start launches the process. On error the process never ran and Wait
// must not be called.
func (t *Task) Start() error {
	return t.cmd.Start()
}

// Wait blocks until the output stream drains and the process exits,
// returning the accumulated combined output and the process's wait error.
func (t *Task) Wait() ([]byte, error) {
	t.streamWg.Wait()
	err := t.cmd.Wait()
	return t.outputBuf.Bytes(), err
}

// Run is a convenience wrapper for New + Start + Wait. A start failure is
// returned with nil output; callers that must distinguish start failures
// from run failures should use the Task API directly.
func Run(ctx context.Context, opts Options) ([]byte, error) {
	t := New(ctx, opts)
	if err := t.Start(); err != nil {
		return nil, err
	}
	return t.Wait()
}

// IsMakeTargetMissing reports whether a failed command was a make
// invocation whose target does not exist ("No rule to make target").
// Callers treat that case as "skipped" rather than failed.
func IsMakeTargetMissing(command []string, output string) bool {
	isMake := len(command) == 0 || command[0] == "make"
	if !isMake {
		return false
	}
	return strings.Contains(output, "No rule to make target") ||
		strings.Contains(output, "no rule to make target")
}
