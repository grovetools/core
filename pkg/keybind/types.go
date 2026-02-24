// Package keybind provides cross-layer key binding detection and orchestration.
// It aggregates bindings from OS, terminal, shell, tmux, and applications,
// enabling conflict detection, key tracing, and availability analysis.
package keybind

// Layer represents a key processing layer in the terminal stack.
// When a key is pressed, it traverses these layers in order; each layer
// can consume the key or pass it through.
type Layer int

const (
	// LayerOS represents system-level shortcuts (Cmd+Space, Cmd+Tab on macOS).
	// These are consumed before reaching the terminal.
	LayerOS Layer = iota
	// LayerTerminal represents terminal emulator shortcuts (Cmd+K, Cmd+D in iTerm2).
	// These are handled by the terminal app itself.
	LayerTerminal
	// LayerShell represents shell readline bindings (fish/bash/zsh).
	// These are active when the shell has focus.
	LayerShell
	// LayerTmuxRoot represents tmux root table bindings (-n bindings).
	// These intercept keys before the shell sees them.
	LayerTmuxRoot
	// LayerTmuxPrefix represents tmux prefix table bindings.
	// Active after the prefix key (C-b by default) is pressed.
	LayerTmuxPrefix
	// LayerTmuxCustomTable represents custom tmux tables (grove-popups, nav-workspaces).
	// Entered via switch-client -T from other tables.
	LayerTmuxCustomTable
	// LayerApplication represents focused application bindings (neovim, TUIs).
	// Only receives keys if lower layers pass them through.
	LayerApplication
)

// String returns a human-readable name for the layer.
func (l Layer) String() string {
	switch l {
	case LayerOS:
		return "OS"
	case LayerTerminal:
		return "Terminal"
	case LayerShell:
		return "Shell"
	case LayerTmuxRoot:
		return "Tmux Root"
	case LayerTmuxPrefix:
		return "Tmux Prefix"
	case LayerTmuxCustomTable:
		return "Tmux Custom"
	case LayerApplication:
		return "Application"
	default:
		return "Unknown"
	}
}

// ShortName returns a short layer identifier for compact display.
func (l Layer) ShortName() string {
	switch l {
	case LayerOS:
		return "L0"
	case LayerTerminal:
		return "L1"
	case LayerShell:
		return "L2"
	case LayerTmuxRoot:
		return "L3"
	case LayerTmuxPrefix:
		return "L4"
	case LayerTmuxCustomTable:
		return "L5"
	case LayerApplication:
		return "L6"
	default:
		return "L?"
	}
}

// Provenance tracks where a binding came from.
type Provenance int

const (
	// ProvenanceDefault indicates a built-in default binding (readline, tmux defaults).
	ProvenanceDefault Provenance = iota
	// ProvenanceUserConfig indicates a user's config file (.tmux.conf, config.fish).
	ProvenanceUserConfig
	// ProvenanceGrove indicates the binding is managed by Grove.
	ProvenanceGrove
	// ProvenanceDetected indicates the binding was detected but not yet categorized.
	ProvenanceDetected
)

// String returns a human-readable name for the provenance.
func (p Provenance) String() string {
	switch p {
	case ProvenanceDefault:
		return "default"
	case ProvenanceUserConfig:
		return "user config"
	case ProvenanceGrove:
		return "grove"
	case ProvenanceDetected:
		return "detected"
	default:
		return "unknown"
	}
}

// Binding represents a key binding at any layer.
type Binding struct {
	// Key is the standardized key notation (e.g., "C-p", "M-f").
	Key string
	// RawKey is the original notation from the source (e.g., "\cp", "^P").
	RawKey string
	// Layer is the processing layer where this binding exists.
	Layer Layer
	// Source identifies where the binding came from (e.g., "fish", "tmux-root", "grove").
	Source string
	// Action is the command or function bound to the key.
	Action string
	// Description is a human-readable description of what the binding does.
	Description string
	// Provenance indicates how the binding was established.
	Provenance Provenance
	// ConfigFile is the path to the config file where this binding is defined.
	ConfigFile string
	// TableName is the tmux table name (for LayerTmuxCustomTable bindings).
	TableName string
}

// Conflict represents a key bound at multiple layers where one shadows another.
type Conflict struct {
	// Key is the conflicting key combination.
	Key string
	// Bindings are all bindings that use this key across layers.
	Bindings []Binding
	// Severity indicates how problematic the conflict is.
	Severity ConflictSeverity
	// Description explains the conflict.
	Description string
}

// ConflictSeverity indicates how problematic a conflict is.
type ConflictSeverity int

const (
	// SeverityInfo indicates informational overlap (no functional conflict).
	SeverityInfo ConflictSeverity = iota
	// SeverityWarning indicates a binding that shadows another.
	SeverityWarning
	// SeverityError indicates a critical conflict that will cause issues.
	SeverityError
)

// String returns a human-readable severity name.
func (s ConflictSeverity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarning:
		return "warning"
	case SeverityError:
		return "error"
	default:
		return "unknown"
	}
}

// Stack represents the full binding environment across all layers.
type Stack struct {
	// Layers maps each layer to its bindings.
	Layers map[Layer][]Binding
	// CustomTables maps table names to their bindings (for LayerTmuxCustomTable).
	CustomTables map[string][]Binding
}

// NewStack creates an empty binding stack.
func NewStack() *Stack {
	return &Stack{
		Layers:       make(map[Layer][]Binding),
		CustomTables: make(map[string][]Binding),
	}
}

// AddBinding adds a binding to the stack.
func (s *Stack) AddBinding(b Binding) {
	s.Layers[b.Layer] = append(s.Layers[b.Layer], b)
	if b.Layer == LayerTmuxCustomTable && b.TableName != "" {
		s.CustomTables[b.TableName] = append(s.CustomTables[b.TableName], b)
	}
}

// GetBindings returns all bindings at a specific layer.
func (s *Stack) GetBindings(layer Layer) []Binding {
	return s.Layers[layer]
}

// GetTableBindings returns all bindings for a specific tmux custom table.
func (s *Stack) GetTableBindings(tableName string) []Binding {
	return s.CustomTables[tableName]
}

// AllBindings returns all bindings across all layers.
func (s *Stack) AllBindings() []Binding {
	var all []Binding
	for _, bindings := range s.Layers {
		all = append(all, bindings...)
	}
	return all
}

// TraceStep represents one step in a key traversal trace.
type TraceStep struct {
	// Key is the key being processed in this step.
	Key string
	// Layer is the layer being checked.
	Layer Layer
	// TableName is the custom table name (if Layer is LayerTmuxCustomTable).
	TableName string
	// Result indicates what happened at this layer.
	Result TraceResult
	// Binding is the matched binding (if Result is TraceConsumed).
	Binding *Binding
	// NextTable is the table to enter (if Result is TraceEntersTable).
	NextTable string
}

// TraceResult indicates what happened when a key was checked at a layer.
type TraceResult int

const (
	// TracePassthrough means the layer did not have a binding for this key.
	TracePassthrough TraceResult = iota
	// TraceConsumed means the layer consumed the key.
	TraceConsumed
	// TraceEntersTable means the key enters a custom tmux table.
	TraceEntersTable
	// TraceNotReached means this layer was never reached (earlier layer consumed).
	TraceNotReached
)

// String returns a human-readable trace result.
func (r TraceResult) String() string {
	switch r {
	case TracePassthrough:
		return "passthrough"
	case TraceConsumed:
		return "consumed"
	case TraceEntersTable:
		return "enters table"
	case TraceNotReached:
		return "not reached"
	default:
		return "unknown"
	}
}

// Trace holds the complete trace of a key sequence through the layer stack.
type Trace struct {
	// Keys is the input key sequence.
	Keys []string
	// Steps is the sequence of trace steps.
	Steps []TraceStep
	// FinalResult summarizes what ultimately happened.
	FinalResult string
}
