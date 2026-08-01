package sidecar

import (
	"context"
	"testing"
	"time"

	"github.com/grovetools/core/panelkit/panelproto"
)

// digestPump drains the host side of a connection into a channel.
//
// A channel rather than a read with a deadline, because the framing is a
// bufio.Scanner and a scanner that has returned an error is finished — a
// timed-out read to prove "nothing arrived" would poison every read after it.
// Counting frames as they land answers the same question without ending the
// stream.
func digestPump(t *testing.T, conn *panelproto.Conn) <-chan panelproto.Digest {
	t.Helper()
	out := make(chan panelproto.Digest, 8)
	go func() {
		defer close(out)
		for {
			f, err := conn.Read()
			if err != nil {
				return
			}
			if f.Type != panelproto.TypeDigest {
				continue
			}
			var d panelproto.Digest
			if err := panelproto.DecodePayload(f, &d); err != nil {
				return
			}
			out <- d
		}
	}()
	return out
}

func nextDigest(t *testing.T, ch <-chan panelproto.Digest) panelproto.Digest {
	t.Helper()
	select {
	case d, ok := <-ch:
		if !ok {
			t.Fatal("the control plane closed before a digest arrived")
		}
		return d
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for a digest frame")
		return panelproto.Digest{}
	}
}

// expectNoDigest proves a push was suppressed rather than merely slow: the
// frame it is waiting for would have been written before SetDigest returned.
func expectNoDigest(t *testing.T, ch <-chan panelproto.Digest) {
	t.Helper()
	select {
	case d := <-ch:
		t.Fatalf("a digest arrived (%#v) where the push should have been suppressed", d)
	case <-time.After(250 * time.Millisecond):
	}
}

// The whole point of the helper: a panel may call this on its own tick, and a
// tick that changes nothing must cost the host nothing. Wakes per second are
// publishers times push rate — the host does no diffing — so the suppression
// has to happen here or not at all.
func TestSetDigestSuppressesAnUnchangedPush(t *testing.T) {
	host := startFakeHost(t, panelproto.Welcome{Protocol: panelproto.Version, PanelID: "p"})
	client, err := Connect(context.Background(), Options{App: "t"})
	if err != nil {
		t.Fatalf("Connect() = %v", err)
	}
	defer client.Close()
	frames := digestPump(t, host.conn())

	first := panelproto.Digest{
		Line: "WORK 0:40", Detail: "focus until the bell",
		State: panelproto.DigestStateOK, Icon: "clock",
	}
	if err := client.SetDigest(first); err != nil {
		t.Fatalf("SetDigest() = %v", err)
	}
	if got := nextDigest(t, frames); got != first {
		t.Fatalf("host received %#v, want %#v", got, first)
	}

	// Three identical ticks. None of them is news.
	for i := 0; i < 3; i++ {
		if err := client.SetDigest(first); err != nil {
			t.Fatalf("repeat %d: SetDigest() = %v", i, err)
		}
	}
	expectNoDigest(t, frames)

	// A change in any field is news, including one that only moves the detail.
	changed := first
	changed.Detail = "two minutes left"
	if err := client.SetDigest(changed); err != nil {
		t.Fatalf("SetDigest() = %v", err)
	}
	if got := nextDigest(t, frames); got != changed {
		t.Errorf("host received %#v, want %#v", got, changed)
	}
}

// The suppression is keyed on the payload, so the FIRST push always travels —
// including one that happens to equal the zero digest, which is how a panel
// says "I have nothing to project" before it has ever had anything.
func TestSetDigestAlwaysSendsTheFirstPush(t *testing.T) {
	host := startFakeHost(t, panelproto.Welcome{Protocol: panelproto.Version, PanelID: "p"})
	client, err := Connect(context.Background(), Options{App: "t"})
	if err != nil {
		t.Fatalf("Connect() = %v", err)
	}
	defer client.Close()
	frames := digestPump(t, host.conn())

	if err := client.SetDigest(panelproto.Digest{}); err != nil {
		t.Fatalf("SetDigest() = %v", err)
	}
	if got := nextDigest(t, frames); !got.Empty() {
		t.Errorf("host received %#v, want an empty digest", got)
	}
	if err := client.SetDigest(panelproto.Digest{}); err != nil {
		t.Fatalf("SetDigest() = %v", err)
	}
	expectNoDigest(t, frames)
}

// A panel launched as a plain PTY plugin has no control plane. Publishing is a
// no-op there rather than an error, like every other client send — a panel that
// works in both places should not have to branch.
func TestSetDigestWithNoHostIsANoOp(t *testing.T) {
	t.Setenv(panelproto.EnvSocket, "")
	client, err := Connect(context.Background(), Options{App: "t"})
	if err != nil {
		t.Fatalf("Connect() = %v", err)
	}
	defer client.Close()
	if err := client.SetDigest(panelproto.Digest{Line: "nobody is reading this"}); err != nil {
		t.Errorf("SetDigest() with no host = %v, want nil", err)
	}
}
