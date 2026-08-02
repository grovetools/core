package forgejo_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/grovetools/core/pkg/forge"
	"github.com/grovetools/core/pkg/forge/forgejo"
	"github.com/grovetools/core/pkg/forge/forgetest"
)

// TestConformance runs the shared provider contract against the Forgejo
// implementation. No live instance exists, so the fixtures are an httptest
// server speaking the documented Gitea v1 shapes.
func TestConformance(t *testing.T) {
	forgetest.RunConformance(t, forgetest.Harness{
		Name: "forgejo",
		New: func(t *testing.T, s forgetest.Scenario) forge.Provider {
			return newFixtureProvider(t, s)
		},
		// `gh`-style "the transport binary is missing" has no analog for an
		// HTTP client; FaultOffline covers the unreachable-instance case.
		SkipFaults: []forgetest.Fault{forgetest.FaultTransportMissing},
	})
}

// newFixtureProvider stands up an httptest instance serving s and returns a
// provider pointed at it.
func newFixtureProvider(t *testing.T, s forgetest.Scenario) *forgejo.Provider {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			// Read-only means read-only: any non-GET reaching the fixture is
			// a bug in the provider, not in the fixture.
			t.Errorf("provider issued a %s request to %s; this seam is read-only", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if serveFault(w, s.Fault) {
			return
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/statuses"):
			serveStatuses(w, r, s)
		case strings.Contains(r.URL.Path, "/pulls/"):
			servePR(w, r, s)
		case strings.HasSuffix(r.URL.Path, "/pulls"):
			servePRList(w, r, s)
		default:
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	baseURL := srv.URL
	if s.Fault == forgetest.FaultOffline {
		// Close the server first: dialing a dead listener is exactly what an
		// unreachable instance looks like to the client.
		srv.Close()
	}

	p, err := forgejo.New(baseURL, forgejo.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("forgejo.New failed: %v", err)
	}
	return p
}

func serveFault(w http.ResponseWriter, f forgetest.Fault) bool {
	switch f {
	case forgetest.FaultAuth:
		http.Error(w, `{"message":"token is required"}`, http.StatusUnauthorized)
	case forgetest.FaultRateLimit:
		w.Header().Set("Retry-After", "60")
		http.Error(w, `{"message":"rate limit exceeded"}`, http.StatusTooManyRequests)
	case forgetest.FaultNotFound:
		http.Error(w, `{"message":"repository does not exist"}`, http.StatusNotFound)
	case forgetest.FaultServerError:
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
	default:
		return false
	}
	return true
}

func servePRList(w http.ResponseWriter, r *http.Request, s forgetest.Scenario) {
	q := r.URL.Query()
	page := atoiOr(q.Get("page"), 1)
	limit := atoiOr(q.Get("limit"), forge.DefaultPageSize)

	prs := s.PRs
	if state := q.Get("state"); state != "" && state != "all" {
		filtered := prs[:0:0]
		for _, pr := range prs {
			if matchesGiteaState(pr, state) {
				filtered = append(filtered, pr)
			}
		}
		prs = filtered
	}

	start := (page - 1) * limit
	if start > len(prs) {
		start = len(prs)
	}
	end := start + limit
	if end > len(prs) {
		end = len(prs)
	}

	out := make([]apiPR, 0, end-start)
	for _, pr := range prs[start:end] {
		out = append(out, toAPI(pr))
	}
	writeJSON(w, out)
}

// matchesGiteaState mirrors the real API: Gitea has no "merged" filter, so a
// merged PR is served under "closed".
func matchesGiteaState(pr forge.PullRequest, state string) bool {
	switch state {
	case "open":
		return pr.State == forge.PRStateOpen
	case "closed":
		return pr.State == forge.PRStateClosed || pr.State == forge.PRStateMerged
	default:
		return true
	}
}

func servePR(w http.ResponseWriter, r *http.Request, s forgetest.Scenario) {
	_, numStr, ok := strings.Cut(r.URL.Path, "/pulls/")
	if !ok {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		return
	}
	n, err := strconv.Atoi(strings.Trim(numStr, "/"))
	if err != nil {
		http.Error(w, `{"message":"invalid index"}`, http.StatusUnprocessableEntity)
		return
	}
	for _, pr := range s.PRs {
		if pr.Number == n {
			writeJSON(w, toAPI(pr))
			return
		}
	}
	http.Error(w, `{"message":"pull request does not exist"}`, http.StatusNotFound)
}

func serveStatuses(w http.ResponseWriter, r *http.Request, s forgetest.Scenario) {
	q := r.URL.Query()
	page := atoiOr(q.Get("page"), 1)
	limit := atoiOr(q.Get("limit"), forge.DefaultPageSize)

	checks := s.Checks
	start := (page - 1) * limit
	if start > len(checks) {
		start = len(checks)
	}
	end := start + limit
	if end > len(checks) {
		end = len(checks)
	}

	out := make([]apiStatus, 0, end-start)
	for _, c := range checks[start:end] {
		out = append(out, toAPIStatus(c))
	}
	writeJSON(w, out)
}

func atoiOr(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil && n > 0 {
		return n
	}
	return def
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// ---- Gitea v1 fixture shapes ----------------------------------------------

type apiPR struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	State   string `json:"state"`
	Draft   bool   `json:"draft"`
	Merged  bool   `json:"merged"`
	HTMLURL string `json:"html_url"`
	User    struct {
		Login string `json:"login"`
	} `json:"user"`
	Head struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	MergedAt  *time.Time `json:"merged_at"`
}

func toAPI(pr forge.PullRequest) apiPR {
	out := apiPR{
		Number:    pr.Number,
		Title:     pr.Title,
		Draft:     pr.IsDraft,
		HTMLURL:   pr.URL,
		CreatedAt: pr.CreatedAt,
		UpdatedAt: pr.UpdatedAt,
		MergedAt:  pr.MergedAt,
	}
	// Gitea reports open/closed and carries merged-ness separately.
	switch pr.State {
	case forge.PRStateMerged:
		out.State = "closed"
		out.Merged = true
	case forge.PRStateClosed:
		out.State = "closed"
	default:
		out.State = "open"
	}
	out.User.Login = pr.Author
	out.Head.Ref = pr.HeadRef
	out.Head.SHA = pr.HeadSHA
	out.Base.Ref = pr.BaseRef
	for _, l := range pr.Labels {
		out.Labels = append(out.Labels, struct {
			Name string `json:"name"`
		}{Name: l})
	}
	return out
}

type apiStatus struct {
	Status      string    `json:"status"`
	Context     string    `json:"context"`
	TargetURL   string    `json:"target_url"`
	Description string    `json:"description"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// toAPIStatus renders a desired forge state back into Gitea's own vocabulary,
// so the provider's mapping is what the conformance suite measures.
func toAPIStatus(c forge.Check) apiStatus {
	out := apiStatus{Context: c.Name, TargetURL: c.URL, UpdatedAt: c.CompletedAt}
	switch c.State {
	case forge.CheckStateSuccess:
		out.Status = "success"
	case forge.CheckStateFailure:
		out.Status = "failure"
	case forge.CheckStatePending:
		out.Status = "pending"
	case forge.CheckStateNeutral:
		out.Status = "warning"
	default:
		// A state this code has never seen is exactly what an upgraded forge
		// eventually sends.
		out.Status = "something_new"
	}
	return out
}
