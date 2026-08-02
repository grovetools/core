package forgejo_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/grovetools/core/pkg/forge"
	"github.com/grovetools/core/pkg/forge/forgejo"
	"github.com/grovetools/core/pkg/forge/forgetest"
)

func TestNewRejectsBadBaseURLs(t *testing.T) {
	for _, bad := range []string{
		"",
		"   ",
		"ftp://forge.example.com",
		"file:///etc/passwd",
		"not a url at all",
		"https://",
	} {
		t.Run(bad, func(t *testing.T) {
			if _, err := forgejo.New(bad); err == nil {
				t.Fatalf("New(%q) succeeded; want a permanent error", bad)
			} else if cls := forge.ClassOf(err); cls != forge.ClassPermanent {
				t.Errorf("New(%q) error class = %q, want %q", bad, cls, forge.ClassPermanent)
			}
		})
	}
}

// TestTokenIsInjectedNotDiscovered is the guard on the credential rule: the
// token reaches the wire only because the caller handed it over, and
// token-shaped environment variables change nothing.
func TestTokenIsInjectedNotDiscovered(t *testing.T) {
	var gotAuth string
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	repo := forge.Repo{Host: "forge.test", Owner: "grove", Name: "flow"}

	// Poison the environment with every plausible token variable.
	t.Setenv("FORGE_TOKEN", "env-token")
	t.Setenv("FORGEJO_TOKEN", "env-token")
	t.Setenv("GITEA_TOKEN", "env-token")
	t.Setenv("GROVE_SYNC_TOKEN", "env-token")

	// Without an injected token: no Authorization header at all.
	p, err := forgejo.New(srv.URL, forgejo.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.ListPRs(context.Background(), repo, forge.ListPROptions{}); err != nil {
		t.Fatalf("ListPRs failed: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("Authorization header = %q with no injected token; the package read a credential it should not have", gotAuth)
	}

	// With an injected token: exactly that token, in Gitea's header form.
	p, err = forgejo.New(srv.URL,
		forgejo.WithHTTPClient(srv.Client()),
		forgejo.WithStaticToken("injected-token"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.ListPRs(context.Background(), repo, forge.ListPROptions{}); err != nil {
		t.Fatalf("ListPRs failed: %v", err)
	}
	if gotAuth != "token injected-token" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "token injected-token")
	}
	if strings.Contains(gotAuth, "env-token") {
		t.Error("an environment token reached the wire")
	}
	if calls != 2 {
		t.Errorf("server saw %d calls, want 2", calls)
	}
}

// TestTokenFuncFailureIsUnavailable: a token source that cannot produce a
// token means we cannot talk to the forge — unknown, not empty.
func TestTokenFuncFailureIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("provider issued a request despite an unresolvable token")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p, err := forgejo.New(srv.URL,
		forgejo.WithHTTPClient(srv.Client()),
		forgejo.WithToken(func(context.Context) (string, error) { return "", os.ErrPermission }))
	if err != nil {
		t.Fatal(err)
	}

	repo := forge.Repo{Host: "forge.test", Owner: "grove", Name: "flow"}
	_, err = p.ListPRs(context.Background(), repo, forge.ListPROptions{})
	if err == nil {
		t.Fatal("ListPRs succeeded with an unresolvable token")
	}
	if !forge.IsUnavailable(err) {
		t.Errorf("error class = %q, want %q (err: %v)", forge.ClassOf(err), forge.ClassUnavailable, err)
	}
}

// TestHTTPStatusClassification pins the status→class table, including the 403
// split that separates a throttle from a genuine denial.
func TestHTTPStatusClassification(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		headers map[string]string
		body    string
		want    forge.ErrorClass
	}{
		{"401 unauthenticated", http.StatusUnauthorized, nil, `{"message":"token required"}`, forge.ClassUnavailable},
		{"403 forbidden", http.StatusForbidden, nil, `{"message":"forbidden"}`, forge.ClassPermanent},
		{"403 rate limited by header", http.StatusForbidden, map[string]string{"X-RateLimit-Remaining": "0"}, `{"message":"forbidden"}`, forge.ClassRetryable},
		{"403 rate limited by body", http.StatusForbidden, nil, `{"message":"API rate limit exceeded"}`, forge.ClassRetryable},
		{"404 missing", http.StatusNotFound, nil, `{"message":"not found"}`, forge.ClassPermanent},
		{"408 timeout", http.StatusRequestTimeout, nil, ``, forge.ClassRetryable},
		{"422 unprocessable", http.StatusUnprocessableEntity, nil, ``, forge.ClassPermanent},
		{"429 throttled", http.StatusTooManyRequests, nil, ``, forge.ClassRetryable},
		{"500 server error", http.StatusInternalServerError, nil, ``, forge.ClassRetryable},
		{"501 not implemented", http.StatusNotImplemented, nil, ``, forge.ClassUnsupported},
		{"503 unavailable", http.StatusServiceUnavailable, nil, ``, forge.ClassRetryable},
	}

	repo := forge.Repo{Host: "forge.test", Owner: "grove", Name: "flow"}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for k, v := range tc.headers {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			p, err := forgejo.New(srv.URL, forgejo.WithHTTPClient(srv.Client()))
			if err != nil {
				t.Fatal(err)
			}
			_, err = p.ListPRs(context.Background(), repo, forge.ListPROptions{})
			if err == nil {
				t.Fatalf("HTTP %d did not produce an error", tc.status)
			}
			if cls := forge.ClassOf(err); cls != tc.want {
				t.Errorf("HTTP %d classified as %q, want %q (err: %v)", tc.status, cls, tc.want, err)
			}
		})
	}
}

// TestErrorBodyIsBounded keeps a hostile or broken instance from flooding logs
// through the error path.
func TestErrorBodyIsBounded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(strings.Repeat("x", 100_000)))
	}))
	defer srv.Close()

	p, err := forgejo.New(srv.URL, forgejo.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	repo := forge.Repo{Host: "forge.test", Owner: "grove", Name: "flow"}
	_, err = p.ListPRs(context.Background(), repo, forge.ListPROptions{})
	if err == nil {
		t.Fatal("expected an error")
	}
	if len(err.Error()) > 1024 {
		t.Errorf("error message is %d bytes; the body snippet is not bounded", len(err.Error()))
	}
}

// TestAPIPathsAndPaging pins the request shapes against the documented Gitea
// v1 routes, and that paging stops at the bound.
func TestAPIPathsAndPaging(t *testing.T) {
	var paths []string
	scenario := forgetest.Scenario{
		Repo:     forgetest.DemoRepo,
		PRs:      forgetest.MakePRs(forge.DefaultPageSize + 3),
		CheckRef: "abc123",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path+"?"+r.URL.RawQuery)
		switch {
		case strings.HasSuffix(r.URL.Path, "/statuses"):
			serveStatuses(w, r, scenario)
		case strings.HasSuffix(r.URL.Path, "/pulls"):
			servePRList(w, r, scenario)
		default:
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		}
	}))
	defer srv.Close()

	p, err := forgejo.New(srv.URL, forgejo.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}

	prs, err := p.ListPRs(context.Background(), forgetest.DemoRepo, forge.ListPROptions{State: forge.StateAll})
	if err != nil {
		t.Fatalf("ListPRs failed: %v", err)
	}
	if len(prs) != forge.DefaultPageSize+3 {
		t.Fatalf("ListPRs returned %d PRs, want %d", len(prs), forge.DefaultPageSize+3)
	}
	if len(paths) != 2 {
		t.Fatalf("ListPRs made %d requests, want 2 pages: %v", len(paths), paths)
	}
	if !strings.HasPrefix(paths[0], "/api/v1/repos/grove/flow/pulls?") {
		t.Errorf("unexpected pulls path: %s", paths[0])
	}
	if !strings.Contains(paths[0], "limit=50") || !strings.Contains(paths[0], "page=1") {
		t.Errorf("first page query is wrong: %s", paths[0])
	}
	if !strings.Contains(paths[1], "page=2") {
		t.Errorf("second page query is wrong: %s", paths[1])
	}

	paths = nil
	if _, err := p.Checks(context.Background(), forgetest.DemoRepo, "abc123"); err != nil {
		t.Fatalf("Checks failed: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("Checks made %d requests, want 1: %v", len(paths), paths)
	}
	if !strings.HasPrefix(paths[0], "/api/v1/repos/grove/flow/commits/abc123/statuses?") {
		t.Errorf("unexpected statuses path: %s", paths[0])
	}
}

// TestChecksTruncationIsUnknown: an instance that never stops paging must not
// yield a green rollup computed from a partial set.
func TestChecksTruncationIsUnknown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/statuses") {
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
			return
		}
		// Always a full page: the forge claims there is always more.
		full := make([]forge.Check, forge.DefaultPageSize)
		for i := range full {
			full[i] = forge.Check{Name: "check", State: forge.CheckStateSuccess}
		}
		out := make([]apiStatus, 0, len(full))
		for _, c := range full {
			out = append(out, toAPIStatus(c))
		}
		writeJSON(w, out)
	}))
	defer srv.Close()

	p, err := forgejo.New(srv.URL, forgejo.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.Checks(context.Background(), forgetest.DemoRepo, "abc123")
	if err != nil {
		t.Fatalf("Checks failed: %v", err)
	}
	if !got.Truncated {
		t.Error("rollup is not marked Truncated despite unbounded paging")
	}
	if got.State.IsGreen() {
		t.Errorf("truncated rollup is green (%q)", got.State)
	}
	if len(got.Checks) != forge.MaxItems {
		t.Errorf("fetched %d checks, want the bound of %d", len(got.Checks), forge.MaxItems)
	}
}
