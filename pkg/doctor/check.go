// Package doctor provides a pluggable diagnostic + fixit framework.
//
// Checks register themselves via init() into a shared registry. The grove
// CLI (or any other consumer) iterates the registry, runs each check, and
// optionally applies safe AutoFixes.
package doctor

import "context"

// Status is the outcome of a single check.
type Status string

const (
	StatusOK   Status = "ok"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
)

// CheckResult captures the outcome of one Check invocation. It is shaped
// to render well both as a human checklist and as JSON.
type CheckResult struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Status     Status `json:"status"`
	Message    string `json:"message"`
	Resolution string `json:"resolution,omitempty"`
	Fixable    bool   `json:"fixable"`
	FixApplied bool   `json:"fix_applied,omitempty"`
	Error      string `json:"error,omitempty"`
}

// RunOptions carries shared flags to a check's Run method.
type RunOptions struct {
	Verbose bool
}

// Check is the interface every diagnostic implements. AutoFix is mandatory
// but may be a no-op — return an error such as ErrNotFixable for checks
// that cannot be auto-resolved.
type Check interface {
	ID() string
	Name() string
	Run(ctx context.Context, opts RunOptions) CheckResult
	AutoFix(ctx context.Context) error
}
