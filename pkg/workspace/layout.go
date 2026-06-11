// layout.go centralizes worktree directory layout knowledge for the grove
// ecosystem. These helpers are the single source of truth for where grove
// worktrees live and how a worktree maps back to its owning repository.
//
// Phase 1 semantics are LEGACY-ONLY: every helper resolves exclusively
// against the in-repo <gitRoot>/.grove-worktrees layout, byte-identical to
// the inline logic it replaced. The XDG layout
// (paths.WorktreesDir()/<DirIdentifier(gitRoot)>/<name>) is wired up in a
// later phase.
//
// core/git cannot import this package (import cycle via util/pathutil); it
// uses paths.LegacyWorktreeDirName directly for its single legacy join.
package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/util/pathutil"
	"github.com/grovetools/core/util/sanitize"
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
	abs, err := filepath.Abs(gitRoot)
	if err != nil {
		abs = gitRoot
	}
	normalized, err := pathutil.NormalizeForLookup(abs)
	if err != nil {
		normalized = abs
	}
	sum := sha256.Sum256([]byte(normalized))
	return sanitize.SanitizeForTmuxSession(filepath.Base(abs)) + "-" + hex.EncodeToString(sum[:])[:8]
}

// WorktreeBases returns the ordered, legacy-first list of identifier-level
// directories that can contain worktrees of the repository rooted at
// gitRoot.
//
// Phase 1: returns only the legacy element [<gitRoot>/.grove-worktrees].
// Later phases append paths.WorktreesDir()/<DirIdentifier(gitRoot)>.
func WorktreeBases(gitRoot string) []string {
	return []string{filepath.Join(gitRoot, legacyWorktreeDirName)}
}

// IsWorktreePath reports whether path refers to (or is inside) a grove
// worktree location.
//
// Phase 1: byte-identical to strings.Contains(path, ".grove-worktrees").
// Later phases anchor the check to path components and add the XDG layout
// (with a DataDir()/cx carve-out).
func IsWorktreePath(path string) bool {
	return strings.Contains(path, legacyWorktreeDirName)
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
// Phase 1 (legacy-only): returns exactly what Dir(Dir(worktreePath)) returns
// today for paths inside a worktree base; ok is false for non-worktree
// paths. Later phases resolve the owner from the worktree's .git file
// (gitdir pointer) with the .grove/workspace marker as fallback, which also
// covers the XDG layout.
func WorktreeOwner(worktreePath string) (string, bool) {
	if !IsWorktreePath(worktreePath) {
		return "", false
	}
	return filepath.Dir(filepath.Dir(worktreePath)), true
}

// worktreeRootForPath extracts the worktree root (<base>/<name>) containing
// path. Phase 1 (legacy-only): locates the first .grove-worktrees path
// component and takes one component after it.
func worktreeRootForPath(path string) (string, bool) {
	sep := string(filepath.Separator)
	parts := strings.Split(filepath.Clean(path), sep)
	for i, part := range parts {
		if part == legacyWorktreeDirName && i+1 < len(parts) && parts[i+1] != "" {
			return strings.Join(parts[:i+2], sep), true
		}
	}
	return "", false
}
