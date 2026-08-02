package registry

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// maxTipScanEntries caps how many directory entries one declared root
// contributes. A machine note is a document that replicates to every other
// machine on every structural change; a mis-declared root pointing at a
// 10,000-entry directory must degrade to a truncated list, not to a megabyte
// of frontmatter pushed through the sync pipeline.
const maxTipScanEntries = 256

// CollectRepoTips reads the point-in-time tip of every git repository under
// each declared root, sorted by (root, path).
//
// Tips are read straight out of .git — HEAD, the loose ref, packed-refs — and
// never by shelling out to git. Two reasons: the writer runs on a timer inside
// the daemon, where forking a process per repo per tick is a real cost; and
// the read must never block on a git process that is waiting on an index lock
// held by the user's own terminal.
//
// Only the root itself and its immediate children are scanned. That covers the
// two layouts the design admits — a superrepo whose submodules are its child
// directories, and a bare scan root that is a directory of repos — without a
// recursive walk of the user's whole code tree on every tick.
func CollectRepoTips(roots map[string]string) []NoteRepo {
	var out []NoteRepo
	for _, name := range sortedStringKeys(roots) {
		root := roots[name]
		if root == "" {
			continue
		}
		if branch, sha, ok := ReadRepoTip(root); ok {
			out = append(out, NoteRepo{Root: name, Path: ".", Branch: branch, SHA: sha})
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		if len(entries) > maxTipScanEntries {
			entries = entries[:maxTipScanEntries]
		}
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			child := filepath.Join(root, entry.Name())
			if branch, sha, ok := ReadRepoTip(child); ok {
				out = append(out, NoteRepo{Root: name, Path: entry.Name(), Branch: branch, SHA: sha})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Root != out[j].Root {
			return out[i].Root < out[j].Root
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// ReadRepoTip returns the checked-out branch and commit of a repository
// without invoking git. Branch is empty on a detached HEAD. ok is false when
// dir is not a repository, or when its HEAD cannot be resolved.
func ReadRepoTip(dir string) (branch, sha string, ok bool) {
	gitDir, commonDir := resolveGitDirs(dir)
	if gitDir == "" {
		return "", "", false
	}
	head, err := readTrimmed(filepath.Join(gitDir, "HEAD"))
	if err != nil || head == "" {
		return "", "", false
	}
	if !strings.HasPrefix(head, "ref:") {
		// Detached HEAD: the file holds the object id itself.
		return "", head, isHex(head)
	}
	ref := strings.TrimSpace(strings.TrimPrefix(head, "ref:"))
	branch = strings.TrimPrefix(ref, "refs/heads/")

	// A loose ref in this worktree's git dir wins (a linked worktree keeps its
	// own HEAD there), then the common dir's loose ref, then packed-refs.
	for _, base := range []string{gitDir, commonDir} {
		if base == "" {
			continue
		}
		if v, err := readTrimmed(filepath.Join(base, filepath.FromSlash(ref))); err == nil && isHex(v) {
			return branch, v, true
		}
	}
	for _, base := range []string{gitDir, commonDir} {
		if base == "" {
			continue
		}
		if v := lookupPackedRef(filepath.Join(base, "packed-refs"), ref); v != "" {
			return branch, v, true
		}
	}
	// An unborn branch (a fresh `git init`) has a ref with no commit. The repo
	// is real and the branch is meaningful, so report it with an empty sha
	// rather than dropping the repo from the note entirely.
	return branch, "", true
}

// resolveGitDirs returns a repository's git directory and, for a linked
// worktree or a submodule, the common directory that actually holds refs.
func resolveGitDirs(dir string) (gitDir, commonDir string) {
	candidate := filepath.Join(dir, ".git")
	info, err := os.Stat(candidate)
	if err != nil {
		return "", ""
	}
	if info.IsDir() {
		gitDir = candidate
	} else {
		// A ".git" FILE is a linked worktree or a submodule: it holds
		// "gitdir: <path>", relative to the repo when not absolute.
		line, rerr := readTrimmed(candidate)
		if rerr != nil || !strings.HasPrefix(line, "gitdir:") {
			return "", ""
		}
		target := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
		if target == "" {
			return "", ""
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(dir, target)
		}
		gitDir = filepath.Clean(target)
	}

	// commondir points at the repository whose refs this worktree shares.
	if v, err := readTrimmed(filepath.Join(gitDir, "commondir")); err == nil && v != "" {
		if !filepath.IsAbs(v) {
			v = filepath.Join(gitDir, v)
		}
		commonDir = filepath.Clean(v)
	}
	return gitDir, commonDir
}

// lookupPackedRef scans packed-refs for one fully-qualified ref name.
func lookupPackedRef(path, ref string) string {
	f, err := os.Open(path) //nolint:gosec // path derived from a declared root's .git dir
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || line[0] == '#' || line[0] == '^' {
			continue
		}
		sha, name, found := strings.Cut(line, " ")
		if !found || strings.TrimSpace(name) != ref {
			continue
		}
		if isHex(sha) {
			return sha
		}
	}
	return ""
}

func readTrimmed(path string) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path derived from a declared root's .git dir
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// isHex reports whether s looks like a git object id.
func isHex(s string) bool {
	if len(s) < 7 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

func sortedStringKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
