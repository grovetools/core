package doctor

import "context"

// RunAll executes each registered check in registry order and returns their
// results. It does not apply any fixes — see RunAllWithFix.
func RunAll(ctx context.Context, opts RunOptions) []CheckResult {
	checks := All()
	out := make([]CheckResult, 0, len(checks))
	for _, c := range checks {
		out = append(out, c.Run(ctx, opts))
	}
	return out
}

// RunAllWithFix executes each registered check, then for any result that is
// not OK and is marked Fixable, calls AutoFix and re-runs the check to
// capture the post-fix state. FixApplied reflects whether AutoFix returned
// without error.
func RunAllWithFix(ctx context.Context, opts RunOptions) []CheckResult {
	checks := All()
	out := make([]CheckResult, 0, len(checks))
	for _, c := range checks {
		res := c.Run(ctx, opts)
		if res.Status != StatusOK && res.Fixable {
			if err := c.AutoFix(ctx); err != nil {
				res.Error = err.Error()
				res.FixApplied = false
			} else {
				// Re-run to verify the fix stuck.
				res = c.Run(ctx, opts)
				res.FixApplied = true
			}
		}
		out = append(out, res)
	}
	return out
}

// RunOne runs a single check by ID. Returns false if no check matches.
func RunOne(ctx context.Context, id string, opts RunOptions) (CheckResult, bool) {
	for _, c := range All() {
		if c.ID() == id {
			return c.Run(ctx, opts), true
		}
	}
	return CheckResult{}, false
}

// RunOneWithFix runs a single check by ID and applies AutoFix if eligible.
func RunOneWithFix(ctx context.Context, id string, opts RunOptions) (CheckResult, bool) {
	for _, c := range All() {
		if c.ID() != id {
			continue
		}
		res := c.Run(ctx, opts)
		if res.Status != StatusOK && res.Fixable {
			if err := c.AutoFix(ctx); err != nil {
				res.Error = err.Error()
				res.FixApplied = false
			} else {
				res = c.Run(ctx, opts)
				res.FixApplied = true
			}
		}
		return res, true
	}
	return CheckResult{}, false
}
