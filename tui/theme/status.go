package theme

import "github.com/charmbracelet/lipgloss"

// StatusIconAndStyle returns the appropriate icon and lipgloss style for a job/agent status.
// This is the single source of truth for status icon/color mapping used by both flow and treemux.
// Status strings include: "completed", "running", "failed", "blocked", "pending", "todo",
// "hold", "abandoned", "needs_review", "pending_user", "idle", "stopped", "error", and others.
func StatusIconAndStyle(status string, theme *Theme) (icon string, style lipgloss.Style) {
	switch status {
	case "completed":
		return IconStatusCompleted, theme.Success
	case "running":
		return IconStatusRunning, theme.Info
	case "failed", "error":
		return IconStatusFailed, theme.Error
	case "blocked":
		return IconStatusBlocked, theme.Error
	case "pending":
		return IconPending, theme.Muted
	case "pending_user":
		return IconStatusPendingUser, theme.Muted
	case "todo":
		return IconStatusTodo, theme.Muted
	case "hold":
		return IconStatusHold, theme.Warning
	case "abandoned":
		return IconStatusAbandoned, theme.Muted
	case "needs_review":
		return IconStatusNeedsReview, theme.Info
	case "idle", "stopped":
		return IconPending, theme.Muted
	default:
		// Unknown status: return neutral icon/style
		return IconPending, theme.Muted
	}
}

// StatusIcon returns just the icon glyph for a status.
func StatusIcon(status string) string {
	icon, _ := StatusIconAndStyle(status, DefaultTheme)
	return icon
}

// StatusStyle returns just the lipgloss style for a status.
func StatusStyle(status string, theme *Theme) lipgloss.Style {
	_, style := StatusIconAndStyle(status, theme)
	return style
}

// StyleIcon applies a lipgloss style to an icon, rendering it as styled text.
// This is useful for colored icons that need to preserve their base glyph.
func StyleIcon(icon string, style lipgloss.Style) string {
	return style.Render(icon)
}
