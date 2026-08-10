package config

import (
	"os"
	"testing"
)

// Config loading is intentionally sensitive to process-wide topology paths.
// Give the package a hostile-ambient-proof empty home by default; individual
// tests that exercise a particular hierarchy override these variables with
// t.Setenv. This keeps a developer's live roots/notebooks/machine files out of
// otherwise unrelated parser and cache tests.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "grove-config-tests-")
	if err != nil {
		panic(err)
	}
	_ = os.Unsetenv("GROVE_HOME")
	_ = os.Unsetenv("XDG_CONFIG_HOME")
	_ = os.Setenv("HOME", home)
	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}
