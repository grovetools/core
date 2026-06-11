package checks

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/grovetools/core/pkg/daemon"
	"github.com/grovetools/core/pkg/doctor"
	"github.com/grovetools/core/pkg/models"
	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/workspace"
)

func init() {
	doctor.Register(&daemonVersionCheck{
		getenv:          os.Getenv,
		getwd:           os.Getwd,
		resolveScope:    workspace.ResolveScope,
		socketPath:      paths.SocketPath,
		queryDaemonInfo: queryDaemonInfoViaSocket,
	})
}

type daemonVersionCheck struct {
	getenv          func(string) string
	getwd           func() (string, error)
	resolveScope    func(dir string) string
	socketPath      func(...string) string
	queryDaemonInfo func(socketPath string) (*models.SystemInfo, error)
}

func (c *daemonVersionCheck) ID() string   { return "daemon_version" }
func (c *daemonVersionCheck) Name() string { return "daemon version exposable via /api/system/info" }

func (c *daemonVersionCheck) Run(ctx context.Context, opts doctor.RunOptions) doctor.CheckResult {
	res := doctor.CheckResult{ID: c.ID(), Name: c.Name()}

	currentScope := c.currentScope()
	if currentScope == "" {
		res.Status = doctor.StatusOK
		res.Message = "no current scope; skipping daemon version check"
		return res
	}

	// Get the daemon socket path for this scope
	socketPath := c.socketPath(currentScope)

	// Query daemon for its version info
	daemonInfo, err := c.queryDaemonInfo(socketPath)
	if err != nil {
		res.Status = doctor.StatusWarn
		res.Message = fmt.Sprintf("could not query daemon (/api/system/info): %v", err)
		return res
	}

	if daemonInfo.Commit == "" {
		res.Status = doctor.StatusWarn
		res.Message = "daemon returned empty commit; daemon binary may be stale or dev-built"
		return res
	}

	res.Status = doctor.StatusOK
	res.Message = fmt.Sprintf("daemon running commit %s (built %s)", shortSHA(daemonInfo.Commit), daemonInfo.BuildDate)
	return res
}

func (c *daemonVersionCheck) AutoFix(ctx context.Context) error {
	// Version checks are informational only; not auto-fixable
	return doctor.ErrNotFixable
}

func (c *daemonVersionCheck) currentScope() string {
	if s := strings.TrimSpace(c.getenv("GROVE_SCOPE")); s != "" {
		return s
	}
	cwd, err := c.getwd()
	if err != nil {
		return ""
	}
	return c.resolveScope(cwd)
}

// queryDaemonInfoViaSocket queries the daemon's /api/system/info endpoint.
// It connects directly to the Unix socket and makes an HTTP GET request.
func queryDaemonInfoViaSocket(socketPath string) (*models.SystemInfo, error) {
	// Try to create a remote client to verify the socket is reachable
	_, err := daemon.NewRemoteClient(socketPath)
	if err != nil {
		return nil, fmt.Errorf("daemon socket unreachable: %w", err)
	}

	// Dial the Unix socket
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to dial socket %s: %w", socketPath, err)
	}
	defer conn.Close()

	// Send HTTP GET request
	req := "GET /api/system/info HTTP/1.1\r\nHost: daemon\r\nConnection: close\r\n\r\n"
	if _, err := fmt.Fprint(conn, req); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Parse HTTP response
	httpResp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to read HTTP response: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned status %d", httpResp.StatusCode)
	}

	var info models.SystemInfo
	if err := json.NewDecoder(httpResp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode system info: %w", err)
	}

	return &info, nil
}

// shortSHA returns the first 7 characters of a SHA or the full string if shorter
func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
