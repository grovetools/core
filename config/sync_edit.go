package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// The shared role-aware sync.toml editor.
//
// It was extracted from `grove satellite up`'s createLaptopSyncConfig /
// mergeLaptopSyncConfig, which were the only writers of a sync.toml and were
// written under a VM threat model: reject `pull = true`, unconditionally.
// That guarantee is preserved exactly — for satellite and legacy entries — but
// the refusal is now scoped to the RELATIONSHIP (SyncWorkspace.Role) so `join`
// and `materialize` can write the pull-enabled entries they legitimately need
// (a machine mirroring its own notebook; the machine registry).
//
// Four properties are load-bearing and shared by every caller:
//
//  1. Append-only merge. An existing file's bytes remain a byte-for-byte
//     PREFIX of the result: comments, formatting and existing entries are
//     never rewritten, only new entries appended.
//  2. Rendering is the single choke point. Every entry this package writes
//     goes through renderSyncWorkspaces, which refuses a pull-enabled entry
//     whose role forbids it. There is no second path to the file.
//  3. Re-parse before persisting. Nothing is written that does not parse back
//     into a valid SyncConfig, and the pull-enabled set may not grow beyond
//     what the roles allow.
//  4. The server line is never rewritten. A mismatch is warned about; the file
//     belongs to the user.

// SyncEdit is one create-or-merge of a sync.toml.
type SyncEdit struct {
	// Server is the syncd base URL, written only when creating the file. On an
	// existing file a disagreeing server is warned about, never rewritten.
	Server string
	// TokenCommand is the shell command yielding the bearer token, written
	// only when creating the file.
	TokenCommand string
	// Token is a static bearer token, written only when creating the file and
	// only when TokenCommand is empty.
	Token string
	// Workspaces are the subscriptions to ensure exist. An entry whose name is
	// already present is left exactly as it is — the file is the user's.
	Workspaces []SyncWorkspace
	// Header is the comment block for a freshly created file (lines include
	// their own `#`).
	Header []string
	// Note labels the appended block in an existing file, e.g.
	// "Added by `grove satellite up`".
	Note string
}

// SyncEditResult reports what the edit did, so callers can print an accurate
// summary instead of guessing.
type SyncEditResult struct {
	Path    string
	Created bool
	// Added names the workspaces appended by this edit, in the order written.
	Added []string
	// Present names the workspaces that already existed and were left alone.
	Present []string
	// Warnings are non-fatal observations (currently: a server URL that does
	// not match the one the caller expected).
	Warnings []string
}

// Changed reports whether the file was written.
func (r SyncEditResult) Changed() bool { return r.Created || len(r.Added) > 0 }

// ApplySyncEdit creates or append-merges the sync config at path.
func ApplySyncEdit(path string, edit SyncEdit) (SyncEditResult, error) {
	res := SyncEditResult{Path: path}
	if path == "" {
		return res, fmt.Errorf("sync config path is not resolvable")
	}
	for _, ws := range edit.Workspaces {
		if err := validateSyncWorkspaceRole(ws); err != nil {
			return res, err
		}
	}

	existing, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return res, fmt.Errorf("failed to read sync config %s: %w", path, err)
		}
		return createSyncConfig(path, edit)
	}
	return mergeSyncConfig(path, string(existing), edit)
}

// createSyncConfig writes a fresh sync.toml.
func createSyncConfig(path string, edit SyncEdit) (SyncEditResult, error) {
	res := SyncEditResult{Path: path, Created: true}

	entries, err := renderSyncWorkspaces(edit.Workspaces)
	if err != nil {
		return res, err
	}

	var b strings.Builder
	for _, line := range edit.Header {
		b.WriteString(line)
		b.WriteString("\n")
	}
	if edit.Server != "" {
		fmt.Fprintf(&b, "server = %s\n", strconv.Quote(edit.Server))
	}
	switch {
	case edit.TokenCommand != "":
		fmt.Fprintf(&b, "token_command = %s\n", strconv.Quote(edit.TokenCommand))
	case edit.Token != "":
		fmt.Fprintf(&b, "token = %s\n", strconv.Quote(edit.Token))
	}
	b.WriteString(entries)
	content := b.String()

	if err := verifySyncContent(path, content, edit.Workspaces); err != nil {
		return res, err
	}
	if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr != nil {
		return res, fmt.Errorf("failed to create config directory for %s: %w", path, mkErr)
	}
	if wErr := os.WriteFile(path, []byte(content), 0o600); wErr != nil {
		return res, wErr
	}
	for _, ws := range edit.Workspaces {
		res.Added = append(res.Added, ws.Name)
	}
	return res, nil
}

// mergeSyncConfig appends the missing subscriptions to an existing sync.toml.
// The previous content is kept byte-for-byte as a prefix.
func mergeSyncConfig(path, content string, edit SyncEdit) (SyncEditResult, error) {
	res := SyncEditResult{Path: path}

	existing, err := ParseSyncContent(path, content)
	if err != nil {
		return res, fmt.Errorf("existing sync config is not usable — fix it (or move it aside) and re-run: %w", err)
	}
	if warning := syncServerMismatch(path, existing.Server, edit.Server); warning != "" {
		res.Warnings = append(res.Warnings, warning)
	}

	// An existing pull-enabled entry whose role forbids pulling means the file
	// is not in a state this editor may manage. Refusing is the whole point:
	// preserving it would let a guest's writes bypass review.
	for _, ws := range existing.Workspaces {
		if ws.Pull && RolePushOnly(ws.Role) {
			return res, fmt.Errorf("refusing to edit %s: existing workspace %q has pull = true with role %q, which is push-only; remove or re-role that entry first",
				path, ws.Name, ws.Role)
		}
	}

	have := make(map[string]bool, len(existing.Workspaces))
	for _, ws := range existing.Workspaces {
		have[ws.Name] = true
	}
	var missing []SyncWorkspace
	for _, ws := range edit.Workspaces {
		if have[ws.Name] {
			res.Present = append(res.Present, ws.Name)
			continue
		}
		missing = append(missing, ws)
		have[ws.Name] = true
	}
	if len(missing) == 0 {
		return res, nil
	}

	appended, err := renderSyncWorkspaces(missing)
	if err != nil {
		return res, err
	}
	updated := content
	if !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	if edit.Note != "" {
		updated += "\n# " + edit.Note
	}
	updated += appended

	if err := verifySyncContent(path, updated, append(append([]SyncWorkspace{}, existing.Workspaces...), missing...)); err != nil {
		return res, err
	}
	if wErr := os.WriteFile(path, []byte(updated), 0o600); wErr != nil {
		return res, wErr
	}
	for _, ws := range missing {
		res.Added = append(res.Added, ws.Name)
	}
	return res, nil
}

// RenderSyncWorkspaces renders [[workspaces]] entries through the same choke
// point ApplySyncEdit uses, for callers that need the text rather than a file
// write (the satellite bootstrap templates the VM's own sync.toml).
func RenderSyncWorkspaces(entries []SyncWorkspace) (string, error) {
	return renderSyncWorkspaces(entries)
}

// renderSyncWorkspaces renders [[workspaces]] entries. HARD INVARIANT: it
// refuses to emit a pull-enabled entry whose role is push-only (satellite, or
// legacy/role-less). Every entry this package writes flows through here, on
// both the create and merge paths, which makes this the single enforcement
// point for the push-only property.
func renderSyncWorkspaces(entries []SyncWorkspace) (string, error) {
	var b strings.Builder
	for _, e := range entries {
		if err := validateSyncWorkspaceRole(e); err != nil {
			return "", err
		}
		b.WriteString("\n[[workspaces]]\n")
		fmt.Fprintf(&b, "name = %s\n", strconv.Quote(e.Name))
		if e.Role != "" {
			fmt.Fprintf(&b, "role = %s\n", strconv.Quote(e.Role))
		}
		if e.Mode != "" {
			fmt.Fprintf(&b, "mode = %s\n", strconv.Quote(e.Mode))
		}
		if e.Pull {
			b.WriteString("pull = true\n")
		}
		if e.MaxFileSize > 0 {
			fmt.Fprintf(&b, "max_file_size = %d\n", e.MaxFileSize)
		}
		for _, ex := range e.Excludes {
			fmt.Fprintf(&b, "excludes = [%s]\n", strconv.Quote(ex))
		}
	}
	return b.String(), nil
}

// validateSyncWorkspaceRole enforces the role vocabulary and the push-only
// rule for one entry.
func validateSyncWorkspaceRole(ws SyncWorkspace) error {
	if strings.TrimSpace(ws.Name) == "" {
		return fmt.Errorf("refusing to write a sync workspace with an empty name")
	}
	switch ws.Role {
	case "", SyncRoleSatellite, SyncRolePeer, SyncRoleRegistry:
	default:
		return fmt.Errorf("sync workspace %q has invalid role %q (expected %s, %s, or %s)",
			ws.Name, ws.Role, SyncRoleSatellite, SyncRolePeer, SyncRoleRegistry)
	}
	if ws.Pull && RolePushOnly(ws.Role) {
		if ws.Role == SyncRoleSatellite {
			return fmt.Errorf("refusing to write sync workspace %q with pull = true: a satellite-role entry is PUSH-ONLY (pulling would let a disposable VM overwrite local notebooks); pull belongs in the VM's own sync.toml", ws.Name)
		}
		return fmt.Errorf("refusing to write sync workspace %q with pull = true: an entry with no role is legacy and therefore push-only; declare role = %q or role = %q to pull",
			ws.Name, SyncRolePeer, SyncRoleRegistry)
	}
	return nil
}

// verifySyncContent is the defense-in-depth pass before persisting: the result
// must parse as a sync config, and no workspace may be pull-enabled under a
// push-only role. `want` is the set the caller believes it is writing; a
// workspace in the file but not in `want` is a decode surprise and fails.
func verifySyncContent(path, content string, want []SyncWorkspace) error {
	parsed, err := ParseSyncContent(path, content)
	if err != nil {
		return fmt.Errorf("refusing to write %s: %w", path, err)
	}
	expected := make(map[string]SyncWorkspace, len(want))
	for _, ws := range want {
		expected[ws.Name] = ws
	}
	for _, ws := range parsed.Workspaces {
		if ws.Pull && RolePushOnly(ws.Role) {
			return fmt.Errorf("internal: generated %s would contain a pull-enabled %q workspace %q under a push-only role; refusing to write",
				path, ws.Role, ws.Name)
		}
		if _, ok := expected[ws.Name]; !ok {
			return fmt.Errorf("internal: generated %s contains an unexpected workspace %q; refusing to write", path, ws.Name)
		}
	}
	return nil
}

// ParseSyncContent decodes sync.toml content into the canonical SyncConfig
// schema — the exact shape LoadSyncConfigFrom reads, so what validates here is
// what the daemon will load.
func ParseSyncContent(path, content string) (*SyncConfig, error) {
	var cfg SyncConfig
	if err := toml.Unmarshal([]byte(content), &cfg); err != nil {
		return nil, fmt.Errorf("%s does not parse as TOML: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("%s is not a valid sync config: %w", path, err)
	}
	return &cfg, nil
}

// syncServerMismatch returns a warning when an existing sync.toml's server
// disagrees with the one the caller expected. The file is never rewritten —
// it is the user's, and silently repointing where their notes go would be the
// worst kind of helpful.
func syncServerMismatch(path, actual, expected string) string {
	if actual == "" || expected == "" {
		return ""
	}
	if strings.TrimRight(actual, "/") == strings.TrimRight(expected, "/") {
		return ""
	}
	// Same endpoint spelled differently (localhost vs 127.0.0.1) is not a
	// mismatch worth reporting.
	au, aerr := url.Parse(actual)
	eu, eerr := url.Parse(expected)
	if aerr == nil && eerr == nil && au.Port() != "" && au.Port() == eu.Port() && isLoopbackHost(au.Hostname()) && isLoopbackHost(eu.Hostname()) {
		return ""
	}
	return fmt.Sprintf("%s server = %q does not match the expected %s; leaving it unchanged — align the flag or edit the file manually", path, actual, expected)
}

func isLoopbackHost(host string) bool {
	switch host {
	case "127.0.0.1", "localhost", "::1":
		return true
	}
	return false
}
