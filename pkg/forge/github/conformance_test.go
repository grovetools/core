package github_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/grovetools/core/pkg/forge"
	"github.com/grovetools/core/pkg/forge/forgetest"
	"github.com/grovetools/core/pkg/forge/github"
)

// TestConformance runs the shared provider contract against the `gh`-backed
// implementation using an injected runner: no GitHub account, no network, and
// the real argv builder and JSON parser under test.
func TestConformance(t *testing.T) {
	forgetest.RunConformance(t, forgetest.Harness{
		Name: "github",
		New: func(t *testing.T, s forgetest.Scenario) forge.Provider {
			return github.New(github.WithRunner(&fakeGH{scenario: s}))
		},
	})
}

// fakeGH is a `gh` stand-in: it parses the argv the provider built and answers
// with the JSON shapes the real CLI emits.
type fakeGH struct {
	scenario forgetest.Scenario
	// calls records every argv for assertions.
	calls [][]string
}

func (f *fakeGH) Run(_ context.Context, args []string) ([]byte, []byte, error) {
	f.calls = append(f.calls, append([]string(nil), args...))

	if out, errOut, err, handled := f.fault(); handled {
		return out, errOut, err
	}

	switch {
	case len(args) >= 2 && args[0] == "pr" && args[1] == "list":
		return f.prList(args)
	case len(args) >= 2 && args[0] == "pr" && args[1] == "view":
		return f.prView(args)
	case len(args) >= 1 && args[0] == "api":
		return f.api(args)
	}
	return nil, []byte("unknown command"), errors.New("exit status 1")
}

// fault renders the scenario's fault as `gh` renders it: exit status plus a
// human-readable stderr, which is all the real CLI gives us to classify on.
func (f *fakeGH) fault() (stdout, stderr []byte, err error, handled bool) {
	exit := errors.New("exit status 1")
	switch f.scenario.Fault {
	case forgetest.FaultNone:
		return nil, nil, nil, false
	case forgetest.FaultAuth:
		return nil, []byte("gh: To get started with GitHub CLI, please run: gh auth login"), exit, true
	case forgetest.FaultRateLimit:
		return nil, []byte("gh: API rate limit exceeded for user ID 1. (HTTP 403)"), exit, true
	case forgetest.FaultNotFound:
		return nil, []byte("gh: Could not resolve to a Repository with the name 'grove/flow'. (HTTP 404)"), exit, true
	case forgetest.FaultOffline:
		return nil, []byte("dial tcp: lookup api.github.com: no such host"), exit, true
	case forgetest.FaultServerError:
		return nil, []byte("gh: HTTP 503: Service Unavailable"), exit, true
	case forgetest.FaultTransportMissing:
		return nil, nil, github.ErrGHNotFound, true
	}
	return nil, nil, nil, false
}

func (f *fakeGH) prList(args []string) ([]byte, []byte, error) {
	limit := forge.MaxItems
	if v, ok := flagValue(args, "--limit"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	prs := f.scenario.PRs
	if len(prs) > limit {
		prs = prs[:limit]
	}
	out := make([]ghPR, 0, len(prs))
	for _, pr := range prs {
		out = append(out, toGH(pr))
	}
	return mustJSON(out), nil, nil
}

func (f *fakeGH) prView(args []string) ([]byte, []byte, error) {
	if len(args) < 3 {
		return nil, []byte("gh: missing pull request number"), errors.New("exit status 1")
	}
	n, err := strconv.Atoi(args[2])
	if err != nil {
		return nil, []byte("gh: invalid number"), errors.New("exit status 1")
	}
	for _, pr := range f.scenario.PRs {
		if pr.Number == n {
			return mustJSON(toGH(pr)), nil, nil
		}
	}
	return nil, []byte("gh: Could not resolve to a PullRequest (HTTP 404)"), errors.New("exit status 1")
}

func (f *fakeGH) api(args []string) ([]byte, []byte, error) {
	path := args[len(args)-1]
	switch {
	case strings.Contains(path, "/check-runs"):
		page, perPage := pageParams(path)
		checks := f.scenario.Checks
		start := (page - 1) * perPage
		if start > len(checks) {
			start = len(checks)
		}
		end := start + perPage
		if end > len(checks) {
			end = len(checks)
		}
		runs := make([]ghCheckRun, 0, end-start)
		for _, c := range checks[start:end] {
			runs = append(runs, toGHCheckRun(c))
		}
		return mustJSON(map[string]any{
			"total_count": len(checks),
			"check_runs":  runs,
		}), nil, nil

	case strings.HasSuffix(strings.SplitN(path, "?", 2)[0], "/status"):
		// This implementation exercises check-runs; the legacy combined
		// status is empty in every fixture.
		return mustJSON(map[string]any{
			"state":    "pending",
			"statuses": []any{},
		}), nil, nil
	}
	return nil, []byte("gh: HTTP 404: Not Found"), errors.New("exit status 1")
}

func flagValue(args []string, flag string) (string, bool) {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

func pageParams(path string) (page, perPage int) {
	page, perPage = 1, forge.DefaultPageSize
	_, query, ok := strings.Cut(path, "?")
	if !ok {
		return page, perPage
	}
	for _, kv := range strings.Split(query, "&") {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			continue
		}
		switch k {
		case "page":
			page = n
		case "per_page":
			perPage = n
		}
	}
	return page, perPage
}

// ---- fixture encoders (the shapes `gh --json` actually emits) --------------

type ghPR struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	State       string `json:"state"`
	IsDraft     bool   `json:"isDraft"`
	URL         string `json:"url"`
	HeadRefName string `json:"headRefName"`
	HeadRefOid  string `json:"headRefOid"`
	BaseRefName string `json:"baseRefName"`
	Author      struct {
		Login string `json:"login"`
	} `json:"author"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	MergedAt  *time.Time `json:"mergedAt"`
	Labels    []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

func toGH(pr forge.PullRequest) ghPR {
	out := ghPR{
		Number:      pr.Number,
		Title:       pr.Title,
		State:       strings.ToUpper(string(pr.State)), // gh emits OPEN/CLOSED/MERGED
		IsDraft:     pr.IsDraft,
		URL:         pr.URL,
		HeadRefName: pr.HeadRef,
		HeadRefOid:  pr.HeadSHA,
		BaseRefName: pr.BaseRef,
		CreatedAt:   pr.CreatedAt,
		UpdatedAt:   pr.UpdatedAt,
		MergedAt:    pr.MergedAt,
	}
	out.Author.Login = pr.Author
	for _, l := range pr.Labels {
		out.Labels = append(out.Labels, struct {
			Name string `json:"name"`
		}{Name: l})
	}
	return out
}

type ghCheckRun struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Conclusion  string `json:"conclusion"`
	HTMLURL     string `json:"html_url"`
	CompletedAt string `json:"completed_at"`
}

// toGHCheckRun renders a desired forge state back into GitHub's
// status/conclusion vocabulary, so the provider's own mapping is what the
// conformance suite measures.
func toGHCheckRun(c forge.Check) ghCheckRun {
	run := ghCheckRun{Name: c.Name, HTMLURL: c.URL, Status: "completed"}
	if !c.CompletedAt.IsZero() {
		run.CompletedAt = c.CompletedAt.Format(time.RFC3339)
	}
	switch c.State {
	case forge.CheckStateSuccess:
		run.Conclusion = "success"
	case forge.CheckStateFailure:
		run.Conclusion = "failure"
	case forge.CheckStateNeutral:
		run.Conclusion = "neutral"
	case forge.CheckStatePending:
		run.Status = "in_progress"
	default:
		// An unrecognized conclusion is exactly how a real forge surfaces a
		// state this code has never seen.
		run.Conclusion = "something_new"
	}
	return run
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("fixture encode failed: %v", err))
	}
	return b
}
