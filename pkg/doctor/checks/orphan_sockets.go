package checks

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/grovetools/core/pkg/doctor"
	"github.com/grovetools/core/pkg/paths"
)

func init() {
	doctor.Register(&orphanSocketsCheck{
		runtimeDir: paths.RuntimeDir,
		stateDir:   paths.StateDir,
		dial:       defaultUnixDial,
	})
}

type orphanSocketsCheck struct {
	runtimeDir func() string
	stateDir   func() string
	dial       func(path string, timeout time.Duration) error

	// orphans found during Run(); consumed by AutoFix().
	orphans []string
}

func (c *orphanSocketsCheck) ID() string   { return "orphan_sockets" }
func (c *orphanSocketsCheck) Name() string { return "no orphan groved sockets/pidfiles" }

func (c *orphanSocketsCheck) Run(ctx context.Context, opts doctor.RunOptions) doctor.CheckResult {
	res := doctor.CheckResult{ID: c.ID(), Name: c.Name()}
	c.orphans = nil

	runtime := c.runtimeDir()
	state := c.stateDir()

	sockPattern := filepath.Join(runtime, "groved-*.sock")
	sockets, _ := filepath.Glob(sockPattern)
	// Also consider the legacy unscoped socket.
	if legacy := filepath.Join(runtime, "groved.sock"); fileExists(legacy) {
		sockets = append(sockets, legacy)
	}

	var orphanDescs []string
	for _, sock := range sockets {
		if err := c.dial(sock, 100*time.Millisecond); err == nil {
			continue // alive
		}
		orphanDescs = append(orphanDescs, filepath.Base(sock))
		c.orphans = append(c.orphans, sock)

		// Companion pid file under state dir (same basename, .pid ext).
		if pid := companionPid(sock, state); pid != "" {
			c.orphans = append(c.orphans, pid)
		}
	}

	// Pid files whose process is gone but the socket is already absent.
	pidPattern := filepath.Join(state, "groved-*.pid")
	pids, _ := filepath.Glob(pidPattern)
	if legacy := filepath.Join(state, "groved.pid"); fileExists(legacy) {
		pids = append(pids, legacy)
	}
	for _, pidPath := range pids {
		if containsString(c.orphans, pidPath) {
			continue
		}
		if pidAlive(pidPath) {
			continue
		}
		orphanDescs = append(orphanDescs, filepath.Base(pidPath))
		c.orphans = append(c.orphans, pidPath)
	}

	if len(c.orphans) == 0 {
		res.Status = doctor.StatusOK
		res.Message = "no orphan sockets or pid files under " + runtime
		return res
	}
	res.Status = doctor.StatusWarn
	res.Message = fmt.Sprintf("%d orphan file(s): %s", len(orphanDescs), strings.Join(orphanDescs, ", "))
	res.Resolution = "run 'grove doctor --fix' to remove them"
	res.Fixable = true
	return res
}

func (c *orphanSocketsCheck) AutoFix(ctx context.Context) error {
	for _, p := range c.orphans {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", p, err)
		}
	}
	c.orphans = nil
	return nil
}

func defaultUnixDial(path string, timeout time.Duration) error {
	conn, err := net.DialTimeout("unix", path, timeout)
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}

// companionPid returns the pid file that shares the socket's "groved-<scope>-<hash>"
// prefix, or "" if none.
func companionPid(sock, stateDir string) string {
	base := filepath.Base(sock)
	if !strings.HasSuffix(base, ".sock") {
		return ""
	}
	pid := filepath.Join(stateDir, strings.TrimSuffix(base, ".sock")+".pid")
	if fileExists(pid) {
		return pid
	}
	return ""
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func containsString(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func pidAlive(pidPath string) bool {
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return false
	}
	pid := 0
	for _, ch := range strings.TrimSpace(string(data)) {
		if ch < '0' || ch > '9' {
			return false
		}
		pid = pid*10 + int(ch-'0')
	}
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
