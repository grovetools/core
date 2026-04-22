package checks

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/grovetools/core/pkg/doctor"
)

func init() {
	doctor.Register(&scopeMismatchCheck{
		getenv:  os.Getenv,
		getwd:   os.Getwd,
		gitRoot: defaultGitRoot,
	})
}

type scopeMismatchCheck struct {
	getenv  func(string) string
	getwd   func() (string, error)
	gitRoot func(dir string) (string, error)
}

func (c *scopeMismatchCheck) ID() string   { return "scope_mismatch" }
func (c *scopeMismatchCheck) Name() string { return "GROVE_SCOPE matches cwd's git root" }

func (c *scopeMismatchCheck) Run(ctx context.Context, opts doctor.RunOptions) doctor.CheckResult {
	res := doctor.CheckResult{ID: c.ID(), Name: c.Name()}

	scope := strings.TrimSpace(c.getenv("GROVE_SCOPE"))
	cwd, err := c.getwd()
	if err != nil {
		res.Status = doctor.StatusWarn
		res.Message = fmt.Sprintf("unable to read cwd: %v", err)
		return res
	}

	root, err := c.gitRoot(cwd)
	if err != nil || root == "" {
		res.Status = doctor.StatusOK
		res.Message = "cwd is not a git repository; scope routing not applicable"
		return res
	}

	if scope == "" {
		res.Status = doctor.StatusOK
		res.Message = fmt.Sprintf("GROVE_SCOPE unset; daemon calls will auto-resolve from cwd (%s)", root)
		return res
	}

	absScope, _ := filepath.Abs(scope)
	absRoot, _ := filepath.Abs(root)
	if absScope == absRoot {
		res.Status = doctor.StatusOK
		res.Message = fmt.Sprintf("cwd matches GROVE_SCOPE (%s)", absRoot)
		return res
	}

	res.Status = doctor.StatusWarn
	res.Message = fmt.Sprintf("GROVE_SCOPE=%s but cwd git root is %s; daemon calls route to the scoped daemon, not cwd", scope, root)
	res.Resolution = fmt.Sprintf("run `export GROVE_SCOPE=%s` or `unset GROVE_SCOPE` to align routing", root)
	res.Fixable = false
	return res
}

func (c *scopeMismatchCheck) AutoFix(ctx context.Context) error {
	return fmt.Errorf("%w: scope mismatch must be fixed manually — run the suggested export in your shell", doctor.ErrNotFixable)
}

func defaultGitRoot(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
