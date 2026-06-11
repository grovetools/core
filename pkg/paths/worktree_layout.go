package paths

// LegacyWorktreeDirName is the in-repo directory that holds legacy grove
// worktrees (<gitRoot>/.grove-worktrees/<name>). The legacy layout is
// supported indefinitely.
//
// The worktree layout helpers (DirIdentifier, ResolveNewWorktreePath,
// WorktreeBases, ...) live in core/pkg/workspace/layout.go. Only this
// constant and WorktreesDir live here: core/git needs them, and richer
// helpers would pull util/pathutil, which imports core/git — an import
// cycle.
const LegacyWorktreeDirName = ".grove-worktrees"
