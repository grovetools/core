package logging

import (
	"context"
	"fmt"
	"io"
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
		writer: GetGlobalOutput(),
		styles: DefaultPrettyStyles(),
	}
}

// WithWriter sets a custom writer for pretty output
func (p *PrettyLogger) WithWriter(w io.Writer) *PrettyLogger {
	p.writer = w
	return p
}

// SuccessCtx logs a success message to the writer from the context.
func (p *PrettyLogger) SuccessCtx(ctx context.Context, message string) {
	writer := GetWriter(ctx)
	fmt.Fprintf(writer, "%s %s\n",
		p.styles.Success.Render(theme.IconSuccess),
		p.styles.Success.Render(message))
}

// Success logs a success message with a checkmark
func (p *PrettyLogger) Success(message string) {
	p.SuccessCtx(context.Background(), message)
}

// InfoPrettyCtx logs an info message with pretty formatting to the writer from the context.
func (p *PrettyLogger) InfoPrettyCtx(ctx context.Context, message string) {
	writer := GetWriter(ctx)
	fmt.Fprintf(writer, "%s\n", p.styles.Info.Render(message))
}

// InfoPretty logs an info message with pretty formatting
func (p *PrettyLogger) InfoPretty(message string) {
	p.InfoPrettyCtx(context.Background(), message)
}

// WarnPrettyCtx logs a warning with pretty formatting to the writer from the context.
func (p *PrettyLogger) WarnPrettyCtx(ctx context.Context, message string) {
	writer := GetWriter(ctx)
	fmt.Fprintf(writer, "%s %s\n",
		p.styles.Warning.Render(theme.IconWarning),
		p.styles.Warning.Render(message))
}

// WarnPretty logs a warning with pretty formatting
func (p *PrettyLogger) WarnPretty(message string) {
	p.WarnPrettyCtx(context.Background(), message)
}

// ErrorPrettyCtx logs an error with pretty formatting to the writer from the context.
func (p *PrettyLogger) ErrorPrettyCtx(ctx context.Context, message string, err error) {
	writer := GetWriter(ctx)
	fmt.Fprintf(writer, "%s %s",
		p.styles.Error.Render(theme.IconError),
		p.styles.Error.Render(message))
	if err != nil {
		fmt.Fprintf(writer, ": %s", p.styles.Error.Render(err.Error()))
	}
	fmt.Fprintln(writer)
}

// ErrorPretty logs an error with pretty formatting
func (p *PrettyLogger) ErrorPretty(message string, err error) {
	p.ErrorPrettyCtx(context.Background(), message, err)
}

// FieldCtx logs a key-value pair with pretty formatting to the writer from the context.
func (p *PrettyLogger) FieldCtx(ctx context.Context, key string, value interface{}) {
	writer := GetWriter(ctx)
	fmt.Fprintf(writer, "%s: %s\n",
		p.styles.Key.Render(key),
		p.styles.Value.Render(fmt.Sprint(value)))
}

// Field logs a key-value pair with pretty formatting
func (p *PrettyLogger) Field(key string, value interface{}) {
	p.FieldCtx(context.Background(), key, value)
}

// PathCtx logs a file path with special formatting to the writer from the context.
func (p *PrettyLogger) PathCtx(ctx context.Context, label string, path string) {
	writer := GetWriter(ctx)
	fmt.Fprintf(writer, "%s: %s\n",
		p.styles.Key.Render(label),
		p.styles.Path.Render(path))
}

// Path logs a file path with special formatting
func (p *PrettyLogger) Path(label string, path string) {
	p.PathCtx(context.Background(), label, path)
}

// CodeCtx logs code or command output to the writer from the context.
func (p *PrettyLogger) CodeCtx(ctx context.Context, content string) {
	writer := GetWriter(ctx)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		fmt.Fprintf(writer, "  %s\n", p.styles.Code.Render(line))
	}
}

// Code logs code or command output
func (p *PrettyLogger) Code(content string) {
	p.CodeCtx(context.Background(), content)
}

// DividerCtx prints a visual divider to the writer from the context.
func (p *PrettyLogger) DividerCtx(ctx context.Context) {
	writer := GetWriter(ctx)
	divider := strings.Repeat("─", 60)
	fmt.Fprintln(writer, p.styles.Key.Render(divider))
}

// Divider prints a visual divider
func (p *PrettyLogger) Divider() {
	p.DividerCtx(context.Background())
}

// BlankCtx prints a blank line to the writer from the context.
func (p *PrettyLogger) BlankCtx(ctx context.Context) {
	writer := GetWriter(ctx)
	fmt.Fprintln(writer)
}

// Blank prints a blank line
func (p *PrettyLogger) Blank() {
	p.BlankCtx(context.Background())
}

// SectionCtx prints a section header with the Grove theme to the writer from the context.
func (p *PrettyLogger) SectionCtx(ctx context.Context, title string) {
	writer := GetWriter(ctx)
	fmt.Fprintf(writer, "\n%s %s\n\n",
		theme.DefaultTheme.Header.Render(theme.IconTree),
		theme.DefaultTheme.Header.Render(title))
}

// Section prints a section header with the Grove theme
func (p *PrettyLogger) Section(title string) {
	p.SectionCtx(context.Background(), title)
}

// ProgressCtx logs a progress message to the writer from the context.
func (p *PrettyLogger) ProgressCtx(ctx context.Context, message string) {
	writer := GetWriter(ctx)
	fmt.Fprintf(writer, "%s %s\n",
		p.styles.Info.Render(theme.IconRunning),
		p.styles.Info.Render(message))
}

// Progress logs a progress message
func (p *PrettyLogger) Progress(message string) {
	p.ProgressCtx(context.Background(), message)
}

// KeyValuesCtx logs multiple key-value pairs to the writer from the context.
func (p *PrettyLogger) KeyValuesCtx(ctx context.Context, pairs map[string]interface{}) {
	for key, value := range pairs {
		p.FieldCtx(ctx, key, value)
	}
}

// KeyValue logs multiple key-value pairs
func (p *PrettyLogger) KeyValues(pairs map[string]interface{}) {
	p.KeyValuesCtx(context.Background(), pairs)
}

// ListCtx logs a list of items to the writer from the context.
func (p *PrettyLogger) ListCtx(ctx context.Context, items []string) {
	writer := GetWriter(ctx)
	for _, item := range items {
		fmt.Fprintf(writer, "  %s %s\n",
			theme.DefaultTheme.Highlight.Render(theme.IconBullet),
			item)
	}
}

// List logs a list of items
func (p *PrettyLogger) List(items []string) {
	p.ListCtx(context.Background(), items)
}

// BoxCtx prints content in a styled box to the writer from the context.
func (p *PrettyLogger) BoxCtx(ctx context.Context, title, content string) {
	writer := GetWriter(ctx)
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Padding(1, 2).
		Width(60)

	if title != "" {
		fmt.Fprintf(writer, "%s\n", theme.DefaultTheme.Header.Render(title))
	}
	fmt.Fprintf(writer, "%s\n", boxStyle.Render(content))
}

// Box prints content in a styled box
func (p *PrettyLogger) Box(title, content string) {
	p.BoxCtx(context.Background(), title, content)
}

// StatusCtx logs a status with an appropriate icon to the writer from the context.
func (p *PrettyLogger) StatusCtx(ctx context.Context, status, message string) {
	writer := GetWriter(ctx)
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

	fmt.Fprintf(writer, "%s %s\n", style.Render(icon), style.Render(message))
}

// Status logs a status with an appropriate icon
func (p *PrettyLogger) Status(status, message string) {
	p.StatusCtx(context.Background(), status, message)
}