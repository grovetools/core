package sidecar

import (
	"context"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/grovetools/core/panelkit/panelproto"
	"github.com/grovetools/core/tui/components/pager"
	"github.com/grovetools/core/tui/theme"
)

// The bubbletea adapter: host frames arriving as tea.Msgs.
//
// This is what makes an out-of-process panel the same program as an in-tree
// one. Focus and blur arrive as pager.FocusMsg and pager.BlurMsg — the exact
// types an in-process page already matches on — so a page written for the
// drawer works unchanged in a sidecar. The rest arrive as the messages below,
// which have no in-process equivalent because in-process there is nothing to
// send them over.
//
// Focus has two sources now, and the terminal is the preferred one: the host
// emits mode-1004 focus reports into every pane's PTY, bubbletea decodes them
// natively, and the control plane's focus/blur frames are deprecated (still
// delivered in v1, gone in v2). focusPreference below reconciles the two so a
// model sees exactly one focus message per transition whichever plane carried
// it.

// WelcomeMsg is the handshake result. It arrives before the first frame is
// drawn, so a model can size, theme and configure itself in one place instead
// of accumulating four half-initialised states.
//
// Client is the connection the welcome arrived on: capturing it here is how a
// Run model gets to call Log, Navigate, OpenEditor, RequestClose, Done and DeclareKeys.
// Nil only in a no-host run, where no WelcomeMsg is delivered anyway.
type WelcomeMsg struct {
	Welcome *panelproto.Welcome
	Client  *Client
}

// ThemeMsg is a live re-theme. Run applies it to core/tui/theme before
// delivering, so a model built from kit widgets is already re-themed by the
// time it sees the message; it needs to act on it only if it caches styles.
type ThemeMsg struct{ Theme panelproto.Theme }

// IconsMsg is a live icon-set change. Run applies it to core/tui/theme before
// delivering, on the same terms as ThemeMsg.
type IconsMsg struct{ Icons panelproto.Icons }

// WorkspaceMsg repoints a workspace-scoped panel.
type WorkspaceMsg struct{ Workspace *panelproto.Workspace }

// WorkspacesUpdatedMsg says the workspace set changed.
type WorkspacesUpdatedMsg struct{}

// SettingsMsg carries reconfigured settings and label — the frame that lets a
// panel be reconfigured without being restarted.
type SettingsMsg struct{ Config panelproto.Config }

// DisconnectedMsg reports the control plane dropping. Run quits after
// delivering it — see RunOptions.NoQuitOnDisconnect — so a model needs this
// case only if it has something to do on the way out.
type DisconnectedMsg struct{ Err error }

// HostErrorMsg is a protocol fault reported by the host.
type HostErrorMsg struct{ Error panelproto.Error }

// ToMsg converts a host event into the tea.Msg a model sees. Focus and blur
// become the pager's own message types so an in-process page matches them
// unchanged; a CloseEvent becomes tea.Quit's message, because the only useful
// response to it is to exit.
//
// Exported for a sidecar wiring its own tea.Program instead of using Run.
func ToMsg(ev Event) tea.Msg {
	switch e := ev.(type) {
	case WelcomeEvent:
		return WelcomeMsg{Welcome: e.Welcome, Client: e.Client}
	case ThemeEvent:
		return ThemeMsg{Theme: e.Theme}
	case IconsEvent:
		return IconsMsg{Icons: e.Icons}
	case FocusEvent:
		return pager.FocusMsg{}
	case BlurEvent:
		return pager.BlurMsg{}
	case WorkspaceEvent:
		return WorkspaceMsg{Workspace: e.Workspace}
	case WorkspacesUpdatedEvent:
		return WorkspacesUpdatedMsg{}
	case SettingsEvent:
		return SettingsMsg{Config: e.Config}
	case DisconnectedEvent:
		return DisconnectedMsg{Err: e.Err}
	case ErrorEvent:
		return HostErrorMsg{Error: e.Error}
	case CloseEvent:
		return tea.QuitMsg{}
	default:
		return nil
	}
}

// ApplyTheme pushes a wire theme into core/tui/theme, so every kit widget in
// the process re-renders in the host's colors on its next frame.
//
// It applies by NAME, not by the delivered tokens. The host and the panel link
// the same palette registry, so naming the theme gets the full palette —
// adaptive colors, every role, not just the dozen tokens the wire carries. The
// tokens are the fallback for a name this build does not know, and the whole
// answer for a panel in another language.
func ApplyTheme(t panelproto.Theme) {
	if t.Name != "" {
		if err := theme.SetTheme(t.Name); err == nil {
			return
		}
	}
	// An unknown theme name leaves the process on its default palette. The
	// delivered tokens still describe the host's chrome, so a panel that wants
	// to match it exactly should render through a Palette rather than through
	// theme.DefaultTheme.
}

// ApplyIcons pushes a wire icon mode into core/tui/theme.
func ApplyIcons(i panelproto.Icons) {
	// An unrecognised mode reads as ASCII: glyphs that always render beat
	// glyphs that might be tofu.
	if i.Mode == panelproto.IconsNerd {
		theme.SetIcons("nerd")
		return
	}
	theme.SetIcons("ascii")
}

// RunOptions configures Run.
type RunOptions struct {
	Options

	// TeaOptions are passed to tea.NewProgram. Run supplies the alt-screen and
	// focus-reporting options by default; anything here is appended, so a caller
	// can add mouse reporting or a custom output.
	TeaOptions []tea.ProgramOption

	// NoAltScreen leaves the panel on the main screen buffer.
	NoAltScreen bool

	// NoReportFocus stops Run from enabling terminal focus reporting
	// (mode 1004), leaving the deprecated focus/blur frames as the only source
	// of focus. There is rarely a reason: in-band is the standard, it costs one
	// DECSET, and a host that does not emit it just never sends anything.
	NoReportFocus bool

	// NoApplyTheme stops Run from pushing delivered themes and icon modes into
	// core/tui/theme. Set it for a panel that renders entirely through a
	// Palette and does not want the process's global theme moved.
	NoApplyTheme bool

	// NoQuitOnDisconnect keeps the program running after the host's event
	// stream ends: a control plane that dropped with Reconnect off, a reconnect
	// that gave up because the socket is gone for good, the context passed to
	// Run being cancelled, or the panel closing the Client itself.
	//
	// The default is to quit, because the alternative is an orphan. A panel
	// that loses its host is a process drawing frames into a PTY nobody owns,
	// and nothing else in the system will end it: the host that would have
	// killed it is what died. Leaving that to every panel's Update meant every
	// panel had to remember a case whose omission is invisible in development,
	// where the host does not die, and a stray process in the field.
	//
	// This is not the same question as Options.Reconnect, and the two do not
	// fight. While the client is redialing the stream is still open, so nothing
	// here fires; a panel that means to outlive host restarts sets Reconnect
	// and keeps this default. Set NoQuitOnDisconnect only for a panel that is
	// genuinely useful with no host at all and wants to keep drawing — the same
	// panel that would render standalone, where no disconnect arrives anyway
	// because there was never a connection.
	NoQuitOnDisconnect bool

	// CaptureStderr points the process's stderr at LogPath(Options.App) for a
	// hosted panel, leaving it alone for a standalone one. See CaptureStderr.
	//
	// This is the odd one out in this struct: everything else here turns
	// something off, and this turns something on. It is opt-in because it
	// changes process-global state — fd 2, os.Stderr and the log package's
	// output — and a library that takes those without being asked is a worse
	// surprise than the corruption it prevents.
	//
	// Set it anyway for a panel that runs in front of users. Without it, one
	// forgotten log.Printf, one chatty dependency or one panic writes into the
	// pane's PTY, and under the alternate screen those bytes sit in the middle
	// of the frame until the pane is resized or closed. With it they go to a
	// file, and Options.Logf keeps working as it did — the two cover different
	// lines, the SDK's and everyone else's.
	CaptureStderr bool

	// NoQuitOnCtrlC leaves ctrl+c to the model.
	//
	// Run quits on it by default. Bubbletea does not: in raw mode ctrl+c is a
	// key byte, not a signal, so an unhandled ctrl+c does nothing at all and a
	// panel run straight from a shell — the ordinary way to debug one — cannot
	// be stopped from the terminal it is running in. Hosted, the question does
	// not arise: ctrl+c is non-deferrable, so the host keeps it and it never
	// reaches this process.
	//
	// The model still sees the key first and can act on it; the quit follows.
	NoQuitOnCtrlC bool
}

// Run connects to the host and runs model as an ordinary bubbletea program,
// with host frames delivered as messages.
//
// This is the convergence the SDK is for: the body of a sidecar becomes
//
//	sidecar.Run(context.Background(), myModel, sidecar.RunOptions{
//	    Options: sidecar.Options{App: "my-panel", Capabilities: sidecar.DefaultCapabilities},
//	})
//
// and myModel is a bubbletea model of pager.Pages and kit widgets — the same
// thing an in-tree panel is. Raw mode, the alternate screen and SIGWINCH are
// bubbletea's, the handshake and the frame pump are this package's, and the
// model sees only messages.
//
// A panel with no host still runs: it gets no WelcomeMsg and no host messages,
// and should render whatever it can without them.
//
// Run ends on any of four things: the model returning tea.Quit, a close frame
// from the host (ToMsg turns it into tea.QuitMsg), ctrl+c, or the host going
// away for good. The last two are the safe defaults a panel gets without
// writing a case for them; NoQuitOnCtrlC and NoQuitOnDisconnect turn them off.
func Run(ctx context.Context, model tea.Model, opts RunOptions) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Before anything else, so that a failure in the handshake itself — which
	// is where a dependency is most likely to have something to say — does not
	// print into the pane.
	var stderrPath string
	var stderrErr error
	if opts.CaptureStderr {
		stderrPath, stderrErr = CaptureStderr(opts.App)
	}

	client, err := Connect(ctx, opts.Options)
	if err != nil {
		return err
	}
	defer client.Close()

	// Report where stderr went once there is somewhere to report it to. A
	// redirect nobody can find is a redirect that reads as lost output.
	switch {
	case stderrErr != nil:
		client.warnf("stderr capture failed: %v", stderrErr)
	case stderrPath != "":
		client.diag(LogInfo, "stderr redirected to %s", stderrPath)
	}

	// Apply the initial theme and icon mode before the program starts, so the
	// first frame is already correct. This is what Welcome.Theme and
	// Welcome.Icons exist for — without them the opening frame is drawn in
	// whatever the environment implied and corrects itself a moment later.
	if w := client.Welcome(); w != nil && !opts.NoApplyTheme {
		if w.Theme != nil {
			ApplyTheme(*w.Theme)
		}
		if w.Icons != nil {
			ApplyIcons(*w.Icons)
		}
		// Drop the host's GROVE_THEME pin now that the opening appearance is
		// settled. The host sets it on every plugin child so the first frame is
		// drawn in the right palette before any welcome arrives — but while it
		// is set, theme.SetTheme is a no-op, which would turn every later
		// theme frame into a silent miss. The pin's job is done; live
		// re-themes take it from here.
		_ = os.Unsetenv(panelproto.EnvTheme)
	}

	teaOpts := opts.TeaOptions
	if !opts.NoAltScreen {
		teaOpts = append([]tea.ProgramOption{tea.WithAltScreen()}, teaOpts...)
	}
	// Prefer the terminal's own focus reporting. WithReportFocus sends
	// CSI ? 1004 h on the pane's PTY, which is what makes the host emit
	// CSI I / CSI O into it; the focusPreference wrapper then makes the frames
	// the fallback rather than a duplicate.
	if !opts.NoReportFocus {
		teaOpts = append([]tea.ProgramOption{tea.WithReportFocus()}, teaOpts...)
		model = &focusPreference{inner: model}
	}
	if !opts.NoQuitOnCtrlC {
		model = &interruptQuit{inner: model}
	}
	program := tea.NewProgram(model, teaOpts...)

	// Whether there was ever a host. A no-host run closes Events immediately,
	// which must not be read as a lost host: it is the supported PTY-only mode,
	// and quitting on it would mean a panel launched from a shell exits before
	// its first frame.
	hadHost := client.Connected()

	// Pump host events into the program. Started before Run so a Welcome that
	// is already queued is not dropped; tea.Program buffers messages sent
	// before it starts.
	go func() {
		for ev := range client.Events() {
			if !opts.NoApplyTheme {
				switch e := ev.(type) {
				case ThemeEvent:
					ApplyTheme(e.Theme)
				case IconsEvent:
					ApplyIcons(e.Icons)
				case WelcomeEvent:
					if e.Welcome.Theme != nil {
						ApplyTheme(*e.Welcome.Theme)
					}
					if e.Welcome.Icons != nil {
						ApplyIcons(*e.Welcome.Icons)
					}
				}
			}
			if msg := ToMsg(ev); msg != nil {
				program.Send(msg)
			}
		}
		// The host is gone for good — the channel closes only when the pump
		// stops, which with Reconnect on means it stopped redialing. The last
		// event was DisconnectedMsg (or nothing, if the pump stopped for a
		// cancelled context or a Client the panel closed itself), and it was
		// sent on this goroutine, so the model has already been offered its
		// chance to react before this quit lands behind it.
		if hadHost && !opts.NoQuitOnDisconnect {
			program.Quit()
		}
	}()

	_, err = program.Run()
	return err
}

// interruptQuit makes ctrl+c end the program.
//
// Bubbletea only quits on a SIGINT, and a sidecar never gets one: raw mode
// clears ISIG, so ctrl+c arrives as an ordinary key and an unhandled one does
// nothing. The model sees the key first — a panel with something to report on
// its way out can still send it — and the quit is appended to whatever it
// returned.
type interruptQuit struct{ inner tea.Model }

func (q *interruptQuit) Init() tea.Cmd { return q.inner.Init() }

func (q *interruptQuit) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	inner, cmd := q.inner.Update(msg)
	q.inner = inner
	if k, ok := msg.(tea.KeyMsg); ok && k.Type == tea.KeyCtrlC {
		return q, tea.Batch(cmd, tea.Quit)
	}
	return q, cmd
}

func (q *interruptQuit) View() string { return q.inner.View() }

// focusPreference makes the terminal the authority on focus and the control
// plane the fallback.
//
// Both sources now exist and say the same thing. Mode 1004 is the standard, the
// host emits it for every pane, and bubbletea decodes it for free — so once a
// tea.FocusMsg has arrived, the deprecated focus/blur frames are a second
// delivery of a fact the model already has, and a model that acts on focus (a
// timer that pauses, a poll that stops) would do it twice. Until one arrives the
// frames are all there is: an older host, a NoReportFocus panel, or a terminal
// between the DECSET and the first transition.
//
// Both arrive at the inner model as pager.FocusMsg / pager.BlurMsg either way,
// so nothing downstream has to know which plane won.
type focusPreference struct {
	inner  tea.Model
	inBand bool
}

func (f *focusPreference) Init() tea.Cmd { return f.inner.Init() }

func (f *focusPreference) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case tea.FocusMsg:
		f.inBand = true
		msg = pager.FocusMsg{}
	case tea.BlurMsg:
		f.inBand = true
		msg = pager.BlurMsg{}
	case pager.FocusMsg, pager.BlurMsg:
		// Frame-derived (ToMsg translated it). Drop it once the terminal is
		// answering, and let it through until then.
		if f.inBand {
			return f, nil
		}
	}
	inner, cmd := f.inner.Update(msg)
	f.inner = inner
	return f, cmd
}

func (f *focusPreference) View() string { return f.inner.View() }

// InBandFocus reports whether a terminal focus event has arrived, which is the
// signal that the control plane's focus frames are now redundant.
func (f *focusPreference) InBandFocus() bool { return f.inBand }

// There is deliberately no InitialWindowSize here. bubbletea's checkResize
// reads term.GetSize on its own tty output, which for a sidecar IS the pane's
// PTY — so its first tea.WindowSizeMsg is already the pane's size, measured
// before the first frame. The host used to send one in Welcome on the theory
// that the first SIGWINCH arrives after the first paint; that is true and
// irrelevant, because measuring never needed a signal.
