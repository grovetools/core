package doctor

import (
	"context"
	"errors"
	"testing"
)

type fakeCheck struct {
	id       string
	status   Status
	fixable  bool
	fixErr   error
	runCount int
	fixCount int
}

func (f *fakeCheck) ID() string   { return f.id }
func (f *fakeCheck) Name() string { return "fake " + f.id }
func (f *fakeCheck) Run(_ context.Context, _ RunOptions) CheckResult {
	f.runCount++
	// After a successful fix, flip to OK on the next Run.
	status := f.status
	if f.fixCount > 0 && f.fixErr == nil {
		status = StatusOK
	}
	return CheckResult{ID: f.id, Name: f.Name(), Status: status, Fixable: f.fixable}
}
func (f *fakeCheck) AutoFix(_ context.Context) error {
	f.fixCount++
	return f.fixErr
}

func TestRegistryRunsAllRegistered(t *testing.T) {
	t.Cleanup(Reset)
	Reset()
	a := &fakeCheck{id: "a", status: StatusOK}
	b := &fakeCheck{id: "b", status: StatusWarn}
	Register(a)
	Register(b)

	results := RunAll(context.Background(), RunOptions{})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if a.runCount != 1 || b.runCount != 1 {
		t.Fatalf("each check should run once, got a=%d b=%d", a.runCount, b.runCount)
	}
	if results[0].ID != "a" || results[1].ID != "b" {
		t.Fatalf("expected order [a,b], got [%s,%s]", results[0].ID, results[1].ID)
	}
}

func TestRunAllWithFixOnlyFixesFixable(t *testing.T) {
	t.Cleanup(Reset)
	Reset()
	fixable := &fakeCheck{id: "fixable", status: StatusFail, fixable: true}
	manual := &fakeCheck{id: "manual", status: StatusWarn, fixable: false}
	Register(fixable)
	Register(manual)

	results := RunAllWithFix(context.Background(), RunOptions{})
	if len(results) != 2 {
		t.Fatalf("expected 2 results")
	}
	if fixable.fixCount != 1 {
		t.Fatalf("fixable should have been fixed once, got %d", fixable.fixCount)
	}
	if manual.fixCount != 0 {
		t.Fatalf("manual should not have been fixed")
	}
	if !results[0].FixApplied || results[0].Status != StatusOK {
		t.Fatalf("fixable should report FixApplied=true and StatusOK after fix, got %+v", results[0])
	}
}

func TestRunAllWithFixSurfacesFixError(t *testing.T) {
	t.Cleanup(Reset)
	Reset()
	c := &fakeCheck{id: "c", status: StatusFail, fixable: true, fixErr: errors.New("boom")}
	Register(c)

	results := RunAllWithFix(context.Background(), RunOptions{})
	if results[0].FixApplied {
		t.Fatalf("expected FixApplied=false on fix error")
	}
	if results[0].Error != "boom" {
		t.Fatalf("expected Error='boom', got %q", results[0].Error)
	}
}

func TestRunOne(t *testing.T) {
	t.Cleanup(Reset)
	Reset()
	Register(&fakeCheck{id: "only", status: StatusOK})

	res, ok := RunOne(context.Background(), "only", RunOptions{})
	if !ok || res.ID != "only" {
		t.Fatalf("expected to find 'only', got ok=%v res=%+v", ok, res)
	}
	if _, ok := RunOne(context.Background(), "missing", RunOptions{}); ok {
		t.Fatalf("expected not-found for missing id")
	}
}
