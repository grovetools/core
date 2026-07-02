package taskexec

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRunStreamsCombinedOutput(t *testing.T) {
	var mu sync.Mutex
	var lines []string
	output, err := Run(context.Background(), Options{
		Command: []string{"sh", "-c", "echo out1; echo err1 >&2; echo out2"},
		Dir:     t.TempDir(),
		Env:     os.Environ(),
		OnOutput: func(line string) {
			mu.Lock()
			lines = append(lines, line)
			mu.Unlock()
		},
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	got := string(output)
	for _, want := range []string{"out1", "err1", "out2"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q, got: %q", want, got)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(lines) != 3 {
		t.Errorf("expected 3 streamed lines, got %d: %v", len(lines), lines)
	}
}

func TestRunReturnsExitError(t *testing.T) {
	output, err := Run(context.Background(), Options{
		Command: []string{"sh", "-c", "echo failing; exit 3"},
		Dir:     t.TempDir(),
		Env:     os.Environ(),
	})
	if err == nil {
		t.Fatal("expected error for exit 3")
	}
	if !strings.Contains(string(output), "failing") {
		t.Errorf("output not captured on failure: %q", string(output))
	}
}

func TestStartFailure(t *testing.T) {
	task := New(context.Background(), Options{
		Command: []string{"/nonexistent/binary/for/taskexec/test"},
		Dir:     t.TempDir(),
		Env:     os.Environ(),
	})
	if err := task.Start(); err == nil {
		t.Fatal("expected start error for missing binary")
	}
}

func TestCancelKillsProcessGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	task := New(ctx, Options{
		// Spawn a child under sh so the group kill (not just the direct
		// child kill) is what unblocks the wait.
		Command: []string{"sh", "-c", "sleep 30 & wait"},
		Dir:     t.TempDir(),
		Env:     os.Environ(),
	})
	if err := task.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	done := make(chan error, 1)
	go func() {
		_, err := task.Wait()
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected error after cancellation")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("process group was not killed within 15s of cancellation")
	}
}

func TestDefaultCommandIsMakeVerb(t *testing.T) {
	dir := t.TempDir()
	makefile := "check:\n\t@echo make-check-ran\n"
	if err := os.WriteFile(dir+"/Makefile", []byte(makefile), 0o644); err != nil {
		t.Fatal(err)
	}
	output, err := Run(context.Background(), Options{
		Verb: "check",
		Dir:  dir,
		Env:  os.Environ(),
	})
	if err != nil {
		t.Fatalf("make check failed: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "make-check-ran") {
		t.Errorf("expected make target output, got: %q", string(output))
	}
}

func TestIsMakeTargetMissing(t *testing.T) {
	cases := []struct {
		name    string
		command []string
		output  string
		want    bool
	}{
		{"default make with missing target", nil, "make: *** No rule to make target `lint'.  Stop.", true},
		{"explicit make lowercase message", []string{"make", "lint"}, "make: *** no rule to make target 'lint'", true},
		{"non-make command", []string{"go", "vet"}, "No rule to make target", false},
		{"make real failure", []string{"make", "build"}, "compile error", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsMakeTargetMissing(tc.command, tc.output); got != tc.want {
				t.Errorf("IsMakeTargetMissing(%v, %q) = %v, want %v", tc.command, tc.output, got, tc.want)
			}
		})
	}
}
