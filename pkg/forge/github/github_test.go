package github_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/grovetools/core/pkg/forge"
	"github.com/grovetools/core/pkg/forge/forgetest"
	"github.com/grovetools/core/pkg/forge/github"
)

// TestArgvBuilder pins the exact `gh` invocations. The argv is the wire
// protocol of this provider: a silently dropped --state or a --limit that
// stops being passed is a correctness bug the JSON-level tests cannot see.
func TestArgvBuilder(t *testing.T) {
	repo := forge.Repo{Host: "github.com", Owner: "grove", Name: "flow"}

	t.Run("ListPRs", func(t *testing.T) {
		fake := &fakeGH{scenario: forgetest.Scenario{Repo: repo}}
		p := github.New(github.WithRunner(fake))
		if _, err := p.ListPRs(context.Background(), repo, forge.ListPROptions{State: forge.StateAll, Limit: 25}); err != nil {
			t.Fatalf("ListPRs failed: %v", err)
		}
		want := []string{
			"pr", "list",
			"--repo", "grove/flow",
			"--state", "all",
			"--limit", "25",
			"--json", "number,title,state,isDraft,author,headRefName,headRefOid,baseRefName,url,createdAt,updatedAt,mergedAt,labels",
		}
		assertArgv(t, fake.calls[0], want)
	})

	t.Run("ListPRs defaults to open and the item ceiling", func(t *testing.T) {
		fake := &fakeGH{scenario: forgetest.Scenario{Repo: repo}}
		p := github.New(github.WithRunner(fake))
		if _, err := p.ListPRs(context.Background(), repo, forge.ListPROptions{}); err != nil {
			t.Fatalf("ListPRs failed: %v", err)
		}
		got := strings.Join(fake.calls[0], " ")
		if !strings.Contains(got, "--state open") {
			t.Errorf("default state filter missing from argv: %s", got)
		}
		if !strings.Contains(got, "--limit 500") {
			t.Errorf("default limit (forge.MaxItems) missing from argv: %s", got)
		}
	})

	t.Run("GetPR", func(t *testing.T) {
		fake := &fakeGH{scenario: forgetest.Scenario{Repo: repo, PRs: forgetest.MakePRs(3)}}
		p := github.New(github.WithRunner(fake))
		if _, err := p.GetPR(context.Background(), repo, 2); err != nil {
			t.Fatalf("GetPR failed: %v", err)
		}
		assertArgv(t, fake.calls[0], []string{
			"pr", "view", "2",
			"--repo", "grove/flow",
			"--json", "number,title,state,isDraft,author,headRefName,headRefOid,baseRefName,url,createdAt,updatedAt,mergedAt,labels",
		})
	})

	t.Run("Checks reads check-runs and the legacy combined status", func(t *testing.T) {
		fake := &fakeGH{scenario: forgetest.Scenario{Repo: repo, CheckRef: "abc123"}}
		p := github.New(github.WithRunner(fake))
		if _, err := p.Checks(context.Background(), repo, "abc123"); err != nil {
			t.Fatalf("Checks failed: %v", err)
		}
		if len(fake.calls) != 2 {
			t.Fatalf("Checks made %d gh calls, want 2 (check-runs + status)", len(fake.calls))
		}
		first := strings.Join(fake.calls[0], " ")
		if !strings.Contains(first, "api --method GET repos/grove/flow/commits/abc123/check-runs?per_page=50&page=1") {
			t.Errorf("unexpected check-runs argv: %s", first)
		}
		second := strings.Join(fake.calls[1], " ")
		if !strings.Contains(second, "repos/grove/flow/commits/abc123/status") {
			t.Errorf("unexpected combined-status argv: %s", second)
		}
	})
}

func assertArgv(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("argv = %v\nwant %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("argv[%d] = %q, want %q\nfull: %v", i, got[i], want[i], got)
		}
	}
}

// TestChecksTruncationIsUnknown pins the bound: once the provider stops paging
// it must say so, and a rollup computed from a partial set is not a state.
func TestChecksTruncationIsUnknown(t *testing.T) {
	repo := forge.Repo{Host: "github.com", Owner: "grove", Name: "flow"}

	// More checks than MaxPages*DefaultPageSize, all passing — so only the
	// truncation rule can keep this from rolling up green.
	checks := make([]forge.Check, forge.MaxItems+10)
	for i := range checks {
		checks[i] = forge.Check{Name: "check", State: forge.CheckStateSuccess}
	}
	fake := &fakeGH{scenario: forgetest.Scenario{Repo: repo, CheckRef: "abc123", Checks: checks}}
	p := github.New(github.WithRunner(fake))

	got, err := p.Checks(context.Background(), repo, "abc123")
	if err != nil {
		t.Fatalf("Checks failed: %v", err)
	}
	if !got.Truncated {
		t.Error("rollup is not marked Truncated despite exceeding the page bound")
	}
	if got.State.IsGreen() {
		t.Errorf("truncated rollup is green (%q); a partial set is not a pass", got.State)
	}
	if got.State != forge.CheckStateUnknown {
		t.Errorf("truncated rollup state = %q, want %q", got.State, forge.CheckStateUnknown)
	}
	// The page bound is a bound: it must not keep calling.
	checkRunCalls := 0
	for _, c := range fake.calls {
		if strings.Contains(strings.Join(c, " "), "check-runs") {
			checkRunCalls++
		}
	}
	if checkRunCalls > forge.MaxPages {
		t.Errorf("made %d check-runs calls, want at most %d", checkRunCalls, forge.MaxPages)
	}
}

// TestGHMissingIsUnavailable covers the real transport when `gh` is not
// installed: ClassUnavailable, and no panic.
func TestGHMissingIsUnavailable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	p := github.New()
	repo := forge.Repo{Host: "github.com", Owner: "grove", Name: "flow"}

	_, err := p.ListPRs(context.Background(), repo, forge.ListPROptions{})
	if err == nil {
		t.Fatal("ListPRs succeeded with no gh on PATH")
	}
	if !forge.IsUnavailable(err) {
		t.Errorf("error class = %q, want %q (err: %v)", forge.ClassOf(err), forge.ClassUnavailable, err)
	}
	if github.Available() {
		t.Error("Available() = true with an empty PATH")
	}

	// Identity resolution must still work: it does no I/O.
	if _, err := p.ResolveRepo("https://github.com/grove/flow"); err != nil {
		t.Errorf("ResolveRepo needs no transport, but failed: %v", err)
	}
}

// TestExecRunnerAgainstFakeGHOnPATH exercises the real exec path end to end
// against a fake `gh` script, proving the argv the provider builds is the argv
// a process actually receives.
func TestExecRunnerAgainstFakeGHOnPATH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake gh is a POSIX shell script")
	}

	dir := t.TempDir()
	argvLog := filepath.Join(dir, "argv.log")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> " + argvLog + "\n" +
		`cat <<'EOF'
[{"number":7,"title":"seven","state":"OPEN","isDraft":false,"url":"https://github.com/grove/flow/pull/7",
  "headRefName":"feature/7","headRefOid":"deadbeef","baseRefName":"main",
  "author":{"login":"alice"},"createdAt":"2026-08-01T12:00:00Z","updatedAt":"2026-08-01T12:00:00Z",
  "mergedAt":null,"labels":[{"name":"enhancement"}]}]
EOF
`
	ghPath := filepath.Join(dir, "gh")
	if err := os.WriteFile(ghPath, []byte(script), 0o755); err != nil { //nolint:gosec // test fixture must be executable
		t.Fatal(err)
	}
	// dir first, so the fake shadows any real gh; the rest of PATH stays so
	// the script's own tools resolve.
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	p := github.New()
	repo := forge.Repo{Host: "github.com", Owner: "grove", Name: "flow"}
	prs, err := p.ListPRs(context.Background(), repo, forge.ListPROptions{State: forge.StateAll, Limit: 5})
	if err != nil {
		t.Fatalf("ListPRs against fake gh failed: %v", err)
	}
	if len(prs) != 1 || prs[0].Number != 7 {
		t.Fatalf("ListPRs = %+v, want one PR numbered 7", prs)
	}
	if prs[0].State != forge.PRStateOpen {
		t.Errorf("state = %q, want %q", prs[0].State, forge.PRStateOpen)
	}
	if prs[0].Author != "alice" || prs[0].HeadSHA != "deadbeef" {
		t.Errorf("PR fields not parsed: %+v", prs[0])
	}

	logged, err := os.ReadFile(argvLog)
	if err != nil {
		t.Fatalf("fake gh was never invoked: %v", err)
	}
	got := strings.TrimSpace(string(logged))
	want := "pr list --repo grove/flow --state all --limit 5 --json " +
		"number,title,state,isDraft,author,headRefName,headRefOid,baseRefName,url,createdAt,updatedAt,mergedAt,labels"
	if got != want {
		t.Errorf("gh received argv:\n  %s\nwant:\n  %s", got, want)
	}
}

// TestNeverIssuesAMutatingCommand is the read-only guard: every method on the
// provider is exercised against a runner that fails the test the moment it
// sees an argv that could change anything on GitHub.
func TestNeverIssuesAMutatingCommand(t *testing.T) {
	mutating := []string{
		"create", "edit", "close", "reopen", "merge", "comment", "delete",
		"review", "ready", "lock", "unlock", "checkout", "sync", "push",
	}
	forbiddenFlags := []string{"--method POST", "--method PUT", "--method PATCH", "--method DELETE", "-X POST", "-f "}

	guard := &guardRunner{
		t: t, mutating: mutating, forbiddenFlags: forbiddenFlags,
		inner: &fakeGH{scenario: forgetest.Scenario{Repo: forgetest.DemoRepo, PRs: forgetest.MakePRs(2), CheckRef: "abc123"}},
	}
	p := github.New(github.WithRunner(guard))

	ctx := context.Background()
	if _, err := p.ListPRs(ctx, forgetest.DemoRepo, forge.ListPROptions{State: forge.StateAll}); err != nil {
		t.Fatalf("ListPRs failed: %v", err)
	}
	if _, err := p.GetPR(ctx, forgetest.DemoRepo, 1); err != nil {
		t.Fatalf("GetPR failed: %v", err)
	}
	if _, err := p.Checks(ctx, forgetest.DemoRepo, "abc123"); err != nil {
		t.Fatalf("Checks failed: %v", err)
	}
	if guard.seen == 0 {
		t.Fatal("the guard saw no gh calls at all")
	}
}

type guardRunner struct {
	t              *testing.T
	inner          *fakeGH
	mutating       []string
	forbiddenFlags []string
	seen           int
}

func (g *guardRunner) Run(ctx context.Context, args []string) ([]byte, []byte, error) {
	g.seen++
	joined := strings.Join(args, " ")
	for _, verb := range g.mutating {
		for _, a := range args {
			if a == verb {
				g.t.Errorf("provider issued a mutating gh command (%q): %s", verb, joined)
			}
		}
	}
	for _, flag := range g.forbiddenFlags {
		if strings.Contains(joined, flag) {
			g.t.Errorf("provider issued a write-shaped gh call (%q): %s", flag, joined)
		}
	}
	return g.inner.Run(ctx, args)
}

// TestNoImplicitTokenRead is a guard on the "no implicit credentials" rule:
// the provider must behave identically whether or not token-shaped variables
// are set, because it never reads them.
func TestNoImplicitTokenRead(t *testing.T) {
	repo := forge.Repo{Host: "github.com", Owner: "grove", Name: "flow"}

	run := func() []string {
		fake := &fakeGH{scenario: forgetest.Scenario{Repo: repo}}
		p := github.New(github.WithRunner(fake))
		if _, err := p.ListPRs(context.Background(), repo, forge.ListPROptions{}); err != nil {
			t.Fatalf("ListPRs failed: %v", err)
		}
		return fake.calls[0]
	}

	before := strings.Join(run(), " ")
	t.Setenv("GH_TOKEN", "ghp_should_never_be_read")
	t.Setenv("GITHUB_TOKEN", "ghp_should_never_be_read")
	t.Setenv("FORGE_TOKEN", "should_never_be_read")
	after := strings.Join(run(), " ")

	if before != after {
		t.Errorf("argv changed when token env vars were set:\n  %s\n  %s", before, after)
	}
	if strings.Contains(after, "should_never_be_read") {
		t.Error("a token from the environment leaked into the gh argv")
	}
}

// TestTransportFailuresAreNeverPermanent covers the pipeline-live trial's
// finding 3 (plan hosted-git-and-prs, .artifacts/forge-pipeline-live/report.md):
// a refused connection surfaced in the poller cache as `(permanent)`, because
// classification is stderr-substring matching and the wording in front of gh
// matched nothing — so the fail-closed fallthrough claimed it.
//
// The class is what a surface renders. "Permanent" says the repository is gone
// or forbidden; "unavailable" says we could not ask. A dead port is always the
// second one, however the transport words it.
func TestTransportFailuresAreNeverPermanent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake gh is a POSIX shell script")
	}
	repo := forge.Repo{Host: "github.com", Owner: "grove", Name: "flow"}

	// Each stderr line is a real refusal wording seen from some transport in
	// front of, or standing in for, gh.
	for _, stderr := range []string{
		"dial tcp 127.0.0.1:1: connect: connection refused",
		"Post \"http://127.0.0.1:1/api\": dial tcp: connection refused",
		"<urlopen error [Errno 61] Connection refused> URLError",
		"ECONNREFUSED 127.0.0.1:1",
		"read tcp 10.0.0.2:53212->140.82.113.6:443: read: connection reset by peer",
		"dial tcp: lookup api.github.com: no such host",
		"connect: no route to host",
		"context deadline exceeded (Client.Timeout exceeded while awaiting headers)",
		"proxyconnect tcp: dial tcp 127.0.0.1:8080: connect: connection refused",
		"Get \"https://api.github.com\": net/http: TLS handshake timeout",
	} {
		t.Run(stderr, func(t *testing.T) {
			dir := t.TempDir()
			script := "#!/bin/sh\nprintf '%s\\n' " + shellQuote(stderr) + " >&2\nexit 1\n"
			if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil { //nolint:gosec // test fixture must be executable
				t.Fatal(err)
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

			_, err := github.New().ListPRs(context.Background(), repo, forge.ListPROptions{})
			if err == nil {
				t.Fatal("ListPRs succeeded against a failing gh")
			}
			if class := forge.ClassOf(err); class == forge.ClassPermanent {
				t.Errorf("class = %q for a transport failure; want retryable or unavailable so the surface renders \"could not ask\" rather than a verdict (err: %v)", class, err)
			}
		})
	}
}

// shellQuote wraps s in single quotes for a POSIX shell, escaping any it
// contains.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
