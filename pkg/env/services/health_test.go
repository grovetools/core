package services

import (
	"net"
	"testing"
	"time"
)

func TestProbeTCP_Success(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	if !ProbeTCP(ln.Addr().String(), 500*time.Millisecond) {
		t.Error("expected probe to succeed")
	}
}

func TestProbeTCP_Failure(t *testing.T) {
	// Bind and immediately close to get a known-free port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	if ProbeTCP(addr, 200*time.Millisecond) {
		t.Error("expected probe to fail on closed port")
	}
}
