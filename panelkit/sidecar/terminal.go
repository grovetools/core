package sidecar

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"golang.org/x/term"

	"github.com/grovetools/core/panelkit/panelproto"
)

// Terminal owns a sidecar's pane: raw mode, the alternate screen, the cursor,
// and the resize signal.
//
// A sidecar built on bubbletea does not need this — bubbletea owns all of it,
// and Run wires that up instead. This is for a panel drawing escape sequences
// directly, which is a legitimate way to write one: it needs no Go TUI
// framework, no cgo, and is the shape a panel in another language would take.
//
// The lifecycle is a matched pair. Enter puts the terminal into the state the
// panel draws in; Close puts it back. Close must run on every exit path
// including a panic, or the user is left in a terminal with no cursor and no
// echo — hence the defer in the Enter documentation, and hence Close being
// idempotent.
type Terminal struct {
	in    *os.File
	out   *os.File
	state *term.State

	resizes chan panelproto.Size
	signals chan os.Signal

	closeOnce sync.Once
}

// TerminalOptions configures Enter.
type TerminalOptions struct {
	// AltScreen switches to the alternate screen buffer, so the panel's
	// drawing does not scroll the user's scrollback and the original contents
	// come back on exit. On by default; set NoAltScreen to opt out.
	NoAltScreen bool

	// ShowCursor leaves the cursor visible. Off by default — a full-screen
	// panel that positions its own content has nowhere honest to leave it.
	ShowCursor bool
}

// Enter puts the terminal into raw mode and, unless opted out, switches to the
// alternate screen and hides the cursor.
//
// Always pair it with a deferred Close on the very next line:
//
//	t, err := sidecar.Enter(sidecar.TerminalOptions{})
//	if err != nil { return err }
//	defer t.Close()
//
// Raw mode is what makes a panel a panel: without it the line discipline eats
// keystrokes until Enter, and ^C kills the process before the panel can decide
// what to do about it.
func Enter(opts TerminalOptions) (*Terminal, error) {
	t := &Terminal{
		in:      os.Stdin,
		out:     os.Stdout,
		resizes: make(chan panelproto.Size, 1),
		// Buffered because SIGWINCH arrives in bursts while a user drags a
		// pane divider, and a dropped one leaves the panel drawn at a size
		// the pane no longer is.
		signals: make(chan os.Signal, 8),
	}

	state, err := term.MakeRaw(int(t.in.Fd()))
	if err != nil {
		return nil, fmt.Errorf("sidecar: raw mode: %w", err)
	}
	t.state = state

	if !opts.NoAltScreen {
		fmt.Fprint(t.out, "\x1b[?1049h")
	}
	if !opts.ShowCursor {
		fmt.Fprint(t.out, "\x1b[?25l")
	}

	signal.Notify(t.signals, syscall.SIGWINCH)
	go t.watchResize()

	// Seed the first size so a panel can paint before any signal arrives.
	// Measuring is the whole answer and needs no host: a sidecar's stdout IS
	// the pane's PTY, and the host sized that PTY before execve, so TIOCGWINSZ
	// reports the pane — not the user's outer terminal — from the first line.
	if size, ok := t.measure(); ok {
		t.resizes <- size
	}

	return t, nil
}

// watchResize converts SIGWINCH into sizes until the terminal closes.
func (t *Terminal) watchResize() {
	for range t.signals {
		size, ok := t.measure()
		if !ok {
			continue
		}
		// Coalesce: a size superseded before anyone read it is not worth
		// delivering, and dropping the stale one keeps a divider drag from
		// queueing a hundred redraws.
		select {
		case <-t.resizes:
		default:
		}
		select {
		case t.resizes <- size:
		default:
		}
	}
}

func (t *Terminal) measure() (panelproto.Size, bool) {
	cols, rows, err := term.GetSize(int(t.out.Fd()))
	if err != nil {
		return panelproto.Size{}, false
	}
	return panelproto.Size{Cols: cols, Rows: rows}, true
}

// Resizes delivers the pane's size: once at startup, then on every SIGWINCH.
// Sizes are coalesced, so a reader that falls behind sees the latest rather
// than a backlog.
func (t *Terminal) Resizes() <-chan panelproto.Size { return t.resizes }

// Out is the pane's output stream. Write frames here; never to a logger.
func (t *Terminal) Out() *os.File { return t.out }

// In is the pane's raw input stream. Feed it to a Decoder.
func (t *Terminal) In() *os.File { return t.in }

// Clear erases the screen and homes the cursor.
func (t *Terminal) Clear() { fmt.Fprint(t.out, "\x1b[2J\x1b[H") }

// MoveTo positions the cursor, 1-indexed, the way the escape sequence counts.
func (t *Terminal) MoveTo(row, col int) { fmt.Fprintf(t.out, "\x1b[%d;%dH", row, col) }

// Close restores everything Enter changed: cursor, screen buffer and line
// discipline, in that order. Idempotent, so a deferred Close after an explicit
// one is harmless.
func (t *Terminal) Close() error {
	var err error
	t.closeOnce.Do(func() {
		signal.Stop(t.signals)
		close(t.signals)
		// Reverse order of Enter: show the cursor while still on the alternate
		// screen, leave it, then hand back the line discipline.
		fmt.Fprint(t.out, "\x1b[?25h\x1b[?1049l")
		if t.state != nil {
			err = term.Restore(int(t.in.Fd()), t.state)
		}
	})
	return err
}
