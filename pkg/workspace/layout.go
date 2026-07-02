// layout.go centralizes worktree directory layout knowledge for the grove
// ecosystem. These helpers are the single source of truth for where grove
// worktrees live and how a worktree maps back to its owning repository.
//
// Two layouts are supported:
//
//   - legacy (in-repo):  <gitRoot>/.grove-worktrees/<name>
//   - XDG (out-of-repo): paths.WorktreesDir()/<DirIdentifier(gitRoot)>/<name>
//
// The legacy layout is supported indefinitely; the XDG layout is used for
// sibling-workspace (ecosystem) worktrees. cx-internal commit-keyed
// checkouts (DataDir()/cx/...) also contain the legacy directory literal
// but are NOT grove worktrees — every helper carves them out.
//
// core/git cannot import this package (import cycle via util/pathutil); it
// uses paths.LegacyWorktreeDirName directly for its single legacy join.
package workspace

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/worktreeregistry"
	"github.com/grovetools/core/util/pathutil"
)

// legacyWorktreeDirName mirrors paths.LegacyWorktreeDirName for use inside
// this package.
const legacyWorktreeDirName = paths.LegacyWorktreeDirName

// DirIdentifier returns the XDG worktree subdirectory name for the
// repository rooted at gitRoot:
//
//	<sanitized basename>-<sha256(normalized abs gitRoot)[:8]>
//
// The path is normalized via pathutil.NormalizeForLookup before hashing so
// different spellings on case-insensitive filesystems agree. The result is
// human-readable AND collision-safe (two same-basename roots get distinct
// identifiers), and deliberately short to respect sandbox socket-path
// limits.
func DirIdentifier(gitRoot string) string {
	return pathutil.WorktreeID(gitRoot)
}

// WorktreeBases returns the ordered, legacy-first list of identifier-level
// directories that can contain worktrees of the repository rooted at
// gitRoot:
//
//	[<gitRoot>/.grove-worktrees, paths.WorktreesDir()/<DirIdentifier(gitRoot)>]
//
// Callers enumerate or probe each base; neither is guaranteed to exist.
func WorktreeBases(gitRoot string) []string {
	bases := []string{filepath.Join(gitRoot, legacyWorktreeDirName)}
	if wtd := paths.WorktreesDir(); wtd != "" {
		bases = append(bases, filepath.Join(wtd, DirIdentifier(gitRoot)))
	}
	return bases
}

// IsWorktreePath reports whether path refers to (or is inside) a grove
// worktree location. The check is anchored to path components:
//
//   - true when path has a .grove-worktrees component, EXCEPT inside
//     DataDir()/cx (cx-internal commit-keyed checkouts are not worktrees);
//   - true when path is strictly under paths.WorktreesDir()/<identifier>/
//     — the XDG base itself and the identifier-level dirs are containers,
//     not worktrees.
func IsWorktreePath(path string) bool {
	if path == "" {
		return false
	}
	clean := filepath.Clean(path)

	// XDG layout: must be at least <base>/<identifier>/<name> deep.
	if wtd := paths.WorktreesDir(); wtd != "" {
		if rel, ok := relUnderAnyForm(wtd, clean); ok {
			return strings.Contains(rel, string(filepath.Separator))
		}
	}

	// Legacy layout: an anchored .grove-worktrees path component...
	if !hasPathComponent(clean, legacyWorktreeDirName) {
		return false
	}
	// ...outside the cx checkout area, which embeds the same literal under
	// the grove data root (DataDir()/cx/repos/<repo>/.grove-worktrees/<commit>).
	if dataDir := paths.DataDir(); dataDir != "" {
		cxRoot := filepath.Join(dataDir, "cx")
		if clean == filepath.Clean(cxRoot) {
			return false
		}
		if _, ok := relUnderAnyForm(cxRoot, clean); ok {
			return false
		}
	}
	return true
}

// FindWorktreePath probes the known worktree bases of gitRoot for an
// EXISTING worktree named name, legacy base first. name may contain '/'
// (branch-style names); Join nests it the same way in every layout.
// Probing is pure per-call (no caching), so long-running daemons always see
// fresh filesystem state.
func FindWorktreePath(gitRoot, name string) (string, bool) {
	for _, base := range WorktreeBases(gitRoot) {
		candidate := filepath.Join(base, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}
	}
	return "", false
}

// ResolveWorktreePathByName resolves the absolute path of an EXISTING worktree
// named name, using the per-worktree registry as the primary source of truth
// and the on-disk layout bases as a fallback. It is the single helper every
// consumer should use to answer "where does the worktree called <name> live?"
// so that anchored worktrees (created with `--anchor <sub-repo>`, which live
// under the ANCHOR repo's XDG base rather than gitRoot's) resolve everywhere.
//
// Resolution order:
//
//  1. Registry-first. Scan worktreeregistry.ListAll() for an entry whose
//     AbsPath basename == name and whose directory still exists on disk. When
//     acceptOwners is non-empty, the entry's Owner must (after abs/symlink
//     normalization) match one of acceptOwners — this keeps the match scoped
//     and unambiguous in a multi-ecosystem registry. When acceptOwners is nil,
//     any owner is accepted (gitRoot is still implicitly acceptable).
//  2. gitRoot's own bases. Probe WorktreeBases(gitRoot) (legacy + XDG) — the
//     pre-anchor default location for an ecosystem worktree.
//  3. Owner bases. Probe WorktreeBases(owner) for each acceptOwner — covers an
//     anchored worktree whose registry entry is missing/corrupt but whose
//     directory still exists under the anchor's XDG base.
//
// Returns ("", false) when nothing resolves.
func ResolveWorktreePathByName(gitRoot, name string, acceptOwners []string) (string, bool) {
	// Normalize the accepted owner set once for comparison.
	ownerSet := map[string]struct{}{}
	addOwner := func(p string) {
		if p == "" {
			return
		}
		abs := p
		if a, err := filepath.Abs(p); err == nil {
			abs = a
		}
		if r, err := filepath.EvalSymlinks(abs); err == nil {
			abs = r
		}
		// Lowercase the key: owner paths come from heterogeneous sources (the
		// workspace provider can return a case-folded path while the registry
		// stores real on-disk casing), and macOS/Windows filesystems are
		// case-insensitive. EvalSymlinks does NOT correct case, so compare folded.
		ownerSet[strings.ToLower(filepath.Clean(abs))] = struct{}{}
	}
	for _, o := range acceptOwners {
		addOwner(o)
	}
	// gitRoot is always an acceptable owner (an ecosystem worktree's owner IS
	// the ecosystem root).
	addOwner(gitRoot)

	// gitRootCanon is the normalized ecosystem root, used to accept any owner
	// that lives strictly UNDER it (every sub-repo of the ecosystem is a child
	// of gitRoot on disk, so an `--anchor <sub-repo>` owner is always under it).
	// This makes owner-scoping independent of provider spelling/discovery: we
	// don't need the caller to enumerate every sub-repo path exactly.
	gitRootCanon := func() string {
		abs := gitRoot
		if a, err := filepath.Abs(gitRoot); err == nil {
			abs = a
		}
		if r, err := filepath.EvalSymlinks(abs); err == nil {
			abs = r
		}
		return strings.ToLower(filepath.Clean(abs))
	}()

	ownerAccepted := func(owner string) bool {
		// An empty caller-supplied acceptOwners means "accept any owner"; the
		// set still contains gitRoot, so size 1 (just gitRoot) is the
		// any-owner-but-prefer-scoped case only when the caller passed extra
		// owners. We treat len(acceptOwners)==0 as accept-any.
		if len(acceptOwners) == 0 {
			return true
		}
		// A scoped lookup cannot accept an entry with no owner (malformed/partial
		// registry rows): there is nothing to scope-check, so reject it rather
		// than letting it match on a cwd-derived empty path.
		if owner == "" {
			return false
		}
		abs := owner
		if a, err := filepath.Abs(owner); err == nil {
			abs = a
		}
		if r, err := filepath.EvalSymlinks(abs); err == nil {
			abs = r
		}
		// Compare folded: see addOwner — case-insensitive FS + provider casing.
		abs = strings.ToLower(filepath.Clean(abs))
		if _, ok := ownerSet[abs]; ok {
			return true
		}
		// Accept any owner under the ecosystem root (sub-repos / anchor targets).
		if gitRootCanon != "" && strings.HasPrefix(abs, gitRootCanon+string(filepath.Separator)) {
			return true
		}
		return false
	}

	// 1. Registry-first.
	if entries, err := worktreeregistry.ListAll(); err == nil {
		for _, e := range entries {
			if e == nil || e.AbsPath == "" || filepath.Base(e.AbsPath) != name {
				continue
			}
			// Never resolve a name into the archive: archived entries keep a
			// live AbsPath (under WorktreeArchiveDir) but are not workable.
			if e.IsArchived() {
				continue
			}
			if !ownerAccepted(e.Owner) {
				continue
			}
			if _, statErr := os.Stat(e.AbsPath); statErr == nil {
				return e.AbsPath, true
			}
		}
	}

	// 2. gitRoot's own bases (legacy + XDG).
	if dir, ok := FindWorktreePath(gitRoot, name); ok {
		return dir, true
	}

	// 3. Owner bases (covers anchored worktrees with a missing registry entry).
	for owner := range ownerSet {
		if owner == filepath.Clean(gitRoot) {
			continue // already probed in step 2
		}
		if dir, ok := FindWorktreePath(owner, name); ok {
			return dir, true
		}
	}

	return "", false
}

// ResolveNewWorktreePath returns the target path for a NEW worktree named
// name of the repository rooted at gitRoot.
//
//	useXDG=false → <gitRoot>/.grove-worktrees/<name> (legacy layout)
//	useXDG=true  → paths.WorktreesDir()/<DirIdentifier(gitRoot)>/<name>
//
// The identifier is computed internally from gitRoot — callers don't need a
// WorkspaceNode.
func ResolveNewWorktreePath(gitRoot, name string, useXDG bool) string {
	if useXDG {
		return filepath.Join(paths.WorktreesDir(), DirIdentifier(gitRoot), name)
	}
	return filepath.Join(gitRoot, legacyWorktreeDirName, name)
}

// WorktreeOwner resolves the repository root that owns the worktree at
// worktreePath. It is the layout-independent replacement for the historical
// filepath.Dir(filepath.Dir(worktreePath)) parent inference.
//
// Resolution order:
//
//  1. The worktree's .git FILE: "gitdir: <owner>/.git/worktrees/<name>"
//     (or "<bare>/worktrees/<name>" for bare owners) names the owner
//     directly in any layout.
//  2. The .grove/workspace marker's owner: key (written at creation since
//     Phase 4), so zombie worktrees (deleted .git file) in any layout still
//     resolve.
//  3. Legacy-shaped paths fall back to the historical Dir(Dir()) grandparent
//     inference. An XDG worktree without a gitdir pointer or a marker
//     cannot be resolved (ok=false).
func WorktreeOwner(worktreePath string) (string, bool) {
	if !IsWorktreePath(worktreePath) {
		return "", false
	}
	root := worktreePath
	if r, ok := worktreeRootForPath(worktreePath); ok {
		root = r
	}
	if owner, ok := ownerFromGitdir(root); ok {
		return owner, true
	}
	if entry, err := worktreeregistry.Load(pathutil.WorktreeID(root)); err == nil && entry.Owner != "" {
		return entry.Owner, true
	}
	if owner, ok := ownerFromMarker(root); ok {
		return owner, true
	}
	if hasPathComponent(filepath.Clean(worktreePath), legacyWorktreeDirName) {
		return filepath.Dir(filepath.Dir(worktreePath)), true
	}
	return "", false
}

// ownerFromMarker reads the owner: key from the worktree's .grove/workspace
// marker. The key is additive (written at creation since Phase 4); markers
// from older worktrees simply miss.
func ownerFromMarker(worktreeRoot string) (string, bool) {
	content, err := os.ReadFile(filepath.Join(worktreeRoot, ".grove", "workspace"))
	if err != nil {
		return "", false
	}
	const prefix = "owner:"
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		owner := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		if owner != "" && filepath.IsAbs(owner) {
			return filepath.Clean(owner), true
		}
		return "", false
	}
	return "", false
}

// ownerFromGitdir parses the .git FILE of the worktree rooted at
// worktreeRoot and extracts the owning repository root from its gitdir
// pointer. Returns ok=false when .git is missing, a directory, or does not
// look like a worktree pointer.
func ownerFromGitdir(worktreeRoot string) (string, bool) {
	content, err := os.ReadFile(filepath.Join(worktreeRoot, ".git"))
	if err != nil {
		return "", false
	}
	line := strings.TrimSpace(string(content))
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	const prefix = "gitdir:"
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	gitdir := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if gitdir == "" {
		return "", false
	}
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(worktreeRoot, gitdir)
	}
	gitdir = filepath.Clean(gitdir)

	sep := string(filepath.Separator)
	// Normal owners: <owner>/.git/worktrees/<name>
	if i := strings.LastIndex(gitdir, sep+".git"+sep+"worktrees"+sep); i >= 0 {
		return gitdir[:i], true
	}
	// Bare owners: <bare>/worktrees/<name>
	if i := strings.LastIndex(gitdir, sep+"worktrees"+sep); i >= 0 {
		return gitdir[:i], true
	}
	return "", false
}

// WorktreeRootForPath extracts the worktree root (<base>/<name>) containing
// path, in either layout. The returned root preserves the spelling of the
// input path. Returns ("", false) when path is not inside a grove worktree.
func WorktreeRootForPath(path string) (string, bool) {
	return worktreeRootForPath(path)
}

// worktreeRootForPath extracts the worktree root (<base>/<name>) containing
// path, in either layout. The returned root preserves the spelling of the
// input path.
func worktreeRootForPath(path string) (string, bool) {
	clean := filepath.Clean(path)
	sep := string(filepath.Separator)

	// XDG layout: WorktreesDir()/<identifier>/<name>.
	if wtd := paths.WorktreesDir(); wtd != "" {
		if rel, ok := relUnderAnyForm(wtd, clean); ok {
			relParts := strings.Split(rel, sep)
			if len(relParts) < 2 {
				return "", false
			}
			// Drop everything below <identifier>/<name> from the input
			// spelling.
			cleanParts := strings.Split(clean, sep)
			drop := len(relParts) - 2
			return strings.Join(cleanParts[:len(cleanParts)-drop], sep), true
		}
	}

	// Legacy layout: first .grove-worktrees component plus one component.
	parts := strings.Split(clean, sep)
	for i, part := range parts {
		if part == legacyWorktreeDirName && i+1 < len(parts) && parts[i+1] != "" {
			return strings.Join(parts[:i+2], sep), true
		}
	}
	return "", false
}

// relUnder returns path relative to base when path is strictly inside base.
func relUnder(base, path string) (string, bool) {
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == "." || rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}

// relUnderAnyForm returns path relative to base when path is strictly
// inside base, trying the literal spellings first and falling back to
// normalized forms (symlink-resolved, case-folded) so differently-spelled
// but identical locations agree — lookup paths arrive normalized while the
// base comes raw from the environment. NormalizeForLookup memoizes, so the
// fallback stays cheap in walkers.
func relUnderAnyForm(base, path string) (string, bool) {
	if rel, ok := relUnder(filepath.Clean(base), filepath.Clean(path)); ok {
		return rel, true
	}
	normBase, err1 := pathutil.NormalizeForLookup(base)
	normPath, err2 := pathutil.NormalizeForLookup(path)
	if err1 != nil || err2 != nil {
		return "", false
	}
	return relUnder(normBase, normPath)
}

// hasPathComponent reports whether cleanPath contains name as a full path
// component (NOT a substring — ".grove-worktreesX" does not match).
func hasPathComponent(cleanPath, name string) bool {
	for _, part := range strings.Split(cleanPath, string(filepath.Separator)) {
		if part == name {
			return true
		}
	}
	return false
}
