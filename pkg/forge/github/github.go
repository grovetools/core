// Package github implements the read-only [forge.Provider] on top of the `gh`
// CLI — the transport nb and grove already use, and the reason no token
// handling appears anywhere in this package.
//
// It is written fresh rather than extracted: nb/pkg/sync/github/provider.go and
// grove/pkg/gh/client.go remain the transports their own callers use, and are
// read references only.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/grovetools/core/pkg/forge"
)

const providerName = "github"

// prJSONFields is the --json field set requested for pull requests. Keep it in
// sync with ghPullRequest.
const prJSONFields = "number,title,state,isDraft,author,headRefName,headRefOid,baseRefName,url,createdAt,updatedAt,mergedAt,labels"

// Provider is the read-only GitHub forge provider.
type Provider struct {
	runner Runner
}

var _ forge.Provider = (*Provider)(nil)

// Option configures a Provider.
type Option func(*Provider)

// WithRunner substitutes the `gh` transport. The conformance suite uses this
// to exercise the argv builder and JSON parsing hermetically.
func WithRunner(r Runner) Option {
	return func(p *Provider) { p.runner = r }
}

// New builds a GitHub provider. With no options it shells out to `gh`.
func New(opts ...Option) *Provider {
	p := &Provider{runner: execRunner{}}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Name implements forge.Provider.
func (p *Provider) Name() string { return providerName }

// Capabilities implements forge.Provider. Reviews and issues are reported as
// unknown rather than unsupported: this wave never probed them, and claiming
// they are absent would be a stronger statement than the code can back.
func (p *Provider) Capabilities() forge.Capabilities {
	return forge.Capabilities{
		forge.CapResolveRepo: forge.SupportSupported,
		forge.CapListPRs:     forge.SupportSupported,
		forge.CapGetPR:       forge.SupportSupported,
		forge.CapChecks:      forge.SupportSupported,
		forge.CapPagination:  forge.SupportSupported,
		forge.CapDraftPRs:    forge.SupportSupported,
		forge.CapPRReviews:   forge.SupportUnknown,
		forge.CapIssues:      forge.SupportUnknown,
		forge.CapMutations:   forge.SupportUnsupported,
	}
}

// ResolveRepo implements forge.Provider. It is pure parsing; it accepts any
// host (github.com and GitHub Enterprise alike) and records it verbatim.
func (p *Provider) ResolveRepo(remoteURL string) (forge.Repo, error) {
	return forge.ParseRemoteURL(remoteURL)
}

// ListPRs implements forge.Provider.
//
// `gh pr list` performs its own cursor paging internally; the bound this
// package owns is the item ceiling passed as --limit, which is clamped to
// forge.MaxItems so a repo with thousands of PRs cannot stall a poller.
func (p *Provider) ListPRs(ctx context.Context, repo forge.Repo, opts forge.ListPROptions) ([]forge.PullRequest, error) {
	const op = "ListPRs"
	if err := validateRepo(op, repo); err != nil {
		return nil, err
	}
	state, err := ghStateFilter(op, opts.EffectiveState())
	if err != nil {
		return nil, err
	}

	args := []string{
		"pr", "list",
		"--repo", repo.Slug(),
		"--state", state,
		"--limit", strconv.Itoa(opts.EffectiveLimit()),
		"--json", prJSONFields,
	}
	out, err := p.run(ctx, op, args)
	if err != nil {
		return nil, err
	}

	var raw []ghPullRequest
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, parseFailure(op, err)
	}
	prs := make([]forge.PullRequest, 0, len(raw))
	for i := range raw {
		prs = append(prs, raw[i].toForge())
	}
	return prs, nil
}

// GetPR implements forge.Provider.
func (p *Provider) GetPR(ctx context.Context, repo forge.Repo, number int) (forge.PullRequest, error) {
	const op = "GetPR"
	if err := validateRepo(op, repo); err != nil {
		return forge.PullRequest{}, err
	}
	if number <= 0 {
		return forge.PullRequest{}, forge.Errorf(forge.ClassPermanent, providerName, op, nil,
			"pull request number must be positive, got %d", number)
	}

	args := []string{
		"pr", "view", strconv.Itoa(number),
		"--repo", repo.Slug(),
		"--json", prJSONFields,
	}
	out, err := p.run(ctx, op, args)
	if err != nil {
		return forge.PullRequest{}, err
	}

	var raw ghPullRequest
	if err := json.Unmarshal(out, &raw); err != nil {
		return forge.PullRequest{}, parseFailure(op, err)
	}
	return raw.toForge(), nil
}

// Checks implements forge.Provider.
//
// It merges two GitHub surfaces, because either alone is a partial picture:
// check-runs (the Actions-era API, paginated here with an explicit bound) and
// the legacy combined commit status. A repo using only classic statuses would
// otherwise roll up as "none".
//
// If the page bound is reached before GitHub runs out of check-runs, the
// rollup is marked Truncated and its state is forced to unknown — a state
// computed from an incomplete set is not a state worth trusting.
func (p *Provider) Checks(ctx context.Context, repo forge.Repo, ref string) (forge.CheckRollup, error) {
	const op = "Checks"
	if err := validateRepo(op, repo); err != nil {
		return forge.UnknownRollup(ref), err
	}
	if err := forge.ValidateRef(ref); err != nil {
		return forge.UnknownRollup(ref), err
	}

	checks, truncated, err := p.fetchCheckRuns(ctx, op, repo, ref)
	if err != nil {
		return forge.UnknownRollup(ref), err
	}
	statuses, err := p.fetchCombinedStatus(ctx, op, repo, ref)
	if err != nil {
		return forge.UnknownRollup(ref), err
	}
	checks = append(checks, statuses...)

	rollup := forge.CheckRollup{Ref: ref, Checks: checks, Truncated: truncated}
	if truncated {
		rollup.State = forge.CheckStateUnknown
	} else {
		rollup.State = forge.RollupState(checks)
	}
	return rollup, nil
}

func (p *Provider) fetchCheckRuns(ctx context.Context, op string, repo forge.Repo, ref string) ([]forge.Check, bool, error) {
	var checks []forge.Check
	for page := 1; page <= forge.MaxPages; page++ {
		path := fmt.Sprintf("repos/%s/%s/commits/%s/check-runs?per_page=%d&page=%d",
			url.PathEscape(repo.Owner), url.PathEscape(repo.Name), url.PathEscape(ref),
			forge.DefaultPageSize, page)
		out, err := p.run(ctx, op, []string{"api", "--method", "GET", path})
		if err != nil {
			return nil, false, err
		}
		var resp ghCheckRunsResponse
		if err := json.Unmarshal(out, &resp); err != nil {
			return nil, false, parseFailure(op, err)
		}
		for _, cr := range resp.CheckRuns {
			checks = append(checks, cr.toForge())
		}
		if len(resp.CheckRuns) < forge.DefaultPageSize {
			return checks, false, nil
		}
		if len(checks) >= resp.TotalCount && resp.TotalCount > 0 {
			return checks, false, nil
		}
	}
	// Bound reached with pages still outstanding.
	return checks, true, nil
}

func (p *Provider) fetchCombinedStatus(ctx context.Context, op string, repo forge.Repo, ref string) ([]forge.Check, error) {
	path := fmt.Sprintf("repos/%s/%s/commits/%s/status?per_page=%d",
		url.PathEscape(repo.Owner), url.PathEscape(repo.Name), url.PathEscape(ref),
		forge.DefaultPageSize)
	out, err := p.run(ctx, op, []string{"api", "--method", "GET", path})
	if err != nil {
		return nil, err
	}
	var resp ghCombinedStatus
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, parseFailure(op, err)
	}
	checks := make([]forge.Check, 0, len(resp.Statuses))
	for _, s := range resp.Statuses {
		checks = append(checks, s.toForge())
	}
	return checks, nil
}

// run executes gh and classifies any failure.
func (p *Provider) run(ctx context.Context, op string, args []string) ([]byte, error) {
	runner := p.runner
	if runner == nil {
		runner = execRunner{}
	}
	stdout, stderr, err := runner.Run(ctx, args)
	if err != nil {
		return nil, classify(op, stderr, err)
	}
	return stdout, nil
}

func validateRepo(op string, repo forge.Repo) error {
	if repo.Owner == "" || repo.Name == "" {
		return forge.Errorf(forge.ClassPermanent, providerName, op, nil,
			"repository identity is incomplete (%q)", repo.Slug())
	}
	// The slug is about to become an argv element; re-validate rather than
	// trust that it came from ParseRemoteURL.
	if _, err := forge.ParseRemoteURL("https://" + hostOrDefault(repo.Host) + "/" + repo.Slug()); err != nil {
		return forge.Errorf(forge.ClassPermanent, providerName, op, err,
			"unsafe repository identity %q", repo.Slug())
	}
	return nil
}

func hostOrDefault(host string) string {
	if host == "" {
		return "github.com"
	}
	return host
}

func ghStateFilter(op string, state forge.PRState) (string, error) {
	switch state {
	case forge.StateAll:
		return "all", nil
	case forge.PRStateOpen:
		return "open", nil
	case forge.PRStateClosed:
		return "closed", nil
	case forge.PRStateMerged:
		return "merged", nil
	default:
		return "", forge.Errorf(forge.ClassPermanent, providerName, op, nil,
			"cannot filter pull requests by state %q", state)
	}
}

func parseFailure(op string, err error) error {
	return forge.Errorf(forge.ClassPermanent, providerName, op, err,
		"could not parse gh JSON output")
}

// ---- gh JSON shapes -------------------------------------------------------

type ghPullRequest struct {
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

func (g *ghPullRequest) toForge() forge.PullRequest {
	labels := make([]string, 0, len(g.Labels))
	for _, l := range g.Labels {
		labels = append(labels, l.Name)
	}
	if len(labels) == 0 {
		labels = nil
	}

	// gh reports MERGED/OPEN/CLOSED; ParsePRState lowercases and maps
	// anything else to unknown rather than guessing.
	state := forge.ParsePRState(g.State)

	mergedAt := g.MergedAt
	if mergedAt != nil && mergedAt.IsZero() {
		mergedAt = nil
	}

	return forge.PullRequest{
		Number:    g.Number,
		Title:     g.Title,
		State:     state,
		IsDraft:   g.IsDraft,
		Author:    g.Author.Login,
		HeadRef:   g.HeadRefName,
		HeadSHA:   g.HeadRefOid,
		BaseRef:   g.BaseRefName,
		URL:       g.URL,
		Labels:    labels,
		CreatedAt: g.CreatedAt,
		UpdatedAt: g.UpdatedAt,
		MergedAt:  mergedAt,
	}
}

type ghCheckRunsResponse struct {
	TotalCount int          `json:"total_count"`
	CheckRuns  []ghCheckRun `json:"check_runs"`
}

type ghCheckRun struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Conclusion  string `json:"conclusion"`
	HTMLURL     string `json:"html_url"`
	CompletedAt string `json:"completed_at"`
}

func (c ghCheckRun) toForge() forge.Check {
	raw := c.Status
	if c.Conclusion != "" {
		raw = c.Status + "/" + c.Conclusion
	}
	var completed time.Time
	if c.CompletedAt != "" {
		if t, err := time.Parse(time.RFC3339, c.CompletedAt); err == nil {
			completed = t
		}
	}
	return forge.Check{
		Name:        c.Name,
		State:       checkRunState(c.Status, c.Conclusion),
		RawState:    raw,
		URL:         c.HTMLURL,
		CompletedAt: completed,
	}
}

// checkRunState maps a GitHub check-run onto a forge state. Every value not
// explicitly listed — including a completed run with an empty or novel
// conclusion — is unknown, never success.
func checkRunState(status, conclusion string) forge.CheckState {
	switch strings.ToLower(status) {
	case "queued", "in_progress", "waiting", "requested", "pending":
		return forge.CheckStatePending
	case "completed":
		switch strings.ToLower(conclusion) {
		case "success":
			return forge.CheckStateSuccess
		case "failure", "timed_out", "cancelled", "canceled", "action_required", "startup_failure":
			return forge.CheckStateFailure
		case "neutral", "skipped":
			return forge.CheckStateNeutral
		default:
			return forge.CheckStateUnknown
		}
	default:
		return forge.CheckStateUnknown
	}
}

type ghCombinedStatus struct {
	State    string     `json:"state"`
	Statuses []ghStatus `json:"statuses"`
}

type ghStatus struct {
	Context   string `json:"context"`
	State     string `json:"state"`
	TargetURL string `json:"target_url"`
	UpdatedAt string `json:"updated_at"`
}

func (s ghStatus) toForge() forge.Check {
	var updated time.Time
	if s.UpdatedAt != "" {
		if t, err := time.Parse(time.RFC3339, s.UpdatedAt); err == nil {
			updated = t
		}
	}
	return forge.Check{
		Name:        s.Context,
		State:       commitStatusState(s.State),
		RawState:    s.State,
		URL:         s.TargetURL,
		CompletedAt: updated,
	}
}

// commitStatusState maps a legacy commit status onto a forge state.
func commitStatusState(state string) forge.CheckState {
	switch strings.ToLower(state) {
	case "success":
		return forge.CheckStateSuccess
	case "pending":
		return forge.CheckStatePending
	case "failure", "error":
		return forge.CheckStateFailure
	default:
		return forge.CheckStateUnknown
	}
}
