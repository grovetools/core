package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolveConfigWriteTarget reviews a config path before a writer mutates it and
// reports the regular file the write must actually land on.
//
// Canonical Grove config paths are routinely symlinks into a machine-specific
// dotfiles checkout — that is the intended future ownership of roots.toml,
// notebooks.toml, machine.toml and sync.toml. A writer that renames a fresh
// temp file over the canonical path replaces the link with a plain file, so the
// machine silently detaches from its dotfiles and the next dotfiles sync
// reverts, duplicates, or conflicts with the edit. The contract here is the
// opposite: the link itself is preserved and its resolved regular-file target
// is what gets rewritten.
//
// Reviewability is the limit. A link is only safe to write through when the
// whole chain resolves to an existing regular file, because that is the only
// state a caller can snapshot and restore. Dangling links, cycles, and
// non-regular targets are refused loudly rather than followed, replaced, or
// silently recreated at the logical path — recreating is exactly the data loss
// this contract exists to prevent.
//
// A missing path and a plain regular file are both written in place, unchanged
// from the pre-contract behaviour.
func ResolveConfigWriteTarget(path string) (target string, viaSymlink bool, err error) {
	if path == "" {
		return "", false, fmt.Errorf("config write path is not resolvable")
	}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return path, false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("review config write path %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		if !info.Mode().IsRegular() {
			return "", false, fmt.Errorf("refusing to write %s: expected a regular file or symlink, found %s", path, info.Mode().Type())
		}
		return path, false, nil
	}

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		link, readErr := os.Readlink(path)
		if readErr != nil {
			link = "<unreadable>"
		}
		return "", true, fmt.Errorf("refusing to write through unreviewable config symlink %s -> %s (dangling or cyclic target): %w", path, link, err)
	}
	if abs, absErr := filepath.Abs(resolved); absErr == nil {
		resolved = abs
	}
	resolved = filepath.Clean(resolved)
	targetInfo, err := os.Stat(resolved)
	if err != nil {
		return "", true, fmt.Errorf("review config symlink target %s -> %s: %w", path, resolved, err)
	}
	if !targetInfo.Mode().IsRegular() {
		return "", true, fmt.Errorf("refusing to write through config symlink %s -> %s: target is not a regular file", path, resolved)
	}
	return resolved, true, nil
}

// reviewConfigWritePath refuses an unwritable destination before a writer does
// any reading or editing. Without it a cyclic or dangling link surfaces as a
// raw ELOOP/ENOENT read error, which reads like a corrupt file rather than the
// symlink problem it is.
func reviewConfigWritePath(path string) error {
	_, _, err := ResolveConfigWriteTarget(path)
	return err
}

// writeTargetLabel names a write destination in an error the way an operator
// reads it: the path they asked for, plus the dotfiles file it stands for.
func writeTargetLabel(path, target string, viaSymlink bool) string {
	if !viaSymlink {
		return path
	}
	return path + " -> " + target
}
