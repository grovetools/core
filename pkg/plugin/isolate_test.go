package plugin

import (
	"path/filepath"
	"testing"

	"github.com/grovetools/core/pkg/exectrust"
)

// isolate points every grove-managed location at a temp root, so a test reads
// and writes its own config, data, bin and trust store and never touches the
// developer's.
func isolate(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("GROVE_HOME", root)
	t.Setenv("GROVE_BIN", "") // let BinDir fall through to GROVE_HOME/data/bin
	t.Setenv(exectrust.EnvStorePath, filepath.Join(root, "state", "grove", "exec-trust.json"))
	return root
}
