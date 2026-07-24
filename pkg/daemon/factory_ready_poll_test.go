package daemon

import (
	"fmt"
	"net"
	"os"
	"testing"
	"time"
)

// shortSocketPath returns a unix socket path short enough for macOS's
// sun_path limit; t.TempDir can exceed it.
func shortSocketPath(t *testing.T) string {
	t.Helper()
	path := fmt.Sprintf("/tmp/grove-rp-%d-%d.sock", os.Getpid(), time.Now().UnixNano()%1_000_000)
	t.Cleanup(func() { _ = os.Remove(path) })
	return path
}

// TestWaitForDaemonReadyConnectsWithoutPipeEOF is the connect-retry cadence
// regression for the 44s "Plan data: connecting" finding: a daemon whose
// readiness pipe never EOFs (older groved leaks the ready-fd write end into
// long-lived boot children) must still be connected within a poll interval or
// two of its socket binding — never after a multi-second backoff or the full
// handshake timeout.
func TestWaitForDaemonReadyConnectsWithoutPipeEOF(t *testing.T) {
	socketPath := shortSocketPath(t)

	// A pipe whose write end we deliberately keep open for the whole test —
	// the leaked-fd signature. The old implementation blocked on it for the
	// entire handshake timeout.
	readR, readW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer readW.Close()

	// Socket binds 300ms after the "spawn", mimicking a normal boot.
	bindDelay := 300 * time.Millisecond
	go func() {
		time.Sleep(bindDelay)
		listener, listenErr := net.Listen("unix", socketPath)
		if listenErr != nil {
			t.Errorf("test listener: %v", listenErr)
			return
		}
		go func() {
			for {
				conn, acceptErr := listener.Accept()
				if acceptErr != nil {
					return
				}
				_ = conn.Close()
			}
		}()
		time.Sleep(5 * time.Second)
		_ = listener.Close()
	}()

	start := time.Now()
	client := waitForDaemonReady(readR, nil, socketPath, 30*time.Second)
	elapsed := time.Since(start)
	if client == nil {
		t.Fatalf("no client after %.2fs despite bound socket", elapsed.Seconds())
	}
	defer client.Close()

	// bind delay + a couple of poll intervals + dial slack. The point is
	// that this is ~0.5s, not the 30s handshake timeout.
	if elapsed > bindDelay+10*readyPollInterval {
		t.Fatalf("connect took %.2fs; dense polling should connect within ~%.1fs of socket bind",
			elapsed.Seconds(), (bindDelay + 3*readyPollInterval).Seconds())
	}
}

// TestWaitForDaemonReadyFailsFastWhenChildDies verifies the other side of the
// cadence contract: when the spawned child exits without ever binding the
// socket, the caller falls back after a short grace period instead of holding
// the TUI for the full handshake timeout.
func TestWaitForDaemonReadyFailsFastWhenChildDies(t *testing.T) {
	socketPath := shortSocketPath(t)

	exited := make(chan struct{})
	go func() {
		time.Sleep(100 * time.Millisecond)
		close(exited)
	}()

	start := time.Now()
	client := waitForDaemonReady(nil, exited, socketPath, 30*time.Second)
	elapsed := time.Since(start)
	if client != nil {
		client.Close()
		t.Fatal("got a client with no listener bound")
	}
	if elapsed > exitedGracePeriod+2*time.Second {
		t.Fatalf("dead-child fallback took %.2fs; want ~%.1fs grace, never the full timeout",
			elapsed.Seconds(), exitedGracePeriod.Seconds())
	}
}
