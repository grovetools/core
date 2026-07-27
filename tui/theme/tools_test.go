package theme

import "testing"

// TestToolIconNormalizesProviderNaming pins the cross-provider matching: the
// same underlying tool must land on one icon whether it arrived as Claude's
// CamelCase, pi's snake_case or codex's own spelling.
func TestToolIconNormalizesProviderNaming(t *testing.T) {
	groups := [][]string{
		{"Bash", "bash", "shell", "exec_command", "local_shell"},
		{"Read", "read", "view_file"},
		{"Edit", "edit", "MultiEdit", "apply_patch", "str_replace_editor"},
		{"Write", "write", "create_file"},
		{"Grep", "grep", "codebase_search"},
		{"TodoWrite", "todowrite", "todo_write"},
		{"Agent", "task", "sub_agent"},
	}
	for _, group := range groups {
		want, _ := ToolIconAndStyle(group[0], DefaultTheme)
		for _, alias := range group[1:] {
			if got, _ := ToolIconAndStyle(alias, DefaultTheme); got != want {
				t.Errorf("ToolIconAndStyle(%q) = %q, want %q (same tool as %q)", alias, got, want, group[0])
			}
		}
	}
}

// TestToolIconFamilyColors pins the color-carries-the-family contract. Reading,
// mutating and delegating must never be told apart by glyph alone — a user
// scanning an outline reads the color first.
func TestToolIconFamilyColors(t *testing.T) {
	colors := DefaultTheme.Colors
	cases := []struct {
		tool string
		icon string
		want any
	}{
		{"Bash", IconShell, colors.Blue},
		{"Read", IconFile, colors.Cyan},
		{"grove_concepts", IconNotebook, colors.Cyan},
		{"Write", IconFilePlus, colors.Orange},
		{"Edit", IconDiff, colors.Orange},
		{"Agent", IconHeadlessAgent, colors.Violet},
		{"flow_subjob", IconWorktree, colors.Violet},
		{"TaskUpdate", IconChecklist, colors.Yellow},
		{"AskUserQuestion", IconHelp, colors.Pink},
		{"PushNotification", IconBell, colors.Pink},
	}
	for _, tt := range cases {
		icon, style := ToolIconAndStyle(tt.tool, DefaultTheme)
		if icon != tt.icon || style.GetForeground() != tt.want {
			t.Errorf("ToolIconAndStyle(%q) = %q/%v, want %q/%v", tt.tool, icon, style.GetForeground(), tt.icon, tt.want)
		}
	}
}

// TestToolIconUnknownStaysNeutral keeps an unmapped tool looking like a tool
// rather than silently borrowing a family's meaning.
func TestToolIconUnknownStaysNeutral(t *testing.T) {
	icon, style := ToolIconAndStyle("SomeToolNobodyMappedYet", DefaultTheme)
	if icon != IconTool || style.GetForeground() != DefaultTheme.Colors.Cyan {
		t.Errorf("unknown tool = %q/%v, want %q/%v", icon, style.GetForeground(), IconTool, DefaultTheme.Colors.Cyan)
	}
}

// TestToolIconMCPServersShareOneGlyph: MCP tool names are server-qualified and
// unbounded in variety, so they are identified as a class, not one by one.
func TestToolIconMCPServersShareOneGlyph(t *testing.T) {
	for _, name := range []string{"mcp__claude_ai_Gmail__authenticate", "mcp__linear__create_issue"} {
		icon, style := ToolIconAndStyle(name, DefaultTheme)
		if icon != IconSparkle || style.GetForeground() != DefaultTheme.Colors.Violet {
			t.Errorf("ToolIconAndStyle(%q) = %q/%v, want %q/violet", name, icon, style.GetForeground(), IconSparkle)
		}
	}
	// A local tool merely starting with "mcp" is not an MCP call.
	if icon, _ := ToolIconAndStyle("mcpconfig", DefaultTheme); icon != IconTool {
		t.Errorf("ToolIconAndStyle(\"mcpconfig\") = %q, want the generic %q", icon, IconTool)
	}
}

// TestToolIconFollowsIconSet: callers render whatever set is active, so the
// registry must read the live icon variables rather than a cached Nerd glyph.
func TestToolIconFollowsIconSet(t *testing.T) {
	initialASCII := ASCIIIcons
	t.Cleanup(func() { applyIcons(initialASCII) })

	SetIcons("ascii")
	if got := ToolIcon("Bash"); got != asciiIconShell {
		t.Errorf("ascii ToolIcon(\"Bash\") = %q, want %q", got, asciiIconShell)
	}
	SetIcons("nerd")
	if got := ToolIcon("Bash"); got != nerdIconShell {
		t.Errorf("nerd ToolIcon(\"Bash\") = %q, want %q", got, nerdIconShell)
	}
}
