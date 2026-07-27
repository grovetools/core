package mux

import (
	"context"
	"errors"
	"testing"
)

// The typed-nil hazard these tests pin down: NewTuimuxEngine/NewTmuxEngine
// return concrete pointers, so forwarding their failure result straight into a
// MuxEngine-typed return produces a non-nil interface wrapping a nil pointer.
// Callers guarding with `engine != nil` then call a method on a nil receiver
// and panic — which is how a missing tuimux daemon crashed Flow's job
// completion instead of skipping best-effort window cleanup.

var errConstructor = errors.New("boom: multiplexer unavailable")

func failingTuimux() (*TuimuxEngine, error) { return nil, errConstructor }

func failingTmux() (*TmuxEngine, error) { return nil, errConstructor }

// useConstructors installs constructor seams and clears the detection cache for
// the duration of one test.
func useConstructors(t *testing.T, tuimux func() (*TuimuxEngine, error), tmux func() (*TmuxEngine, error)) {
	t.Helper()
	prevTuimux, prevTmux := tuimuxConstructor, tmuxConstructor
	tuimuxConstructor, tmuxConstructor = tuimux, tmux
	ResetDetection()
	t.Cleanup(func() {
		tuimuxConstructor, tmuxConstructor = prevTuimux, prevTmux
		ResetDetection()
	})
}

// assertNoUsableEngine checks the package invariant: err != nil => engine is
// genuinely nil, not an interface holding a nil pointer.
func assertNoUsableEngine(t *testing.T, engine MuxEngine, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error from a failing constructor, got nil")
	}
	if engine != nil {
		t.Fatalf("expected a genuinely nil MuxEngine, got a non-nil interface holding %T", engine)
	}
	if !IsNilEngine(engine) {
		t.Fatal("IsNilEngine disagreed with a nil interface")
	}
}

func TestDetectMuxEngineNormalizesTuimuxConstructorFailure(t *testing.T) {
	t.Setenv(EnvGroveMux, "tuimux")
	useConstructors(t, failingTuimux, failingTmux)

	engine, err := DetectMuxEngine(context.Background())
	assertNoUsableEngine(t, engine, err)
	if !errors.Is(err, errConstructor) {
		t.Fatalf("constructor error was not propagated: %v", err)
	}
}

func TestDetectMuxEngineNormalizesTmuxConstructorFailure(t *testing.T) {
	t.Setenv(EnvGroveMux, "tmux")
	useConstructors(t, failingTuimux, failingTmux)

	engine, err := DetectMuxEngine(context.Background())
	assertNoUsableEngine(t, engine, err)
}

func TestDetectMuxEngineNormalizesAutoDetectFailure(t *testing.T) {
	// No GROVE_MUX and no active mux: detection falls through to the tmux
	// constructor, whose failure must not leak a typed nil either.
	t.Setenv(EnvGroveMux, "")
	t.Setenv(EnvGroveTmuxSocket, "")
	t.Setenv(EnvTmux, "")
	t.Setenv(EnvTuimuxPTY, "")
	useConstructors(t, failingTuimux, failingTmux)

	engine, err := DetectMuxEngine(context.Background())
	assertNoUsableEngine(t, engine, err)
}

func TestGetEngineNormalizesConstructorFailure(t *testing.T) {
	useConstructors(t, failingTuimux, failingTmux)

	for _, name := range []string{"tuimux", "tmux"} {
		engine, err := GetEngine(name)
		assertNoUsableEngine(t, engine, err)
	}
}

func TestIsAvailableIsFalseWhenDetectionFails(t *testing.T) {
	t.Setenv(EnvGroveMux, "tuimux")
	useConstructors(t, failingTuimux, failingTmux)

	if IsAvailable(context.Background()) {
		t.Fatal("IsAvailable reported an engine after the constructor failed")
	}
}

// A constructor that reports success while returning a nil pointer is a bug in
// the constructor, but it must never reach a caller as a callable engine.
func TestNormalizeEngineRejectsTypedNilWithoutError(t *testing.T) {
	engine, err := normalizeEngine(nilTuimuxEngine())
	if err == nil {
		t.Fatal("expected an error for a nil engine returned without one")
	}
	if engine != nil {
		t.Fatalf("expected a nil MuxEngine, got %T", engine)
	}
}

func nilTuimuxEngine() (*TuimuxEngine, error) { return nil, nil }

func TestIsNilEngineDetectsTypedNilDoubles(t *testing.T) {
	var tuimuxNil *TuimuxEngine
	var tmuxNil *TmuxEngine

	cases := []struct {
		name   string
		engine MuxEngine
	}{
		{"nil interface", nil},
		{"typed nil *TuimuxEngine", tuimuxNil},
		{"typed nil *TmuxEngine", tmuxNil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !IsNilEngine(tc.engine) {
				t.Fatalf("IsNilEngine(%s) = false; a caller would panic on the next method call", tc.name)
			}
		})
	}

	// The plain != nil guard the old code relied on is exactly what fails here.
	var typedNil MuxEngine = tuimuxNil
	if typedNil == nil {
		t.Fatal("expected a typed nil to compare non-nil; this test no longer covers the hazard")
	}

	if IsNilEngine(&TuimuxEngine{}) {
		t.Fatal("IsNilEngine rejected a real engine")
	}
}

// Real end-to-end failure with no seams: with tmux selected and nothing on
// PATH, tmux.NewClient cannot find the binary.
func TestDetectMuxEngineWithoutTmuxBinary(t *testing.T) {
	t.Setenv(EnvGroveMux, "tmux")
	t.Setenv("PATH", "")
	ResetDetection()
	t.Cleanup(ResetDetection)

	engine, err := DetectMuxEngine(context.Background())
	assertNoUsableEngine(t, engine, err)
}

// Cleanup paths must not bring up a multiplexer. With tuimux demanded but no
// daemon reachable, detection reports failure instead of spawning one.
func TestDetectExistingMuxEngineDoesNotStartTuimux(t *testing.T) {
	t.Setenv(EnvGroveMux, "tuimux")
	t.Setenv(EnvGroveTuimuxSocket, t.TempDir()+"/absent.sock")
	// A constructor that would start a daemon must never be reached.
	useConstructors(t, func() (*TuimuxEngine, error) {
		t.Fatal("DetectExistingMuxEngine started a tuimux daemon")
		return nil, nil
	}, failingTmux)

	engine, err := DetectExistingMuxEngine(context.Background())
	assertNoUsableEngine(t, engine, err)
}

func TestDetectExistingMuxEngineNormalizesTmuxFailure(t *testing.T) {
	t.Setenv(EnvGroveMux, "tmux")
	t.Setenv(EnvGroveTmuxSocket, "")
	useConstructors(t, failingTuimux, failingTmux)

	engine, err := DetectExistingMuxEngine(context.Background())
	assertNoUsableEngine(t, engine, err)
}
