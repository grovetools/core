package theme

import "testing"

// TestSetIconsLiveSwitch pins the runtime icon-set switch: SetIcons("ascii")
// swaps the exported icon variables to the ASCII set in-process (no restart),
// and switching back restores the Nerd Font set. Runs both directions inside
// one test because the icon variables are package globals.
func TestSetIconsLiveSwitch(t *testing.T) {
	// Restore whatever set the process started with.
	initialASCII := ASCIIIcons
	t.Cleanup(func() { applyIcons(initialASCII) })

	SetIcons("ascii")
	if !ASCIIIcons {
		t.Error("SetIcons(\"ascii\"): ASCIIIcons = false, want true")
	}
	if IconTree != asciiIconTree {
		t.Errorf("SetIcons(\"ascii\"): IconTree = %q, want %q", IconTree, asciiIconTree)
	}
	if IconGitBranch != asciiIconGitBranch {
		t.Errorf("SetIcons(\"ascii\"): IconGitBranch = %q, want %q", IconGitBranch, asciiIconGitBranch)
	}
	if IconWarning != asciiIconWarning {
		t.Errorf("SetIcons(\"ascii\"): IconWarning = %q, want %q", IconWarning, asciiIconWarning)
	}
	if IconFolder != asciiIconFolder {
		t.Errorf("SetIcons(\"ascii\"): IconFolder = %q, want %q", IconFolder, asciiIconFolder)
	}
	if IconPlan != asciiIconPlan {
		t.Errorf("SetIcons(\"ascii\"): IconPlan = %q, want %q", IconPlan, asciiIconPlan)
	}

	SetIcons("nerd")
	if ASCIIIcons {
		t.Error("SetIcons(\"nerd\"): ASCIIIcons = true, want false")
	}
	if IconTree != nerdIconTree {
		t.Errorf("SetIcons(\"nerd\"): IconTree = %q, want %q", IconTree, nerdIconTree)
	}
	if IconPlan != nerdIconPlan {
		t.Errorf("SetIcons(\"nerd\"): IconPlan = %q, want %q", IconPlan, nerdIconPlan)
	}
}

// TestSetIconsModeNormalization: only "ascii" (any case, surrounding space)
// selects the ASCII set; unknown modes fall back to Nerd Font.
func TestSetIconsModeNormalization(t *testing.T) {
	initialASCII := ASCIIIcons
	t.Cleanup(func() { applyIcons(initialASCII) })

	cases := []struct {
		mode      string
		wantASCII bool
	}{
		{"ascii", true},
		{"ASCII", true},
		{" ascii ", true},
		{"nerd", false},
		{"", false},
		{"unknown", false},
	}
	for _, tc := range cases {
		SetIcons(tc.mode)
		if ASCIIIcons != tc.wantASCII {
			t.Errorf("SetIcons(%q): ASCIIIcons = %v, want %v", tc.mode, ASCIIIcons, tc.wantASCII)
		}
	}
}
