package git

// The multi-remote model: everything the ecosystem knows about a repo's remotes
// WITHOUT contacting one.
//
// Every read in this file answers from local state alone — the configured
// remotes and the remote-TRACKING refs git already has on disk. Nothing here
// fetches implicitly, so a count is never "how far am I from the remote right
// now"; it is "how far am I from the last thing this repo heard from that
// remote". That distinction is the whole reason the freshness struct exists:
// see TrackingFreshness for exactly what git does and does not record, and
// RemoteBranchState.Freshness for how a surface is expected to say it.
//
// The one network path is FetchRemote, which is EXPLICIT: it exists so a user
// can ask for fresh tracking refs, and no read path calls it.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/grovetools/core/command"
)

// DefaultRemoteName is git's conventional name for a repo's upstream remote.
// It is the name every pre-multi-remote read in this package assumed, so the
// origin-only case stays byte-for-byte what it was: those call sites now pass
// this constant instead of spelling "origin" inline.
const DefaultRemoteName = "origin"

// Remote is one configured git remote: the name it is enrolled under and the
// URL it fetches from. A repo with no remotes at all — this ecosystem's
// local-first norm — yields an empty slice, not an error.
type Remote struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// TrackingFreshness is what git actually records about how current a
// remote-tracking ref is. Both fields are best-effort and BOTH can be zero; a
// surface must render an unknown timestamp as unknown, never as "now".
//
// What git does NOT record, and what this type therefore cannot tell you: when
// a given remote was last contacted. There is no per-remote fetch timestamp
// anywhere in a git repository. The two signals below bracket the answer from
// opposite sides, and an honest surface presents them as bounds rather than as
// a freshness verdict:
//
//   - RefUpdatedAt is the last time the tracking ref MOVED, read from its own
//     reflog. A fetch that found nothing new writes no reflog entry, so this is
//     a LOWER bound on freshness: the ref is at least this current, possibly
//     much more. It is zero when the ref has no reflog (reflogs for
//     refs/remotes depend on core.logAllRefUpdates) or does not exist.
//
//   - LastFetchAt is the modification time of this checkout's FETCH_HEAD — the
//     last `git fetch` run from THIS working tree, for ANY remote. FETCH_HEAD
//     lives in the per-worktree git dir, so it says nothing about fetches run
//     from a sibling worktree of the same repository, and it does not identify
//     which remote was fetched. It is zero when no fetch has ever run here.
type TrackingFreshness struct {
	RefUpdatedAt time.Time `json:"ref_updated_at,omitempty"`
	LastFetchAt  time.Time `json:"last_fetch_at,omitempty"`
}

// Known reports whether either signal is available. When false a surface has
// no basis at all for dating the counts and must say so.
func (f TrackingFreshness) Known() bool {
	return !f.RefUpdatedAt.IsZero() || !f.LastFetchAt.IsZero()
}

// MarshalJSON OMITS an unknown timestamp rather than emitting time.Time's zero
// value, which serializes as "0001-01-01T00:00:00Z" and reads as a real (very
// old) date to anything that does not know to special-case it. Absent means
// unknown, which is the whole point of this type. Decoding needs no counterpart:
// the struct tags parse the RFC3339 strings back, and a missing key leaves the
// zero time — unknown again.
func (f TrackingFreshness) MarshalJSON() ([]byte, error) {
	out := make(map[string]string, 2)
	if !f.RefUpdatedAt.IsZero() {
		out["ref_updated_at"] = f.RefUpdatedAt.Format(time.RFC3339)
	}
	if !f.LastFetchAt.IsZero() {
		out["last_fetch_at"] = f.LastFetchAt.Format(time.RFC3339)
	}
	return json.Marshal(out)
}

// RemoteBranchState is one (remote, branch) pair's position relative to a local
// ref, measured entirely against the tracking ref already on disk.
//
// Ahead/Behind are ONLY meaningful when Exists is true; a surface must render
// a non-existent tracking ref as "not published there" rather than as 0/0,
// which would read as "in sync".
type RemoteBranchState struct {
	Remote string `json:"remote"`
	Branch string `json:"branch"`

	// Exists reports whether refs/remotes/<Remote>/<Branch> is present locally.
	// It is emphatically NOT "the branch exists on the remote": a branch pushed
	// by someone else and never fetched here is absent, and a branch deleted on
	// the remote lingers until a pruning fetch. It is the honest local answer —
	// "this repo has heard of that branch" — and the freshness fields are what
	// let a caller say how long ago it heard.
	Exists bool `json:"exists"`

	// Ahead/Behind are the local ref's position relative to the tracking ref:
	// Ahead = commits the local ref has that the tracking ref does not,
	// Behind = the reverse. Both zero when Exists is false.
	Ahead  int `json:"ahead"`
	Behind int `json:"behind"`

	Freshness TrackingFreshness `json:"freshness"`
}

// RemoteTrackingRef spells the fully-qualified tracking ref for a
// (remote, branch) pair. Callers use the full refs/remotes/... spelling rather
// than the short "<remote>/<branch>" one because the short form goes through
// rev-parse's disambiguation ladder, where a local branch literally named
// "origin/main" would win over the tracking ref of the same name.
func RemoteTrackingRef(remote, branch string) string {
	return "refs/remotes/" + remote + "/" + branch
}

// ListRemotes returns the repo's configured remotes with their fetch URLs,
// sorted by name (git's own `remote -v` ordering). A repo with no remotes
// yields an empty slice and a nil error — having none is a normal state here,
// not a failure.
func ListRemotes(repoPath string) ([]Remote, error) {
	cmdBuilder := command.NewSafeBuilder()
	cmd, err := cmdBuilder.Build(context.Background(), "git", "remote", "-v")
	if err != nil {
		return nil, fmt.Errorf("failed to build command: %w", err)
	}
	execCmd := cmd.Exec()
	execCmd.Dir = repoPath
	output, err := execCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list remotes in %s: %w", repoPath, err)
	}
	return parseRemotesV(string(output)), nil
}

// parseRemotesV parses `git remote -v` output. Each remote contributes a
// "<name>\t<url> (fetch)" line and a "(push)" line; the FETCH url is the one
// this model reports, since every read here is against tracking refs that a
// fetch populates. A remote whose push URL differs is not a distinct remote and
// gets no extra row. Split out so the parsing is unit-testable without a repo.
func parseRemotesV(out string) []Remote {
	seen := make(map[string]bool)
	remotes := []Remote{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		name, rest, found := strings.Cut(line, "\t")
		if !found {
			continue
		}
		url, kind, found := strings.Cut(rest, " (")
		if !found || strings.TrimSuffix(kind, ")") != "fetch" {
			continue
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		remotes = append(remotes, Remote{Name: name, URL: url})
	}
	sort.Slice(remotes, func(i, j int) bool { return remotes[i].Name < remotes[j].Name })
	return remotes
}

// RemoteBranchExists reports whether <remote>/<branch> is present as a
// remote-tracking ref in repoPath — `git show-ref --verify --quiet
// refs/remotes/<remote>/<branch>`, the same probe LocalMainBranch uses for
// local refs, and exactly the probe HasRemoteBranch has always run for origin.
//
// It answers from tracking refs alone and NEVER fetches, so "false" means "this
// repo has not heard of that branch", not "the branch does not exist on the
// remote". A detached branch name is never remote-tracked.
func RemoteBranchExists(repoPath, remote, branch string) bool {
	if IsDetachedHead(branch) || !validRefComponent(remote) || !validRefComponent(branch) {
		return false
	}
	cmdBuilder := command.NewSafeBuilder()
	cmd, err := cmdBuilder.Build(context.Background(), "git", "show-ref", "--verify", "--quiet", RemoteTrackingRef(remote, branch))
	if err != nil {
		return false
	}
	execCmd := cmd.Exec()
	execCmd.Dir = repoPath
	return execCmd.Run() == nil
}

// GetRemoteBranchState computes localRef's position relative to
// <remote>/<branch> plus the freshness of the tracking ref the counts came
// from. localRef defaults to "HEAD".
//
// Cost when the tracking ref exists: one show-ref, one rev-list, one reflog
// read, and one stat. When it does not: one show-ref and one stat — the counts
// stay zero and Exists=false is the caller's signal not to render them, but the
// last-fetch bound is still reported so the absence can be dated.
//
// It never fetches. See TrackingFreshness for what the returned timestamps do
// and do not mean.
func GetRemoteBranchState(repoPath, remote, branch, localRef string) RemoteBranchState {
	if localRef == "" {
		localRef = "HEAD"
	}
	st := RemoteBranchState{Remote: remote, Branch: branch}
	// LastFetchAt is read even when the tracking ref is ABSENT, and that is the
	// point: "not published there" is an observation, and how old the
	// observation is decides how much it is worth. An absence backed by a fetch
	// a minute ago is near-certain; one backed by a fetch three days ago is a
	// guess. Reporting the bound either way is what keeps the two apart.
	st.Freshness.LastFetchAt = lastFetchAt(repoPath)

	if !RemoteBranchExists(repoPath, remote, branch) {
		return st
	}
	st.Exists = true

	ref := RemoteTrackingRef(remote, branch)
	// GetCommitsDivergence(base, target) reports target's ahead/behind relative
	// to base, so the tracking ref is the base and the local ref the target.
	if ahead, behind, err := GetCommitsDivergence(repoPath, ref, localRef); err == nil {
		st.Ahead, st.Behind = ahead, behind
	}
	st.Freshness.RefUpdatedAt = refLastUpdated(repoPath, ref)
	return st
}

// RemoteBranchDistance is the narrow probe the pre-multi-remote origin reads
// used: does <remote>/<branch> exist, and how far is HEAD behind it. It is the
// shared core behind the Rebase page's origin column, kept as its own entry
// point so that path costs exactly what it always did — no reflog read, no
// stat — and its behavior is unchanged.
func RemoteBranchDistance(repoPath, remote, branch string) (exists bool, behind int) {
	if !RemoteBranchExists(repoPath, remote, branch) {
		return false, 0
	}
	if _, behind, err := GetCommitsDivergence(repoPath, RemoteTrackingRef(remote, branch), "HEAD"); err == nil {
		return true, behind
	}
	// A tracking ref that exists but whose divergence cannot be computed is
	// still a pushed branch; reporting zero distance is what this probe has
	// always done rather than claiming the branch is unpublished.
	return true, 0
}

// ListRemoteBranchStates computes the state of branch against EVERY configured
// remote, in ListRemotes order. A repo with no remotes yields an empty slice —
// the local-first norm, rendered as "no remotes" rather than as an error.
func ListRemoteBranchStates(repoPath, branch, localRef string) ([]RemoteBranchState, error) {
	remotes, err := ListRemotes(repoPath)
	if err != nil {
		return nil, err
	}
	states := make([]RemoteBranchState, 0, len(remotes))
	for _, r := range remotes {
		states = append(states, GetRemoteBranchState(repoPath, r.Name, branch, localRef))
	}
	return states, nil
}

// refLastUpdated reads the last time ref MOVED from its own reflog:
// `git reflog show --date=unix -n 1 <ref>` prints
// "<sha> <ref>@{<unix>}: <message>". A ref with no reflog (or none at all)
// yields the zero time, which callers must render as unknown.
func refLastUpdated(repoPath, ref string) time.Time {
	cmdBuilder := command.NewSafeBuilder()
	cmd, err := cmdBuilder.Build(context.Background(), "git", "reflog", "show", "--date=unix", "-n", "1", ref)
	if err != nil {
		return time.Time{}
	}
	execCmd := cmd.Exec()
	execCmd.Dir = repoPath
	output, err := execCmd.Output()
	if err != nil {
		return time.Time{}
	}
	return parseReflogUnixStamp(string(output))
}

// parseReflogUnixStamp pulls the seconds-since-epoch out of a `--date=unix`
// reflog line's "@{<unix>}" selector. Split out so the (fiddly) parsing is
// testable without a repo. Returns the zero time on anything unexpected.
func parseReflogUnixStamp(out string) time.Time {
	line, _, _ := strings.Cut(out, "\n")
	_, after, found := strings.Cut(line, "@{")
	if !found {
		return time.Time{}
	}
	stamp, _, found := strings.Cut(after, "}")
	if !found {
		return time.Time{}
	}
	secs, err := strconv.ParseInt(strings.TrimSpace(stamp), 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(secs, 0)
}

// lastFetchAt is the mtime of this checkout's FETCH_HEAD — the last `git fetch`
// run from THIS working tree, for any remote. FETCH_HEAD is per-worktree (it
// lives in the gitdir, not the common dir), so a linked worktree reports its
// own fetch history and not its owner's. Zero when no fetch has run here.
func lastFetchAt(repoPath string) time.Time {
	gitDir, _, err := resolveGitDirs(filepath.Clean(repoPath))
	if err != nil {
		return time.Time{}
	}
	info, err := os.Stat(filepath.Join(gitDir, "FETCH_HEAD"))
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// FetchRemote runs `git fetch <remote>` in repoPath, refreshing that remote's
// tracking refs.
//
// It is the ONE network path in this file and it is deliberately not called by
// any read: every other function here answers from refs already on disk, so a
// surface can list remotes and their divergence without contacting anything.
// Callers must invoke this only in response to an explicit user action.
//
// It is a network READ — it updates local tracking refs and mutates nothing on
// the remote. No --prune (which would delete local tracking refs), no refspec
// override, no push.
func FetchRemote(ctx context.Context, repoPath, remote string) error {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return fmt.Errorf("cannot fetch: empty remote name")
	}
	if !validRefComponent(remote) {
		return fmt.Errorf("refusing to fetch remote %q: illegal remote name", remote)
	}
	cmdBuilder := command.NewSafeBuilder()
	cmd, err := cmdBuilder.Build(ctx, "git", "fetch", "--", remote)
	if err != nil {
		return fmt.Errorf("failed to build command: %w", err)
	}
	execCmd := cmd.Exec()
	execCmd.Dir = repoPath
	if output, err := execCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git fetch %s failed: %s", remote, strings.TrimSpace(string(output)))
	}
	return nil
}

// validRefComponent rejects names that would be read as flags by the git
// commands that carry them, or that could escape the ref path they are pasted
// into. Remote names reaching here come from git's own config or from user
// config, so this is a belt-and-braces guard, not the primary validation.
func validRefComponent(name string) bool {
	if name == "" || strings.HasPrefix(name, "-") || strings.Contains(name, "..") {
		return false
	}
	return !strings.ContainsAny(name, " \t\n\\:?[]^~*")
}
