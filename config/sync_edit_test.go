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

func readSync(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
