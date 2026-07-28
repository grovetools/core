// Package procsample provides system-wide process sampling built on a single
// `ps` invocation per snapshot. It computes interval-true CPU percentages from
// cumulative cputime deltas between consecutive samples, exposes the process
// tree (children, transitive subtrees), aggregates subtrees into rollups,
// classifies "orphan" processes of interest that live outside tracked
// subtrees, and offers an escalating subtree kill (TERM, grace, KILL).
//
// The package never polls on its own — callers own the sampling cadence.
// It works on darwin and linux and uses no cgo.
package procsample

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Proc is one parsed process from a system-wide snapshot.
type Proc struct {
	PID  int
	PPID int
	// Comm is the executable name (basename only; ps may report a full path
	// on darwin). It can contain spaces, e.g. "Google Chrome Helper".
	Comm  string
	RSSKB int64
	// CPUTime is the cumulative CPU time consumed by the process (ps time=).
	CPUTime time.Duration
	// PctCPU is the decaying-average CPU% reported by ps pcpu=. It is only a
	// fallback for the first sample; prefer Sample.CPU for interval-true
	// values.
	PctCPU float64
}

// runPS executes the ps snapshot command. It is a variable so tests can
// substitute fixture output without spawning processes.
var runPS = func() ([]byte, error) {
	return exec.Command("ps", "-axo", "pid=,ppid=,pcpu=,rss=,time=,comm=").Output()
}

// Snapshot runs one `ps -axo pid=,ppid=,pcpu=,rss=,time=,comm=` and parses the
// output into a pid-keyed map. Malformed lines are skipped.
func Snapshot() (map[int]Proc, error) {
	out, err := runPS()
	if err != nil {
		return nil, fmt.Errorf("procsample: ps failed: %w", err)
	}
	return parseSnapshot(out), nil
}

// parseSnapshot parses full ps output, skipping lines that fail to parse.
func parseSnapshot(out []byte) map[int]Proc {
	procs := make(map[int]Proc)
	for _, line := range strings.Split(string(out), "\n") {
		p, ok := parseLine(line)
		if !ok {
			continue
		}
		procs[p.PID] = p
	}
	return procs
}

// parseLine parses one ps output line. The first five fields are
// whitespace-delimited; everything after them is the command, which may
// itself contain spaces.
func parseLine(line string) (Proc, bool) {
	fields, comm := splitFields(line, 5)
	if len(fields) != 5 || comm == "" {
		return Proc{}, false
	}
	pid, err := strconv.Atoi(fields[0])
	if err != nil {
		return Proc{}, false
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return Proc{}, false
	}
	pcpu, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return Proc{}, false
	}
	rss, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil {
		return Proc{}, false
	}
	cputime, err := parseCPUTime(fields[4])
	if err != nil {
		return Proc{}, false
	}
	if strings.ContainsRune(comm, '/') {
		comm = filepath.Base(comm)
	}
	return Proc{
		PID:     pid,
		PPID:    ppid,
		Comm:    comm,
		RSSKB:   rss,
		CPUTime: cputime,
		PctCPU:  pcpu,
	}, true
}

// splitFields extracts up to n whitespace-delimited fields from the front of
// line and returns them together with the trimmed remainder. Unlike
// strings.Fields it preserves the remainder verbatim (minus surrounding
// whitespace), so a trailing command containing spaces survives intact.
func splitFields(line string, n int) ([]string, string) {
	rest := line
	fields := make([]string, 0, n)
	for len(fields) < n {
		rest = strings.TrimLeft(rest, " \t")
		if rest == "" {
			break
		}
		j := strings.IndexAny(rest, " \t")
		if j < 0 {
			fields = append(fields, rest)
			rest = ""
			break
		}
		fields = append(fields, rest[:j])
		rest = rest[j:]
	}
	return fields, strings.TrimSpace(rest)
}

// parseCPUTime parses cumulative CPU time as printed by ps time=:
// [[HH:]MM:]SS with an optional .fraction (darwin prints e.g. "0:00.12"),
// and the linux days form "D-HH:MM:SS" (e.g. "1-02:03:04").
func parseCPUTime(s string) (time.Duration, error) {
	orig := s
	var days int
	if i := strings.IndexByte(s, '-'); i >= 0 {
		d, err := strconv.Atoi(s[:i])
		if err != nil || d < 0 {
			return 0, fmt.Errorf("procsample: bad time %q", orig)
		}
		days = d
		s = s[i+1:]
	}
	var frac float64
	if i := strings.IndexByte(s, '.'); i >= 0 {
		f, err := strconv.ParseFloat("0"+s[i:], 64)
		if err != nil {
			return 0, fmt.Errorf("procsample: bad time %q", orig)
		}
		frac = f
		s = s[:i]
	}
	parts := strings.Split(s, ":")
	if len(parts) == 0 || len(parts) > 3 {
		return 0, fmt.Errorf("procsample: bad time %q", orig)
	}
	var secs int64
	for _, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil || v < 0 {
			return 0, fmt.Errorf("procsample: bad time %q", orig)
		}
		secs = secs*60 + int64(v)
	}
	return time.Duration(days)*24*time.Hour +
		time.Duration(secs)*time.Second +
		time.Duration(frac*float64(time.Second)), nil
}
