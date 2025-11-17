package table

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	ltable "github.com/charmbracelet/lipgloss/table"
	"github.com/mattsolo1/grove-core/tui/theme"
)

// NewStyledTable creates a new lipgloss table with Grove's default styling
func NewStyledTable() *ltable.Table {
	t := theme.DefaultTheme

	// Create the table with Grove styling
	table := ltable.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(theme.Border)).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == 0 {
				// Header row with padding
				return t.TableHeader.Padding(0, 1)
			}
			// Regular rows with subtle alternating background
			baseStyle := lipgloss.NewStyle().Padding(0, 1)
			if row%2 == 0 {
				// Very subtle alternating background that won't interfere with text colors
				return baseStyle.Background(theme.VerySubtleBackground)
			}
			return baseStyle
		})

	return table
}

// Options provides additional configuration for the table
type Options struct {
	ShowRowNumbers bool
	Bordered       bool
	HeaderStyle    lipgloss.Style
	RowStyle       lipgloss.Style
	AlternateRows  bool
	Theme          *theme.Theme
}

// DefaultOptions returns the default table options
func DefaultOptions() Options {
	return Options{
		ShowRowNumbers: false,
		Bordered:       true,
		HeaderStyle:    theme.DefaultTheme.TableHeader,
		RowStyle:       theme.DefaultTheme.TableRow,
		AlternateRows:  true, // Subtle alternating with VerySubtleBackground
		Theme:          theme.DefaultTheme,
	}
}

// NewStyledTableWithOptions creates a table with custom options
func NewStyledTableWithOptions(opts Options) *ltable.Table {
	if opts.Theme == nil {
		opts.Theme = theme.DefaultTheme
	}

	table := ltable.New()

	// Apply border if requested
	if opts.Bordered {
		table = table.
			Border(lipgloss.RoundedBorder()).
			BorderStyle(lipgloss.NewStyle().Foreground(theme.Border))
	}

	// Apply styling function
	table = table.StyleFunc(func(row, col int) lipgloss.Style {
		if row == 0 {
			// Header row
			return opts.HeaderStyle
		}

		// Data rows
		style := opts.RowStyle

		// Add alternating row colors if enabled
		if opts.AlternateRows && row%2 == 0 {
			style = style.Background(theme.VerySubtleBackground)
		}

		// Highlight row numbers if enabled
		if opts.ShowRowNumbers && col == 0 {
			style = style.Foreground(theme.MutedText)
		}

		return style
	})

	return table
}

// Builder provides a fluent interface for creating styled tables
type Builder struct {
	table   *ltable.Table
	options Options
}

// NewBuilder creates a new table builder
func NewBuilder() *Builder {
	return &Builder{
		table:   ltable.New(),
		options: DefaultOptions(),
	}
}

// WithTheme sets the theme
func (b *Builder) WithTheme(t *theme.Theme) *Builder {
	b.options.Theme = t
	b.options.HeaderStyle = t.TableHeader
	b.options.RowStyle = t.TableRow
	return b
}

// WithBorder enables or disables the border
func (b *Builder) WithBorder(bordered bool) *Builder {
	b.options.Bordered = bordered
	return b
}

// WithRowNumbers enables or disables row numbers
func (b *Builder) WithRowNumbers(show bool) *Builder {
	b.options.ShowRowNumbers = show
	return b
}

// WithAlternateRows enables or disables alternating row colors
func (b *Builder) WithAlternateRows(alternate bool) *Builder {
	b.options.AlternateRows = alternate
	return b
}

// WithHeaders sets the table headers
func (b *Builder) WithHeaders(headers ...string) *Builder {
	b.table = b.table.Headers(headers...)
	return b
}

// WithRows sets the table rows
func (b *Builder) WithRows(rows ...[]string) *Builder {
	for _, row := range rows {
		b.table = b.table.Row(row...)
	}
	return b
}

// WithWidth sets specific column widths
func (b *Builder) WithWidth(width int) *Builder {
	b.table = b.table.Width(width)
	return b
}

// WithHeight sets the table height
func (b *Builder) WithHeight(height int) *Builder {
	b.table = b.table.Height(height)
	return b
}

// Build creates the styled table
func (b *Builder) Build() *ltable.Table {
	// Apply options
	if b.options.Bordered {
		b.table = b.table.
			Border(lipgloss.RoundedBorder()).
			BorderStyle(lipgloss.NewStyle().Foreground(theme.Border))
	}

	// Apply styling
	b.table = b.table.StyleFunc(func(row, col int) lipgloss.Style {
		if row == 0 {
			return b.options.HeaderStyle
		}

		style := b.options.RowStyle

		if b.options.AlternateRows && row%2 == 0 {
			style = style.Background(theme.VerySubtleBackground)
		}

		if b.options.ShowRowNumbers && col == 0 {
			style = style.Foreground(theme.MutedText)
		}

		return style
	})

	return b.table
}

// Quick helpers for common table patterns

// SimpleTable creates a basic table with headers and rows
func SimpleTable(headers []string, rows [][]string) string {
	table := NewBuilder().
		WithHeaders(headers...).
		WithRows(rows...).
		Build()

	return table.String()
}

// StatusTable creates a table for displaying status information
func StatusTable(items [][]string) string {
	table := NewBuilder().
		WithBorder(false).
		WithAlternateRows(false).
		Build()

	for _, item := range items {
		if len(item) >= 2 {
			// First column is the label (muted), second is the value
			label := theme.DefaultTheme.Muted.Render(item[0] + ":")
			value := item[1]
			table = table.Row(label, value)
		}
	}

	return table.String()
}

// SelectableTable creates a table suitable for selection interfaces
// The selection indicator (▶) is rendered on the right side of the table, outside the border
func SelectableTable(headers []string, rows [][]string, selectedIndex int) string {
	return SelectableTableWithOptions(headers, rows, selectedIndex, SelectableTableOptions{})
}

// SelectableTableOptions provides configuration for SelectableTable
type SelectableTableOptions struct {
	HighlightColumn int // Column index to highlight (0-based), -1 for no highlight
}

// SelectableTableWithOptions creates a table with custom highlighting options
func SelectableTableWithOptions(headers []string, rows [][]string, selectedIndex int, opts SelectableTableOptions) string {
	t := theme.DefaultTheme

	// Pre-style headers with the theme's TableHeader style
	styledHeaders := make([]string, len(headers))
	for i, h := range headers {
		styledHeaders[i] = t.TableHeader.Render(h)
	}

	table := ltable.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(theme.Border)).
		Headers(styledHeaders...)

	// Apply styling without selection background
	// IMPORTANT: In lipgloss table StyleFunc, when headers are set via .Headers(),
	// they are styled SEPARATELY and row indices in StyleFunc start at 0 for DATA rows only!
	// So: row 0 = first data row, row 1 = second data row, etc.
	table = table.StyleFunc(func(row, col int) lipgloss.Style {
		// Regular data rows with padding
		style := t.TableRow.Copy().Padding(0, 1)

		// Note: HighlightColumn feature removed - all columns use same style

		// Apply alternating background for even data rows (2nd, 4th, etc.)
		// Only if the theme supports it (disabled for terminal theme)
		if t.UseAlternatingRows && row%2 == 1 {
			style = style.Background(theme.VerySubtleBackground)
		}
		return style
	})

	// Add rows
	for _, r := range rows {
		table = table.Row(r...)
	}

	// Render the table and add selection indicator on the left
	tableStr := table.String()
	lines := strings.Split(tableStr, "\n")

	// Calculate which line the selected row is on
	// If headers are present:
	// Line 0: top border
	// Line 1: header row
	// Line 2: separator line after header
	// Line 3+: data rows (first data row is at index 3)
	// If no headers:
	// Line 0: top border
	// Line 1+: data rows (first data row is at index 1)
	var selectedLineIndex int
	if len(headers) > 0 {
		selectedLineIndex = 3 + selectedIndex
	} else {
		selectedLineIndex = 1 + selectedIndex
	}

	// Add the indicator to each line
	result := ""
	arrow := theme.DefaultTheme.Highlight.Render(theme.IconArrowRightBold)
	for i, line := range lines {
		// Add the indicator on the left for the selected row
		if i == selectedLineIndex {
			result += arrow + " " + line
		} else {
			result += "  " + line
		}
		result += "\n"
	}

	// Remove the trailing newline to match original behavior
	if len(result) > 0 && result[len(result)-1] == '\n' {
		result = result[:len(result)-1]
	}

	return result
}