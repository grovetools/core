package theme

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

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
	if IconTool != asciiIconTool {
		t.Errorf("SetIcons(\"ascii\"): IconTool = %q, want %q", IconTool, asciiIconTool)
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
	if IconTool != nerdIconTool {
		t.Errorf("SetIcons(\"nerd\"): IconTool = %q, want %q", IconTool, nerdIconTool)
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

// TestNoIconLiteralIsBlank guards a bug class this file has now hit twice:
// an icon constant whose comment advertises a glyph but whose literal is
// empty, so every call site renders a blank slot and nothing fails loudly.
// nerdIconConfig shipped that way, and so did nerdIconDiff. The literals are
// invisible to reflection (package-level constants), so the source is parsed.
func TestNoIconLiteralIsBlank(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "icons.go", nil, 0)
	if err != nil {
		t.Fatalf("parse icons.go: %v", err)
	}
	seen := 0
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range value.Names {
				if !strings.HasPrefix(name.Name, "nerdIcon") && !strings.HasPrefix(name.Name, "asciiIcon") {
					continue
				}
				if i >= len(value.Values) {
					continue
				}
				lit, ok := value.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				glyph, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("%s: unquote %s: %v", name.Name, lit.Value, err)
				}
				seen++
				if strings.TrimSpace(glyph) == "" {
					t.Errorf("%s = %q: blank literal — the glyph its comment names is missing", name.Name, glyph)
				}
			}
		}
	}
	if seen < 100 {
		t.Fatalf("only %d icon constants inspected; the parse walk is not finding them", seen)
	}
}
