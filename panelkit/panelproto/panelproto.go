// Package panelproto defines the embed-over-socket panel protocol
// ("embed/v1"): the control plane a sidecar panel speaks to its treemux host.
//
// A protocol panel has two planes:
//
//   - The RENDERING plane is the PTY. The sidecar is spawned exactly like a
//     plain [tui.plugins] entry and draws by writing ANSI to its terminal; the
//     ghostty grid the host already owns renders it. Nothing in this package
//     touches that plane.
//   - The CONTROL plane is a per-panel unix socket. The host creates it,
//     passes its path down in GROVE_PANEL_SOCKET, and JSON-lines frames flow
//     both ways over it. That is what this package defines.
//
// The vocabulary is a serializable subset of core/tui/embed's in-process
// message set (see treemux/docs/panel-protocol-v1.md for the mapping, the v2
// backlog, and the audit of which embed messages cannot cross a process
// boundary).
//
// The package pulls in nothing but the standard library and
// core/tui/hostedkeys (itself stdlib-only), so a Go sidecar can import it
// without bubbletea, ghostty, a config loader or a TUI runtime. A sidecar in
// any other language needs none of it — the wire format is the contract, and
// treemux/examples/grove-panel-sh implements it in bash and jq.
//
// # Why this is in core and not in the host
//
// It lived in treemux until the reference panel proved the cost: a Go panel
// imported core for the widgets and treemux for the protocol, and treemux is
// untagged, so the second import could only be satisfied by a `replace`
// pointing at a local checkout. The protocol never depended on the host —
// stdlib plus hostedkeys is the whole graph — so it moved to the module a
// panel already has to import. treemux/pkg/panelproto is a frozen alias of
// this package, kept for one release.
package panelproto

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/grovetools/core/tui/hostedkeys"
)

// Version is the protocol identifier carried in the handshake and declared by
// a plugin manifest as `protocol = "embed/v1"`.
const Version = "embed/v1"

// Environment variables the host sets on a protocol panel's child process.
// A sidecar that finds EnvSocket unset is running as a plain PTY plugin and
// must still work — degrading to render-only is the contract, not an error.
const (
	// EnvSocket is the absolute path of the per-panel control socket.
	EnvSocket = "GROVE_PANEL_SOCKET"
	// EnvPanelID is the host's session window ID for this panel
	// ("plugin-<name>"), the same ID a NavigateMsg would address.
	EnvPanelID = "GROVE_PANEL_ID"
	// EnvProtocol is the protocol version the host is offering, so a sidecar
	// can refuse a version it does not implement before dialing.
	EnvProtocol = "GROVE_PANEL_PROTOCOL"
	// EnvTheme and EnvIcons are the ecosystem-wide appearance variables every
	// grove program already honors (core/tui/theme reads them at init). The
	// host sets them on a panel child for the same reason it sets them on an
	// editor pane: a sidecar built on core's theme package is then themed
	// correctly on its FIRST frame, before it has dialed anything.
	//
	// They are not a second source of truth. The control plane's theme and
	// icons frames are authoritative and are the only way to hear about a
	// LIVE change; these two only remove the flash of wrong glyphs that a
	// handshake-shaped delay would otherwise guarantee. A sidecar in another
	// language can read them directly and skip the socket entirely.
	EnvTheme = "GROVE_THEME"
	EnvIcons = "GROVE_ICONS"
)

// Frame types, app→host.
const (
	// TypeHello is the first frame on a fresh connection. Payload: Hello.
	TypeHello = "hello"
	// TypeNavigate asks the host to focus a panel (and optionally a tab).
	// Payload: Navigate. Maps to embed.NavigateMsg.
	TypeNavigate = "navigate"
	// TypeCloseRequest asks the host to close this panel. No payload.
	// Maps to embed.CloseRequestMsg, re-addressed to the emitting panel.
	TypeCloseRequest = "close_request"
	// TypeDone reports terminal completion of the sidecar's primary
	// lifecycle. Payload: Done. Maps to embed.DoneMsg.
	TypeDone = "done"
	// TypeKeys re-declares the sidecar's key claims mid-flight, replacing
	// whatever Hello declared. Payload: KeyReference.
	TypeKeys = "keys"
	// TypeLog writes one diagnostic line to the host's log. Payload: Log.
	TypeLog = "log"
)

// Frame types, host→app.
const (
	// TypeWelcome answers Hello with the accepted version, the key claims the
	// host actually granted, and the initial host state. Payload: Welcome.
	TypeWelcome = "welcome"
	// TypeFocus says this panel gained input focus. No payload.
	// Maps to embed.FocusMsg.
	//
	// Deprecated: focus belongs in band. The host now emits mode-1004 focus
	// reports (CSI I / CSI O) into every pane's PTY, which is the standard every
	// mainstream framework already decodes — bubbletea's WithReportFocus,
	// crossterm's EnableFocusChange, textual's AppFocus. Still delivered for the
	// whole of v1 so no shipped sidecar breaks; removed in v2. New panels should
	// enable CSI ? 1004 h and read focus from their own input stream.
	TypeFocus = "focus"
	// TypeBlur says this panel lost input focus. No payload.
	// Maps to embed.BlurMsg.
	//
	// Deprecated: see TypeFocus.
	TypeBlur = "blur"
	// TypeWorkspace repoints a workspace-scoped sidecar. Payload: Workspace.
	// Maps to embed.SetWorkspaceMsg (the node projected to a flat DTO).
	TypeWorkspace = "workspace"
	// TypeWorkspacesUpdated says the workspace set changed and any cached
	// view of it is stale. No payload. Maps to embed.WorkspacesUpdatedMsg.
	TypeWorkspacesUpdated = "workspaces_updated"
	// TypeTheme announces a host re-theme. Payload: Theme.
	TypeTheme = "theme"
	// TypeIcons announces an icon-set change. Payload: Icons. Sent when the
	// user switches between the Nerd Font and ASCII sets at runtime; the
	// initial mode arrives in Welcome.Icons.
	TypeIcons = "icons_changed"
	// TypeConfig announces that this panel's configuration changed —
	// [tui.plugins.<name>] was edited and the daemon's config_reload landed.
	// Payload: Config. The initial values arrive in Welcome.
	//
	// This is the one host→app frame that carries user-authored data rather
	// than host state, and it is the reason a panel can be configured without
	// being restarted: `args` and `env` are fixed at spawn, so before this the
	// only way to change a panel's behavior was to kill it.
	TypeConfig = "config_changed"
	// TypeClose tells the sidecar it is being shut down. No payload. The host
	// closes the PTY and kills the process immediately after sending it — there
	// is no grace period, so this is a notice, not a negotiation, and not a
	// window to do work in. Persist as you go and report results with TypeDone
	// from your own quit path instead. Delivered whatever capabilities were
	// declared: welcome and close bracket every connection.
	TypeClose = "close"
	// TypeError reports a protocol-level fault. Payload: Error. The host
	// closes the connection after sending one.
	TypeError = "error"
)

// Frame is the wire envelope. Exactly one JSON object per line; Payload is
// omitted for the frames that carry none.
type Frame struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Hello is the sidecar's opening declaration.
type Hello struct {
	// Protocol must equal Version. A mismatch is answered with TypeError
	// (CodeVersionMismatch) and the connection is closed.
	Protocol string `json:"protocol"`
	// App is the sidecar's own name, used in host logs and diagnostics.
	App string `json:"app"`
	// Version is the sidecar's build version. Informational.
	Version string `json:"version,omitempty"`
	// Capabilities names the host→app frames the sidecar wants delivered.
	// The host sends only the intersection of this and what it supports, so a
	// sidecar that declares nothing gets nothing but Welcome and Close.
	Capabilities []string `json:"capabilities,omitempty"`
	// Keys is the sidecar's key-claim declaration. It is a REQUEST: the host
	// filters it and reports what it granted in Welcome.AcceptedKeys.
	Keys *KeyReference `json:"keys,omitempty"`
}

// Capability names a sidecar may declare in Hello.Capabilities. They gate
// host→app traffic only; app→host frames are always accepted.
const (
	// CapFocus subscribes to TypeFocus and TypeBlur.
	//
	// Deprecated: with the frames it gates. Declaring it is harmless and covers
	// a host that predates in-band focus; a panel on a current host should read
	// focus from its PTY instead. See TypeFocus.
	CapFocus     = "focus"
	CapWorkspace = "workspace" // TypeWorkspace, TypeWorkspacesUpdated
	CapTheme     = "theme"     // TypeTheme
	CapIcons     = "icons"     // TypeIcons
	CapSettings  = "settings"  // TypeConfig
	// CapCloseHooks declares that the sidecar means to act on TypeClose rather
	// than simply be killed. The host echoes it in
	// Welcome.AcceptedCapabilities and sends TypeClose either way: declaring it
	// is a statement of intent, not a subscription, and it earns no time to act
	// — the kill follows the frame immediately (see TypeClose). Anything that
	// must survive shutdown belongs on the sidecar's own quit path, not here.
	CapCloseHooks = "closehooks"
)

// Welcome is the host's answer to Hello.
type Welcome struct {
	// Protocol is the version the host accepted, always Version in v1.
	Protocol string `json:"protocol"`
	// Host names the host implementation ("treemux"); a future desktop shell
	// implementing this same contract would put its own name here.
	Host string `json:"host"`
	// HostVersion is the host's build version. Informational.
	HostVersion string `json:"host_version,omitempty"`
	// PanelID is the host's ID for this panel, matching EnvPanelID.
	PanelID string `json:"panel_id"`
	// AcceptedCapabilities is the subset of Hello.Capabilities the host will
	// actually honor.
	AcceptedCapabilities []string `json:"accepted_capabilities,omitempty"`
	// AcceptedKeys are the chords the host will defer to this panel while it
	// holds focus.
	AcceptedKeys []string `json:"accepted_keys,omitempty"`
	// RejectedKeys explains every declared chord the host did NOT grant, so a
	// sidecar can rebind rather than silently losing a binding.
	RejectedKeys []RejectedKey `json:"rejected_keys,omitempty"`
	// Workspace is the host's current workspace scope, or nil if the host has
	// none yet. Equivalent to an immediate TypeWorkspace.
	Workspace *Workspace `json:"workspace,omitempty"`
	// Focused reports whether this panel holds input focus right now, so a
	// late-connecting sidecar does not have to wait for the next transition.
	Focused bool `json:"focused"`
	// Theme is the host's current theme. Equivalent to an immediate TypeTheme.
	Theme *Theme `json:"theme,omitempty"`
	// Icons is the host's current icon set. Equivalent to an immediate
	// TypeIcons. nil from a host that does not deliver icon mode, which a
	// sidecar should read as "assume nothing and use ASCII" rather than as
	// "Nerd Font": glyphs that render as tofu are worse than plain ones.
	Icons *Icons `json:"icons,omitempty"`
	// Label is the human-readable name the host shows for this panel
	// ([tui.plugins.<name>].label, falling back to the config key). A sidecar
	// that titles its own view can match what the rail says.
	Label string `json:"label,omitempty"`
	// Settings is the free-form [tui.plugins.<name>.settings] table, verbatim.
	// Equivalent to an immediate TypeConfig. See Config and Settings.
	Settings Settings `json:"settings,omitempty"`
}

// Config projects the handshake's user-configuration fields onto the same
// Config a TypeConfig frame carries.
//
// Welcome and TypeConfig deliver the same two facts — this panel's label and
// its settings table — and a panel that reads them at two entry points writes
// its decode twice or forgets to. This makes both entry points one type, so
// there is one apply function with one signature:
//
//	case sidecar.WelcomeMsg:  m.apply(msg.Welcome.Config())
//	case sidecar.SettingsMsg: m.apply(msg.Config)
//
// Nil-safe, because the welcome a sidecar holds is a pointer that is nil when
// there is no host: a panel running as a plain PTY plugin gets an empty Config
// and keeps its own defaults, which is the correct answer for that mode.
func (w *Welcome) Config() Config {
	if w == nil {
		return Config{}
	}
	return Config{Label: w.Label, Settings: w.Settings}
}

// Size is a pane's size in terminal cells.
//
// It is NOT a wire field: no frame carries a size, and Welcome deliberately no
// longer does. A pane is a PTY, so its size is a question the child's own
// TIOCGWINSZ answers at t=0 and SIGWINCH answers thereafter — the host cannot
// know it better, and a size racing on a second transport is how a panel ends
// up laid out against a size its terminal disagrees with. The type survives
// because the SDK reports the size it measured in it (sidecar.Terminal.Resizes).
type Size struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

// Icons is the host's active icon set.
//
// It exists because glyph availability is a property of the user's TERMINAL,
// not of the sidecar: a panel that hard-codes Nerd Font glyphs renders tofu
// for every user without the font, and one that hard-codes ASCII looks broken
// next to host chrome that does not. The host already knows the answer — it
// resolved it from GROVE_ICONS or [tui].icons at startup — so it says so.
type Icons struct {
	// Mode is IconsNerd or IconsASCII. An unrecognized value should be read
	// as IconsASCII: the safe direction to fail is toward glyphs that always
	// render.
	Mode string `json:"mode"`
}

// Icon-set modes, matching core/tui/theme's vocabulary and the [tui].icons
// config value.
const (
	IconsNerd  = "nerd"
	IconsASCII = "ascii"
)

// Config carries this panel's user configuration: the free-form settings
// table and the display label. Sent as TypeConfig when [tui.plugins.<name>]
// changes, and replayed in Welcome.
//
// Settings is delivered VERBATIM and the host never interprets it. That is the
// whole contract: grove cannot know what a third-party panel's options mean,
// so it does not try, and the panel owns validation of its own keys. A missing
// or malformed setting is the panel's problem to report, in its own pane,
// where the user is already looking. Settings.Decode and the typed accessors
// beside it are what a panel validates WITH — in particular they close the
// float64 trap that makes a hand-rolled `.(int)` miss a correct value.
//
// A settings table reaching a panel came from a config layer the user owns:
// core/config's exec gate quarantines the whole `tui.plugins.*` entry from
// repo-controlled layers, so a settings table cannot arrive without the
// `command` that reads it having been trusted first.
type Config struct {
	Label    string   `json:"label,omitempty"`
	Settings Settings `json:"settings,omitempty"`
}

// KeyReference is the sidecar's machine-readable key contract, KeyBinding is
// one row of it, and RejectedKey is the host's per-chord refusal.
//
// All three are the shared shapes from core/tui/hostedkeys — the same types
// flow's in-process panel publishes (flow/pkg/tui/view.HostedKeyReference) and
// the same ones treemux's arbitration consumes
// (treemux/pkg/keymap.GrantHostedClaims). They were field-for-field copies
// during the spike so the JSON would line up without a shared package; the
// package now exists, and it is stdlib-only for the same reason this one is —
// importing it costs a sidecar nothing but the types.
type (
	KeyReference = hostedkeys.Reference
	KeyBinding   = hostedkeys.Binding
	RejectedKey  = hostedkeys.Rejection
)

// Key-rejection reasons, re-exported so a sidecar can branch on them without a
// second import. See core/tui/hostedkeys for what each one means.
const (
	ReasonNonDeferrable = hostedkeys.ReasonNonDeferrable
	ReasonNoCollision   = hostedkeys.ReasonNoCollision
	ReasonHostWins      = hostedkeys.ReasonHostWins
	ReasonContextual    = hostedkeys.ReasonContextual
)

// Workspace is the flat projection of embed.SetWorkspaceMsg's
// *workspace.WorkspaceNode. Only the identity fields cross the boundary; a
// sidecar that needs more reads it from disk or from a future daemon
// capability (out of scope for v1 — see the security note in the spec).
type Workspace struct {
	Name                string `json:"name"`
	Path                string `json:"path"`
	Kind                string `json:"kind,omitempty"`
	ParentProjectPath   string `json:"parent_project_path,omitempty"`
	ParentEcosystemPath string `json:"parent_ecosystem_path,omitempty"`
	RootEcosystemPath   string `json:"root_ecosystem_path,omitempty"`
}

// Theme carries the host's active theme as RESOLVED colors — hex strings or
// ANSI indices a sidecar can write straight into an SGR sequence.
//
// Resolved is the operative word. The host's palette holds adaptive colors
// that pick a value from the detected terminal background, and a sidecar
// cannot do that resolution itself. Appearance travels alongside for the same
// reason: once a color is resolved it no longer says which canvas it was
// resolved for, and a sidecar choosing its own shades needs to know.
//
// It is deliberately NOT the whole palette. The tokens below are the semantic
// projection treemux already hands its own framework chrome
// (internal/app.muxTheme → tuimux.Theme), so a sidecar styled from them is
// styled from the same roles the pane borders and overlays around it use. The
// palette itself is a host-internal structure that changes shape between
// releases, and a wire contract mirroring it would break on every refactor.
//
// Every field is additive and optional. A host that sends only the original
// five is still speaking embed/v1, and a sidecar reading only those five keeps
// working against a host that sends all twelve.
type Theme struct {
	// Name is the resolved theme name ("kanagawa", "gruvbox-light").
	Name string `json:"name"`
	// Appearance is IconsNerd's counterpart for color: "dark" or "light",
	// describing the canvas the colors below were resolved against. Empty from
	// a host that predates the field.
	//
	// It is now ALSO answerable in band: the host installs the color-scheme
	// report, so CSI ? 996 n gets a reply, and OSC 11 ; ? returns the actual
	// background. This field stays because CSI ? 996 n is young enough that a
	// one-field convenience is cheap insurance, and because a sidecar reading
	// the twelve tokens below is already parsing this frame.
	Appearance string `json:"appearance,omitempty"`

	// --- the original five, unchanged in meaning ---------------------------

	// Background is the host's subtle panel background.
	Background string `json:"background,omitempty"`
	// Foreground is ordinary text. Same role as Text in the projection below;
	// both are sent so neither vocabulary has a hole in it.
	Foreground string `json:"foreground,omitempty"`
	// Accent is the HOST CHROME's accent — the color treemux's focused pane
	// border, tab bar and preview headers use (tuimux.Theme.Accent).
	//
	// Note for anyone joining this against the config vocabulary: writing
	// `color = "accent"` in grove.toml resolves through
	// theme.Colors.ResolveColor, which maps the NAME "accent" to the palette's
	// violet. Those are two different roles that were given the same word, and
	// the wire deliberately keeps the chrome one: a sidecar tinted with it
	// matches the frame drawn around it, which is the entire point of sending
	// a theme to a panel. The config-facing role is reachable as a palette
	// color by name and has no business changing what a panel looks like.
	Accent string `json:"accent,omitempty"`
	// Muted is de-emphasized text.
	Muted string `json:"muted,omitempty"`
	// Border is the panel/separator border color.
	Border string `json:"border,omitempty"`

	// --- the rest of the semantic projection -------------------------------

	// Text is ordinary foreground text (the projection's name for Foreground).
	Text string `json:"text,omitempty"`
	// Error, Warning and Success are the status roles.
	Error   string `json:"error,omitempty"`
	Warning string `json:"warning,omitempty"`
	Success string `json:"success,omitempty"`
	// Broadcast marks panes taking broadcast input.
	Broadcast string `json:"broadcast,omitempty"`
	// SelectionBg and SelectionFg are the selected-row pair. Sent as a pair
	// because using one without the other is how a selected row ends up
	// unreadable.
	SelectionBg string `json:"selection_bg,omitempty"`
	SelectionFg string `json:"selection_fg,omitempty"`
	// OnAccent is text drawn ON an accent-filled background — the other half
	// of Accent, and the token a sidecar reaches for the moment it renders a
	// filled tab or a selected pill.
	OnAccent string `json:"on_accent,omitempty"`
}

// Navigate asks the host to focus PanelID, optionally activating TabID.
// Mirrors embed.NavigateMsg.
type Navigate struct {
	PanelID string `json:"panel_id"`
	TabID   string `json:"tab_id,omitempty"`
}

// Done reports the sidecar's primary lifecycle completing. Result is any JSON
// value; Error is a string because Go's error interface does not serialize.
type Done struct {
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// Log is one diagnostic line for the host log. Level is "debug", "info",
// "warn" or "error"; anything else is treated as "info".
type Log struct {
	Level   string `json:"level,omitempty"`
	Message string `json:"message"`
}

// Error is a protocol fault. The sender closes the connection after it.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
}

// Error codes.
const (
	CodeVersionMismatch   = "version_mismatch"
	CodeAlreadyConnected  = "already_connected"
	CodeBadFrame          = "bad_frame"
	CodeHandshakeExpected = "handshake_expected"
)

// MaxFrameBytes caps one JSON line. A sidecar that needs to move more than
// this is using the wrong plane — the PTY is the bulk channel.
const MaxFrameBytes = 1 << 20 // 1 MiB

// ErrClosed is returned by Conn operations after the connection is closed.
var ErrClosed = errors.New("panelproto: connection closed")

// ErrBadFrame wraps every read failure that is the PEER's fault rather than
// the transport's: unparseable JSON, or a line past MaxFrameBytes. Both sides
// end the connection on one — after a malformed frame the stream position is
// no longer trustworthy — so the distinction from an ordinary I/O error is
// what lets the receiver answer with TypeError before hanging up instead of
// vanishing silently.
var ErrBadFrame = errors.New("panelproto: bad frame")

// Conn is one framed control-plane connection. It is safe for concurrent
// writes; reads are single-consumer, as the framing is a strict line stream.
type Conn struct {
	c   net.Conn
	sc  *bufio.Scanner
	mu  sync.Mutex
	w   *bufio.Writer
	off bool
}

// NewConn wraps an accepted or dialed connection.
func NewConn(c net.Conn) *Conn {
	sc := bufio.NewScanner(c)
	sc.Buffer(make([]byte, 0, 64*1024), MaxFrameBytes)
	return &Conn{c: c, sc: sc, w: bufio.NewWriter(c)}
}

// Read returns the next frame, io.EOF when the peer hangs up, or an error
// wrapping ErrBadFrame when the peer sent something unparseable or oversized.
func (c *Conn) Read() (Frame, error) {
	if !c.sc.Scan() {
		if err := c.sc.Err(); err != nil {
			if errors.Is(err, bufio.ErrTooLong) {
				return Frame{}, fmt.Errorf("%w: line exceeds %d bytes", ErrBadFrame, MaxFrameBytes)
			}
			return Frame{}, err
		}
		return Frame{}, io.EOF
	}
	line := c.sc.Bytes()
	if len(line) == 0 {
		// A bare newline is a permitted keep-alive; report it as a no-op
		// frame rather than a parse error.
		return Frame{}, nil
	}
	var f Frame
	if err := json.Unmarshal(line, &f); err != nil {
		return Frame{}, fmt.Errorf("%w: %v", ErrBadFrame, err)
	}
	return f, nil
}

// Send marshals payload (nil for the payload-less frames) and writes one line.
func (c *Conn) Send(typ string, payload any) error {
	f := Frame{Type: typ}
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("panelproto: marshal %s: %w", typ, err)
		}
		f.Payload = b
	}
	b, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("panelproto: marshal frame %s: %w", typ, err)
	}
	if len(b)+1 > MaxFrameBytes {
		return fmt.Errorf("panelproto: frame %s exceeds %d bytes", typ, MaxFrameBytes)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.off {
		return ErrClosed
	}
	if _, err := c.w.Write(append(b, '\n')); err != nil {
		return err
	}
	return c.w.Flush()
}

// SetReadDeadline bounds the next Read.
func (c *Conn) SetReadDeadline(t time.Time) error { return c.c.SetReadDeadline(t) }

// Close shuts the connection down. Idempotent.
func (c *Conn) Close() error {
	c.mu.Lock()
	if c.off {
		c.mu.Unlock()
		return nil
	}
	c.off = true
	c.mu.Unlock()
	return c.c.Close()
}

// DecodePayload unmarshals a frame's payload into v. A payload-less frame
// leaves v untouched.
func DecodePayload(f Frame, v any) error {
	if len(f.Payload) == 0 {
		return nil
	}
	return json.Unmarshal(f.Payload, v)
}

// Dial is the sidecar-side handshake helper: it reads GROVE_PANEL_SOCKET,
// connects, sends hello, and returns the connection plus the host's Welcome.
//
// It returns (nil, nil, nil) when GROVE_PANEL_SOCKET is unset — the panel is
// running as a plain PTY plugin, which is a supported mode, not a failure.
// Callers must handle that case by rendering without host context.
func Dial(hello Hello) (*Conn, *Welcome, error) {
	path := os.Getenv(EnvSocket)
	if path == "" {
		return nil, nil, nil
	}
	if hello.Protocol == "" {
		hello.Protocol = Version
	}
	c, err := net.DialTimeout("unix", path, 5*time.Second)
	if err != nil {
		return nil, nil, fmt.Errorf("panelproto: dial %s: %w", path, err)
	}
	conn := NewConn(c)
	if err := conn.Send(TypeHello, hello); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	f, err := conn.Read()
	if err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("panelproto: awaiting welcome: %w", err)
	}
	_ = conn.SetReadDeadline(time.Time{})
	if f.Type == TypeError {
		var e Error
		_ = DecodePayload(f, &e)
		_ = conn.Close()
		return nil, nil, fmt.Errorf("panelproto: host refused: %s %s", e.Code, e.Message)
	}
	if f.Type != TypeWelcome {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("panelproto: expected welcome, got %q", f.Type)
	}
	var w Welcome
	if err := DecodePayload(f, &w); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	return conn, &w, nil
}
