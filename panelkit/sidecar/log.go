package sidecar

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// Where a panel's diagnostics can go, and why the obvious answers are wrong.
//
// A plugin child is spawned on a PTY the host owns, and fd 1 and fd 2 are both
// that PTY. Under the alternate screen the renderer only repaints cells it
// drew, so a byte written to either one is not merely noisy: it lands in the
// middle of the user's frame and stays there until the pane is resized or
// closed. That rules out the whole reflex — fmt.Println, log.Printf (which is
// stderr), a library's warning, panic output — and it rules it out silently,
// because the author testing standalone sees the same corruption and reads it
// as a rendering bug.
//
// There are two safe destinations and this file is both:
//
//   - the host's log, over the control plane. Client.Log writes one line to it,
//     and it is where Options.Logf goes when a panel does not set one.
//   - a file. FileLogf points Options.Logf at one; LogPath names the
//     conventional place; RedirectStderr and CaptureStderr send everything the
//     process writes to fd 2 there instead of the pane.

// LogPath is the conventional logfile for a panel: <state>/grove/panels/<app>.log,
// where <state> is $XDG_STATE_HOME or ~/.local/state. It is the same tree
// treemux funnels its own embedded-TUI stderr into, so a user debugging a panel
// looks in one place for both.
//
// The path is returned whether or not anything exists at it; FileLogf and
// RedirectStderr create the directory when they first write.
func LogPath(app string) string {
	name := strings.Map(func(r rune) rune {
		if r == os.PathSeparator || r == '/' || r == 0 {
			return '-'
		}
		return r
	}, strings.TrimSpace(app))
	if name == "" || name == "." || name == ".." {
		name = "panel"
	}
	return filepath.Join(logDir(), name+".log")
}

func logDir() string {
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "grove", "panels")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// No home is not a reason to fall back to stderr — stderr is the pane.
		return filepath.Join(os.TempDir(), "grove", "panels")
	}
	return filepath.Join(home, ".local", "state", "grove", "panels")
}

// FileLogf returns an Options.Logf that appends timestamped lines to path,
// creating the directory on the first write:
//
//	Logf: sidecar.FileLogf(sidecar.LogPath("my-panel")),
//
// It is the answer for a panel that wants its diagnostics even with no host —
// the standalone run, where the control plane the default logs over does not
// exist.
//
// Nothing it does can reach the pane. A path that cannot be opened discards
// instead of reporting, because the only channels left to report on are the two
// this package exists to keep off the user's screen. The file is opened once
// and stays open for the life of the process; there is nothing to close.
func FileLogf(path string) func(format string, args ...any) {
	w := &fileSink{path: path}
	return w.logf
}

type fileSink struct {
	path string

	mu     sync.Mutex
	f      *os.File
	failed bool
}

func (w *fileSink) logf(format string, args ...any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		if w.failed {
			return
		}
		f, err := openLogFile(w.path)
		if err != nil {
			w.failed = true
			return
		}
		w.f = f
	}
	line := strings.TrimRight(fmt.Sprintf(format, args...), "\n")
	_, _ = fmt.Fprintf(w.f, "%s %s\n", time.Now().Format(time.RFC3339), line)
}

func openLogFile(path string) (*os.File, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
}

// RedirectStderr points everything this process writes to stderr at path: fd 2
// itself, os.Stderr, and the log package's default output.
//
// This is the one thing Options.Logf cannot fix, because the lines it catches
// are not the SDK's. A stray log.Printf left in an Update, a dependency that
// warns on startup, a runtime panic — all of them go to fd 2, and fd 2 is the
// pane. Redirecting at the descriptor is what catches the two that a Go-level
// swap misses: code that captured fd 2 at init, and the runtime's own panic
// output, which is the one worth keeping most.
//
// It changes process-global state, which is why it is a call and not a default:
// a library that takes fd 2 without being asked is a worse surprise than the
// corruption it prevents. CaptureStderr is this with the decision of when
// already made.
func RedirectStderr(path string) error {
	f, err := openLogFile(path)
	if err != nil {
		return fmt.Errorf("sidecar: redirect stderr: %w", err)
	}
	if err := unix.Dup2(int(f.Fd()), int(os.Stderr.Fd())); err != nil {
		_ = f.Close()
		return fmt.Errorf("sidecar: redirect stderr: %w", err)
	}
	// The descriptor is the important half; these two are for code that holds
	// the Go-level values rather than the number.
	os.Stderr = f
	log.SetOutput(f)
	return nil
}

// CaptureStderr redirects this process's stderr to LogPath(app) when the panel
// is hosted, and leaves it alone when it is not. It returns the path it
// redirected to, or "" when it left stderr where it was.
//
// The condition is the whole point. Hosted, fd 2 is a pane the user is looking
// at and nothing good can come out of it. Standalone — no GROVE_PANEL_SOCKET,
// the panel run straight from a shell — fd 2 is the developer's own terminal,
// and swallowing a panic there would take away the reason they ran it that way.
//
// RunOptions.CaptureStderr calls this. A sidecar built on Connect rather than
// Run calls it itself, first thing in main.
func CaptureStderr(app string) (string, error) {
	if _, _, hosted := Environment(); !hosted {
		return "", nil
	}
	path := LogPath(app)
	if err := RedirectStderr(path); err != nil {
		return "", err
	}
	return path, nil
}
