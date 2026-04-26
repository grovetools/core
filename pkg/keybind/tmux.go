package keybind

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

func init() {
	// Check if tmux is available
	if _, err := exec.LookPath("tmux"); err == nil {
		// Check if we're in a tmux session
		if os.Getenv("TMUX") != "" {
			tmuxAvailable = true
		}
	}
}

// TmuxRootCollector collects key bindings from tmux root table.
type TmuxRootCollector struct {
	socket string
}

// NewTmuxRootCollector creates a new tmux root table binding collector.
func NewTmuxRootCollector() *TmuxRootCollector {
	return &TmuxRootCollector{
		socket: os.Getenv("GROVE_TMUX_SOCKET"),
	}
}

func (c *TmuxRootCollector) Name() string {
	return "tmux-root"
}

func (c *TmuxRootCollector) Layer() Layer {
	return LayerTmuxRoot
}

func (c *TmuxRootCollector) Collect(ctx context.Context) ([]Binding, error) {
	args := []string{"list-keys", "-T", "root"}
	if c.socket != "" {
		args = append([]string{"-L", c.socket}, args...)
	}

	cmd := exec.CommandContext(ctx, "tmux", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, nil // tmux not running or not available
	}

	return parseTmuxKeys(string(output), "tmux-root", LayerTmuxRoot, "root")
}

// TmuxPrefixCollector collects key bindings from tmux prefix table.
type TmuxPrefixCollector struct {
	socket string
}

// NewTmuxPrefixCollector creates a new tmux prefix table binding collector.
func NewTmuxPrefixCollector() *TmuxPrefixCollector {
	return &TmuxPrefixCollector{
		socket: os.Getenv("GROVE_TMUX_SOCKET"),
	}
}

func (c *TmuxPrefixCollector) Name() string {
	return "tmux-prefix"
}

func (c *TmuxPrefixCollector) Layer() Layer {
	return LayerTmuxPrefix
}

func (c *TmuxPrefixCollector) Collect(ctx context.Context) ([]Binding, error) {
	args := []string{"list-keys", "-T", "prefix"}
	if c.socket != "" {
		args = append([]string{"-L", c.socket}, args...)
	}

	cmd := exec.CommandContext(ctx, "tmux", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, nil
	}

	bindings, err := parseTmuxKeys(string(output), "tmux-prefix", LayerTmuxPrefix, "prefix")
	if err != nil {
		return nil, err
	}

	// Add known defaults that might not be listed
	defaults := GetKnownDefaults("tmux-prefix")
	for key, action := range defaults {
		found := false
		for _, b := range bindings {
			if b.Key == key {
				found = true
				break
			}
		}
		if !found && action != "" {
			bindings = append(bindings, Binding{
				Key:        key,
				RawKey:     key,
				Layer:      LayerTmuxPrefix,
				Source:     "tmux-prefix",
				Action:     action,
				Provenance: ProvenanceDefault,
				TableName:  "prefix",
			})
		}
	}

	return bindings, nil
}

// TmuxCustomCollector collects key bindings from custom tmux tables.
type TmuxCustomCollector struct {
	socket string
}

// NewTmuxCustomCollector creates a collector for custom tmux key tables.
func NewTmuxCustomCollector() *TmuxCustomCollector {
	return &TmuxCustomCollector{
		socket: os.Getenv("GROVE_TMUX_SOCKET"),
	}
}

func (c *TmuxCustomCollector) Name() string {
	return "tmux-custom"
}

func (c *TmuxCustomCollector) Layer() Layer {
	return LayerTmuxCustomTable
}

func (c *TmuxCustomCollector) Collect(ctx context.Context) ([]Binding, error) {
	// First, get all bindings to find custom tables
	args := []string{"list-keys"}
	if c.socket != "" {
		args = append([]string{"-L", c.socket}, args...)
	}

	cmd := exec.CommandContext(ctx, "tmux", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, nil
	}

	// Find all custom tables (not root or prefix)
	tables := findCustomTables(string(output))

	var allBindings []Binding
	for _, table := range tables {
		tableArgs := []string{"list-keys", "-T", table}
		if c.socket != "" {
			tableArgs = append([]string{"-L", c.socket}, tableArgs...)
		}

		tableCmd := exec.CommandContext(ctx, "tmux", tableArgs...)
		tableOutput, err := tableCmd.Output()
		if err != nil {
			continue
		}

		bindings, _ := parseTmuxKeys(string(tableOutput), "tmux-table", LayerTmuxCustomTable, table)
		allBindings = append(allBindings, bindings...)
	}

	return allBindings, nil
}

// parseTmuxKeys parses tmux list-keys output.
// Format: bind-key [-flags] [-T table] KEY COMMAND
func parseTmuxKeys(output, source string, layer Layer, tableName string) ([]Binding, error) {
	var bindings []Binding

	// Pattern to match tmux list-keys output
	// Examples:
	//   bind-key -T root C-g switch-client -T grove-popups
	//   bind-key -T prefix d detach-client
	//   bind-key    -T prefix       ? list-keys
	bindPattern := regexp.MustCompile(`bind-key\s+(?:-[rn]\s+)?(?:-T\s+(\S+)\s+)?(\S+)\s+(.+)`)

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		matches := bindPattern.FindStringSubmatch(line)
		if len(matches) < 4 {
			continue
		}

		table := matches[1]
		if table == "" {
			table = tableName
		}

		key := matches[2]
		action := strings.TrimSpace(matches[3])

		// Skip if table doesn't match expected
		if table != tableName {
			continue
		}

		normalizedKey := Normalize(key, "tmux")
		var provenance Provenance

		if strings.Contains(action, "grove") || strings.Contains(action, "flow") ||
			strings.Contains(action, "nb ") || strings.Contains(action, "nav ") ||
			strings.Contains(action, "cx ") || strings.Contains(action, "tend ") ||
			strings.Contains(action, "hooks ") {
			provenance = ProvenanceGrove
		} else if defaultAction, ok := GetDefaultBinding("tmux-prefix", normalizedKey); ok && tableName == "prefix" {
			if defaultAction == action {
				provenance = ProvenanceDefault
			} else {
				provenance = ProvenanceUserConfig
			}
		} else {
			provenance = ProvenanceUserConfig
		}

		bindings = append(bindings, Binding{
			Key:        normalizedKey,
			RawKey:     key,
			Layer:      layer,
			Source:     source,
			Action:     action,
			Provenance: provenance,
			TableName:  table,
		})
	}

	return bindings, nil
}

// findCustomTables finds non-standard tmux key tables from list-keys output.
func findCustomTables(output string) []string {
	tables := make(map[string]bool)
	standardTables := map[string]bool{
		"root":         true,
		"prefix":       true,
		"copy-mode":    true,
		"copy-mode-vi": true,
	}

	// Look for -T TABLE in bind-key lines
	tablePattern := regexp.MustCompile(`-T\s+(\S+)`)
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		matches := tablePattern.FindStringSubmatch(line)
		if len(matches) >= 2 {
			table := matches[1]
			if !standardTables[table] {
				tables[table] = true
			}
		}
	}

	// Convert to slice
	result := make([]string, 0, len(tables))
	for table := range tables {
		result = append(result, table)
	}
	return result
}

// TmuxTableCollector collects bindings from a specific tmux table.
type TmuxTableCollector struct {
	tableName string
	socket    string
}

// NewTmuxTableCollector creates a collector for a specific tmux table.
func NewTmuxTableCollector(tableName string) *TmuxTableCollector {
	return &TmuxTableCollector{
		tableName: tableName,
		socket:    os.Getenv("GROVE_TMUX_SOCKET"),
	}
}

func (c *TmuxTableCollector) Name() string {
	return "tmux-" + c.tableName
}

func (c *TmuxTableCollector) Layer() Layer {
	return LayerTmuxCustomTable
}

func (c *TmuxTableCollector) TableName() string {
	return c.tableName
}

func (c *TmuxTableCollector) Collect(ctx context.Context) ([]Binding, error) {
	args := []string{"list-keys", "-T", c.tableName}
	if c.socket != "" {
		args = append([]string{"-L", c.socket}, args...)
	}

	cmd := exec.CommandContext(ctx, "tmux", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, nil
	}

	return parseTmuxKeys(string(output), "tmux-"+c.tableName, LayerTmuxCustomTable, c.tableName)
}

// GetTmuxPrefix returns the current tmux prefix key.
func GetTmuxPrefix(ctx context.Context) (string, error) {
	socket := os.Getenv("GROVE_TMUX_SOCKET")
	args := []string{"show-option", "-gv", "prefix"}
	if socket != "" {
		args = append([]string{"-L", socket}, args...)
	}

	cmd := exec.CommandContext(ctx, "tmux", args...)
	output, err := cmd.Output()
	if err != nil {
		return "C-B", nil // default
	}

	return strings.TrimSpace(string(output)), nil
}
