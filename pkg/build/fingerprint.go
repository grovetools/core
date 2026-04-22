// Package build provides content-fingerprinting of container build contexts
// so callers (grove's env providers) can decide whether a rebuild is needed.
//
// The fingerprint is a SHA-256 over a deterministic stream:
//
//	"file\x00" relpath "\x00" mode "\x00" <content bytes>
//	"dockerfile\x00" <dockerfile bytes>
//
// Paths are walked in sorted order, .dockerignore is honored via moby's
// reference patternmatcher, and symlinks that escape the context directory
// cause the whole operation to fail (fail-closed — see chat turn 2).
package build

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/moby/patternmatcher"
)

// HashContext produces a deterministic SHA-256 fingerprint of the docker build
// context rooted at contextDir. The Dockerfile at dockerfilePath is always
// folded in, even when it lives outside contextDir.
//
// Both paths must be absolute. contextDir must exist. A missing .dockerignore
// is treated as "no exclusions". Errors from symlinks that escape contextDir
// or from .dockerignore parse failures are surfaced to the caller; the caller
// is expected to fail the operation.
func HashContext(contextDir, dockerfilePath string) (string, error) {
	absCtx, err := filepath.Abs(contextDir)
	if err != nil {
		return "", fmt.Errorf("resolve context dir: %w", err)
	}
	info, err := os.Stat(absCtx)
	if err != nil {
		return "", fmt.Errorf("stat context dir %s: %w", absCtx, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("context %s is not a directory", absCtx)
	}

	patterns, err := readDockerignore(absCtx)
	if err != nil {
		return "", fmt.Errorf("read .dockerignore: %w", err)
	}
	pm, err := patternmatcher.New(patterns)
	if err != nil {
		return "", fmt.Errorf("compile .dockerignore patterns: %w", err)
	}

	var entries []walkEntry
	err = filepath.Walk(absCtx, func(path string, fi os.FileInfo, werr error) error {
		if werr != nil {
			return werr
		}
		rel, rerr := filepath.Rel(absCtx, path)
		if rerr != nil {
			return rerr
		}
		if rel == "." {
			return nil
		}
		// Normalize to forward slashes for stable hashing + matcher input.
		relSlash := filepath.ToSlash(rel)

		ignored, mErr := pm.MatchesOrParentMatches(relSlash)
		if mErr != nil {
			return fmt.Errorf("match %s: %w", relSlash, mErr)
		}
		if ignored {
			if fi.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if fi.Mode()&os.ModeSymlink != 0 {
			target, lerr := filepath.EvalSymlinks(path)
			if lerr != nil {
				return fmt.Errorf("resolve symlink %s: %w", relSlash, lerr)
			}
			if !strings.HasPrefix(target+string(filepath.Separator), absCtx+string(filepath.Separator)) && target != absCtx {
				return fmt.Errorf("symlink %s escapes build context", relSlash)
			}
			return nil
		}

		if fi.IsDir() {
			return nil
		}
		if !fi.Mode().IsRegular() {
			return nil
		}

		entries = append(entries, walkEntry{rel: relSlash, mode: fi.Mode(), path: path})
		return nil
	})
	if err != nil {
		return "", err
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].rel < entries[j].rel })

	h := sha256.New()
	for _, e := range entries {
		fmt.Fprintf(h, "file\x00%s\x00%o\x00", e.rel, e.mode.Perm())
		f, err := os.Open(e.path)
		if err != nil {
			return "", fmt.Errorf("open %s: %w", e.rel, err)
		}
		if _, err := io.Copy(h, f); err != nil {
			f.Close()
			return "", fmt.Errorf("read %s: %w", e.rel, err)
		}
		f.Close()
		h.Write([]byte{0})
	}

	if dockerfilePath != "" {
		dfAbs, err := filepath.Abs(dockerfilePath)
		if err != nil {
			return "", fmt.Errorf("resolve dockerfile path: %w", err)
		}
		dfBytes, err := os.ReadFile(dfAbs)
		if err != nil {
			return "", fmt.Errorf("read dockerfile %s: %w", dfAbs, err)
		}
		// Only fold dockerfile bytes separately when it's outside the context;
		// if it's inside, it was already streamed via the walk above.
		inside := strings.HasPrefix(dfAbs+string(filepath.Separator), absCtx+string(filepath.Separator))
		if !inside {
			fmt.Fprint(h, "dockerfile\x00")
			h.Write(dfBytes)
			h.Write([]byte{0})
		}
	}

	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

type walkEntry struct {
	rel  string
	mode os.FileMode
	path string
}

func readDockerignore(contextDir string) ([]string, error) {
	f, err := os.Open(filepath.Join(contextDir, ".dockerignore"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var patterns []string
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return patterns, nil
}
