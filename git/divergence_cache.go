package git

// Divergence caching for GetCommitsDivergenceFromMain /
// GetCommitsDivergenceFromRemoteMain.
//
// Those functions cost 2-3 git forks per call (branch resolution probes plus
// `git rev-list --left-right --count`), and the daemon's status sweep calls
// them for hundreds of repos even when nothing has moved. Ahead/behind counts
// are a pure function of (HEAD SHA, base-ref SHA), so a cached result stays
// valid as long as both SHAs are unchanged — and both SHAs can be resolved
// with zero forks by reading git's own files (HEAD, loose refs, packed-refs).
//
// The filesystem resolver is strictly a cache-VALIDITY probe, never an
// authority: on any anomaly — missing files, unparseable content, symlinked
// HEAD, nested symrefs, unknown layouts — it reports failure and the caller
// silently falls back to the forked git path (and does not cache). Correctness
// therefore never depends on the resolver keeping up with git's on-disk
// formats.
//
// Layouts handled:
//   - normal repos: <repo>/.git is a directory; refs live under it.
//   - linked worktrees: <repo>/.git is a FILE containing "gitdir: <path>";
//     that per-worktree gitdir holds HEAD, and its "commondir" file points at
//     the shared common dir holding refs/ and packed-refs.
//   - HEAD as a symref ("ref: refs/heads/x"), resolved through loose refs
//     then packed-refs in the common dir; detached HEAD as a raw SHA.
//
// Punted (resolver returns !ok, forked path used): symlinked .git or HEAD,
// nested symrefs (a loose ref containing "ref: ..."), refs/ subtree neither
// loose nor packed (e.g. reftable repos), unborn branches, uppercase or
// non-SHA ref contents.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// localMainRefCandidates / remoteMainRefCandidates mirror the branch probing
// order of LocalMainBranch and GetCommitsDivergenceFromRemoteMain.
var (
	localMainRefCandidates  = []string{"refs/heads/main", "refs/heads/master"}
	remoteMainRefCandidates = []string{"refs/remotes/origin/main", "refs/remotes/origin/master"}
)

// divergenceSHAs is a filesystem-resolved snapshot of the two endpoints a
// divergence count depends on.
type divergenceSHAs struct {
	headSHA string
	baseRef string // the candidate ref that resolved, e.g. "refs/heads/main"
	baseSHA string
}

// divergenceEntry caches the ahead/behind counts produced by a forked
// rev-list run, pinned to the exact SHAs they were computed from.
type divergenceEntry struct {
	divergenceSHAs
	ahead  int
	behind int
}

const maxDivergenceCacheEntries = 8192

var (
	divergenceCacheMu sync.Mutex
	divergenceCache   = make(map[string]divergenceEntry) // keyed by cleaned repo path
)

// Cache effectiveness counters. They live as package globals because the
// cache itself does (there is no injectable type here, and GetExtendedStatus'
// signature is on the hot path of every git sweep). The daemon reads them via
// DivergenceCacheStats and publishes them on /api/system/stats: a collapsing
// hit rate is the earliest signal that git status forks are about to dominate
// daemon CPU again.
var (
	divergenceHits    atomic.Uint64
	divergenceMisses  atomic.Uint64
	divergenceWastedC atomic.Uint64
)

// DivergenceCacheStats reports the process-wide ahead/behind cache counters.
// wasted counts forked computations that were discarded because a ref moved
// while rev-list ran — work paid for and thrown away.
func DivergenceCacheStats() (hits, misses, wasted uint64) {
	return divergenceHits.Load(), divergenceMisses.Load(), divergenceWastedC.Load()
}

// lookupDivergence returns the cached counts for cleanPath if the entry was
// computed from exactly the given SHAs (and the same base ref).
func lookupDivergence(cleanPath string, shas divergenceSHAs) (ahead, behind int, ok bool) {
	divergenceCacheMu.Lock()
	defer divergenceCacheMu.Unlock()
	e, ok := divergenceCache[cleanPath]
	if !ok || e.divergenceSHAs != shas {
		divergenceMisses.Add(1)
		return 0, 0, false
	}
	divergenceHits.Add(1)
	return e.ahead, e.behind, true
}

// storeDivergence records counts computed by the forked path. The coarse
// size backstop prevents a long-running daemon from retaining an unbounded
// number of repositories that no longer exist.
func storeDivergence(cleanPath string, shas divergenceSHAs, ahead, behind int) {
	divergenceCacheMu.Lock()
	defer divergenceCacheMu.Unlock()
	if len(divergenceCache) >= maxDivergenceCacheEntries {
		divergenceCache = make(map[string]divergenceEntry)
	}
	divergenceCache[cleanPath] = divergenceEntry{divergenceSHAs: shas, ahead: ahead, behind: behind}
}

// storeDivergenceIfCurrent re-resolves both endpoints after the fork and only
// caches its result when refs are unchanged from the pre-fork snapshot.
func storeDivergenceIfCurrent(cleanPath string, baseCandidates []string, before divergenceSHAs, ahead, behind int) bool {
	after, ok := resolveDivergenceSHAs(cleanPath, baseCandidates)
	if !ok || after != before {
		divergenceWastedC.Add(1)
		return false
	}
	storeDivergence(cleanPath, after, ahead, behind)
	return true
}

// resolveDivergenceSHAs resolves the current HEAD SHA and the first base
// candidate ref that exists, entirely from the filesystem. ok is false on any
// anomaly (including no candidate existing), in which case the caller must use
// the forked path and skip caching.
func resolveDivergenceSHAs(cleanPath string, baseCandidates []string) (divergenceSHAs, bool) {
	gitDir, commonDir, err := resolveGitDirs(cleanPath)
	if err != nil {
		return divergenceSHAs{}, false
	}
	headSHA, err := resolveHeadSHA(gitDir, commonDir)
	if err != nil {
		return divergenceSHAs{}, false
	}
	for _, ref := range baseCandidates {
		sha, err := resolveRefSHA(commonDir, ref)
		if err == nil {
			return divergenceSHAs{headSHA: headSHA, baseRef: ref, baseSHA: sha}, true
		}
		if !errors.Is(err, errRefNotFound) {
			// Cleanly absent refs mean "try the next candidate"; anything else
			// is an anomaly the resolver must not paper over.
			return divergenceSHAs{}, false
		}
	}
	return divergenceSHAs{}, false
}

// resolveGitDirs locates the repository's per-worktree git dir (holding HEAD)
// and its common dir (holding refs/ and packed-refs) without forking git.
func resolveGitDirs(repoPath string) (gitDir, commonDir string, err error) {
	dotGit := filepath.Join(repoPath, ".git")
	fi, err := os.Lstat(dotGit)
	if err != nil {
		return "", "", err
	}
	switch {
	case fi.IsDir():
		gitDir = dotGit
	case fi.Mode().IsRegular():
		// Linked worktree: .git is a file "gitdir: <path>".
		target, err := readFirstLine(dotGit)
		if err != nil {
			return "", "", err
		}
		target, found := strings.CutPrefix(target, "gitdir: ")
		if !found || target == "" {
			return "", "", fmt.Errorf("unrecognized .git file in %s", repoPath)
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(repoPath, target)
		}
		gitDir = filepath.Clean(target)
	default:
		// Symlinks and anything more exotic are punted to the forked path.
		return "", "", fmt.Errorf(".git is neither a directory nor a regular file in %s", repoPath)
	}

	// A "commondir" file (present in linked-worktree git dirs) points at the
	// shared dir owning refs/ and packed-refs; without one they live here.
	commonDir = gitDir
	switch line, err := readFirstLine(filepath.Join(gitDir, "commondir")); {
	case err == nil:
		if line == "" {
			return "", "", fmt.Errorf("empty commondir file in %s", gitDir)
		}
		if !filepath.IsAbs(line) {
			line = filepath.Join(gitDir, line)
		}
		commonDir = filepath.Clean(line)
	case !errors.Is(err, os.ErrNotExist):
		return "", "", err
	}
	return gitDir, commonDir, nil
}

// resolveHeadSHA resolves gitDir/HEAD to a commit SHA: either a raw SHA
// (detached) or a "ref: refs/heads/x" symref followed through the common dir.
func resolveHeadSHA(gitDir, commonDir string) (string, error) {
	headPath := filepath.Join(gitDir, "HEAD")
	fi, err := os.Lstat(headPath)
	if err != nil {
		return "", err
	}
	if !fi.Mode().IsRegular() {
		return "", fmt.Errorf("HEAD is not a regular file in %s", gitDir)
	}
	line, err := readFirstLine(headPath)
	if err != nil {
		return "", err
	}
	if ref, found := strings.CutPrefix(line, "ref: "); found {
		return resolveRefSHA(commonDir, strings.TrimSpace(ref))
	}
	if isHexSHA(line) {
		return line, nil
	}
	return "", fmt.Errorf("unparseable HEAD content %q in %s", line, gitDir)
}

// errRefNotFound marks a ref that is cleanly absent (no loose file, not in
// packed-refs) as opposed to present-but-unreadable.
var errRefNotFound = errors.New("ref not found")

// resolveRefSHA resolves a fully-qualified ref (e.g. "refs/heads/main") to a
// SHA via its loose file, falling back to packed-refs. Returns errRefNotFound
// when the ref cleanly does not exist.
func resolveRefSHA(commonDir, ref string) (string, error) {
	if ref == "" || !strings.HasPrefix(ref, "refs/") || strings.Contains(ref, "..") {
		return "", fmt.Errorf("refusing to resolve ref %q", ref)
	}
	switch line, err := readFirstLine(filepath.Join(commonDir, filepath.FromSlash(ref))); {
	case err == nil:
		// A nested symref ("ref: ...") or other non-SHA content is an anomaly.
		if !isHexSHA(line) {
			return "", fmt.Errorf("unparseable loose ref %s: %q", ref, line)
		}
		return line, nil
	case errors.Is(err, os.ErrNotExist):
		return lookupPackedRef(commonDir, ref)
	default:
		return "", err
	}
}

// lookupPackedRef scans commonDir/packed-refs for the ref. Lines are
// "<sha> <refname>"; comment ("#") and peeled ("^") lines are skipped.
func lookupPackedRef(commonDir, ref string) (string, error) {
	data, err := os.ReadFile(filepath.Join(commonDir, "packed-refs"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errRefNotFound
		}
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "^") {
			continue
		}
		sha, name, found := strings.Cut(line, " ")
		if !found || name != ref {
			continue
		}
		if !isHexSHA(sha) {
			return "", fmt.Errorf("unparseable packed-refs entry for %s: %q", ref, line)
		}
		return sha, nil
	}
	return "", errRefNotFound
}

// readFirstLine reads path and returns its first line, trimmed. Git's HEAD,
// loose ref, .git-file, and commondir files are all single-line.
func readFirstLine(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	line, _, _ := strings.Cut(string(data), "\n")
	return strings.TrimSpace(line), nil
}

// isHexSHA reports whether s looks like a full object name as git writes them
// into ref files: 40 (SHA-1) or 64 (SHA-256) lowercase hex characters.
func isHexSHA(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
