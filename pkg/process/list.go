package process

import (
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Entry is one live process as reported by List: its identity, its parent, how
// long it has been running, and its full command line.
//
// Args is the whole argv joined by spaces, exactly as ps renders it. Callers
// that need to identify a process by what it was asked to do — "which daemon
// is serving THIS socket path" — match against Args rather than against the
// executable name, because a process name alone is never proof of ownership.
type Entry struct {
	PID  int
	PPID int
	// Elapsed is how long the process has been running. Zero when ps reported
	// an elapsed time this parser did not understand.
	Elapsed time.Duration
	Args    string
}

// List enumerates every process visible to the caller.
//
// It shells out to ps rather than reading /proc because macOS has no /proc and
// this package must behave identically on both platforms. Errors from ps are
// returned as-is; a caller that only wants a best-effort census should treat a
// failure as "no candidates" rather than as fatal.
func List() ([]Entry, error) {
	// args= must come last: ps renders it unpadded to the end of the line, so
	// any column after it would be swallowed by the command line itself.
	out, err := exec.Command("ps", "-axo", "pid=,ppid=,etime=,args=").Output()
	if err != nil {
		return nil, err
	}

	var entries []Entry
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// pid, ppid and etime are whitespace-free and right-aligned by ps, so
		// they cannot be split with a fixed separator count — peel them off one
		// run of spaces at a time and keep whatever remains as argv.
		fields := make([]string, 0, 4)
		rest := line
		for len(fields) < 3 {
			rest = strings.TrimLeft(rest, " ")
			idx := strings.IndexByte(rest, ' ')
			if idx < 0 {
				break
			}
			fields = append(fields, rest[:idx])
			rest = rest[idx+1:]
		}
		if len(fields) < 3 {
			continue
		}
		fields = append(fields, strings.TrimLeft(rest, " "))

		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid <= 0 {
			continue
		}
		ppid, _ := strconv.Atoi(fields[1])
		entries = append(entries, Entry{
			PID:     pid,
			PPID:    ppid,
			Elapsed: ParseElapsed(fields[2]),
			Args:    fields[3],
		})
	}
	return entries, nil
}

// ParseElapsed converts ps's etime column ("[[dd-]hh:]mm:ss") into a duration.
// Anything it cannot parse yields 0, which callers must read as "unknown", not
// as "just started" — a sweep that kills on age must skip an unknown age.
func ParseElapsed(s string) time.Duration {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	var days int
	if dash := strings.IndexByte(s, '-'); dash >= 0 {
		d, err := strconv.Atoi(s[:dash])
		if err != nil {
			return 0
		}
		days = d
		s = s[dash+1:]
	}

	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0
	}
	nums := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0
		}
		nums[i] = n
	}

	var hours, minutes, seconds int
	if len(parts) == 3 {
		hours, minutes, seconds = nums[0], nums[1], nums[2]
	} else {
		minutes, seconds = nums[0], nums[1]
	}

	return time.Duration(days)*24*time.Hour +
		time.Duration(hours)*time.Hour +
		time.Duration(minutes)*time.Minute +
		time.Duration(seconds)*time.Second
}
