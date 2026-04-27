package checks

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/grovetools/core/pkg/doctor"
	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/workspace"
)

func init() {
	doctor.Register(&staleDaemonCheck{
		stateDir:       paths.StateDir,
		binDir:         paths.BinDir,
		getenv:         os.Getenv,
		getwd:          os.Getwd,
		resolveScope:   workspace.ResolveScope,
		lsofBinaryPath: defaultLsofBinaryPath,
		statInode:      defaultStatInode,
		killProcess:    defaultKill,
	})
}

type staleDaemonCheck struct {
	stateDir       func() string
	binDir         func() string
	getenv         func(string) string
	getwd          func() (string, error)
	resolveScope   func(dir string) string
	lsofBinaryPath func(pid int) (string, error)
	statInode      func(path string) (uint64, error)
	killProcess    func(pid int, sig syscall.Signal) error

	stalePID   int
	staleScope string
}

func (c *staleDaemonCheck) ID() string   { return "stale_daemon" }
func (c *staleDaemonCheck) Name() string { return "daemon binary matches on-disk groved" }

func (c *staleDaemonCheck) Run(ctx context.Context, opts doctor.RunOptions) doctor.CheckResult {
	res := doctor.CheckResult{ID: c.ID(), Name: c.Name()}
	c.stalePID = 0
	c.staleScope = ""

	currentScope := c.currentScope()
	if currentScope == "" {
		res.Status = doctor.StatusOK
		res.Message = "no current scope (cwd/GROVE_SCOPE); skipping stale-binary check"
		return res
	}

	targetBinary := filepath.Join(c.binDir(), "groved")
	targetInode, err := c.statInode(targetBinary)
	if err != nil {
		res.Status = doctor.StatusWarn
		res.Message = fmt.Sprintf("could not stat %s: %v", targetBinary, err)
		return res
	}

	pidPath := paths.PidFilePath(currentScope)
	if _, statErr := os.Stat(pidPath); statErr != nil {
		res.Status = doctor.StatusOK
		res.Message = fmt.Sprintf("no daemon pid file for scope %s", currentScope)
		return res
	}
	pid, err := readPid(pidPath)
	if err != nil {
		res.Status = doctor.StatusWarn
		res.Message = fmt.Sprintf("pid file %s unreadable: %v", pidPath, err)
		return res
	}
	if !processAlive(pid) {
		res.Status = doctor.StatusOK
		res.Message = fmt.Sprintf("daemon pid %d for scope %s not running (orphan pid handled by orphan_sockets)", pid, currentScope)
		return res
	}

	running, err := c.lsofBinaryPath(pid)
	if err != nil {
		res.Status = doctor.StatusWarn
		res.Message = fmt.Sprintf("could not inspect pid %d via lsof: %v", pid, err)
		return res
	}
	runningInode, err := c.statInode(running)
	if err != nil {
		res.Status = doctor.StatusWarn
		res.Message = fmt.Sprintf("could not stat running binary %s: %v", running, err)
		return res
	}

	if runningInode == targetInode {
		res.Status = doctor.StatusOK
		res.Message = fmt.Sprintf("daemon pid %d (scope %s) runs current groved", pid, currentScope)
		return res
	}

	c.stalePID = pid
	c.staleScope = currentScope
	res.Status = doctor.StatusFail
	res.Message = fmt.Sprintf("daemon pid %d (scope %s) is running a stale binary (%s); on-disk target is %s", pid, currentScope, running, targetBinary)
	res.Resolution = fmt.Sprintf("run 'grove doctor --fix' to stop pid %d; next client call will auto-start the current binary", pid)
	res.Fixable = true
	return res
}

func (c *staleDaemonCheck) AutoFix(ctx context.Context) error {
	if c.stalePID == 0 {
		return fmt.Errorf("no stale daemon recorded; run the check first")
	}
	// Safety: refuse to touch the unscoped or grovetools-main daemon.
	if c.staleScope == "" || strings.HasPrefix(filepath.Base(c.staleScope), "grovetools") {
		return fmt.Errorf("refusing to kill daemon for protected scope %q", c.staleScope)
	}
	if err := c.killProcess(c.stalePID, syscall.SIGTERM); err != nil {
		return fmt.Errorf("SIGTERM pid %d: %w", c.stalePID, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !processAlive(c.stalePID) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	if processAlive(c.stalePID) {
		if err := c.killProcess(c.stalePID, syscall.SIGKILL); err != nil {
			return fmt.Errorf("SIGKILL pid %d: %w", c.stalePID, err)
		}
	}
	return nil
}

func (c *staleDaemonCheck) currentScope() string {
	if s := strings.TrimSpace(c.getenv("GROVE_SCOPE")); s != "" {
		return s
	}
	cwd, err := c.getwd()
	if err != nil {
		return ""
	}
	return c.resolveScope(cwd)
}

func readPid(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// defaultLsofBinaryPath parses `lsof -p <pid> -Fn` output and returns the
// path of the process's executable (txt-type file descriptor entry). The
// -F n output alternates `p<pid>` / `f<fd>` / `t<type>` / `n<name>` records;
// we hold the most recent `t` value and emit the `n` when t == "txt".
func defaultLsofBinaryPath(pid int) (string, error) {
	out, err := exec.Command("lsof", "-p", strconv.Itoa(pid), "-F", "ftn").Output() //nolint:gosec // pid is from trusted internal source
	if err != nil {
		return "", err
	}
	var curType, firstN string
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		tag, rest := line[0], line[1:]
		switch tag {
		case 'f':
			curType = ""
		case 't':
			curType = rest
		case 'n':
			if curType == "txt" && looksLikeGroved(rest) {
				return rest, nil
			}
			if curType == "txt" && firstN == "" {
				firstN = rest
			}
		}
	}
	if firstN != "" {
		return firstN, nil
	}
	return "", fmt.Errorf("no txt mapping found for pid %d", pid)
}

var grovedRe = regexp.MustCompile(`(^|/)groved(\.[^/]*)?$`)

func looksLikeGroved(p string) bool {
	return grovedRe.MatchString(p)
}

func defaultStatInode(path string) (uint64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	sys, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("stat: unexpected sys type for %s", path)
	}
	return sys.Ino, nil
}

func defaultKill(pid int, sig syscall.Signal) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(sig)
}
