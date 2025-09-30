package logging

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattsolo1/grove-core/tui/theme"
)

// PrettyLogger provides pretty formatted console output
type PrettyLogger struct {
	writer io.Writer
	styles PrettyStyles
}

// PrettyStyles contains lipgloss styles for different log types
type PrettyStyles struct {
	Success   lipgloss.Style
	Info      lipgloss.Style
	Warning   lipgloss.Style
	Error     lipgloss.Style
	Icon      lipgloss.Style
	Key       lipgloss.Style
	Value     lipgloss.Style
	Path      lipgloss.Style
	Code      lipgloss.Style
	Component lipgloss.Style
}

// DefaultPrettyStyles returns the default styling for pretty logs using Grove theme
func DefaultPrettyStyles() PrettyStyles {
	t := theme.DefaultTheme
	return PrettyStyles{
		Success:   t.Success,
		Info:      t.Info,
		Warning:   t.Warning,
		Error:     t.Error,
		Icon:      lipgloss.NewStyle(),
		Key:       t.Muted,
		Value:     t.Bold,
		Path:      lipgloss.NewStyle().Foreground(theme.Cyan).Italic(true),
		Code:      t.Code,
		Component: t.Accent.Background(theme.SubtleBackground),
	}
}

// NewPrettyLogger creates a pretty logger wrapper
func NewPrettyLogger() *PrettyLogger {
	return &PrettyLogger{
		writer: os.Stderr,
		styles: DefaultPrettyStyles(),
	}
}

// WithWriter sets a custom writer for pretty output
func (p *PrettyLogger) WithWriter(w io.Writer) *PrettyLogger {
	p.writer = w
	return p
}

// Success logs a success message with a checkmark
func (p *PrettyLogger) Success(message string) {
	// Pretty print to console
	fmt.Fprintf(p.writer, "%s %s\n", 
		p.styles.Success.Render("✓"),
		p.styles.Success.Render(message))
}

// InfoPretty logs an info message with pretty formatting
func (p *PrettyLogger) InfoPretty(message string) {
	// Pretty print to console
	fmt.Fprintf(p.writer, "%s\n", p.styles.Info.Render(message))
}

// WarnPretty logs a warning with pretty formatting
func (p *PrettyLogger) WarnPretty(message string) {
	// Pretty print to console
	fmt.Fprintf(p.writer, "%s %s\n",
		p.styles.Warning.Render("⚠"),
		p.styles.Warning.Render(message))
}

// ErrorPretty logs an error with pretty formatting
func (p *PrettyLogger) ErrorPretty(message string, err error) {
	// Pretty print to console
	fmt.Fprintf(p.writer, "%s %s",
		p.styles.Error.Render("✗"),
		p.styles.Error.Render(message))
	if err != nil {
		fmt.Fprintf(p.writer, ": %s", p.styles.Error.Render(err.Error()))
	}
	fmt.Fprintln(p.writer)
}

// Field logs a key-value pair with pretty formatting
func (p *PrettyLogger) Field(key string, value interface{}) {
	// Pretty print
	fmt.Fprintf(p.writer, "%s: %s\n",
		p.styles.Key.Render(key),
		p.styles.Value.Render(fmt.Sprint(value)))
}

// Path logs a file path with special formatting
func (p *PrettyLogger) Path(label string, path string) {
	// Pretty print
	fmt.Fprintf(p.writer, "%s: %s\n",
		p.styles.Key.Render(label),
		p.styles.Path.Render(path))
}

// Code logs code or command output
func (p *PrettyLogger) Code(content string) {
	// Pretty print with indentation
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		fmt.Fprintf(p.writer, "  %s\n", p.styles.Code.Render(line))
	}
}

// Divider prints a visual divider
func (p *PrettyLogger) Divider() {
	divider := strings.Repeat("─", 60)
	fmt.Fprintln(p.writer, p.styles.Key.Render(divider))
}

// Blank prints a blank line
func (p *PrettyLogger) Blank() {
	fmt.Fprintln(p.writer)
}

// Section prints a section header with the Grove theme
func (p *PrettyLogger) Section(title string) {
	fmt.Fprintf(p.writer, "\n%s %s\n\n",
		theme.DefaultTheme.Header.Render(theme.IconTree),
		theme.DefaultTheme.Header.Render(title))
}

// Progress logs a progress message
func (p *PrettyLogger) Progress(message string) {
	fmt.Fprintf(p.writer, "%s %s\n",
		p.styles.Info.Render(theme.IconRunning),
		p.styles.Info.Render(message))
}

// KeyValue logs multiple key-value pairs
func (p *PrettyLogger) KeyValues(pairs map[string]interface{}) {
	for key, value := range pairs {
		p.Field(key, value)
	}
}

// List logs a list of items
func (p *PrettyLogger) List(items []string) {
	for _, item := range items {
		fmt.Fprintf(p.writer, "  %s %s\n",
			theme.DefaultTheme.Highlight.Render(theme.IconBullet),
			item)
	}
}

// Box prints content in a styled box
func (p *PrettyLogger) Box(title, content string) {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(1, 2).
		Width(60)

	if title != "" {
		fmt.Fprintf(p.writer, "%s\n", theme.DefaultTheme.Header.Render(title))
	}
	fmt.Fprintf(p.writer, "%s\n", boxStyle.Render(content))
}

// Status logs a status with an appropriate icon
func (p *PrettyLogger) Status(status, message string) {
	var icon string
	var style lipgloss.Style

	switch status {
	case "success":
		icon = theme.IconSuccess
		style = p.styles.Success
	case "error":
		icon = theme.IconError
		style = p.styles.Error
	case "warning":
		icon = theme.IconWarning
		style = p.styles.Warning
	case "info":
		icon = theme.IconInfo
		style = p.styles.Info
	case "pending":
		icon = theme.IconPending
		style = p.styles.Warning
	case "running":
		icon = theme.IconRunning
		style = p.styles.Info
	default:
		icon = theme.IconBullet
		style = lipgloss.NewStyle()
	}

	fmt.Fprintf(p.writer, "%s %s\n", style.Render(icon), style.Render(message))
}