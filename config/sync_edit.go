package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
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
//  1. Additive merge. Nothing an existing file says is rewritten or removed:
//     comments, formatting and existing entries survive verbatim. New
//     [[workspaces]] entries are appended; ABSENT top-level scalars (server,
//     token_command) are inserted above the first table, because TOML puts
//     bare keys after a table header INSIDE that table and appending them
//     would silently produce `workspaces.server`.
//  2. Rendering is the single choke point. Every entry this package writes
//     goes through renderSyncWorkspaces, which refuses a pull-enabled entry
//     whose role forbids it. There is no second path to the file.
//  3. Re-parse before persisting. Nothing is written that does not parse back
//     into a valid SyncConfig, the pull-enabled set may not grow beyond what
//     the roles allow, and a scalar this edit claims to have filled must read
//     back as that value.
//  4. A DECLARED scalar is never rewritten. A server that disagrees is warned
//     about and left alone; only an ABSENT one is filled. The file belongs to
//     the user, but a file that declares nothing has said nothing to respect —
//     and leaving it that way is what let `grove join` write a subscription
//     onto a config with no server and still report completeness.

// SyncEdit is one create-or-merge of a sync.toml.
type SyncEdit struct {
	// Server is the syncd base URL. It is written when creating the file and
	// FILLED when an existing file declares none. A file that already declares
	// a different server is warned about, never rewritten.
	Server string
	// TokenCommand is the shell command yielding the bearer token. Same rule as
	// Server: written on create, filled when absent, never overwritten.
	TokenCommand string
	// Token is a static bearer token, written only when creating the file and
	// only when TokenCommand is empty. It is deliberately NOT filled into an
	// existing file: a literal secret is the one value this editor will not
	// introduce into a config the user is already maintaining.
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
	// Filled names the top-level scalar keys ("server", "token_command") this
	// edit inserted because the existing file declared none. It is what lets a
	// caller say "filled in the absent server" instead of guessing, and it is
	// empty on the create path (where Created already says everything).
	Filled []string
	// Warnings are non-fatal observations: a server URL that does not match the
	// one the caller expected, or a key declared-but-empty that this edit
	// therefore refused to touch.
	Warnings []string
}

// Changed reports whether the file was written.
func (r SyncEditResult) Changed() bool {
	return r.Created || len(r.Added) > 0 || len(r.Filled) > 0
}

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

// mergeSyncConfig converges an existing sync.toml: it appends the missing
// subscriptions and fills the top-level scalars the file does not declare.
//
// "Fills only ABSENT keys" is the rule `grove machine migrate` already states
// for itself, and applying it here is what makes `grove join` re-runnable —
// and what makes `--repair` fall out of the existing semantics instead of
// being a second code path. Before this, an existing-but-empty sync.toml got a
// subscription, no server, and no warning: a workspace the daemon can never
// replicate, reported as a complete configuration.
func mergeSyncConfig(path, content string, edit SyncEdit) (SyncEditResult, error) {
	res := SyncEditResult{Path: path}

	existing, err := ParseSyncContent(path, content)
	if err != nil {
		return res, fmt.Errorf("existing sync config is not usable — fix it (or move it aside) and re-run: %w", err)
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

	fills, warnings := planSyncScalarFills(path, content, existing, edit)
	res.Warnings = append(res.Warnings, warnings...)

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
	if len(missing) == 0 && len(fills) == 0 {
		return res, nil
	}

	updated := content
	if len(fills) > 0 {
		updated = insertSyncScalars(updated, fills, edit.Note)
	}
	if len(missing) > 0 {
		appended, rErr := renderSyncWorkspaces(missing)
		if rErr != nil {
			return res, rErr
		}
		if !strings.HasSuffix(updated, "\n") {
			updated += "\n"
		}
		if edit.Note != "" {
			updated += "\n# " + edit.Note
		}
		updated += appended
	}

	if err := verifySyncContent(path, updated, append(append([]SyncWorkspace{}, existing.Workspaces...), missing...)); err != nil {
		return res, err
	}
	// The fills are the reason this function exists, so they are checked
	// against the re-parsed result rather than assumed: an inserted key that
	// landed under a table header would decode as a table field and read back
	// empty, which is exactly the silent failure being fixed.
	if err := verifySyncScalarFills(path, updated, fills); err != nil {
		return res, err
	}
	if wErr := os.WriteFile(path, []byte(updated), 0o600); wErr != nil {
		return res, wErr
	}
	for _, ws := range missing {
		res.Added = append(res.Added, ws.Name)
	}
	for _, f := range fills {
		res.Filled = append(res.Filled, f.key)
	}
	return res, nil
}

// syncScalarFill is one absent top-level key this edit will insert.
type syncScalarFill struct {
	key   string
	value string
}

// planSyncScalarFills decides which top-level scalars to fill, and reports
// what it deliberately left alone.
//
// A key is filled only when the caller supplied a value AND the parsed config
// declares none AND the raw text contains no assignment of it. The last check
// is not redundant: `server = ""` parses to the empty string, and inserting a
// second `server` line would produce a duplicate-key TOML error at the verify
// step — a failure whose message would name TOML rather than the config the
// user actually has to fix.
func planSyncScalarFills(path, content string, existing *SyncConfig, edit SyncEdit) ([]syncScalarFill, []string) {
	var (
		fills    []syncScalarFill
		warnings []string
	)
	consider := func(key, want, have string) {
		want = strings.TrimSpace(want)
		if want == "" {
			return
		}
		if key == "server" {
			if w := syncServerMismatch(path, have, want); w != "" {
				warnings = append(warnings, w)
			}
		}
		if strings.TrimSpace(have) != "" {
			return
		}
		if declaresSyncKey(content, key) {
			warnings = append(warnings, fmt.Sprintf("%s declares %s but leaves it empty; leaving it unchanged — set it by hand or remove the line and re-run", path, key))
			return
		}
		fills = append(fills, syncScalarFill{key: key, value: want})
	}
	consider("server", edit.Server, existing.Server)
	// A file that already resolves a token some other way (a literal `token`,
	// or its own token_command) is left alone: filling a second source would
	// change which credential the daemon presents.
	if strings.TrimSpace(existing.Token) == "" {
		consider("token_command", edit.TokenCommand, existing.TokenCommand)
	}
	return fills, warnings
}

// syncTableHeader matches a TOML table or array-of-tables header line. It is
// deliberately strict (no brackets inside) so a multi-line array value is not
// mistaken for the start of a table.
var syncTableHeader = regexp.MustCompile(`^\[\[?[^\[\]]+\]\]?$`)

// insertSyncScalars puts the fills at the last position still ABOVE every
// table header, which for TOML is the only place a bare key means what it
// looks like. Everything else in the file is preserved verbatim.
func insertSyncScalars(content string, fills []syncScalarFill, note string) string {
	block := make([]string, 0, len(fills)+1)
	if strings.TrimSpace(note) != "" {
		block = append(block, "# "+strings.TrimSpace(note)+" (filled in keys the file did not declare)")
	}
	for _, f := range fills {
		block = append(block, fmt.Sprintf("%s = %s", f.key, strconv.Quote(f.value)))
	}

	lines := strings.Split(content, "\n")
	insertAt := len(lines)
	for i, line := range lines {
		if syncTableHeader.MatchString(strings.TrimSpace(line)) {
			insertAt = i
			break
		}
	}
	// Land after the preamble's own trailing blank lines rather than in the
	// middle of them, so the inserted block reads as part of the header.
	for insertAt > 0 && strings.TrimSpace(lines[insertAt-1]) == "" {
		insertAt--
	}

	out := make([]string, 0, len(lines)+len(block)+2)
	out = append(out, lines[:insertAt]...)
	if insertAt > 0 {
		out = append(out, "")
	}
	out = append(out, block...)
	if insertAt < len(lines) {
		out = append(out, "")
	}
	out = append(out, lines[insertAt:]...)
	joined := strings.Join(out, "\n")
	if !strings.HasSuffix(joined, "\n") {
		joined += "\n"
	}
	return joined
}

// declaresSyncKey reports whether the raw text assigns key at the top level,
// ignoring comments. Only the region above the first table header counts: a
// `server = ...` under [[workspaces]] is a different key entirely.
func declaresSyncKey(content, key string) bool {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if syncTableHeader.MatchString(trimmed) {
			return false
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		name, _, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		if strings.Trim(strings.TrimSpace(name), `"'`) == key {
			return true
		}
	}
	return false
}

// verifySyncScalarFills re-parses the generated content and asserts every fill
// reads back as the value that was written. It is the post-condition for the
// insertion point: an inserted key that landed below a table header parses as
// that table's field and this catches it before anything is persisted.
func verifySyncScalarFills(path, content string, fills []syncScalarFill) error {
	if len(fills) == 0 {
		return nil
	}
	parsed, err := ParseSyncContent(path, content)
	if err != nil {
		return fmt.Errorf("refusing to write %s: %w", path, err)
	}
	for _, f := range fills {
		got := ""
		switch f.key {
		case "server":
			got = parsed.Server
		case "token_command":
			got = parsed.TokenCommand
		}
		if strings.TrimSpace(got) != strings.TrimSpace(f.value) {
			return fmt.Errorf("internal: generated %s does not read back %s = %q (got %q); refusing to write",
				path, f.key, f.value, got)
		}
	}
	return nil
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
// DISAGREES with the one the caller expected. The file is never rewritten —
// it is the user's, and silently repointing where their notes go would be the
// worst kind of helpful.
//
// An EMPTY existing server is deliberately not a mismatch and never was; what
// changed is that it is no longer silence either. It is a FILL, handled by
// planSyncScalarFills and reported through SyncEditResult.Filled. This guard
// returning "" for the empty case is what made the whole class invisible: it
// could not fire on the one input that actually needed saying something about.
func syncServerMismatch(path, actual, expected string) string {
	if strings.TrimSpace(actual) == "" || strings.TrimSpace(expected) == "" {
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
