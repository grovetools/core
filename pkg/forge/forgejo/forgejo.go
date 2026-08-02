// Package forgejo implements the read-only [forge.Provider] against Forgejo's
// Gitea-compatible REST API (/api/v1).
//
// No live instance exists yet, so this implementation is exercised entirely by
// httptest fixtures through the shared conformance suite. That is a deliberate
// constraint, not a gap: the wire shapes below are the documented Gitea v1
// shapes, and the day a real instance appears the same suite runs against it.
//
// # Credentials
//
// This package never resolves a token itself. It has no environment access, no
// config access, and no knowledge of `[forge] token_command` — the daemon
// resolves the token and injects it as a [TokenFunc]. A provider built without
// one simply sends unauthenticated requests, which is the correct behavior for
// a public instance and produces a clean ClassUnavailable on a private one.
package forgejo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/grovetools/core/pkg/forge"
)

const providerName = "forgejo"

// defaultTimeout bounds a single HTTP request when the caller supplies no
// client of its own.
const defaultTimeout = 30 * time.Second

// maxResponseBytes bounds how much of a response body is read, so a
// misbehaving or hostile instance cannot exhaust memory.
const maxResponseBytes = 32 << 20 // 32 MiB

// TokenFunc supplies a bearer token for a request. It is called per request so
// the daemon can rotate or lazily resolve credentials. Returning ("", nil)
// means "send unauthenticated"; returning an error fails the call with
// ClassUnavailable.
type TokenFunc func(ctx context.Context) (string, error)

// Provider is the read-only Forgejo/Gitea forge provider.
type Provider struct {
	baseURL *url.URL
	client  *http.Client
	token   TokenFunc
	// host overrides the identity host used by ResolveRepo validation; it
	// defaults to the base URL's hostname.
	host string
}

var _ forge.Provider = (*Provider)(nil)

// Option configures a Provider.
type Option func(*Provider)

// WithHTTPClient substitutes the HTTP client (httptest, instrumented
// transports, custom TLS).
func WithHTTPClient(c *http.Client) Option {
	return func(p *Provider) { p.client = c }
}

// WithToken injects the bearer-token source. Without it the provider sends
// unauthenticated requests.
func WithToken(f TokenFunc) Option {
	return func(p *Provider) { p.token = f }
}

// WithStaticToken is the convenience form of WithToken for a token the caller
// already holds. The token is never read from the environment by this package.
func WithStaticToken(token string) Option {
	return WithToken(func(context.Context) (string, error) { return token, nil })
}

// New builds a Forgejo provider for the instance at baseURL (e.g.
// "https://forge.example.com"). An unparseable or non-HTTP base URL is a
// ClassPermanent error: nothing about it will improve at call time.
func New(baseURL string, opts ...Option) (*Provider, error) {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return nil, forge.Errorf(forge.ClassPermanent, providerName, "New", nil,
			"forge URL is empty")
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return nil, forge.Errorf(forge.ClassPermanent, providerName, "New", err,
			"unparseable forge URL %q", baseURL)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return nil, forge.Errorf(forge.ClassPermanent, providerName, "New", nil,
			"forge URL %q must be http or https", baseURL)
	}
	if u.Host == "" {
		return nil, forge.Errorf(forge.ClassPermanent, providerName, "New", nil,
			"forge URL %q has no host", baseURL)
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	u.RawQuery = ""
	u.Fragment = ""

	p := &Provider{
		baseURL: u,
		client:  &http.Client{Timeout: defaultTimeout},
		host:    strings.ToLower(u.Hostname()),
	}
	for _, opt := range opts {
		opt(p)
	}
	if p.client == nil {
		p.client = &http.Client{Timeout: defaultTimeout}
	}
	return p, nil
}

// Name implements forge.Provider.
func (p *Provider) Name() string { return providerName }

// Capabilities implements forge.Provider. Reviews and issues are unknown, not
// unsupported: Gitea exposes both, but this wave never probed them, and the
// matrix reports what was verified rather than what is likely.
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

// ResolveRepo implements forge.Provider.
func (p *Provider) ResolveRepo(remoteURL string) (forge.Repo, error) {
	return forge.ParseRemoteURL(remoteURL)
}

// ListPRs implements forge.Provider, paging through /pulls up to
// forge.MaxPages pages or the caller's limit, whichever comes first.
func (p *Provider) ListPRs(ctx context.Context, repo forge.Repo, opts forge.ListPROptions) ([]forge.PullRequest, error) {
	const op = "ListPRs"
	if err := validateRepo(op, repo); err != nil {
		return nil, err
	}
	state, err := apiStateFilter(op, opts.EffectiveState())
	if err != nil {
		return nil, err
	}
	limit := opts.EffectiveLimit()

	var out []forge.PullRequest
	for page := 1; page <= forge.MaxPages; page++ {
		pageSize := forge.DefaultPageSize
		if remaining := limit - len(out); remaining < pageSize {
			pageSize = remaining
		}
		if pageSize <= 0 {
			break
		}

		q := url.Values{}
		q.Set("state", state)
		q.Set("page", fmt.Sprint(page))
		q.Set("limit", fmt.Sprint(pageSize))

		var raw []apiPullRequest
		if err := p.getJSON(ctx, op, repoPath(repo, "pulls"), q, &raw); err != nil {
			return nil, err
		}
		for i := range raw {
			out = append(out, raw[i].toForge())
		}
		if len(raw) < pageSize {
			break
		}
	}
	return out, nil
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

	var raw apiPullRequest
	path := repoPath(repo, "pulls/"+fmt.Sprint(number))
	if err := p.getJSON(ctx, op, path, nil, &raw); err != nil {
		return forge.PullRequest{}, err
	}
	return raw.toForge(), nil
}

// Checks implements forge.Provider using Gitea's commit-statuses endpoint.
//
// Gitea has no check-runs API; commit statuses are the whole surface. Reaching
// the page bound forces the rollup to unknown for the same reason as the
// GitHub implementation: a state computed from a partial set is not a state.
func (p *Provider) Checks(ctx context.Context, repo forge.Repo, ref string) (forge.CheckRollup, error) {
	const op = "Checks"
	if err := validateRepo(op, repo); err != nil {
		return forge.UnknownRollup(ref), err
	}
	if err := forge.ValidateRef(ref); err != nil {
		return forge.UnknownRollup(ref), err
	}

	var checks []forge.Check
	truncated := true
	path := repoPath(repo, "commits/"+url.PathEscape(ref)+"/statuses")
	for page := 1; page <= forge.MaxPages; page++ {
		q := url.Values{}
		q.Set("page", fmt.Sprint(page))
		q.Set("limit", fmt.Sprint(forge.DefaultPageSize))

		var raw []apiCommitStatus
		if err := p.getJSON(ctx, op, path, q, &raw); err != nil {
			return forge.UnknownRollup(ref), err
		}
		for _, s := range raw {
			checks = append(checks, s.toForge())
		}
		if len(raw) < forge.DefaultPageSize {
			truncated = false
			break
		}
	}

	rollup := forge.CheckRollup{Ref: ref, Checks: checks, Truncated: truncated}
	if truncated {
		rollup.State = forge.CheckStateUnknown
	} else {
		rollup.State = forge.RollupState(checks)
	}
	return rollup, nil
}

// ---- transport ------------------------------------------------------------

// getJSON performs a bounded GET against the instance API and decodes the
// response into out. Every failure is classified (see classifyStatus).
func (p *Provider) getJSON(ctx context.Context, op, path string, query url.Values, out any) error {
	endpoint := *p.baseURL
	endpoint.Path = strings.TrimSuffix(endpoint.Path, "/") + "/api/v1/" + strings.TrimPrefix(path, "/")
	if query != nil {
		endpoint.RawQuery = query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return forge.Errorf(forge.ClassPermanent, providerName, op, err, "could not build request")
	}
	req.Header.Set("Accept", "application/json")

	if p.token != nil {
		token, terr := p.token(ctx)
		if terr != nil {
			return forge.Errorf(forge.ClassUnavailable, providerName, op, terr,
				"could not resolve forge token")
		}
		if token != "" {
			req.Header.Set("Authorization", "token "+token)
		}
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return forge.Errorf(forge.ClassUnavailable, providerName, op, err,
			"forge is unreachable")
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return forge.Errorf(forge.ClassRetryable, providerName, op, err,
			"could not read forge response")
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return classifyStatus(op, resp, body)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return forge.Errorf(forge.ClassPermanent, providerName, op, err,
			"could not parse forge response")
	}
	return nil
}

func repoPath(repo forge.Repo, suffix string) string {
	return "repos/" + url.PathEscape(repo.Owner) + "/" + url.PathEscape(repo.Name) + "/" + suffix
}

func validateRepo(op string, repo forge.Repo) error {
	if repo.Owner == "" || repo.Name == "" {
		return forge.Errorf(forge.ClassPermanent, providerName, op, nil,
			"repository identity is incomplete (%q)", repo.Slug())
	}
	// The slug becomes a URL path; re-validate rather than trust provenance.
	if _, err := forge.ParseRemoteURL("https://forge.invalid/" + repo.Slug()); err != nil {
		return forge.Errorf(forge.ClassPermanent, providerName, op, err,
			"unsafe repository identity %q", repo.Slug())
	}
	return nil
}

func apiStateFilter(op string, state forge.PRState) (string, error) {
	switch state {
	case forge.StateAll:
		return "all", nil
	case forge.PRStateOpen:
		return "open", nil
	// Gitea's /pulls has no "merged" filter: merged PRs are closed PRs with a
	// merge timestamp. Ask for closed and let the caller read State/MergedAt
	// rather than silently returning nothing.
	case forge.PRStateClosed, forge.PRStateMerged:
		return "closed", nil
	default:
		return "", forge.Errorf(forge.ClassPermanent, providerName, op, nil,
			"cannot filter pull requests by state %q", state)
	}
}

// ---- Gitea v1 wire shapes -------------------------------------------------

type apiPullRequest struct {
	Number  int    `json:"number"`
	Title   string `json:"title"`
	State   string `json:"state"`
	Draft   bool   `json:"draft"`
	HTMLURL string `json:"html_url"`
	Merged  bool   `json:"merged"`
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

func (a *apiPullRequest) toForge() forge.PullRequest {
	labels := make([]string, 0, len(a.Labels))
	for _, l := range a.Labels {
		labels = append(labels, l.Name)
	}
	if len(labels) == 0 {
		labels = nil
	}

	// Gitea reports state "open"/"closed" and carries merged-ness separately;
	// a merged PR must surface as merged, not as closed.
	state := forge.ParsePRState(a.State)
	if a.Merged || a.MergedAt != nil {
		state = forge.PRStateMerged
	}

	mergedAt := a.MergedAt
	if mergedAt != nil && mergedAt.IsZero() {
		mergedAt = nil
	}

	return forge.PullRequest{
		Number:    a.Number,
		Title:     a.Title,
		State:     state,
		IsDraft:   a.Draft,
		Author:    a.User.Login,
		HeadRef:   a.Head.Ref,
		HeadSHA:   a.Head.SHA,
		BaseRef:   a.Base.Ref,
		URL:       a.HTMLURL,
		Labels:    labels,
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
		MergedAt:  mergedAt,
	}
}

type apiCommitStatus struct {
	Status      string    `json:"status"`
	Context     string    `json:"context"`
	TargetURL   string    `json:"target_url"`
	Description string    `json:"description"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (a apiCommitStatus) toForge() forge.Check {
	name := a.Context
	if name == "" {
		name = a.Description
	}
	return forge.Check{
		Name:        name,
		State:       commitStatusState(a.Status),
		RawState:    a.Status,
		URL:         a.TargetURL,
		CompletedAt: a.UpdatedAt,
	}
}

// commitStatusState maps a Gitea commit-status state onto a forge state.
// Gitea's vocabulary is pending/success/error/failure/warning; anything else —
// including the empty string — is unknown, never success.
func commitStatusState(state string) forge.CheckState {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "success":
		return forge.CheckStateSuccess
	case "pending":
		return forge.CheckStatePending
	case "failure", "error":
		return forge.CheckStateFailure
	case "warning":
		return forge.CheckStateNeutral
	default:
		return forge.CheckStateUnknown
	}
}
