package sidecar

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/grovetools/core/panelkit/panelproto"
	"github.com/grovetools/core/tui/components/pager"
)

// fakeHost is a control plane on a real unix socket. Running the tests against
// the actual framing rather than a stubbed Conn is the point: the handshake,
// the line protocol and the capability echo are what a sidecar has to get
// right, and a fake that skips them proves nothing.
type fakeHost struct {
	t        *testing.T
	ln       net.Listener
	conns    chan *panelproto.Conn
	welcome  panelproto.Welcome
	lastHell chan panelproto.Hello
}

func startFakeHost(t *testing.T, welcome panelproto.Welcome) *fakeHost {
	t.Helper()
	// A short path: unix socket paths are capped near 104 bytes. t.TempDir
	// embeds the test's name, which pushed the longer tests here past the cap —
	// and the failure mode was a skip, so they silently stopped running. Take
	// TMPDIR without the name instead, so a descriptive test name stays free.
	dir, err := os.MkdirTemp("", "gp")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s")

	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("unix sockets unavailable: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	t.Setenv(panelproto.EnvSocket, sock)

	h := &fakeHost{
		t:        t,
		ln:       ln,
		conns:    make(chan *panelproto.Conn, 1),
		welcome:  welcome,
		lastHell: make(chan panelproto.Hello, 1),
	}
	go h.accept()
	return h
}

func (h *fakeHost) accept() {
	for {
		c, err := h.ln.Accept()
		if err != nil {
			return
		}
		conn := panelproto.NewConn(c)
		f, err := conn.Read()
		if err != nil || f.Type != panelproto.TypeHello {
			_ = conn.Close()
			continue
		}
		var hello panelproto.Hello
		_ = panelproto.DecodePayload(f, &hello)
		select {
		case h.lastHell <- hello:
		default:
		}
		if err := conn.Send(panelproto.TypeWelcome, h.welcome); err != nil {
			_ = conn.Close()
			continue
		}
		h.conns <- conn
	}
}

// conn returns the accepted connection, so a test can push frames.
func (h *fakeHost) conn() *panelproto.Conn {
	h.t.Helper()
	select {
	case c := <-h.conns:
		return c
	case <-time.After(3 * time.Second):
		h.t.Fatal("host never accepted a connection")
		return nil
	}
}

func nextEvent(t *testing.T, c *Client) Event {
	t.Helper()
	select {
	case ev, ok := <-c.Events():
		if !ok {
			t.Fatal("event channel closed early")
		}
		return ev
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for an event")
		return nil
	}
}

func TestConnectHandshake(t *testing.T) {
	host := startFakeHost(t, panelproto.Welcome{
		Protocol:     panelproto.Version,
		Host:         "treemux",
		PanelID:      "plugin-test",
		AcceptedKeys: []string{"ctrl+f"},
	})

	client, err := Connect(context.Background(), Options{
		App:          "test-panel",
		Capabilities: DefaultCapabilities,
	})
	if err != nil {
		t.Fatalf("Connect() = %v", err)
	}
	defer client.Close()
	host.conn()

	if !client.Connected() {
		t.Error("Connected() = false with a live host")
	}

	hello := <-host.lastHell
	if hello.App != "test-panel" {
		t.Errorf("hello.App = %q, want test-panel", hello.App)
	}
	if hello.Protocol != panelproto.Version {
		t.Errorf("hello.Protocol = %q, want %q", hello.Protocol, panelproto.Version)
	}

	ev := nextEvent(t, client)
	w, ok := ev.(WelcomeEvent)
	if !ok {
		t.Fatalf("first event = %T, want WelcomeEvent", ev)
	}
	if w.Welcome.PanelID != "plugin-test" {
		t.Errorf("PanelID = %q", w.Welcome.PanelID)
	}

	// The grant is what a key binding has to be checked against: a chord the
	// host did not grant never reaches this process.
	if !client.Granted("ctrl+f") {
		t.Error("Granted(ctrl+f) = false after the host accepted it")
	}
	if client.Granted("ctrl+z") {
		t.Error("Granted(ctrl+z) = true for a chord the host never mentioned")
	}
}

func TestClientDeliversHostFrames(t *testing.T) {
	host := startFakeHost(t, panelproto.Welcome{Protocol: panelproto.Version, PanelID: "p"})
	client, err := Connect(context.Background(), Options{App: "t", Capabilities: DefaultCapabilities})
	if err != nil {
		t.Fatalf("Connect() = %v", err)
	}
	defer client.Close()
	conn := host.conn()
	nextEvent(t, client) // welcome

	if err := conn.Send(panelproto.TypeFocus, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := nextEvent(t, client).(FocusEvent); !ok {
		t.Error("focus frame did not become a FocusEvent")
	}

	if err := conn.Send(panelproto.TypeTheme, panelproto.Theme{Name: "gruvbox", Accent: "#fabd2f"}); err != nil {
		t.Fatal(err)
	}
	ev := nextEvent(t, client)
	th, ok := ev.(ThemeEvent)
	if !ok {
		t.Fatalf("theme frame became %T", ev)
	}
	if th.Theme.Name != "gruvbox" {
		t.Errorf("theme name = %q", th.Theme.Name)
	}

	if err := conn.Send(panelproto.TypeConfig, panelproto.Config{Label: "My Panel"}); err != nil {
		t.Fatal(err)
	}
	if s, ok := nextEvent(t, client).(SettingsEvent); !ok || s.Config.Label != "My Panel" {
		t.Error("config frame did not become a SettingsEvent carrying the label")
	}
}

// An unknown frame is dropped, not treated as a fault. That is what makes the
// protocol additive: a sidecar built today keeps working against a host that
// grew a frame tomorrow.
func TestClientIgnoresUnknownFrames(t *testing.T) {
	host := startFakeHost(t, panelproto.Welcome{Protocol: panelproto.Version, PanelID: "p"})
	client, err := Connect(context.Background(), Options{App: "t"})
	if err != nil {
		t.Fatalf("Connect() = %v", err)
	}
	defer client.Close()
	conn := host.conn()
	nextEvent(t, client) // welcome

	if err := conn.Send("some_future_frame", map[string]int{"x": 1}); err != nil {
		t.Fatal(err)
	}
	if err := conn.Send(panelproto.TypeBlur, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := nextEvent(t, client).(BlurEvent); !ok {
		t.Error("an unknown frame swallowed the frame after it")
	}
}

func TestClientReportsDisconnect(t *testing.T) {
	host := startFakeHost(t, panelproto.Welcome{Protocol: panelproto.Version, PanelID: "p"})
	client, err := Connect(context.Background(), Options{App: "t"})
	if err != nil {
		t.Fatalf("Connect() = %v", err)
	}
	defer client.Close()
	conn := host.conn()
	nextEvent(t, client) // welcome

	_ = conn.Close()
	if _, ok := nextEvent(t, client).(DisconnectedEvent); !ok {
		t.Error("a dropped connection did not produce a DisconnectedEvent")
	}
}

// A panel launched as a plain PTY plugin has no socket. That is a supported
// mode, and every method has to stay usable in it.
func TestNoHostIsNotAnError(t *testing.T) {
	t.Setenv(panelproto.EnvSocket, "")

	client, err := Connect(context.Background(), Options{App: "t"})
	if err != nil {
		t.Fatalf("Connect() with no socket = %v, want nil", err)
	}
	defer client.Close()

	if client.Connected() {
		t.Error("Connected() = true with no socket")
	}
	if client.Welcome() != nil {
		t.Error("Welcome() is non-nil with no host")
	}
	if client.Granted("ctrl+f") {
		t.Error("Granted() = true with no host")
	}
	// Sends are no-ops rather than errors, so a panel needs no branch for it.
	if err := client.Log("info", "hello"); err != nil {
		t.Errorf("Log() with no host = %v, want nil", err)
	}
	if err := client.Navigate("p", ""); err != nil {
		t.Errorf("Navigate() with no host = %v, want nil", err)
	}
	if _, ok := <-client.Events(); ok {
		t.Error("Events() delivered something with no host")
	}
}

func TestConnectRequiresAnAppName(t *testing.T) {
	if _, err := Connect(context.Background(), Options{}); err == nil {
		t.Error("Connect() without App = nil error")
	}
}

// Focus and blur become the pager's own types, which is what lets a page
// written for the in-process drawer run unchanged in a sidecar.
func TestToMsgUsesPagerFocusTypes(t *testing.T) {
	if _, ok := ToMsg(FocusEvent{}).(pager.FocusMsg); !ok {
		t.Errorf("FocusEvent became %T, want pager.FocusMsg", ToMsg(FocusEvent{}))
	}
	if _, ok := ToMsg(BlurEvent{}).(pager.BlurMsg); !ok {
		t.Errorf("BlurEvent became %T, want pager.BlurMsg", ToMsg(BlurEvent{}))
	}
}

func TestToMsgCoversEveryEvent(t *testing.T) {
	events := []Event{
		WelcomeEvent{Welcome: &panelproto.Welcome{}},
		ThemeEvent{},
		IconsEvent{},
		FocusEvent{},
		BlurEvent{},
		WorkspaceEvent{},
		WorkspacesUpdatedEvent{},
		SettingsEvent{},
		DisconnectedEvent{},
		ErrorEvent{},
		CloseEvent{},
	}
	for _, ev := range events {
		if ToMsg(ev) == nil {
			t.Errorf("ToMsg(%T) = nil; every event needs a message", ev)
		}
	}
}

// The only useful response to a close frame is to exit.
func TestToMsgCloseQuits(t *testing.T) {
	if _, ok := ToMsg(CloseEvent{}).(tea.QuitMsg); !ok {
		t.Errorf("CloseEvent became %T, want tea.QuitMsg", ToMsg(CloseEvent{}))
	}
}

// There is no InitialWindowSize test because there is no InitialWindowSize: a
// sidecar's stdout IS the pane's PTY, so bubbletea's own measurement is already
// the pane's size and the host never had a better answer to send.

func TestEnvironment(t *testing.T) {
	t.Setenv(panelproto.EnvPanelID, "plugin-x")
	t.Setenv(panelproto.EnvProtocol, panelproto.Version)
	t.Setenv(panelproto.EnvSocket, "/tmp/whatever")

	id, proto, has := Environment()
	if id != "plugin-x" || proto != panelproto.Version || !has {
		t.Errorf("Environment() = %q, %q, %v", id, proto, has)
	}

	t.Setenv(panelproto.EnvSocket, "")
	if _, _, has := Environment(); has {
		t.Error("Environment() reported a socket when the variable is empty")
	}
}
