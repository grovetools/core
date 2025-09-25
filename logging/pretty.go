package logging

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// PrettyLogger provides pretty formatted console output
type PrettyLogger struct {
	writer io.Writer
	styles PrettyStyles
}

// PrettyStyles contains lipgloss styles for different log types
type PrettyStyles struct {
	Success lipgloss.Style
	Info    lipgloss.Style
	Warning lipgloss.Style
	Error   lipgloss.Style
	Icon    lipgloss.Style
	Key     lipgloss.Style
	Value   lipgloss.Style
	Path    lipgloss.Style
	Code    lipgloss.Style
}

// DefaultPrettyStyles returns the default styling for pretty logs
func DefaultPrettyStyles() PrettyStyles {
	return PrettyStyles{
		Success: lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true),  // Green
		Info:    lipgloss.NewStyle().Foreground(lipgloss.Color("12")),              // Blue
		Warning: lipgloss.NewStyle().Foreground(lipgloss.Color("11")),              // Yellow
		Error:   lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true),    // Red
		Icon:    lipgloss.NewStyle(),
		Key:     lipgloss.NewStyle().Foreground(lipgloss.Color("8")),               // Gray
		Value:   lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true),  // Cyan
		Path:    lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Italic(true), // Dark cyan
		Code:    lipgloss.NewStyle().Foreground(lipgloss.Color("5")),               // Magenta
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