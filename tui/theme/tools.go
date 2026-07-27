package theme

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ToolIconAndStyle maps an agent tool-call name to an icon and color. It is the
// single source of truth for tool iconography, shared by every surface that
// renders a transcript (treemux's outline today, agentlogs' renderers next), so
// the same tool reads identically wherever it appears.
//
// Names arrive in whatever casing and separator style the provider chose —
// Claude's "TaskUpdate", pi's "grove_concepts", codex's "exec_command",
// opencode's "todowrite" — so matching happens on a normalized key (lowercase,
// separators removed). Unrecognized tools keep the generic wrench in the
// neutral tool color rather than borrowing a family's meaning.
//
// Color carries the family, the glyph carries the specific tool:
//
//	Blue    exec — shell commands and repo plumbing
//	Cyan    inspect — reads, searches, listings, knowledge lookups, the web
//	Orange  mutate — anything that writes to disk
//	Violet  delegate — spawning agents, subjobs, skills, MCP servers
//	Yellow  track — todos, tasks, plans, schedules, monitors
//	Pink    interact — questions, messages, notifications
func ToolIconAndStyle(tool string, t *Theme) (icon string, style lipgloss.Style) {
	if t == nil {
		t = DefaultTheme
	}
	colors := t.Colors
	fg := func(c lipgloss.TerminalColor) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(c)
	}

	name := normalizeToolName(tool)

	// MCP tools arrive as mcp__<server>__<tool>; the server, not the verb, is
	// what distinguishes them from the agent's built-ins.
	if strings.HasPrefix(name, "mcp") && strings.Contains(strings.ToLower(tool), "__") {
		return IconSparkle, fg(colors.Violet)
	}

	switch name {
	// --- exec (Blue) ---
	case "bash", "shell", "sh", "exec", "execcommand", "runcommand", "runterminalcmd", "localshell", "terminal", "bashoutput", "killshell", "killbash":
		return IconShell, fg(colors.Blue)
	case "git", "gitstatus", "gitdiff", "gitcommit":
		return IconGit, fg(colors.Blue)
	case "enterworktree", "exitworktree", "worktree":
		return IconPineTreeBox, fg(colors.Blue)

	// --- inspect (Cyan) ---
	case "read", "readfile", "view", "viewfile", "viewimage", "openfile", "cat":
		return IconFile, fg(colors.Cyan)
	case "grep", "search", "rg", "ripgrep", "grepsearch", "searchfiles", "codebasesearch":
		return IconSearch, fg(colors.Cyan)
	case "glob", "findfiles", "fileglob":
		return IconFolderSearch, fg(colors.Cyan)
	case "ls", "list", "listdir", "listfiles", "listdirectory", "tree":
		return IconFileTree, fg(colors.Cyan)
	case "webfetch", "websearch", "fetch", "browser":
		return IconEarth, fg(colors.Cyan)
	case "toolsearch":
		return IconFilter, fg(colors.Cyan)
	case "lsp", "diagnostics":
		return IconCode, fg(colors.Cyan)
	case "groveconcepts", "concepts":
		return IconNotebook, fg(colors.Cyan)
	case "grovememory", "memory":
		return IconArchive, fg(colors.Cyan)
	case "grovecontext", "context", "cx":
		return IconFolderMultiple, fg(colors.Cyan)
	case "nb", "note", "notes":
		return IconNote, fg(colors.Cyan)

	// --- mutate (Orange) ---
	case "write", "writefile", "create", "createfile", "newfile":
		return IconFilePlus, fg(colors.Orange)
	case "edit", "editfile", "multiedit", "applypatch", "patch", "strreplace", "strreplaceeditor", "replace", "insert":
		return IconDiff, fg(colors.Orange)
	case "delete", "deletefile", "removefile", "rm":
		return IconFileCancel, fg(colors.Orange)
	case "notebookedit":
		return IconNotebook, fg(colors.Orange)

	// --- delegate (Violet) ---
	case "agent", "task", "subagent", "dispatchagent", "spawnagent":
		return IconHeadlessAgent, fg(colors.Violet)
	case "flowsubjob", "flowpipeline", "subjob", "pipeline", "workflow":
		return IconWorktree, fg(colors.Violet)
	case "flowchat", "flowdesignchat", "floworaclechat":
		return IconSchool, fg(colors.Violet)
	case "floworaclereview", "review", "codereview":
		return IconStatusNeedsReview, fg(colors.Violet)
	case "skill", "skills", "invokeskill":
		return IconSchool, fg(colors.Violet)

	// --- track (Yellow) ---
	case "todowrite", "todoread", "todo":
		return IconChecklist, fg(colors.Yellow)
	case "taskcreate", "taskupdate", "taskget", "tasklist", "taskstop", "taskoutput":
		return IconChecklist, fg(colors.Yellow)
	case "updateplan", "plan", "enterplanmode", "exitplanmode":
		return IconPlan, fg(colors.Yellow)
	case "schedulewakeup", "schedule", "cron", "croncreate", "cronlist", "crondelete":
		return IconClock, fg(colors.Yellow)
	case "monitor", "watch":
		return IconClockFast, fg(colors.Yellow)

	// --- interact (Pink) ---
	case "askuserquestion", "ask", "question":
		return IconHelp, fg(colors.Pink)
	case "sendmessage", "message", "reply":
		return IconChat, fg(colors.Pink)
	case "pushnotification", "notify", "notification":
		return IconBell, fg(colors.Pink)

	default:
		return IconTool, fg(colors.Cyan)
	}
}

// ToolIcon returns just the glyph for a tool name.
func ToolIcon(tool string) string {
	icon, _ := ToolIconAndStyle(tool, DefaultTheme)
	return icon
}

// normalizeToolName folds provider naming conventions into one key so a single
// case arm covers "TaskUpdate", "task_update" and "task-update".
func normalizeToolName(tool string) string {
	lower := strings.ToLower(strings.TrimSpace(tool))
	var b strings.Builder
	b.Grow(len(lower))
	for _, r := range lower {
		switch r {
		case '_', '-', '.', ' ':
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
