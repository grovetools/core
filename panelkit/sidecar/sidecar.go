// Package sidecar is the runtime half of panelkit: everything an
// out-of-process panel needs between "my process started" and "I am drawing
// frames the host understands".
//
// panelproto gives a sidecar the wire types and Dial, and stops there. What
// every sidecar then wrote for itself was the same four things: raw mode and
// the alternate screen, a read loop over the control plane, SIGWINCH plumbing,
// and a byte-level decoder turning stdin into key names. Both shipped examples
// hand-rolled all four, plus their own hex→SGR converter, and neither handled
// SIGWINCH. That is the floor this package raises.
//
// # Where this lives
//
// Here, next to the widget half of the kit and the protocol bindings it speaks,
// so that a Go panel depends on exactly one grove module. It lived in treemux
// until the reference panel showed what the second import cost: see the note in
// core/panelkit/doc.go.
//
// # Two ways in
//
// Connect is the low-level door: it hands back a channel of Events and leaves
// rendering entirely to the caller, which is what a sidecar drawing raw escape
// sequences wants. Run is the high-level one: it turns the same events into
// tea.Msgs and runs an ordinary bubbletea program, which is what a sidecar
// built out of pager.Pages and kit widgets wants. Both are the same runtime.
//
// # Degrading without a host
//
// A panel launched as a plain PTY plugin has no GROVE_PANEL_SOCKET. That is a
// supported mode, not an error: Connect returns a Client that reports no host,
// delivers no events, and whose sends are no-ops. A sidecar should render
// whatever it can without host context rather than refusing to start.
package sidecar

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/grovetools/core/panelkit/panelproto"
)

// Options configures a sidecar's handshake and runtime.
type Options struct {
	// App is the sidecar's own name, used in host logs. Required.
	App string

	// Version is the sidecar's build version. Informational.
	Version string

	// Capabilities names the host→app frames to subscribe to. Leaving it
	// empty gets Welcome and Close and nothing else, which is rarely what a
	// panel wants — see DefaultCapabilities.
	Capabilities []string

	// Keys is the key-claim declaration. It is a request; the host filters it
	// and reports what it granted in Welcome.AcceptedKeys, which Client.Granted
	// exposes.
	Keys *panelproto.KeyReference

	// Reconnect makes the client redial after the control plane drops,
	// instead of reporting Disconnected and stopping.
	//
	// Off by default, and that default is deliberate. A dropped control plane
	// usually means the host is gone, and the host kills the panel process
	// shortly after; a sidecar that redials in that window is racing its own
	// termination. Turn it on for a panel that outlives host restarts.
	//
	// Under Run this composes with the quit-on-disconnect default rather than
	// contradicting it: while the client is redialing the event stream is still
	// open and nothing quits, and the stream ends — taking the program with it
	// — only when redialing gives up because the socket is gone.
	Reconnect bool

	// ReconnectBackoff is the delay between redial attempts, doubling up to
	// MaxReconnectBackoff. Zero means 250ms.
	ReconnectBackoff time.Duration

	// MaxReconnectBackoff caps the doubling. Zero means 10s.
	MaxReconnectBackoff time.Duration

	// Logf receives the runtime's own diagnostics: a frame that would not
	// decode, a frame type this build does not know, a redial that failed.
	//
	// Nil does not mean discard. Unset, diagnostics go to the host's log over
	// the control plane — the same place Client.Log writes, at warn for a lost
	// delivery and debug for a note — so a hosted panel has them without
	// configuring anything. With no host there is nowhere safe to put them and
	// they are dropped; FileLogf gives them a file:
	//
	//	Logf: sidecar.FileLogf(sidecar.LogPath("my-panel")),
	//
	// Whatever is set here must write to neither stdout nor stderr. Both are
	// the pane's PTY for a plugin child, so a line on either lands in the middle
	// of the user's frame and stays there — which rules out log.Printf and the
	// log package's default, the two things a Go author reaches for first. See
	// log.go for the whole of that argument, and CaptureStderr for the lines
	// this field cannot catch because they are not the SDK's.
	Logf func(format string, args ...any)
}

// DefaultCapabilities is everything a full-featured panel wants delivered:
// workspace scope, theme and icon changes, its own settings, and — as a
// deprecated fallback — focus transitions. A sidecar that ignores one of these
// is better off not declaring it: the host sends only what was asked for.
//
// CapFocus is still here because it costs nothing and covers an older host, but
// focus belongs in band: the host writes mode-1004 reports into the pane's PTY,
// Run enables them, and focusPreference drops the frames as soon as one arrives.
// A panel that never wants the frame can drop the capability.
var DefaultCapabilities = []string{
	panelproto.CapFocus,
	panelproto.CapWorkspace,
	panelproto.CapTheme,
	panelproto.CapIcons,
	panelproto.CapSettings,
	panelproto.CapCloseHooks,
}

// Event is something the host said. The concrete types below are the whole
// set; a frame type this runtime does not recognise is logged and dropped, so
// a sidecar built today keeps working against a host that grew a frame.
type Event interface{ event() }

// WelcomeEvent is the handshake result: granted keys, initial theme, icon
// mode, workspace and settings. Delivered once per connection, first, and
// again after a reconnect.
//
// Client is the connection the welcome arrived on. It is how a model driven by
// Run reaches the app→host surface — Log, Navigate, OpenEditor, RequestClose, Done,
// DeclareKeys — without wiring its own pump: capture it from the first
// WelcomeMsg. Nil only in a no-host run, where no welcome is delivered anyway.
type WelcomeEvent struct {
	Welcome *panelproto.Welcome
	Client  *Client
}

// ThemeEvent is a live re-theme.
type ThemeEvent struct{ Theme panelproto.Theme }

// IconsEvent is a live icon-set change.
type IconsEvent struct{ Icons panelproto.Icons }

// FocusEvent says this panel gained input focus, and with it the chords the
// host granted.
type FocusEvent struct{}

// BlurEvent says this panel lost input focus. Granted chords go back to the
// host until focus returns.
type BlurEvent struct{}

// WorkspaceEvent repoints a workspace-scoped panel. Workspace is nil when the
// host has no workspace scope.
type WorkspaceEvent struct{ Workspace *panelproto.Workspace }

// WorkspacesUpdatedEvent says the workspace set changed and any cached view of
// it is stale.
type WorkspacesUpdatedEvent struct{}

// SettingsEvent carries this panel's reconfigured settings and label. This is
// the frame that lets a panel be reconfigured without being restarted — args
// and env are fixed at spawn.
type SettingsEvent struct{ Config panelproto.Config }

// CloseEvent says the panel is being shut down. The host closes the PTY and
// kills the process immediately after sending it, so the only useful response
// is to exit: there is no grace period to flush state or report a result in,
// and a handler that tries may not run at all. Run turns this into
// tea.QuitMsg for exactly that reason.
type CloseEvent struct{}

// DisconnectedEvent reports the control plane dropping. Err is nil for a clean
// hang-up. With Options.Reconnect set the client redials and a WelcomeEvent
// follows; without it this is the last event.
type DisconnectedEvent struct{ Err error }

// ErrorEvent is a protocol fault reported by the host. The host closes the
// connection after sending one.
type ErrorEvent struct{ Error panelproto.Error }

func (WelcomeEvent) event()           {}
func (ThemeEvent) event()             {}
func (IconsEvent) event()             {}
func (FocusEvent) event()             {}
func (BlurEvent) event()              {}
func (WorkspaceEvent) event()         {}
func (WorkspacesUpdatedEvent) event() {}
func (SettingsEvent) event()          {}
func (CloseEvent) event()             {}
func (DisconnectedEvent) event()      {}
func (ErrorEvent) event()             {}

// Client is a live connection to the host's control plane.
//
// A Client with no host — the PTY-only mode — is fully usable: Connected
// reports false, Events is a channel that never delivers, and every send is a
// no-op returning nil. Callers do not need a nil check on any of it.
type Client struct {
	opts   Options
	events chan Event

	mu      sync.RWMutex
	conn    *panelproto.Conn
	welcome *panelproto.Welcome
	granted map[string]bool
	// digest is the last digest actually SENT, which is what makes SetDigest
	// coalescing. Held here rather than by the caller because every panel that
	// publishes one would otherwise hold the same field, and the ones that
	// forgot would be the ones costing the host a wake per tick.
	digest    panelproto.Digest
	digestSet bool

	cancel context.CancelFunc
	done   chan struct{}
}

// Connect performs the handshake and starts pumping frames into Events.
//
// A missing GROVE_PANEL_SOCKET is not an error: the returned Client reports
// Connected() == false and the panel should render without host context. A
// socket that is present but refuses the handshake IS an error, because that
// means a host is there and something is wrong between them.
func Connect(ctx context.Context, opts Options) (*Client, error) {
	if opts.App == "" {
		return nil, errors.New("sidecar: Options.App is required")
	}
	ctx, cancel := context.WithCancel(ctx)
	c := &Client{
		opts:   opts,
		events: make(chan Event, 16),
		cancel: cancel,
		done:   make(chan struct{}),
	}

	conn, welcome, err := panelproto.Dial(c.hello())
	if err != nil {
		cancel()
		return nil, err
	}
	if conn == nil {
		// No host. Nothing to pump, but the client stays usable: both channels
		// close immediately so a caller ranging over Events, or waiting on
		// Wait, is not blocked by a connection that will never exist.
		cancel()
		close(c.events)
		close(c.done)
		return c, nil
	}
	c.adopt(conn, welcome)

	go c.pump(ctx, welcome)
	return c, nil
}

func (c *Client) hello() panelproto.Hello {
	return panelproto.Hello{
		Protocol:     panelproto.Version,
		App:          c.opts.App,
		Version:      c.opts.Version,
		Capabilities: c.opts.Capabilities,
		Keys:         c.opts.Keys,
	}
}

func (c *Client) adopt(conn *panelproto.Conn, welcome *panelproto.Welcome) {
	granted := make(map[string]bool, len(welcome.AcceptedKeys))
	for _, k := range welcome.AcceptedKeys {
		granted[k] = true
	}
	c.mu.Lock()
	c.conn, c.welcome, c.granted = conn, welcome, granted
	c.mu.Unlock()
}

// pump reads frames until the connection drops, then either redials or stops.
func (c *Client) pump(ctx context.Context, welcome *panelproto.Welcome) {
	defer close(c.done)
	defer close(c.events)

	backoff := c.opts.ReconnectBackoff
	if backoff <= 0 {
		backoff = 250 * time.Millisecond
	}
	maxBackoff := c.opts.MaxReconnectBackoff
	if maxBackoff <= 0 {
		maxBackoff = 10 * time.Second
	}
	delay := backoff

	for {
		c.emit(ctx, WelcomeEvent{Welcome: welcome, Client: c})
		err := c.readFrames(ctx)
		if ctx.Err() != nil {
			return
		}
		c.emit(ctx, DisconnectedEvent{Err: err})
		if !c.opts.Reconnect {
			return
		}

		// Redial with backoff. Every attempt re-handshakes from scratch: the
		// host may have restarted, so granted keys, theme and settings all
		// have to be re-read rather than assumed to have survived.
		var conn *panelproto.Conn
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			if delay < maxBackoff {
				delay *= 2
				if delay > maxBackoff {
					delay = maxBackoff
				}
			}
			var w *panelproto.Welcome
			var derr error
			conn, w, derr = panelproto.Dial(c.hello())
			if derr != nil {
				c.warnf("reconnect: %v", derr)
				continue
			}
			if conn == nil {
				// The socket went away entirely; there is no host to return
				// to and retrying cannot change that.
				return
			}
			c.adopt(conn, w)
			welcome = w
			delay = backoff
			break
		}
	}
}

// readFrames dispatches host frames to events until the connection ends.
func (c *Client) readFrames(ctx context.Context) error {
	conn := c.connection()
	if conn == nil {
		return errors.New("sidecar: no connection")
	}
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		frame, err := conn.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil // clean hang-up
			}
			return err
		}
		if frame.Type == "" {
			continue // keep-alive
		}
		if ev, ok := c.decode(frame); ok {
			c.emit(ctx, ev)
		}
		if frame.Type == panelproto.TypeClose || frame.Type == panelproto.TypeError {
			// Both are terminal: the host closes the connection after an
			// error, and kills the process after a close.
			return nil
		}
	}
}

// decode turns one frame into an event. An unrecognised type is logged and
// dropped rather than treated as a fault: unknown fields and frames are
// ignorable by design, which is what makes the protocol additive.
func (c *Client) decode(f panelproto.Frame) (Event, bool) {
	switch f.Type {
	case panelproto.TypeFocus:
		return FocusEvent{}, true
	case panelproto.TypeBlur:
		return BlurEvent{}, true
	case panelproto.TypeWorkspacesUpdated:
		return WorkspacesUpdatedEvent{}, true
	case panelproto.TypeClose:
		return CloseEvent{}, true
	case panelproto.TypeTheme:
		var t panelproto.Theme
		if err := panelproto.DecodePayload(f, &t); err != nil {
			c.warnf("theme frame: %v", err)
			return nil, false
		}
		return ThemeEvent{Theme: t}, true
	case panelproto.TypeIcons:
		var i panelproto.Icons
		if err := panelproto.DecodePayload(f, &i); err != nil {
			c.warnf("icons frame: %v", err)
			return nil, false
		}
		return IconsEvent{Icons: i}, true
	case panelproto.TypeWorkspace:
		var w panelproto.Workspace
		if err := panelproto.DecodePayload(f, &w); err != nil {
			c.warnf("workspace frame: %v", err)
			return nil, false
		}
		return WorkspaceEvent{Workspace: &w}, true
	case panelproto.TypeConfig:
		var cfg panelproto.Config
		if err := panelproto.DecodePayload(f, &cfg); err != nil {
			c.warnf("config frame: %v", err)
			return nil, false
		}
		return SettingsEvent{Config: cfg}, true
	case panelproto.TypeError:
		var e panelproto.Error
		_ = panelproto.DecodePayload(f, &e)
		return ErrorEvent{Error: e}, true
	default:
		c.logf("ignoring unknown frame %q", f.Type)
		return nil, false
	}
}

func (c *Client) emit(ctx context.Context, ev Event) {
	select {
	case c.events <- ev:
	case <-ctx.Done():
	}
}

// logf reports something the runtime handled and expects to handle: an
// unknown frame, a note about its own setup. Debug, because a host that grew a
// frame is not a fault.
func (c *Client) logf(format string, args ...any) {
	c.diag(LogDebug, format, args...)
}

// warnf reports something that went wrong and cost the panel a delivery: a
// frame that would not decode, a redial that failed. Warn, because these are
// the lines whose absence leaves a panel author with a silent panel — and the
// host's log runs at info, so debug would be exactly as silent.
func (c *Client) warnf(format string, args ...any) {
	c.diag(LogWarn, format, args...)
}

// diag sends a runtime diagnostic to whatever destination is safe.
//
// With no Logf set that is the host's log, not the floor. A panel author who
// configures nothing still gets the SDK's diagnostics, and gets them somewhere
// that is not the pane — which is the only reason the field can default to nil
// at all. The level is only consulted on that path; a configured Logf takes the
// line as it always did, because the field's signature is a format and its
// arguments and adding a level to it would break every caller.
//
// With no host there is neither destination and the line is dropped; that is
// the case FileLogf exists for.
func (c *Client) diag(level, format string, args ...any) {
	if c.opts.Logf != nil {
		c.opts.Logf(format, args...)
		return
	}
	_ = c.Log(level, fmt.Sprintf(format, args...))
}

func (c *Client) connection() *panelproto.Conn {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn
}

// Events delivers host events until the connection ends and the channel
// closes. A client with no host returns a channel that closes immediately.
func (c *Client) Events() <-chan Event { return c.events }

// Connected reports whether there is a host control plane at all — false when
// the panel was launched as a plain PTY plugin.
func (c *Client) Connected() bool { return c.connection() != nil }

// Welcome returns the most recent handshake result, or nil with no host.
func (c *Client) Welcome() *panelproto.Welcome {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.welcome
}

// Granted reports whether the host will defer a chord to this panel while it
// holds focus. Match a decoded key name against it before acting: a chord the
// host did not grant never reaches this process, and treating it as bound
// leaves a binding that silently does nothing.
func (c *Client) Granted(chord string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.granted[chord]
}

// GrantedKeys returns every granted chord, for building a help table.
func (c *Client) GrantedKeys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]string, 0, len(c.granted))
	for k := range c.granted {
		out = append(out, k)
	}
	return out
}

// Navigate asks the host to focus a panel, optionally activating a tab.
func (c *Client) Navigate(panelID, tabID string) error {
	return c.send(panelproto.TypeNavigate, panelproto.Navigate{PanelID: panelID, TabID: tabID})
}

// OpenEditor asks the host to open path. A quick open (dedicated=false) reuses
// the host's singleton editor when available; a dedicated open gets a pinned
// per-file rail pane. With no host it is a no-op, like every Client send.
func (c *Client) OpenEditor(path string, dedicated bool) error {
	return c.send(panelproto.TypeEditRequest, panelproto.EditRequest{Path: path, Dedicated: dedicated})
}

// RequestClose asks the host to close this panel.
func (c *Client) RequestClose() error {
	return c.send(panelproto.TypeCloseRequest, nil)
}

// Done reports the panel's primary lifecycle completing.
func (c *Client) Done(result []byte, failure error) error {
	d := panelproto.Done{Result: result}
	if failure != nil {
		d.Error = failure.Error()
	}
	return c.send(panelproto.TypeDone, d)
}

// Log levels the host understands, for Client.Log. An unrecognised level is
// logged as info rather than rejected.
const (
	LogDebug = "debug"
	LogInfo  = "info"
	LogWarn  = "warn"
	LogError = "error"
)

// Log writes one line to the host's log, tagged with this panel's name.
//
// This is how a sidecar reports anything at all. Both of the streams a Go
// program would use are the pane: stdout is what the panel draws into, and a
// plugin child's stderr is the same PTY, so fmt.Println and log.Printf alike
// land in the middle of the user's frame and stay there. The line goes over the
// control plane instead, into treemux's structured log, where a log line
// belongs.
//
// A Run model reaches this through WelcomeMsg.Client — which is nil when there
// is no host, so guard the captured field. Options.Logf falls back to this for
// the SDK's own diagnostics; FileLogf and CaptureStderr are the file-based
// alternatives for what the control plane cannot carry.
func (c *Client) Log(level, message string) error {
	return c.send(panelproto.TypeLog, panelproto.Log{Level: level, Message: message})
}

// SetPaneState tells the host whether this pane is worth mounting and what its
// empty state says, so a host that must answer those questions synchronously —
// a drawer compiling a page — can answer them from a cache instead of asking.
//
// Send it whenever the answer CHANGES, and once at startup if the opening
// answer is not the default. A host that has heard nothing treats the pane as
// available with no reason, which is the direction that fails visibly rather
// than by taking the pane's slot away mid-compile (see panelproto.PaneState).
//
// A pane mounted somewhere with no such contract — the icon rail — simply
// ignores it, so a panel that means to be mountable in both places says its
// piece once and does not branch.
func (c *Client) SetPaneState(available *bool, emptyReason string) error {
	return c.send(panelproto.TypeState, panelproto.PaneState{
		Available:   available,
		EmptyReason: emptyReason,
	})
}

// SetDigest publishes what this panel looks like in a slot it could never run
// in — one row, sometimes two, drawn by the host from the fields of
// [panelproto.Digest]. A digest pane elsewhere in the host renders it; a panel
// nobody projects simply has no reader, so publishing costs nothing.
//
// # It coalesces, so a timer is safe
//
// A push whose payload is identical to the last one sent is DROPPED here, and
// that is the whole of the host's protection: a host caches this frame and
// wakes its render loop for every one it receives, without diffing — wakes per
// second are publishers times push rate, exactly. Measured, a 10 Hz publisher
// suppressed panel-side collapses to the 1 Hz figure with no host change at
// all.
//
// So the rule a panel author needs is: PUSH WHEN WHAT A READER WOULD SEE
// CHANGES, NOT ON YOUR TIMER — and calling this from a ticker satisfies it,
// because a tick that changes nothing sends nothing. What it cannot save you
// from is a line that carries more precision than a reader can use: a digest
// rendering seconds genuinely changes every second, and the fix for that is to
// render minutes, not to push less often.
//
// A zero Digest clears the projection. Sending is a no-op with no host, like
// every other client send.
func (c *Client) SetDigest(d panelproto.Digest) error {
	c.mu.Lock()
	if c.digestSet && c.digest == d {
		c.mu.Unlock()
		return nil
	}
	c.digest, c.digestSet = d, true
	c.mu.Unlock()
	return c.send(panelproto.TypeDigest, d)
}

// DeclareKeys re-declares the panel's key claims mid-flight, replacing what
// Hello asked for. The host re-filters, and the new grant arrives as the next
// Welcome — so a panel that rebinds should not assume its request was met.
func (c *Client) DeclareKeys(ref panelproto.KeyReference) error {
	return c.send(panelproto.TypeKeys, ref)
}

func (c *Client) send(typ string, payload any) error {
	conn := c.connection()
	if conn == nil {
		return nil // no host; sending is a no-op, not a failure
	}
	if err := conn.Send(typ, payload); err != nil {
		if errors.Is(err, panelproto.ErrClosed) {
			return nil
		}
		return fmt.Errorf("sidecar: send %s: %w", typ, err)
	}
	return nil
}

// Close stops the pump and closes the control plane. Idempotent.
func (c *Client) Close() error {
	c.cancel()
	conn := c.connection()
	if conn == nil {
		return nil
	}
	err := conn.Close()
	<-c.done
	return err
}

// Wait blocks until the pump stops.
func (c *Client) Wait() { <-c.done }

// Environment reports the host context available before any dialing: the
// panel ID, the offered protocol version, and whether a control socket exists
// at all. A sidecar can use it to refuse a protocol version it does not
// implement without opening a connection first.
func Environment() (panelID, protocol string, hasSocket bool) {
	return os.Getenv(panelproto.EnvPanelID),
		os.Getenv(panelproto.EnvProtocol),
		os.Getenv(panelproto.EnvSocket) != ""
}
