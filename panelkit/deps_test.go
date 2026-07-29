package panelkit_test

import (
	"os/exec"
	"strings"
	"testing"
)

// kitPackages is the curated surface a panel is built from: the decoupled
// core/tui primitives plus the panelkit widgets. panelkit/sidecar is
// deliberately absent — it is allowed to know about the protocol and the host.
var kitPackages = []string{
	"github.com/grovetools/core/tui/theme",
	"github.com/grovetools/core/tui/theme/themecfg",
	"github.com/grovetools/core/tui/keymap",
	"github.com/grovetools/core/tui/components/help",
	"github.com/grovetools/core/tui/components/whichkey",
	"github.com/grovetools/core/tui/components/table",
	"github.com/grovetools/core/tui/components/pager",
	"github.com/grovetools/core/tui/hostedkeys",
	"github.com/grovetools/core/panelkit/window",
	"github.com/grovetools/core/panelkit/table",
	"github.com/grovetools/core/panelkit/tree",
	"github.com/grovetools/core/panelkit/layout",
}

// forbidden are the host-only package trees. Each entry cost the kit something
// concrete before the decoupling: core/config came in through two lines in
// theme and three in keymap; workspace, plan and git came in behind the
// pager's three embed message cases. A panel that is not a grove binary has no
// grove.toml to read and no daemon to call, and linking these makes it
// impossible to build one outside this workspace.
var forbidden = []string{
	"github.com/grovetools/core/config",
	"github.com/grovetools/core/pkg/daemon",
	"github.com/grovetools/core/pkg/workspace",
	"github.com/grovetools/core/pkg/plan",
	"github.com/grovetools/core/pkg/models",
	"github.com/grovetools/core/git",
	"github.com/grovetools/tuimux",
}

func TestKitIsSidecarClean(t *testing.T) {
	if testing.Short() {
		t.Skip("shells out to go list")
	}

	args := append([]string{"list", "-deps"}, kitPackages...)
	out, err := exec.Command("go", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, out)
	}

	for _, dep := range strings.Fields(string(out)) {
		for _, bad := range forbidden {
			if dep == bad || strings.HasPrefix(dep, bad+"/") {
				t.Errorf("kit reaches %s, which is host-only.\n"+
					"Something in %v grew an import that a sidecar panel cannot link. "+
					"Put the host's contribution behind a seam (see themecfg, keymap.KeybindingSource) "+
					"instead of importing it.", dep, kitPackages)
			}
		}
	}
}
