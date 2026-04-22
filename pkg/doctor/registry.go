package doctor

import "errors"

// ErrNotFixable is the conventional error returned by AutoFix implementations
// that cannot apply a programmatic fix (e.g. missing system tooling, parent
// shell env changes).
var ErrNotFixable = errors.New("check is not auto-fixable")

var registry []Check

// Register adds a Check to the global registry. Intended for use from init().
func Register(c Check) {
	registry = append(registry, c)
}

// All returns a copy of the currently-registered checks.
func All() []Check {
	out := make([]Check, len(registry))
	copy(out, registry)
	return out
}

// Reset clears the registry. Intended for tests.
func Reset() {
	registry = nil
}
