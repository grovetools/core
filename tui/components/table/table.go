package table

import (
	"github.com/charmbracelet/lipgloss"
	ltable "github.com/charmbracelet/lipgloss/table"
	"github.com/groveorg/grove-core/tui/theme"
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
				// Header row
				return t.TableHeader
			}
			// Regular rows
			if row%2 == 0 {
				// Subtle alternating row colors for better readability
				return t.TableRow.Background(theme.SubtleBackground)
			}
			return t.TableRow
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
		AlternateRows:  true,
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
			style = style.Background(theme.SubtleBackground)
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
			style = style.Background(theme.SubtleBackground)
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
func SelectableTable(headers []string, rows [][]string, selectedIndex int) string {
	t := theme.DefaultTheme
	table := ltable.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(theme.Border)).
		Headers(headers...)

	// Apply styling with selection highlight
	table = table.StyleFunc(func(row, col int) lipgloss.Style {
		if row == 0 {
			// Header
			return t.TableHeader
		}

		// Check if this is the selected row (row-1 because headers are row 0)
		if row-1 == selectedIndex {
			return t.Selected
		}

		// Regular row
		if row%2 == 0 {
			return t.TableRow.Background(theme.SubtleBackground)
		}
		return t.TableRow
	})

	// Add rows
	for _, r := range rows {
		table = table.Row(r...)
	}

	return table.String()
}