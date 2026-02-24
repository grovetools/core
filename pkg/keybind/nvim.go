package keybind

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
)

// NeovimCollector gathers keybindings from a running Neovim instance.
// It runs a headless Neovim to dump all keymaps using nvim_get_keymap API.
type NeovimCollector struct{}

// NewNeovimCollector creates a new Neovim collector.
func NewNeovimCollector() *NeovimCollector {
	return &NeovimCollector{}
}

// Name returns the collector identifier.
func (c *NeovimCollector) Name() string {
	return "nvim"
}

// Layer returns the layer this collector operates on.
func (c *NeovimCollector) Layer() Layer {
	return LayerApplication
}

// Collect gathers all keybindings from Neovim.
// It runs nvim headless and uses the Lua API to dump keymaps.
func (c *NeovimCollector) Collect(ctx context.Context) ([]Binding, error) {
	// Check if nvim is available
	if _, err := exec.LookPath("nvim"); err != nil {
		return nil, nil // Nvim not installed, not an error
	}

	// Lua script to dump keymaps for all modes
	// We print JSON between markers for easy parsing
	luaScript := `
local modes = {'n', 'i', 'v', 'x', 's', 'o', 't', 'c'}
local maps = {}
for _, mode in ipairs(modes) do
	local success, result = pcall(vim.api.nvim_get_keymap, mode)
	if success then
		for _, map in ipairs(result) do
			table.insert(maps, {
				lhs = map.lhs,
				rhs = map.rhs or "",
				mode = mode,
				desc = map.desc or "",
				silent = map.silent == 1,
				noremap = map.noremap == 1,
			})
		end
	end
end
io.write("GROVE_JSON_START")
io.write(vim.json.encode(maps))
io.write("GROVE_JSON_END")
`

	// Run nvim headless with minimal config to be faster
	// Using --clean to skip user config for speed, or we could use user config
	cmd := exec.CommandContext(ctx, "nvim", "--headless", "-u", "NONE", "-c", "lua "+strings.ReplaceAll(luaScript, "\n", " "), "-c", "q")
	output, err := cmd.Output()
	if err != nil {
		// Nvim failed to run, not critical
		return nil, nil
	}

	return c.parseOutput(string(output))
}

// nvimKeymap represents a keymap entry from Neovim's API.
type nvimKeymap struct {
	Lhs     string `json:"lhs"`
	Rhs     string `json:"rhs"`
	Mode    string `json:"mode"`
	Desc    string `json:"desc"`
	Silent  bool   `json:"silent"`
	NoRemap bool   `json:"noremap"`
}

// parseOutput extracts JSON from the nvim output and converts to Bindings.
func (c *NeovimCollector) parseOutput(output string) ([]Binding, error) {
	// Find JSON between markers
	startMarker := "GROVE_JSON_START"
	endMarker := "GROVE_JSON_END"

	startIdx := strings.Index(output, startMarker)
	endIdx := strings.Index(output, endMarker)

	if startIdx == -1 || endIdx == -1 || startIdx >= endIdx {
		return nil, nil // No valid output
	}

	jsonStr := output[startIdx+len(startMarker) : endIdx]

	var maps []nvimKeymap
	if err := json.Unmarshal([]byte(jsonStr), &maps); err != nil {
		return nil, err
	}

	var bindings []Binding
	for _, m := range maps {
		// Normalize the key from nvim format
		normalizedKey := Normalize(m.Lhs, "nvim")

		// Build description from mode and desc
		desc := m.Desc
		if desc == "" && m.Rhs != "" {
			desc = m.Rhs
		}

		bindings = append(bindings, Binding{
			Key:         normalizedKey,
			RawKey:      m.Lhs,
			Layer:       LayerApplication,
			Source:      "nvim",
			Action:      m.Rhs,
			Description: desc,
			Provenance:  ProvenanceUserConfig,
		})
	}

	return bindings, nil
}

// IsNeovimAvailable checks if nvim is installed.
func IsNeovimAvailable() bool {
	_, err := exec.LookPath("nvim")
	return err == nil
}
