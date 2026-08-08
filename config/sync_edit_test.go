package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func syncPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "sync.toml")
}

// The legacy guarantee, unchanged: an entry with no role is push-only, and a
// pull-enabled one is refused at the single rendering choke point.
func TestSyncEditorRefusesPullForLegacyAndSatelliteRoles(t *testing.T) {
	for _, role := range []string{"", SyncRoleSatellite} {
		_, err := ApplySyncEdit(syncPath(t), SyncEdit{
			Server:     "http://127.0.0.1:8788",
			Workspaces: []SyncWorkspace{{Name: "cloud", Role: role, Pull: true}},
		})
		if err == nil {
			t.Fatalf("role %q: editor accepted pull = true; the push-only guarantee is broken", role)
		}
		if !strings.Contains(err.Error(), "pull = true") {
			t.Fatalf("role %q: refusal does not name the offending key: %v", role, err)
		}
	}
}

// Roles that describe a relationship with this operator's own machines may
// pull — that is the entire point of the role model.
func TestSyncEditorAllowsPullForPeerAndRegistryRoles(t *testing.T) {
	for _, role := range []string{SyncRolePeer, SyncRoleRegistry} {
		path := syncPath(t)
		res, err := ApplySyncEdit(path, SyncEdit{
			Server:     "https://sync.example.com",
			Workspaces: []SyncWorkspace{{Name: "nb", Role: role, Pull: true}},
		})
		if err != nil {
			t.Fatalf("role %q: %v", role, err)
		}
		if !res.Created || len(res.Added) != 1 {
			t.Fatalf("role %q: result = %+v", role, res)
		}
		content := readSync(t, path)
		for _, want := range []string{`name = "nb"`, `role = "` + role + `"`, "pull = true", `server = "https://sync.example.com"`} {
			if !strings.Contains(content, want) {
				t.Fatalf("role %q: written file missing %q:\n%s", role, want, content)
			}
		}
		// And it round-trips through the canonical loader.
		cfg, err := LoadSyncConfigFrom(path)
		if err != nil {
			t.Fatalf("role %q: LoadSyncConfigFrom: %v", role, err)
		}
		if len(cfg.Workspaces) != 1 || cfg.Workspaces[0].Role != role || !cfg.Workspaces[0].Pull {
			t.Fatalf("role %q: round-tripped as %+v", role, cfg.Workspaces)
		}
	}
}

// Merging is append-only: the previous bytes stay a byte-for-byte PREFIX, so
// comments, formatting and existing entries survive verbatim.
func TestSyncEditorMergeKeepsPreviousBytesAsAPrefix(t *testing.T) {
	path := syncPath(t)
	original := `# hand written, keep me
server = "http://127.0.0.1:8788"

[[workspaces]]
name = "cloud"
role = "satellite"
`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	res, err := ApplySyncEdit(path, SyncEdit{
		Server: "http://127.0.0.1:8788",
		Workspaces: []SyncWorkspace{
			{Name: "cloud", Role: SyncRoleSatellite},      // already present
			{Name: "grovetools", Role: SyncRoleSatellite}, // new
		},
		Note: "Added by a test",
	})
	if err != nil {
		t.Fatalf("ApplySyncEdit: %v", err)
	}
	if len(res.Added) != 1 || res.Added[0] != "grovetools" {
		t.Fatalf("added = %v, want [grovetools]", res.Added)
	}
	if len(res.Present) != 1 || res.Present[0] != "cloud" {
		t.Fatalf("present = %v, want [cloud]", res.Present)
	}

	content := readSync(t, path)
	if !strings.HasPrefix(content, original) {
		t.Fatalf("merge did not keep the previous bytes as a prefix:\n%s", content)
	}
	if strings.Count(content, `name = "cloud"`) != 1 {
		t.Fatalf("merge duplicated an existing entry:\n%s", content)
	}
	if !strings.Contains(content, "# Added by a test") {
		t.Fatalf("merge did not label the appended block:\n%s", content)
	}
}

// A no-op merge writes nothing at all.
func TestSyncEditorMergeIsIdempotent(t *testing.T) {
	path := syncPath(t)
	edit := SyncEdit{
		Server:     "http://127.0.0.1:8788",
		Workspaces: []SyncWorkspace{{Name: "cloud", Role: SyncRoleSatellite}},
	}
	if _, err := ApplySyncEdit(path, edit); err != nil {
		t.Fatalf("create: %v", err)
	}
	before := readSync(t, path)

	res, err := ApplySyncEdit(path, edit)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if res.Changed() {
		t.Fatalf("second edit reported a change: %+v", res)
	}
	if after := readSync(t, path); after != before {
		t.Fatalf("second edit rewrote the file:\n--- before ---\n%s\n--- after ---\n%s", before, after)
	}
}

// An existing pull-enabled entry under a push-only role blocks the edit — the
// file is not in a state this editor may manage. A peer-role pull entry does
// NOT block it: pulling one's own notebook is legitimate.
func TestSyncEditorMergeRefusesOnlyPushOnlyPullEntries(t *testing.T) {
	blocked := syncPath(t)
	if err := os.WriteFile(blocked, []byte("[[workspaces]]\nname = \"cloud\"\npull = true\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := ApplySyncEdit(blocked, SyncEdit{Workspaces: []SyncWorkspace{{Name: "new", Role: SyncRoleSatellite}}}); err == nil {
		t.Fatal("editor accepted a file with a legacy pull-enabled entry")
	}

	allowed := syncPath(t)
	if err := os.WriteFile(allowed, []byte("[[workspaces]]\nname = \"nb\"\nrole = \"peer\"\npull = true\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	res, err := ApplySyncEdit(allowed, SyncEdit{Workspaces: []SyncWorkspace{{Name: "cloud", Role: SyncRoleSatellite}}})
	if err != nil {
		t.Fatalf("a peer-role pull entry must not block a satellite edit: %v", err)
	}
	if len(res.Added) != 1 {
		t.Fatalf("result = %+v", res)
	}
}

// The server line belongs to the user: a mismatch is reported, never rewritten.
func TestSyncEditorWarnsOnServerMismatchWithoutRewriting(t *testing.T) {
	path := syncPath(t)
	original := "server = \"https://elsewhere.example.com\"\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	res, err := ApplySyncEdit(path, SyncEdit{
		Server:     "http://127.0.0.1:8788",
		Workspaces: []SyncWorkspace{{Name: "cloud", Role: SyncRoleSatellite}},
	})
	if err != nil {
		t.Fatalf("ApplySyncEdit: %v", err)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "elsewhere.example.com") {
		t.Fatalf("warnings = %v, want a server mismatch notice", res.Warnings)
	}
	if content := readSync(t, path); !strings.HasPrefix(content, original) {
		t.Fatalf("the server line was rewritten:\n%s", content)
	}

	// localhost and 127.0.0.1 on the same port are the same endpoint.
	same := syncPath(t)
	if err := os.WriteFile(same, []byte("server = \"http://localhost:8788\"\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	res, err = ApplySyncEdit(same, SyncEdit{
		Server:     "http://127.0.0.1:8788",
		Workspaces: []SyncWorkspace{{Name: "cloud", Role: SyncRoleSatellite}},
	})
	if err != nil {
		t.Fatalf("ApplySyncEdit: %v", err)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("warned about an equivalent loopback spelling: %v", res.Warnings)
	}
}

func TestSyncConfigValidateRejectsUnknownRole(t *testing.T) {
	cfg := SyncConfig{Workspaces: []SyncWorkspace{{Name: "cloud", Role: "hub"}}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate accepted an unknown role")
	}
	// Empty (legacy) stays valid — every existing sync.toml has it.
	ok := SyncConfig{Workspaces: []SyncWorkspace{{Name: "cloud"}}}
	if err := ok.Validate(); err != nil {
		t.Fatalf("Validate rejected a legacy role-less entry: %v", err)
	}
}

func TestRolePushOnly(t *testing.T) {
	for role, want := range map[string]bool{
		"":                true, // legacy
		SyncRoleSatellite: true,
		SyncRolePeer:      false,
		SyncRoleRegistry:  false,
	} {
		if got := RolePushOnly(role); got != want {
			t.Errorf("RolePushOnly(%q) = %v, want %v", role, got, want)
		}
	}
}

// THE JOB 52 REGRESSION. Job 22 deliberately left this laptop a sync.toml with
// every key commented out; joining against it wrote a registry subscription,
// declared no server, warned about nothing, and reported "the configuration
// above is complete". The merge path must fill what the file does not declare.
func TestSyncEditorFillsAbsentScalarsOnAFullyCommentedFile(t *testing.T) {
	path := syncPath(t)
	original := `# Notebook sync client config.
# server = "https://sync.example.com"
# token_command = "security find-generic-password -s grove-sync -w"
`
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	res, err := ApplySyncEdit(path, SyncEdit{
		Server:       "https://forge.example.com:8788",
		TokenCommand: "security find-generic-password -s grove-sync -a solair -w",
		Workspaces:   []SyncWorkspace{{Name: "registry", Role: SyncRoleRegistry, Pull: true}},
		Note:         "Added by `grove join`",
	})
	if err != nil {
		t.Fatalf("ApplySyncEdit: %v", err)
	}
	if len(res.Filled) != 2 || res.Filled[0] != "server" || res.Filled[1] != "token_command" {
		t.Fatalf("filled = %v, want [server token_command]", res.Filled)
	}
	if !res.Changed() {
		t.Fatal("a fill did not count as a change")
	}

	// The post-condition that matters is what the DAEMON will read back.
	cfg, err := LoadSyncConfigFrom(path)
	if err != nil || cfg == nil {
		t.Fatalf("re-parse: %v (%v)", cfg, err)
	}
	if cfg.Server != "https://forge.example.com:8788" {
		t.Errorf("server = %q, want the filled value", cfg.Server)
	}
	if !strings.Contains(cfg.TokenCommand, "-a solair") {
		t.Errorf("token_command = %q, want the account-pinned command", cfg.TokenCommand)
	}
	if len(cfg.Workspaces) != 1 || cfg.Workspaces[0].Role != SyncRoleRegistry {
		t.Errorf("workspaces = %+v, want one registry entry", cfg.Workspaces)
	}
	if content := readSync(t, path); !strings.HasPrefix(content, original) {
		t.Errorf("the commented preamble was not preserved verbatim:\n%s", content)
	}
}

// The insertion point is the whole trick: appended after a [[workspaces]]
// header, `server = ...` is TOML for `workspaces.server` and reads back empty.
func TestSyncEditorFillsAboveExistingTables(t *testing.T) {
	path := syncPath(t)
	original := "[[workspaces]]\nname = \"cloud\"\nrole = \"satellite\"\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := ApplySyncEdit(path, SyncEdit{
		Server:     "https://forge.example.com:8788",
		Workspaces: []SyncWorkspace{{Name: "cloud", Role: SyncRoleSatellite}},
	}); err != nil {
		t.Fatalf("ApplySyncEdit: %v", err)
	}
	cfg, err := LoadSyncConfigFrom(path)
	if err != nil || cfg == nil {
		t.Fatalf("re-parse: %v (%v)", cfg, err)
	}
	if cfg.Server != "https://forge.example.com:8788" {
		t.Fatalf("server = %q — the fill landed inside a table:\n%s", cfg.Server, readSync(t, path))
	}
	if len(cfg.Workspaces) != 1 || cfg.Workspaces[0].Name != "cloud" {
		t.Fatalf("the existing entry did not survive: %+v", cfg.Workspaces)
	}
}

// A DECLARED scalar is still never rewritten — that half of the contract is
// what the file-belongs-to-the-user rule protects.
func TestSyncEditorNeverOverwritesADeclaredScalar(t *testing.T) {
	path := syncPath(t)
	original := "server = \"https://elsewhere.example.com\"\ntoken_command = \"op read op://grove/sync\"\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	res, err := ApplySyncEdit(path, SyncEdit{
		Server:       "https://forge.example.com:8788",
		TokenCommand: "security find-generic-password -s grove-sync -a solair -w",
		Workspaces:   []SyncWorkspace{{Name: "registry", Role: SyncRoleRegistry, Pull: true}},
	})
	if err != nil {
		t.Fatalf("ApplySyncEdit: %v", err)
	}
	if len(res.Filled) != 0 {
		t.Errorf("filled = %v, want nothing (both keys were declared)", res.Filled)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "elsewhere.example.com") {
		t.Errorf("warnings = %v, want the server mismatch", res.Warnings)
	}
	if content := readSync(t, path); !strings.HasPrefix(content, original) {
		t.Errorf("a declared scalar was rewritten:\n%s", content)
	}
}

// `server = ""` parses as absent but IS declared. Inserting a second one would
// be a duplicate-key TOML error; the editor says so in config terms instead.
func TestSyncEditorReportsADeclaredButEmptyScalar(t *testing.T) {
	path := syncPath(t)
	if err := os.WriteFile(path, []byte("server = \"\"\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	res, err := ApplySyncEdit(path, SyncEdit{
		Server:     "https://forge.example.com:8788",
		Workspaces: []SyncWorkspace{{Name: "registry", Role: SyncRoleRegistry, Pull: true}},
	})
	if err != nil {
		t.Fatalf("ApplySyncEdit: %v", err)
	}
	if len(res.Filled) != 0 {
		t.Errorf("filled = %v, want nothing", res.Filled)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "leaves it empty") {
		t.Fatalf("warnings = %v, want the declared-but-empty notice", res.Warnings)
	}
	cfg, err := LoadSyncConfigFrom(path)
	if err != nil {
		t.Fatalf("the file no longer parses: %v", err)
	}
	if cfg.Server != "" {
		t.Errorf("server = %q, want the user's empty value untouched", cfg.Server)
	}
}

// A file that resolves its token some other way keeps doing so: filling a
// second source would change which credential the daemon presents.
func TestSyncEditorDoesNotFillTokenCommandBesideALiteralToken(t *testing.T) {
	path := syncPath(t)
	if err := os.WriteFile(path, []byte("token = \"literal\"\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	res, err := ApplySyncEdit(path, SyncEdit{
		Server:       "https://forge.example.com:8788",
		TokenCommand: "security find-generic-password -s grove-sync -a solair -w",
		Workspaces:   []SyncWorkspace{{Name: "registry", Role: SyncRoleRegistry, Pull: true}},
	})
	if err != nil {
		t.Fatalf("ApplySyncEdit: %v", err)
	}
	if len(res.Filled) != 1 || res.Filled[0] != "server" {
		t.Fatalf("filled = %v, want [server] only", res.Filled)
	}
	cfg, _ := LoadSyncConfigFrom(path)
	if cfg.TokenCommand != "" {
		t.Errorf("token_command = %q, want it left absent beside the literal token", cfg.TokenCommand)
	}
}

func readSync(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
