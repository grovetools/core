package pathutil

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"

	"github.com/grovetools/core/util/sanitize"
)

// WorktreeID returns the canonical identifier for a worktree at absPath using
// the same recipe as workspace.DirIdentifier so both callers agree on the key
// without either package importing the other:
//
//	<sanitized basename>-<sha256(NormalizeForLookup(absPath))[:8]>
func WorktreeID(absPath string) string {
	abs, err := filepath.Abs(absPath)
	if err != nil {
		abs = absPath
	}
	normalized, err := NormalizeForLookup(abs)
	if err != nil {
		normalized = abs
	}
	sum := sha256.Sum256([]byte(normalized))
	return sanitize.SanitizeForTmuxSession(filepath.Base(abs)) + "-" + hex.EncodeToString(sum[:])[:8]
}
