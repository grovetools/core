package daemon

import (
	"fmt"
	"os"
	"testing"
)

func TestPairPIDEnvFormat(t *testing.T) {
	want := fmt.Sprintf("%s=4321", GroveDaemonPairPIDEnv)
	if got := PairPIDEnv(4321); got != want {
		t.Errorf("PairPIDEnv(4321) = %q, want %q", got, want)
	}
}

// The variable is read by daemons at boot, not by the harness that set it, so
// every value it can carry has to resolve to either "pair with a live process"
// or "do not pair" — never to "pair with something questionable".
func TestPairPIDFromEnv(t *testing.T) {
	cases := []struct {
		name string
		set  bool
		val  string
		want int
	}{
		{name: "unset", want: 0},
		{name: "live pid pairs", set: true, val: fmt.Sprint(os.Getpid()), want: os.Getpid()},
		{name: "empty", set: true, val: "", want: 0},
		{name: "garbage", set: true, val: "not-a-pid", want: 0},
		{name: "zero", set: true, val: "0", want: 0},
		{name: "negative", set: true, val: "-1", want: 0},
		// PID 1 is init: pairing to it would mean "live forever", which is the
		// opposite of what any caller of this variable wants.
		{name: "pid 1 is refused", set: true, val: "1", want: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.set {
				t.Setenv(GroveDaemonPairPIDEnv, tc.val)
			} else {
				t.Setenv(GroveDaemonPairPIDEnv, "")
			}
			if got := PairPIDFromEnv(); got != tc.want {
				t.Errorf("PairPIDFromEnv() = %d, want %d", got, tc.want)
			}
		})
	}
}

// A dead PID must NOT pair. Pairing to a process that has already exited would
// make the daemon shut itself down moments after boot — the failure mode is an
// auto-started daemon that appears to work and then vanishes, which is far
// harder to diagnose than simply not pairing.
func TestPairPIDFromEnvRefusesDeadPID(t *testing.T) {
	t.Setenv(GroveDaemonPairPIDEnv, fmt.Sprint(deadPID(t)))
	if got := PairPIDFromEnv(); got != 0 {
		t.Errorf("PairPIDFromEnv() = %d for a dead pid, want 0", got)
	}
}
