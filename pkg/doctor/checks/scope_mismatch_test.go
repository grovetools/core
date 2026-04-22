package checks

import (
	"context"
	"errors"
	"testing"

	"github.com/grovetools/core/pkg/doctor"
)

func newScopeCheck(envVal, cwd, gitRoot string, gitErr error) *scopeMismatchCheck {
	return &scopeMismatchCheck{
		getenv: func(k string) string {
			if k == "GROVE_SCOPE" {
				return envVal
			}
			return ""
		},
		getwd:   func() (string, error) { return cwd, nil },
		gitRoot: func(string) (string, error) { return gitRoot, gitErr },
	}
}

func TestScopeMismatch_Matches(t *testing.T) {
	c := newScopeCheck("/repo", "/repo/subdir", "/repo", nil)
	res := c.Run(context.Background(), doctor.RunOptions{})
	if res.Status != doctor.StatusOK {
		t.Fatalf("expected OK, got %s: %s", res.Status, res.Message)
	}
}

func TestScopeMismatch_Mismatch(t *testing.T) {
	c := newScopeCheck("/other", "/repo", "/repo", nil)
	res := c.Run(context.Background(), doctor.RunOptions{})
	if res.Status != doctor.StatusWarn {
		t.Fatalf("expected Warn, got %s", res.Status)
	}
	if res.Resolution == "" {
		t.Fatalf("expected resolution text")
	}
}

func TestScopeMismatch_NotAGitRepo(t *testing.T) {
	c := newScopeCheck("/x", "/tmp", "", errors.New("not a git repo"))
	res := c.Run(context.Background(), doctor.RunOptions{})
	if res.Status != doctor.StatusOK {
		t.Fatalf("expected OK for non-git cwd, got %s", res.Status)
	}
}

func TestScopeMismatch_AutoFixReturnsNotFixable(t *testing.T) {
	c := newScopeCheck("/a", "/b", "/b", nil)
	err := c.AutoFix(context.Background())
	if err == nil || !errors.Is(err, doctor.ErrNotFixable) {
		t.Fatalf("expected ErrNotFixable, got %v", err)
	}
}
