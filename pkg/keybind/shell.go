package keybind

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"strings"
)

// DetectShell returns the current shell name ("fish", "bash", "zsh").
func DetectShell() string {
	shell := os.Getenv("SHELL")
	if strings.Contains(shell, "fish") {
		return "fish"
	}
	if strings.Contains(shell, "zsh") {
		return "zsh"
	}
	if strings.Contains(shell, "bash") {
		return "bash"
	}
	// Default to bash if unknown
	return "bash"
}

// FishCollector collects key bindings from fish shell.
type FishCollector struct{}

// NewFishCollector creates a new fish shell binding collector.
func NewFishCollector() *FishCollector {
	return &FishCollector{}
}

func (c *FishCollector) Name() string {
	return "fish"
}

func (c *FishCollector) Layer() Layer {
	return LayerShell
}

func (c *FishCollector) Collect(ctx context.Context) ([]Binding, error) {
	var bindings []Binding

	// First, add known defaults
	defaults := GetKnownDefaults("fish-emacs")
	for key, action := range defaults {
		if action != "" {
			bindings = append(bindings, Binding{
				Key:        key,
				RawKey:     key,
				Layer:      LayerShell,
				Source:     "fish",
				Action:     action,
				Provenance: ProvenanceDefault,
			})
		}
	}

	// Then run fish -c "bind" to get current bindings
	cmd := exec.CommandContext(ctx, "fish", "-c", "bind")
	output, err := cmd.Output()
	if err != nil {
		// Fish not available, return just defaults
		return bindings, nil
	}

	// Parse bind output
	// Format: bind KEY COMMAND
	// Example: bind \cp up-or-search
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 3 || parts[0] != "bind" {
			continue
		}

		rawKey := parts[1]
		action := strings.Join(parts[2:], " ")

		// Skip function bindings (like --mode)
		if strings.HasPrefix(rawKey, "--") {
			continue
		}

		normalizedKey := Normalize(rawKey, "fish")
		binding := Binding{
			Key:        normalizedKey,
			RawKey:     rawKey,
			Layer:      LayerShell,
			Source:     "fish",
			Action:     action,
			Provenance: ProvenanceDetected,
		}

		// Check if this overrides a default
		if defaultAction, ok := GetDefaultBinding("fish-emacs", normalizedKey); ok {
			if defaultAction == action {
				binding.Provenance = ProvenanceDefault
			} else {
				binding.Provenance = ProvenanceUserConfig
			}
		}

		// Update existing binding if it's a default, or add new one
		found := false
		for i, b := range bindings {
			if b.Key == normalizedKey {
				bindings[i] = binding
				found = true
				break
			}
		}
		if !found {
			bindings = append(bindings, binding)
		}
	}

	return bindings, nil
}

// BashCollector collects key bindings from bash shell.
type BashCollector struct{}

// NewBashCollector creates a new bash shell binding collector.
func NewBashCollector() *BashCollector {
	return &BashCollector{}
}

func (c *BashCollector) Name() string {
	return "bash"
}

func (c *BashCollector) Layer() Layer {
	return LayerShell
}

func (c *BashCollector) Collect(ctx context.Context) ([]Binding, error) {
	var bindings []Binding

	// First, add known defaults
	defaults := GetKnownDefaults("bash-emacs")
	for key, action := range defaults {
		if action != "" {
			bindings = append(bindings, Binding{
				Key:        key,
				RawKey:     key,
				Layer:      LayerShell,
				Source:     "bash",
				Action:     action,
				Provenance: ProvenanceDefault,
			})
		}
	}

	// Run bash -c "bind -p" to get bindings
	cmd := exec.CommandContext(ctx, "bash", "-c", "bind -p")
	output, err := cmd.Output()
	if err != nil {
		return bindings, nil
	}

	// Parse bind -p output
	// Format: "KEY": FUNCTION
	// Example: "\C-p": previous-history
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Skip unset bindings
		if strings.Contains(line, "self-insert") || strings.Contains(line, "not bound") {
			continue
		}

		// Parse "KEY": function
		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			continue
		}

		rawKey := strings.Trim(strings.TrimSpace(line[:colonIdx]), "\"")
		action := strings.TrimSpace(line[colonIdx+1:])

		normalizedKey := Normalize(rawKey, "bash")
		binding := Binding{
			Key:        normalizedKey,
			RawKey:     rawKey,
			Layer:      LayerShell,
			Source:     "bash",
			Action:     action,
			Provenance: ProvenanceDetected,
		}

		// Check if this is a default
		if defaultAction, ok := GetDefaultBinding("bash-emacs", normalizedKey); ok {
			if defaultAction == action {
				binding.Provenance = ProvenanceDefault
			} else {
				binding.Provenance = ProvenanceUserConfig
			}
		}

		// Update or add
		found := false
		for i, b := range bindings {
			if b.Key == normalizedKey {
				bindings[i] = binding
				found = true
				break
			}
		}
		if !found {
			bindings = append(bindings, binding)
		}
	}

	return bindings, nil
}

// ZshCollector collects key bindings from zsh shell.
type ZshCollector struct{}

// NewZshCollector creates a new zsh shell binding collector.
func NewZshCollector() *ZshCollector {
	return &ZshCollector{}
}

func (c *ZshCollector) Name() string {
	return "zsh"
}

func (c *ZshCollector) Layer() Layer {
	return LayerShell
}

func (c *ZshCollector) Collect(ctx context.Context) ([]Binding, error) {
	var bindings []Binding

	// First, add known defaults
	defaults := GetKnownDefaults("zsh-emacs")
	for key, action := range defaults {
		if action != "" {
			bindings = append(bindings, Binding{
				Key:        key,
				RawKey:     key,
				Layer:      LayerShell,
				Source:     "zsh",
				Action:     action,
				Provenance: ProvenanceDefault,
			})
		}
	}

	// Run zsh -c "bindkey" to get bindings
	cmd := exec.CommandContext(ctx, "zsh", "-c", "bindkey")
	output, err := cmd.Output()
	if err != nil {
		return bindings, nil
	}

	// Parse bindkey output
	// Format: "KEY" widget
	// Example: "^P" up-line-or-history
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Find quoted key
		if !strings.HasPrefix(line, "\"") {
			continue
		}

		endQuote := strings.Index(line[1:], "\"")
		if endQuote == -1 {
			continue
		}

		rawKey := line[1 : endQuote+1]
		action := strings.TrimSpace(line[endQuote+3:])

		// Skip self-insert and undefined
		if action == "self-insert" || action == "undefined-key" {
			continue
		}

		normalizedKey := Normalize(rawKey, "zsh")
		binding := Binding{
			Key:        normalizedKey,
			RawKey:     rawKey,
			Layer:      LayerShell,
			Source:     "zsh",
			Action:     action,
			Provenance: ProvenanceDetected,
		}

		// Check if this is a default
		if defaultAction, ok := GetDefaultBinding("zsh-emacs", normalizedKey); ok {
			if defaultAction == action {
				binding.Provenance = ProvenanceDefault
			} else {
				binding.Provenance = ProvenanceUserConfig
			}
		}

		// Update or add
		found := false
		for i, b := range bindings {
			if b.Key == normalizedKey {
				bindings[i] = binding
				found = true
				break
			}
		}
		if !found {
			bindings = append(bindings, binding)
		}
	}

	return bindings, nil
}
