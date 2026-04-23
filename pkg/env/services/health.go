package services

import (
	"net"
	"time"
)

// ProbeTCP attempts a single TCP connection to addr with the given timeout.
// Returns true if the connection succeeds.
//
// The retry loop lives in the daemon — this helper is only the single probe.
func ProbeTCP(addr string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
