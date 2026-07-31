package sidecar

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/grovetools/core/panelkit/panelproto"
	"github.com/grovetools/core/tui/theme"
)

// headlessTeaOptions makes Run drivable under go test: no renderer, no tty.
// input is decoded as keys by Run's own program; a test driving the model
// through host frames passes "".
func headlessTeaOptions(input string) []tea.ProgramOption {
	return []tea.ProgramOption{
		tea.WithInput(strings.NewReader(input)),
		tea.WithoutRenderer(),
	}
}

// runModel adapts a plain Update func into the model Run wants. init is
// optional, for a test that needs a command before any message arrives.
type runModel struct {
	init   func() tea.Cmd
	update func(tea.Msg) tea.Cmd
}

func (m *runModel) Init() tea.Cmd {
	if m.init == nil {
		return nil
	}
	return m.init()
}

func (m *runModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m, m.update(msg)
}

func (m *runModel) View() string { return "" }

// runUnderTest runs Run with a watchdog: a model that never quits would
// otherwise hang the whole package.
func runUnderTest(t *testing.T, model tea.Model, opts RunOptions) {
	t.Helper()
	runUnderTestWithInput(t, model, opts, "")
}

// runUnderTestWithInput is runUnderTest with bytes on stdin, for the tests that
// need Run's own program to decode a key.
func runUnderTestWithInput(t *testing.T, model tea.Model, opts RunOptions, input string) {
	t.Helper()
	opts.TeaOptions = append(opts.TeaOptions, headlessTeaOptions(input)...)
	opts.NoAltScreen = true
	done := make(chan error, 1)
	go func() { done <- Run(context.Background(), model, opts) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() never returned")
	}
}

// A model driven by Run has to end up holding the Client, or it can never
// reach the app→host surface — Log, Navigate, RequestClose, Done — and the
// only way to a close request is abandoning Run for a hand-written pump.
func TestRunDeliversClientOnWelcome(t *testing.T) {
	startFakeHost(t, panelproto.Welcome{Protocol: panelproto.Version, PanelID: "plugin-test"})

	var got *Client
	model := &runModel{update: func(msg tea.Msg) tea.Cmd {
		if w, ok := msg.(WelcomeMsg); ok {
			got = w.Client
			return tea.Quit
		}
		return nil
	}}
	runUnderTest(t, model, RunOptions{Options: Options{App: "t"}})

	if got == nil {
		t.Fatal("WelcomeMsg.Client = nil; a Run model has no other way to the Client")
	}
	if !got.Connected() {
		t.Error("WelcomeMsg.Client.Connected() = false with a live host")
	}
}

// stillAliveMsg is how the tests below tell "kept running" from "quit": a model
// that schedules it and later receives it was still being updated. If Run
// quit in between, the program is gone before the command's message lands.
type stillAliveMsg struct{}

func stillAlive() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return stillAliveMsg{} })
}

// The defect this fixes: a panel whose host died kept running until something
// else killed it, because DisconnectedMsg was delivered and nothing else
// happened. Run quits now, and the model below deliberately does not — if the
// SDK ever stops quitting for it, the watchdog in runUnderTest fails the test.
func TestRunQuitsWhenTheHostGoesAway(t *testing.T) {
	host := startFakeHost(t, panelproto.Welcome{Protocol: panelproto.Version, PanelID: "plugin-test"})

	var sawDisconnect bool
	model := &runModel{update: func(msg tea.Msg) tea.Cmd {
		if _, ok := msg.(DisconnectedMsg); ok {
			// Deliberately no tea.Quit. That is the whole point.
			sawDisconnect = true
		}
		return nil
	}}
	// Hang up from the host's side, once the welcome is on the wire: the
	// control plane drops with the panel none the wiser.
	go func() { _ = host.conn().Close() }()
	runUnderTest(t, model, RunOptions{Options: Options{App: "t"}})

	if !sawDisconnect {
		t.Error("the model never saw DisconnectedMsg; it must still be delivered, not swallowed by the quit")
	}
}

// Hanging up is an ending too. A panel that closes the Client has ended the
// event stream by hand, and the same rule applies: no host, no reason to keep a
// process drawing into a pane nobody owns.
func TestRunQuitsWhenThePanelHangsUp(t *testing.T) {
	startFakeHost(t, panelproto.Welcome{Protocol: panelproto.Version, PanelID: "plugin-test"})

	model := &runModel{update: func(msg tea.Msg) tea.Cmd {
		if w, ok := msg.(WelcomeMsg); ok {
			_ = w.Client.Close()
		}
		return nil
	}}
	runUnderTest(t, model, RunOptions{Options: Options{App: "t"}})
}

// The opt-out has to actually opt out, and the model has to get the message
// before the program ends either way.
func TestRunKeepsRunningWithNoQuitOnDisconnect(t *testing.T) {
	host := startFakeHost(t, panelproto.Welcome{Protocol: panelproto.Version, PanelID: "plugin-test"})

	var alive bool
	model := &runModel{update: func(msg tea.Msg) tea.Cmd {
		switch msg.(type) {
		case DisconnectedMsg:
			return stillAlive()
		case stillAliveMsg:
			alive = true
			return tea.Quit
		}
		return nil
	}}
	go func() { _ = host.conn().Close() }()
	runUnderTest(t, model, RunOptions{Options: Options{App: "t"}, NoQuitOnDisconnect: true})

	if !alive {
		t.Error("Run quit after a disconnect with NoQuitOnDisconnect set")
	}
}

// A panel with no host must not be caught by the quit. Its event channel closes
// immediately — there is nothing to pump — and reading that as a lost host
// would exit a panel run from a shell before it drew anything.
func TestRunWithNoHostDoesNotQuitOnTheClosedChannel(t *testing.T) {
	t.Setenv(panelproto.EnvSocket, "")

	var alive bool
	model := &runModel{update: func(msg tea.Msg) tea.Cmd {
		if _, ok := msg.(stillAliveMsg); ok {
			alive = true
			return tea.Quit
		}
		return nil
	}}
	// Init schedules the probe: with no host there is no message to hang it off.
	model.init = stillAlive
	runUnderTest(t, model, RunOptions{Options: Options{App: "t"}})

	if !alive {
		t.Error("Run quit in the no-host mode; the closed event channel was read as a lost host")
	}
}

// ctrl+c is a key, not a signal, once raw mode clears ISIG — so bubbletea does
// nothing with it and a panel debugged from a shell could not be stopped from
// the terminal running it.
func TestRunQuitsOnCtrlC(t *testing.T) {
	t.Setenv(panelproto.EnvSocket, "")

	var sawKey bool
	model := &runModel{update: func(msg tea.Msg) tea.Cmd {
		if k, ok := msg.(tea.KeyMsg); ok && k.Type == tea.KeyCtrlC {
			// The model is offered the key before the quit lands behind it, so
			// a panel with something to report on the way out still can.
			sawKey = true
		}
		return nil
	}}
	runUnderTestWithInput(t, model, RunOptions{Options: Options{App: "t"}}, "\x03")

	if !sawKey {
		t.Error("the model never saw ctrl+c; the interrupt wrapper must pass it through, not swallow it")
	}
}

func TestRunLeavesCtrlCAloneWhenOptedOut(t *testing.T) {
	t.Setenv(panelproto.EnvSocket, "")

	model := &runModel{update: func(msg tea.Msg) tea.Cmd {
		if _, ok := msg.(stillAliveMsg); ok {
			return tea.Quit
		}
		if k, ok := msg.(tea.KeyMsg); ok && k.Type == tea.KeyCtrlC {
			// Nothing quits here; the probe below proves the program survived
			// the key, and then ends the test itself.
			return stillAlive()
		}
		return nil
	}}
	runUnderTestWithInput(t, model, RunOptions{Options: Options{App: "t"}, NoQuitOnCtrlC: true}, "\x03")
}

// The host pins GROVE_THEME on every plugin child so the first frame is drawn
// in the right palette — and while pinned, theme.SetTheme is a no-op. A later
// theme frame must still re-theme the process: Run drops the pin once the
// welcome appearance is applied. This is the regression that motivated the
// fix; without the Unsetenv in Run it fails.
func TestRunReappliesThemeWhilePinned(t *testing.T) {
	t.Setenv(panelproto.EnvTheme, "kanagawa")
	prior := theme.DefaultTheme.Name
	t.Cleanup(func() {
		// Run has unset the pin by now, so this restore actually applies.
		_ = theme.SetTheme(prior)
	})

	host := startFakeHost(t, panelproto.Welcome{
		Protocol: panelproto.Version,
		PanelID:  "plugin-test",
		Theme:    &panelproto.Theme{Name: "kanagawa"},
	})

	var got string
	model := &runModel{update: func(msg tea.Msg) tea.Cmd {
		if th, ok := msg.(ThemeMsg); ok && th.Theme.Name == "gruvbox" {
			// Run applies the theme before delivering the message, so the
			// process theme is already meant to have moved.
			got = theme.DefaultTheme.Name
			return tea.Quit
		}
		return nil
	}}

	go func() {
		conn := host.conn()
		_ = conn.Send(panelproto.TypeTheme, panelproto.Theme{Name: "gruvbox"})
	}()
	runUnderTest(t, model, RunOptions{Options: Options{App: "t"}})

	if got != "gruvbox" {
		t.Fatalf("theme.DefaultTheme.Name = %q after a live theme frame, want gruvbox: the GROVE_THEME pin swallowed the re-theme", got)
	}
}
