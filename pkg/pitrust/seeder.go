// Package pitrust pre-seeds the pi coding agent's project-trust store so
// agents launched inside a freshly-created grove worktree load the project's
// .pi/ resources (settings, extensions, skills, prompts) without stalling at
// the interactive trust prompt. This matters doubly for headless pi (-p /
// --mode json / --mode rpc): those modes never show the trust prompt and
// silently treat an undecided project as UNTRUSTED, so without pre-seeding a
// headless pi agent simply never loads project-local resources
// (packages/coding-agent/src/core/project-trust.ts in the pi source).
//
// Trust lives in ~/.pi/agent/trust.json as a flat JSON object mapping
// canonicalized absolute paths to booleans
// (packages/coding-agent/src/core/trust-manager.ts: TrustFile =
// Record<string, boolean | null | undefined>; keys are
// canonicalizePath(resolvePath(cwd)) = realpath). Lookup walks UP parent
// directories to the nearest decided ancestor, so trusting a worktree
// container covers every member-repo subdir — callers only need to seed the
// container path.
//
// This package is a leaf sibling of core/pkg/claudetrust and mirrors its
// contract: atomic writes, never clobber an unparseable user-owned file,
// env-gated.
package pitrust

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// trustEnvVar gates seeding. When set to "0", "false", or "off" the seeder is
// a no-op and leaves ~/.pi/agent/trust.json untouched. Default (unset or
// anything else) is ON.
const trustEnvVar = "GROVE_PRESEED_PI_TRUST"

// SeedTrust marks each of paths as trusted (true) in the user's
// ~/.pi/agent/trust.json so pi does not prompt — and, critically, so headless
// pi loads project .pi/ resources at all.
//
// Paths should already be canonicalized by the caller (pathutil.CanonicalPath
// resolves symlinks/case the same way pi's canonicalizePath = realpathSync
// does). SeedTrust does not canonicalize. Because pi's trust lookup inherits
// from the nearest decided ancestor, seeding the worktree container path is
// sufficient; extra subdir paths are harmless but unnecessary.
//
// Behavior:
//   - Gate off (GROVE_PRESEED_PI_TRUST in {0,false,off}) -> no-op, nil.
//   - No paths -> no-op, nil.
//   - ~/.pi/agent absent (pi not installed/initialized on this machine) ->
//     no-op, nil. Grove must not conjure a pi config tree for users who don't
//     run pi.
//   - Missing trust.json -> created.
//   - Malformed JSON -> returns an error WITHOUT touching the file.
//   - Existing entries (including explicit false denials for OTHER paths) are
//     preserved verbatim; a seeded path is forced to true.
//
// The write is atomic (tmp file + rename). The output format matches pi's own
// writeTrustFile (sorted keys, two-space indent, trailing newline) so the file
// stays stable under alternating grove/pi writes. pi guards its own writes
// with a proper-lockfile lock which this seeder does not take; seeding happens
// at worktree creation, before any pi process for that path exists, so the
// race window is effectively empty.
func SeedTrust(paths ...string) error {
	return SeedTrustForConfigDir(".pi", paths...)
}

// SeedTrustForConfigDir seeds one Pi-family runtime without crossing into a
// sibling product's store. configDirName must be a single hidden HOME entry,
// such as .pi or .grove-agent.
func SeedTrustForConfigDir(configDirName string, paths ...string) error {
	if configDirName == "" || filepath.Base(configDirName) != configDirName || !strings.HasPrefix(configDirName, ".") {
		return fmt.Errorf("invalid Pi config directory name %q", configDirName)
	}
	switch os.Getenv(trustEnvVar) {
	case "0", "false", "off":
		return nil
	}
	if len(paths) == 0 {
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("locate home dir: %w", err)
	}
	agentDir := filepath.Join(home, configDirName, "agent")
	if _, statErr := os.Stat(agentDir); os.IsNotExist(statErr) {
		// pi isn't set up on this machine; do not create its config tree.
		return nil
	}
	trustPath := filepath.Join(agentDir, "trust.json")

	trust := map[string]any{}
	data, readErr := os.ReadFile(trustPath)
	switch {
	case readErr == nil:
		if uerr := json.Unmarshal(data, &trust); uerr != nil {
			// Never overwrite a file we can't parse.
			return fmt.Errorf("parse %s: %w", trustPath, uerr)
		}
	case os.IsNotExist(readErr):
		// Fresh file.
	default:
		return fmt.Errorf("read %s: %w", trustPath, readErr)
	}

	for _, p := range paths {
		if p == "" {
			continue
		}
		trust[p] = true
	}

	// json.MarshalIndent on a map sorts keys, matching pi's sorted output.
	out, err := json.MarshalIndent(trust, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", trustPath, err)
	}
	out = append(out, '\n') // pi's writeTrustFile ends with a newline

	tmpPath := trustPath + ".tmp"
	if err := os.WriteFile(tmpPath, out, 0o600); err != nil {
		return fmt.Errorf("write tmp %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, trustPath); err != nil {
		_ = os.Remove(tmpPath) // best-effort cleanup of orphaned tmp
		return fmt.Errorf("rename %s -> %s: %w", tmpPath, trustPath, err)
	}
	return nil
}

// IsPermissionDenied reports whether err is the OS-sandbox rejection that
// SeedTrust returns when ~/.pi/agent/trust.json (outside the sandbox's
// writable boundary) cannot be written. Mirrors claudetrust.IsPermissionDenied.
func IsPermissionDenied(err error) bool {
	if err == nil {
		return false
	}
	return os.IsPermission(err) || strings.Contains(err.Error(), "operation not permitted")
}
