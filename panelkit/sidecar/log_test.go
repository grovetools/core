package sidecar

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/sys/unix"

	"github.com/grovetools/core/panelkit/panelproto"
)

// nextFrame reads one app→host frame, so a test can assert on what the sidecar
// actually put on the wire rather than on an internal call.
func nextFrame(t *testing.T, conn *panelproto.Conn) panelproto.Frame {
	t.Helper()
	type result struct {
		f   panelproto.Frame
		err error
	}
	ch := make(chan result, 1)
	go func() {
		f, err := conn.Read()
		ch <- result{f, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("reading an app→host frame: %v", r.err)
		}
		return r.f
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for an app→host frame")
		return panelproto.Frame{}
	}
}

// The defect: Logf nil meant discard, so a panel that configured nothing got no
// diagnostics at all, and the obvious fix — log.Printf — writes to the pane.
// Unset, diagnostics now go where a log line belongs.
func TestLogfDefaultsToTheHostLog(t *testing.T) {
	host := startFakeHost(t, panelproto.Welcome{Protocol: panelproto.Version, PanelID: "p"})

	client, err := Connect(context.Background(), Options{App: "t"}) // no Logf
	if err != nil {
		t.Fatalf("Connect() = %v", err)
	}
	defer client.Close()
	conn := host.conn()
	nextEvent(t, client) // welcome

	// A frame this build does not know is one of the runtime's diagnostics.
	if err := conn.Send("some_future_frame", map[string]int{"x": 1}); err != nil {
		t.Fatal(err)
	}

	l := nextLog(t, conn)
	if !strings.Contains(l.Message, "some_future_frame") {
		t.Errorf("log message = %q, want it to name the frame it dropped", l.Message)
	}
	// A host that grew a frame is not a fault, so this one stays quiet.
	if l.Level != LogDebug {
		t.Errorf("log level = %q, want %q for an unknown frame", l.Level, LogDebug)
	}

	// A frame that would not decode is a delivery the panel lost, and the
	// host's log runs at info — at debug this line would be exactly as silent
	// as the discard it replaced.
	if err := conn.Send(panelproto.TypeTheme, "not a theme object"); err != nil {
		t.Fatal(err)
	}
	l = nextLog(t, conn)
	if l.Level != LogWarn {
		t.Errorf("log level = %q, want %q for a frame that would not decode", l.Level, LogWarn)
	}
	if !strings.Contains(l.Message, "theme frame") {
		t.Errorf("log message = %q, want it to name the frame that failed", l.Message)
	}
}

// nextLog reads the next app→host frame and asserts it is a log line.
func nextLog(t *testing.T, conn *panelproto.Conn) panelproto.Log {
	t.Helper()
	f := nextFrame(t, conn)
	if f.Type != panelproto.TypeLog {
		t.Fatalf("app→host frame = %q, want %q; an unset Logf must reach the host's log", f.Type, panelproto.TypeLog)
	}
	var l panelproto.Log
	if err := panelproto.DecodePayload(f, &l); err != nil {
		t.Fatalf("decoding the log frame: %v", err)
	}
	return l
}

// A configured Logf owns the line: routing it to the host as well would double
// every diagnostic for a panel that did exactly what the field asks.
func TestLogfConfiguredTakesTheLineInsteadOfTheHost(t *testing.T) {
	host := startFakeHost(t, panelproto.Welcome{Protocol: panelproto.Version, PanelID: "p"})

	lines := make(chan string, 4)
	client, err := Connect(context.Background(), Options{
		App:  "t",
		Logf: func(format string, args ...any) { lines <- format },
	})
	if err != nil {
		t.Fatalf("Connect() = %v", err)
	}
	defer client.Close()
	conn := host.conn()
	nextEvent(t, client) // welcome

	if err := conn.Send("some_future_frame", nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-lines:
	case <-time.After(3 * time.Second):
		t.Fatal("the configured Logf never saw the diagnostic")
	}

	// Ordering is the assertion: a log frame would have to arrive before this
	// navigate, because the diagnostic was raised first and one connection
	// carries them in order.
	if err := client.Navigate("notebook", ""); err != nil {
		t.Fatalf("Navigate() = %v", err)
	}
	if f := nextFrame(t, conn); f.Type != panelproto.TypeNavigate {
		t.Errorf("first app→host frame = %q, want %q; a configured Logf must not also log to the host", f.Type, panelproto.TypeNavigate)
	}
}

func TestOpenEditorSendsQuickAndDedicatedRequests(t *testing.T) {
	host := startFakeHost(t, panelproto.Welcome{Protocol: panelproto.Version, PanelID: "plugin-gtd"})
	client, err := Connect(context.Background(), Options{App: "gtd"})
	if err != nil {
		t.Fatalf("Connect() = %v", err)
	}
	defer client.Close()
	conn := host.conn()
	nextEvent(t, client) // welcome

	for _, tc := range []struct {
		path      string
		dedicated bool
	}{{"/notes/one.md", false}, {"/notes/two.md", true}} {
		if err := client.OpenEditor(tc.path, tc.dedicated); err != nil {
			t.Fatalf("OpenEditor() = %v", err)
		}
		f := nextFrame(t, conn)
		if f.Type != panelproto.TypeEditRequest {
			t.Fatalf("frame type = %q, want %q", f.Type, panelproto.TypeEditRequest)
		}
		var got panelproto.EditRequest
		if err := panelproto.DecodePayload(f, &got); err != nil {
			t.Fatalf("decode edit request: %v", err)
		}
		if got.Path != tc.path || got.Dedicated != tc.dedicated {
			t.Errorf("request = %+v, want path=%q dedicated=%v", got, tc.path, tc.dedicated)
		}
	}
}

// With no host there is no control plane to log over, and the fallback must
// still not be stderr. The line is dropped and nothing panics.
func TestLogfWithNoHostDropsTheLine(t *testing.T) {
	t.Setenv(panelproto.EnvSocket, "")

	client, err := Connect(context.Background(), Options{App: "t"})
	if err != nil {
		t.Fatalf("Connect() = %v", err)
	}
	defer client.Close()
	client.logf("diagnostic with nowhere to go: %d", 1)
}

func TestFileLogfWritesLinesAndCreatesTheDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "panel.log")
	logf := FileLogf(path)

	logf("first %s", "line")
	logf("second line\n") // the trailing newline is the caller's habit, not a blank line

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the logfile: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, "first line") || !strings.Contains(got, "second line") {
		t.Errorf("logfile = %q, want both lines", got)
	}
	if n := strings.Count(got, "\n"); n != 2 {
		t.Errorf("logfile has %d lines, want 2: %q", n, got)
	}
}

// A logfile that cannot be opened has to fail quietly. The only streams left to
// complain on are the two this package exists to keep off the user's screen.
func TestFileLogfSwallowsAnUnusablePath(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "notadir")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	logf := FileLogf(filepath.Join(blocker, "panel.log"))
	logf("this goes nowhere")
	logf("and so does this")
}

func TestLogPathHonoursXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/state")

	if got, want := LogPath("breaktimer"), "/state/grove/panels/breaktimer.log"; got != want {
		t.Errorf("LogPath() = %q, want %q", got, want)
	}
	// An app name is a config value, and a config value that walks out of the
	// log directory is not one this function passes on.
	if got := LogPath("../../etc/passwd"); filepath.Dir(got) != "/state/grove/panels" {
		t.Errorf("LogPath(%q) = %q, want it confined to the log directory", "../../etc/passwd", got)
	}
	if got := LogPath(""); got != "/state/grove/panels/panel.log" {
		t.Errorf("LogPath(\"\") = %q, want a usable fallback name", got)
	}
}

// RedirectStderr has to catch the descriptor, not just the Go-level variable:
// the lines worth catching most are the log package's default and the runtime's
// panic output, and neither goes through a variable this package can swap.
func TestRedirectStderrCapturesTheDescriptorAndTheLogPackage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "panel.log")
	restore := saveStderr(t)
	defer restore()

	if err := RedirectStderr(path); err != nil {
		t.Fatalf("RedirectStderr() = %v", err)
	}
	log.Print("from the log package")
	// Written to the number, not to os.Stderr — the case a Go-level swap misses.
	if _, err := unix.Write(2, []byte("straight to fd 2\n")); err != nil {
		t.Fatalf("writing to fd 2: %v", err)
	}
	restore()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the logfile: %v", err)
	}
	for _, want := range []string{"from the log package", "straight to fd 2"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("logfile = %q, want it to contain %q", string(b), want)
		}
	}
}

// Standalone, fd 2 is the developer's own terminal and taking it would swallow
// the panic they ran the binary to see.
func TestCaptureStderrLeavesStderrAloneWithNoHost(t *testing.T) {
	t.Setenv(panelproto.EnvSocket, "")
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	before := os.Stderr
	path, err := CaptureStderr("t")
	if err != nil {
		t.Fatalf("CaptureStderr() = %v", err)
	}
	if path != "" {
		t.Errorf("CaptureStderr() redirected to %q with no host; standalone stderr is the developer's", path)
	}
	if os.Stderr != before {
		t.Error("CaptureStderr() replaced os.Stderr with no host")
	}
}

func TestCaptureStderrRedirectsWhenHosted(t *testing.T) {
	t.Setenv(panelproto.EnvSocket, "/tmp/not-dialled")
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	restore := saveStderr(t)
	defer restore()

	path, err := CaptureStderr("breaktimer")
	if err != nil {
		t.Fatalf("CaptureStderr() = %v", err)
	}
	if _, err := os.Stderr.WriteString("into the file\n"); err != nil {
		t.Fatalf("writing to the redirected stderr: %v", err)
	}
	restore()

	if want := filepath.Join(state, "grove", "panels", "breaktimer.log"); path != want {
		t.Errorf("CaptureStderr() = %q, want %q", path, want)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the logfile: %v", err)
	}
	if !strings.Contains(string(b), "into the file") {
		t.Errorf("logfile = %q, want the redirected line", string(b))
	}
}

// The field has to reach the process, not just the helper: a panel sets
// CaptureStderr and expects the forgotten log.Printf in its own Update to miss
// the frame.
func TestRunCaptureStderrTakesTheStrayLine(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	startFakeHost(t, panelproto.Welcome{Protocol: panelproto.Version, PanelID: "p"})
	restore := saveStderr(t)
	defer restore()

	model := &runModel{update: func(msg tea.Msg) tea.Cmd {
		if _, ok := msg.(WelcomeMsg); ok {
			log.Print("a stray line from an Update")
			return tea.Quit
		}
		return nil
	}}
	runUnderTest(t, model, RunOptions{Options: Options{App: "capture"}, CaptureStderr: true})
	restore()

	b, err := os.ReadFile(filepath.Join(state, "grove", "panels", "capture.log"))
	if err != nil {
		t.Fatalf("reading the logfile: %v", err)
	}
	if !strings.Contains(string(b), "a stray line from an Update") {
		t.Errorf("logfile = %q, want the line the panel wrote to stderr", string(b))
	}
}

// saveStderr duplicates fd 2 and returns a function that puts it, os.Stderr and
// the log package back — without which a test that redirects would take the
// test binary's own stderr with it for the rest of the run.
func saveStderr(t *testing.T) func() {
	t.Helper()
	saved, err := unix.Dup(2)
	if err != nil {
		t.Fatalf("dup(2): %v", err)
	}
	original := os.Stderr
	var once bool
	return func() {
		if once {
			return
		}
		once = true
		_ = unix.Dup2(saved, 2)
		_ = unix.Close(saved)
		os.Stderr = original
		log.SetOutput(original)
	}
}
