// Package forgetest is the shared, fixture-driven conformance suite every
// [forge.Provider] implementation must pass.
//
// It exists because the provider boundary is a contract, not an interface: two
// implementations that both compile can still disagree about whether an empty
// result is an error, whether an unknown check rolls up green, or whether a
// rate limit is retryable. Those disagreements only surface as bugs in the
// poller months later. This suite pins the answers once and runs them against
// every implementation.
//
// An implementation supplies a [Harness]: a function that, given a [Scenario]
// describing what the forge should appear to contain, returns a Provider wired
// to serve exactly that. How it does so is the implementation's business — the
// GitHub harness injects a fake `gh` runner, the Forgejo harness stands up an
// httptest server.
package forgetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/grovetools/core/pkg/forge"
)

// Fault is a failure the harness must make the transport exhibit, so both
// implementations can be asked "what class is this?" and must agree.
type Fault string

const (
	// FaultNone is the healthy path.
	FaultNone Fault = ""
	// FaultAuth is an authentication failure: `gh` not logged in, HTTP 401.
	// Contract: ClassUnavailable — we cannot talk to the forge, so callers
	// must degrade to unknown rather than to "no PRs".
	FaultAuth Fault = "auth"
	// FaultRateLimit is a throttle: GitHub's rate-limit message, HTTP 429.
	// Contract: ClassRetryable.
	FaultRateLimit Fault = "rate_limit"
	// FaultNotFound is a missing repository: HTTP 404.
	// Contract: ClassPermanent.
	FaultNotFound Fault = "not_found"
	// FaultOffline is an unreachable forge: no network, dial failure.
	// Contract: ClassUnavailable.
	FaultOffline Fault = "offline"
	// FaultTransportMissing is the transport itself being absent: `gh` not
	// installed. Contract: ClassUnavailable, and never a panic.
	FaultTransportMissing Fault = "transport_missing"
	// FaultServerError is HTTP 500 / a 5xx from the forge.
	// Contract: ClassRetryable.
	FaultServerError Fault = "server_error"
)

// Scenario is the fixture a harness must serve.
type Scenario struct {
	// Repo is the repository the provider will be asked about.
	Repo forge.Repo
	// PRs are the pull requests the forge should report, in order. The harness
	// is responsible for paging them if its transport pages.
	PRs []forge.PullRequest
	// Checks are the checks the forge should report for CheckRef.
	Checks []forge.Check
	// CheckRef is the ref Checks are attached to.
	CheckRef string
	// Fault, if set, makes every call fail in the named way.
	Fault Fault
}

// Harness adapts one implementation to the suite.
type Harness struct {
	// Name is the provider's name; asserted against Provider.Name().
	Name string
	// New returns a provider serving s. It may register t.Cleanup.
	New func(t *testing.T, s Scenario) forge.Provider
	// SkipFaults lists faults this transport cannot simulate. Keep it empty
	// where possible: a skipped fault is an unverified contract.
	SkipFaults []Fault
}

func (h Harness) skips(f Fault) bool {
	for _, s := range h.SkipFaults {
		if s == f {
			return true
		}
	}
	return false
}

// RunConformance runs the full suite. Call it from each implementation's
// package test.
func RunConformance(t *testing.T, h Harness) {
	t.Helper()

	t.Run("Identity", func(t *testing.T) { testIdentity(t, h) })
	t.Run("Capabilities", func(t *testing.T) { testCapabilities(t, h) })
	t.Run("SlugParsing", func(t *testing.T) { testSlugParsing(t, h) })
	t.Run("ListPRs", func(t *testing.T) { testListPRs(t, h) })
	t.Run("EmptyResults", func(t *testing.T) { testEmptyResults(t, h) })
	t.Run("Pagination", func(t *testing.T) { testPagination(t, h) })
	t.Run("GetPR", func(t *testing.T) { testGetPR(t, h) })
	t.Run("Checks", func(t *testing.T) { testChecks(t, h) })
	t.Run("ChecksUnknownNeverGreen", func(t *testing.T) { testChecksUnknownNeverGreen(t, h) })
	t.Run("ChecksNoneIsNotGreen", func(t *testing.T) { testChecksNoneIsNotGreen(t, h) })
	t.Run("ErrorClasses", func(t *testing.T) { testErrorClasses(t, h) })
	t.Run("RefValidation", func(t *testing.T) { testRefValidation(t, h) })
}

// DemoRepo is the repository identity used by the suite's fixtures.
var DemoRepo = forge.Repo{Host: "forge.test", Owner: "grove", Name: "flow"}

// MakePRs builds n synthetic open pull requests numbered 1..n.
func MakePRs(n int) []forge.PullRequest {
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	prs := make([]forge.PullRequest, 0, n)
	for i := 1; i <= n; i++ {
		prs = append(prs, forge.PullRequest{
			Number:    i,
			Title:     "pr " + itoa(i),
			State:     forge.PRStateOpen,
			Author:    "alice",
			HeadRef:   "feature/" + itoa(i),
			HeadSHA:   "sha" + itoa(i),
			BaseRef:   "main",
			CreatedAt: base,
			UpdatedAt: base,
		})
	}
	return prs
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

func testIdentity(t *testing.T, h Harness) {
	p := h.New(t, Scenario{Repo: DemoRepo})
	if got := p.Name(); got != h.Name {
		t.Errorf("Name() = %q, want %q", got, h.Name)
	}
}

func testCapabilities(t *testing.T, h Harness) {
	p := h.New(t, Scenario{Repo: DemoRepo})
	caps := p.Capabilities()

	// Anything the provider actually implements must be declared supported.
	for _, c := range []forge.Capability{
		forge.CapResolveRepo, forge.CapListPRs, forge.CapGetPR, forge.CapChecks,
	} {
		if got := caps.Get(c); got != forge.SupportSupported {
			t.Errorf("Capabilities().Get(%q) = %q, want %q", c, got, forge.SupportSupported)
		}
	}

	// This wave is read-only: mutations must be affirmatively unsupported,
	// not merely unknown.
	if got := caps.Get(forge.CapMutations); got != forge.SupportUnsupported {
		t.Errorf("Capabilities().Get(mutations) = %q, want %q", got, forge.SupportUnsupported)
	}

	// Capability absence is unknown — never unsupported, never supported.
	const never forge.Capability = "capability_that_was_never_probed"
	if got := caps.Get(never); got != forge.SupportUnknown {
		t.Errorf("Capabilities().Get(unprobed) = %q, want %q", got, forge.SupportUnknown)
	}
	if caps.Supports(never) {
		t.Error("Capabilities().Supports(unprobed) = true; unknown must not gate behavior open")
	}

	// An explicitly stored empty value normalizes to unknown too.
	poisoned := caps.Clone()
	poisoned[never] = ""
	if got := poisoned.Get(never); got != forge.SupportUnknown {
		t.Errorf("Get on empty-valued capability = %q, want %q", got, forge.SupportUnknown)
	}
}

// slugCase is one remote-URL parsing expectation. The same table runs against
// every provider's ResolveRepo, since identity must not vary by provider.
type slugCase struct {
	name    string
	input   string
	want    forge.Repo
	wantErr bool
}

func slugCases() []slugCase {
	return []slugCase{
		{name: "https", input: "https://github.com/grove/flow.git", want: forge.Repo{Host: "github.com", Owner: "grove", Name: "flow"}},
		{name: "https no .git", input: "https://github.com/grove/flow", want: forge.Repo{Host: "github.com", Owner: "grove", Name: "flow"}},
		{name: "https trailing slash", input: "https://github.com/grove/flow/", want: forge.Repo{Host: "github.com", Owner: "grove", Name: "flow"}},
		{name: "http", input: "http://forge.example.com/grove/flow.git", want: forge.Repo{Host: "forge.example.com", Owner: "grove", Name: "flow"}},
		{name: "scp-like", input: "git@github.com:grove/flow.git", want: forge.Repo{Host: "github.com", Owner: "grove", Name: "flow"}},
		{name: "scp-like no user", input: "github.com:grove/flow.git", want: forge.Repo{Host: "github.com", Owner: "grove", Name: "flow"}},
		{name: "ssh url", input: "ssh://git@forge.example.com/grove/flow.git", want: forge.Repo{Host: "forge.example.com", Owner: "grove", Name: "flow"}},
		{name: "ssh url with port", input: "ssh://git@forge.example.com:2222/grove/flow.git", want: forge.Repo{Host: "forge.example.com", Owner: "grove", Name: "flow"}},
		{name: "git protocol", input: "git://forge.example.com/grove/flow", want: forge.Repo{Host: "forge.example.com", Owner: "grove", Name: "flow"}},
		{name: "host case folded", input: "https://GitHub.COM/grove/flow", want: forge.Repo{Host: "github.com", Owner: "grove", Name: "flow"}},
		{name: "userinfo dropped", input: "https://user:pass@github.com/grove/flow", want: forge.Repo{Host: "github.com", Owner: "grove", Name: "flow"}},
		{name: "https with port", input: "https://forge.example.com:8443/grove/flow", want: forge.Repo{Host: "forge.example.com", Owner: "grove", Name: "flow"}},
		{name: "dots in name", input: "https://github.com/grove/grove.nvim", want: forge.Repo{Host: "github.com", Owner: "grove", Name: "grove.nvim"}},

		// Host confusion: a lookalike host must resolve to itself, so an exact
		// host comparison downstream cannot be fooled.
		{name: "lookalike suffix host", input: "https://github.com.evil.example/grove/flow", want: forge.Repo{Host: "github.com.evil.example", Owner: "grove", Name: "flow"}},
		{name: "host in path is not the host", input: "https://evil.example/github.com/grove", want: forge.Repo{Host: "evil.example", Owner: "github.com", Name: "grove"}},
		{name: "userinfo lookalike", input: "https://github.com@evil.example/grove/flow", want: forge.Repo{Host: "evil.example", Owner: "grove", Name: "flow"}},

		// Path traversal and shape abuse.
		{name: "traversal in owner", input: "https://github.com/../flow", wantErr: true},
		{name: "traversal in name", input: "https://github.com/grove/..", wantErr: true},
		{name: "encoded traversal", input: "https://github.com/grove/%2e%2e", wantErr: true},
		{name: "deep path not truncated", input: "https://github.com/a/b/grove/flow", wantErr: true},
		{name: "single segment", input: "https://github.com/flow", wantErr: true},
		{name: "empty owner", input: "https://github.com//flow", wantErr: true},
		{name: "no host", input: "https:///grove/flow", wantErr: true},

		// Argv injection: a leading dash would be read as a flag by `gh`.
		{name: "leading dash owner", input: "https://github.com/-x/flow", wantErr: true},
		{name: "leading dash name", input: "https://github.com/grove/--repo", wantErr: true},

		// Junk.
		{name: "empty", input: "", wantErr: true},
		{name: "whitespace", input: "   ", wantErr: true},
		{name: "space inside", input: "https://github.com/gro ve/flow", wantErr: true},
		{name: "newline inside", input: "https://github.com/grove/flow\nrm -rf", wantErr: true},
		{name: "backslash", input: `https://github.com/grove\flow`, wantErr: true},
		{name: "unsupported scheme", input: "file:///etc/passwd", wantErr: true},
		{name: "not a url", input: "just-a-string", wantErr: true},
	}
}

func testSlugParsing(t *testing.T, h Harness) {
	p := h.New(t, Scenario{Repo: DemoRepo})
	for _, tc := range slugCases() {
		t.Run(tc.name, func(t *testing.T) {
			got, err := p.ResolveRepo(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ResolveRepo(%q) = %+v, want error", tc.input, got)
				}
				// Parsing failures are permanent — retrying a malformed URL
				// is never going to help.
				if cls := forge.ClassOf(err); cls != forge.ClassPermanent {
					t.Errorf("ResolveRepo(%q) error class = %q, want %q", tc.input, cls, forge.ClassPermanent)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveRepo(%q) failed: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("ResolveRepo(%q) = %+v, want %+v", tc.input, got, tc.want)
			}
		})
	}

	// ResolveRepo does no I/O, so it must work even when the transport is
	// dead. Offline identity resolution is what lets a surface label a
	// worktree without a network.
	if !h.skips(FaultOffline) {
		dead := h.New(t, Scenario{Repo: DemoRepo, Fault: FaultOffline})
		if _, err := dead.ResolveRepo("https://github.com/grove/flow"); err != nil {
			t.Errorf("ResolveRepo must not perform I/O; failed with a dead transport: %v", err)
		}
	}
}

func testListPRs(t *testing.T, h Harness) {
	want := MakePRs(3)
	want[1].State = forge.PRStateMerged
	merged := time.Date(2026, 8, 1, 13, 0, 0, 0, time.UTC)
	want[1].MergedAt = &merged
	want[2].IsDraft = true

	p := h.New(t, Scenario{Repo: DemoRepo, PRs: want})
	got, err := p.ListPRs(context.Background(), DemoRepo, forge.ListPROptions{State: forge.StateAll})
	if err != nil {
		t.Fatalf("ListPRs failed: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("ListPRs returned %d PRs, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Number != want[i].Number {
			t.Errorf("PR[%d].Number = %d, want %d", i, got[i].Number, want[i].Number)
		}
		if got[i].State != want[i].State {
			t.Errorf("PR[%d].State = %q, want %q", i, got[i].State, want[i].State)
		}
		if got[i].IsDraft != want[i].IsDraft {
			t.Errorf("PR[%d].IsDraft = %v, want %v", i, got[i].IsDraft, want[i].IsDraft)
		}
		if got[i].HeadRef != want[i].HeadRef {
			t.Errorf("PR[%d].HeadRef = %q, want %q", i, got[i].HeadRef, want[i].HeadRef)
		}
	}
	// A merged PR must not read as merely closed, and must not lose its
	// timestamp: the ticket↔PR join keys lifecycle transitions on exactly this.
	if got[1].State != forge.PRStateMerged {
		t.Errorf("merged PR state = %q, want %q", got[1].State, forge.PRStateMerged)
	}
	if got[1].MergedAt == nil {
		t.Error("merged PR lost its MergedAt timestamp")
	}
}

func testEmptyResults(t *testing.T, h Harness) {
	p := h.New(t, Scenario{Repo: DemoRepo, PRs: nil})
	got, err := p.ListPRs(context.Background(), DemoRepo, forge.ListPROptions{State: forge.StateAll})
	if err != nil {
		t.Fatalf("ListPRs on an empty repo must succeed, got: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListPRs returned %d PRs for an empty repo, want 0", len(got))
	}
}

func testPagination(t *testing.T, h Harness) {
	// More items than one page holds, so a provider that silently returns
	// only the first page fails here.
	const n = forge.DefaultPageSize*2 + 7
	p := h.New(t, Scenario{Repo: DemoRepo, PRs: MakePRs(n)})

	got, err := p.ListPRs(context.Background(), DemoRepo, forge.ListPROptions{State: forge.StateAll})
	if err != nil {
		t.Fatalf("ListPRs failed: %v", err)
	}
	if len(got) != n {
		t.Fatalf("ListPRs returned %d PRs, want all %d across pages", len(got), n)
	}
	// Order and identity must survive paging — no duplicates, no gaps.
	seen := make(map[int]bool, n)
	for i, pr := range got {
		if seen[pr.Number] {
			t.Fatalf("PR #%d returned twice across pages", pr.Number)
		}
		seen[pr.Number] = true
		if pr.Number != i+1 {
			t.Fatalf("PR at index %d has number %d; paging reordered results", i, pr.Number)
		}
	}

	// The caller's limit is honored and never exceeded.
	limited, err := p.ListPRs(context.Background(), DemoRepo, forge.ListPROptions{State: forge.StateAll, Limit: 10})
	if err != nil {
		t.Fatalf("ListPRs with a limit failed: %v", err)
	}
	if len(limited) > 10 {
		t.Errorf("ListPRs(Limit: 10) returned %d PRs; the bound is not enforced", len(limited))
	}
}

func testGetPR(t *testing.T, h Harness) {
	prs := MakePRs(3)
	p := h.New(t, Scenario{Repo: DemoRepo, PRs: prs})

	got, err := p.GetPR(context.Background(), DemoRepo, 2)
	if err != nil {
		t.Fatalf("GetPR(2) failed: %v", err)
	}
	if got.Number != 2 {
		t.Errorf("GetPR(2).Number = %d, want 2", got.Number)
	}

	// A nonsense number is a caller bug, not a transient condition.
	if _, err := p.GetPR(context.Background(), DemoRepo, 0); err == nil {
		t.Error("GetPR(0) succeeded; want a permanent error")
	} else if cls := forge.ClassOf(err); cls != forge.ClassPermanent {
		t.Errorf("GetPR(0) error class = %q, want %q", cls, forge.ClassPermanent)
	}
}

func testChecks(t *testing.T, h Harness) {
	const ref = "abc123"
	cases := []struct {
		name   string
		checks []forge.Check
		want   forge.CheckState
	}{
		{"all success", []forge.Check{
			{Name: "build", State: forge.CheckStateSuccess},
			{Name: "test", State: forge.CheckStateSuccess},
		}, forge.CheckStateSuccess},
		{"one failure dominates", []forge.Check{
			{Name: "build", State: forge.CheckStateSuccess},
			{Name: "test", State: forge.CheckStateFailure},
		}, forge.CheckStateFailure},
		{"pending beats success", []forge.Check{
			{Name: "build", State: forge.CheckStateSuccess},
			{Name: "test", State: forge.CheckStatePending},
		}, forge.CheckStatePending},
		{"failure beats pending", []forge.Check{
			{Name: "build", State: forge.CheckStateFailure},
			{Name: "test", State: forge.CheckStatePending},
		}, forge.CheckStateFailure},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := h.New(t, Scenario{Repo: DemoRepo, CheckRef: ref, Checks: tc.checks})
			got, err := p.Checks(context.Background(), DemoRepo, ref)
			if err != nil {
				t.Fatalf("Checks failed: %v", err)
			}
			if got.State != tc.want {
				t.Errorf("rollup state = %q, want %q (checks: %+v)", got.State, tc.want, got.Checks)
			}
			if got.Ref != ref {
				t.Errorf("rollup ref = %q, want %q", got.Ref, ref)
			}
			if len(got.Checks) != len(tc.checks) {
				t.Errorf("rollup carried %d checks, want %d", len(got.Checks), len(tc.checks))
			}
		})
	}
}

// testChecksUnknownNeverGreen is the load-bearing assertion of D4: an
// indeterminate check must not be rounded off into a passing rollup.
func testChecksUnknownNeverGreen(t *testing.T, h Harness) {
	const ref = "abc123"
	p := h.New(t, Scenario{Repo: DemoRepo, CheckRef: ref, Checks: []forge.Check{
		{Name: "build", State: forge.CheckStateSuccess},
		{Name: "mystery", State: forge.CheckStateUnknown, RawState: "something_new"},
	}})

	got, err := p.Checks(context.Background(), DemoRepo, ref)
	if err != nil {
		t.Fatalf("Checks failed: %v", err)
	}
	if got.State.IsGreen() {
		t.Fatalf("rollup with an unknown check is green (%q); unknown must never render as passing", got.State)
	}
	if got.State != forge.CheckStateUnknown {
		t.Errorf("rollup state = %q, want %q", got.State, forge.CheckStateUnknown)
	}
}

// testChecksNoneIsNotGreen pins the other half: "the forge reports no checks"
// is not "the checks passed".
func testChecksNoneIsNotGreen(t *testing.T, h Harness) {
	const ref = "abc123"
	p := h.New(t, Scenario{Repo: DemoRepo, CheckRef: ref, Checks: nil})
	got, err := p.Checks(context.Background(), DemoRepo, ref)
	if err != nil {
		t.Fatalf("Checks on a ref with no checks must succeed, got: %v", err)
	}
	if got.State.IsGreen() {
		t.Fatalf("rollup with zero checks is green (%q); absence is not a pass", got.State)
	}
	if got.State != forge.CheckStateNone {
		t.Errorf("rollup state = %q, want %q", got.State, forge.CheckStateNone)
	}
}

func testErrorClasses(t *testing.T, h Harness) {
	cases := []struct {
		fault    Fault
		want     forge.ErrorClass
		sentinel error
	}{
		{FaultAuth, forge.ClassUnavailable, forge.ErrUnavailable},
		{FaultRateLimit, forge.ClassRetryable, forge.ErrRetryable},
		{FaultNotFound, forge.ClassPermanent, forge.ErrPermanent},
		{FaultOffline, forge.ClassUnavailable, forge.ErrUnavailable},
		{FaultServerError, forge.ClassRetryable, forge.ErrRetryable},
		{FaultTransportMissing, forge.ClassUnavailable, forge.ErrUnavailable},
	}

	for _, tc := range cases {
		t.Run(string(tc.fault), func(t *testing.T) {
			if h.skips(tc.fault) {
				t.Skipf("%s transport cannot simulate fault %q", h.Name, tc.fault)
			}
			p := h.New(t, Scenario{Repo: DemoRepo, PRs: MakePRs(2), CheckRef: "abc123", Fault: tc.fault})

			// Every read operation must classify the same fault identically:
			// a poller that retries ListPRs but gives up on Checks for the
			// same outage is a bug.
			_, listErr := p.ListPRs(context.Background(), DemoRepo, forge.ListPROptions{State: forge.StateAll})
			assertClass(t, "ListPRs", listErr, tc.want, tc.sentinel)

			_, getErr := p.GetPR(context.Background(), DemoRepo, 1)
			assertClass(t, "GetPR", getErr, tc.want, tc.sentinel)

			rollup, checkErr := p.Checks(context.Background(), DemoRepo, "abc123")
			assertClass(t, "Checks", checkErr, tc.want, tc.sentinel)
			// A failed Checks call must not hand back a rollup that reads as
			// green or as an affirmative "no checks".
			if checkErr != nil {
				if rollup.State.IsGreen() {
					t.Errorf("Checks failed but returned a green rollup (%q)", rollup.State)
				}
				if rollup.State != forge.CheckStateUnknown {
					t.Errorf("Checks failed but returned state %q, want %q", rollup.State, forge.CheckStateUnknown)
				}
			}
		})
	}
}

func assertClass(t *testing.T, op string, err error, want forge.ErrorClass, sentinel error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s succeeded; want a %q failure", op, want)
	}
	if got := forge.ClassOf(err); got != want {
		t.Errorf("%s error class = %q, want %q (err: %v)", op, got, want, err)
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("%s error does not match sentinel %v (err: %v)", op, sentinel, err)
	}
}

// testRefValidation pins that hostile refs are rejected before they reach an
// argv or a URL path.
func testRefValidation(t *testing.T, h Harness) {
	p := h.New(t, Scenario{Repo: DemoRepo, CheckRef: "abc123"})
	bad := []string{
		"",
		"   ",
		"-oProxyCommand=evil",
		"../../etc/passwd",
		"refs/heads/..",
		"main branch",
		"main\nrm -rf",
	}
	for _, ref := range bad {
		got, err := p.Checks(context.Background(), DemoRepo, ref)
		if err == nil {
			t.Errorf("Checks(%q) succeeded; want rejection", ref)
			continue
		}
		if cls := forge.ClassOf(err); cls != forge.ClassPermanent {
			t.Errorf("Checks(%q) error class = %q, want %q", ref, cls, forge.ClassPermanent)
		}
		if got.State.IsGreen() {
			t.Errorf("Checks(%q) returned a green rollup on rejection", ref)
		}
	}

	// A repo identity that never went through ResolveRepo must still be
	// re-validated at the call boundary.
	hostile := forge.Repo{Host: "forge.test", Owner: "--upload-pack=evil", Name: "flow"}
	if _, err := p.ListPRs(context.Background(), hostile, forge.ListPROptions{}); err == nil {
		t.Error("ListPRs accepted a repo whose owner starts with '-'")
	} else if cls := forge.ClassOf(err); cls != forge.ClassPermanent {
		t.Errorf("hostile repo error class = %q, want %q", cls, forge.ClassPermanent)
	}
}
