package registry

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sampleNote() *Note {
	return &Note{
		MachineID:     "01KZ00TTW1TDT7X9ABCDEFGHJK",
		Name:          "mbp",
		Rev:           7,
		LastSeen:      "2026-08-02",
		OriginID:      "6f1c9a2b",
		GrovedVersion: "0.6.3",
		Ecosystems: []NoteEcosystem{
			// Deliberately out of order: Render must sort.
			{
				Name: "zoo", Path: "/Users/x/code/zoo", State: StateDeclaredMissing, Enabled: true,
			},
			{
				Name: "grovetools", Path: "/Users/x/code/grovetools", Notebook: "grovetools",
				State: StatePresent, Enabled: true, Repos: []string{"nav", "core"},
				Card: &NoteCard{
					ID:   "01J8ZZZZZZZZZZZZZZZZZZZZZZ",
					Name: "grovetools",
					Members: []NoteMemberOrigin{
						{Path: "core", Origin: "https://github.com/grovetools/core.git"},
						{Path: ".", Origin: "https://github.com/grovetools/grovetools.git"},
					},
				},
			},
		},
		Roots: []NoteRoot{
			{Name: "chickens", Path: "/Users/x/code/chickens", Notebook: "nb", Enabled: true, Exists: true},
		},
		Subscriptions: []NoteSubscription{
			{Name: "registry", Role: "registry", Mode: "full", Pull: true},
			{Name: "grovetools", Role: "peer", Pull: true},
		},
		Repos: []NoteRepo{
			{Root: "grovetools", Path: "core", Branch: "main", SHA: "af3803c7e4269c968151a4f6e41af22e5e09e757"},
			{Root: "grovetools", Path: ".", Branch: "workspace-identity", SHA: "fdb1774aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		},
	}
}

func TestNoteRoundTrip(t *testing.T) {
	want := sampleNote()
	data := want.Render()

	got, err := ParseNote(data)
	if err != nil {
		t.Fatalf("ParseNote: %v\n---\n%s", err, data)
	}

	// Compare through a second render: the normalized (sorted) form is the
	// contract, not the field-by-field order the fixture happened to use.
	if !bytes.Equal(data, got.Render()) {
		t.Errorf("round trip is not byte-stable\n--- first ---\n%s\n--- second ---\n%s", data, got.Render())
	}

	if got.MachineID != want.MachineID || got.Name != want.Name || got.Rev != want.Rev {
		t.Errorf("identity lost: %+v", got)
	}
	if got.LastSeen != want.LastSeen || got.OriginID != want.OriginID || got.GrovedVersion != want.GrovedVersion {
		t.Errorf("headline fields lost: %+v", got)
	}
	if len(got.Ecosystems) != 2 || got.Ecosystems[0].Name != "grovetools" {
		t.Fatalf("ecosystems not sorted/complete: %+v", got.Ecosystems)
	}
	card := got.Ecosystems[0].Card
	if card == nil || card.ID != "01J8ZZZZZZZZZZZZZZZZZZZZZZ" || card.Name != "grovetools" {
		t.Fatalf("embedded card lost: %+v", card)
	}
	if len(card.Members) != 2 || card.Members[0].Path != "." || card.Members[1].Path != "core" {
		t.Errorf("member origins lost/not sorted: %+v", card.Members)
	}
	if len(got.Ecosystems[0].Repos) != 2 || got.Ecosystems[0].Repos[0] != "core" || got.Ecosystems[0].Repos[1] != "nav" {
		t.Errorf("partial repository intent lost/not sorted: %+v", got.Ecosystems[0].Repos)
	}
	if got.Ecosystems[1].State != StateDeclaredMissing {
		t.Errorf("declared-missing state lost: %+v", got.Ecosystems[1])
	}
	if len(got.Roots) != 1 || got.Roots[0].Name != "chickens" || !got.Roots[0].Exists {
		t.Errorf("roots lost: %+v", got.Roots)
	}
	if len(got.Subscriptions) != 2 || got.Subscriptions[0].Name != "grovetools" {
		t.Errorf("subscriptions not sorted/complete: %+v", got.Subscriptions)
	}
	if len(got.Repos) != 2 || got.Repos[0].Path != "." || got.Repos[1].Path != "core" {
		t.Errorf("repos not sorted/complete: %+v", got.Repos)
	}
}

// TestRenderIsDeterministic is the property the whole write-suppression design
// rests on: same input, same bytes, every time.
func TestRenderIsDeterministic(t *testing.T) {
	first := sampleNote().Render()
	for i := 0; i < 50; i++ {
		if !bytes.Equal(first, sampleNote().Render()) {
			t.Fatalf("render %d differs from render 0", i)
		}
	}
}

func TestRenderHandlesEmptyCollections(t *testing.T) {
	n := &Note{MachineID: "01AAA", Name: "bare", Rev: 1, LastSeen: "2026-08-02"}
	data := n.Render()
	for _, key := range []string{"ecosystems: []", "roots: []", "subscriptions: []", "repos: []"} {
		if !strings.Contains(string(data), key) {
			t.Errorf("missing empty-collection marker %q in:\n%s", key, data)
		}
	}
	got, err := ParseNote(data)
	if err != nil {
		t.Fatalf("ParseNote: %v", err)
	}
	if len(got.Ecosystems) != 0 || len(got.Repos) != 0 {
		t.Errorf("empty collections did not survive: %+v", got)
	}
}

// TestRenderQuotesAmbiguousScalars guards the hand-rolled emitter against the
// values that would otherwise round-trip as the wrong type or break the YAML.
func TestRenderQuotesAmbiguousScalars(t *testing.T) {
	n := &Note{
		MachineID: "01AAA",
		// A machine literally named "no" is YAML 1.1 false; "1.20" is a float;
		// a colon-space would end the scalar early.
		Name:          "no",
		Rev:           1,
		LastSeen:      "2026-08-02",
		OriginID:      "1.20",
		GrovedVersion: "v1: dev",
		Roots: []NoteRoot{
			{Name: "weird", Path: "/Users/x/my code/#notes", Enabled: true},
		},
	}
	got, err := ParseNote(n.Render())
	if err != nil {
		t.Fatalf("ParseNote: %v\n%s", err, n.Render())
	}
	if got.Name != "no" {
		t.Errorf("Name coerced: %q", got.Name)
	}
	if got.OriginID != "1.20" {
		t.Errorf("OriginID coerced: %q", got.OriginID)
	}
	if got.GrovedVersion != "v1: dev" {
		t.Errorf("GrovedVersion mangled: %q", got.GrovedVersion)
	}
	if len(got.Roots) != 1 || got.Roots[0].Path != "/Users/x/my code/#notes" {
		t.Errorf("root path mangled: %+v", got.Roots)
	}
}

func TestParseRejectsNonNotes(t *testing.T) {
	for name, data := range map[string]string{
		"no frontmatter":    "# just a note\n",
		"unterminated":      "---\nmachine_id: x\n",
		"no machine_id":     "---\nname: mbp\n---\n\nbody\n",
		"malformed yaml":    "---\nmachine_id: [\n---\n\nbody\n",
		"empty machine_id":  "---\nmachine_id: \"\"\n---\n\nbody\n",
		"totally empty doc": "",
	} {
		if _, err := ParseNote([]byte(data)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestNotePathIsIDKeyed(t *testing.T) {
	id := "01KZ00TTW1TDT7X9ABCDEFGHJK"
	p := NotePath(id)
	if p != "machines/"+id+".md" {
		t.Fatalf("NotePath = %q", p)
	}
	if got := MachineIDFromPath(p); got != id {
		t.Errorf("MachineIDFromPath(%q) = %q", p, got)
	}
	// Anything outside machines/ is not a machine note, so the pull guard can
	// never match an ordinary document that happens to share a basename.
	for _, bad := range []string{"plans/" + id + ".md", "machines/sub/" + id + ".md", "machines/x.txt", id + ".md"} {
		if got := MachineIDFromPath(bad); got != "" {
			t.Errorf("MachineIDFromPath(%q) = %q, want empty", bad, got)
		}
	}
}

// TestNotePathSurvivesTheSyncExclusionRules is a regression guard on the
// contract's content constraints: no dot-prefixed segment (the watcher's
// MatchesEvent drops those before any subscription matching) and no segment
// from the default exclusion manifest.
func TestNotePathSurvivesTheSyncExclusionRules(t *testing.T) {
	excluded := map[string]bool{
		".obsidian": true, ".stfolder": true, ".stversions": true,
		".cx": true, ".artifacts": true, ".git": true,
	}
	for _, seg := range strings.Split(NotePath("01AAA"), "/") {
		if strings.HasPrefix(seg, ".") {
			t.Errorf("path segment %q is dot-prefixed; it would never replicate", seg)
		}
		if excluded[seg] {
			t.Errorf("path segment %q is in the default exclusion manifest", seg)
		}
	}
	if strings.HasSuffix(NotePath("01AAA"), ".conflict.md") {
		t.Error("note name collides with the conflict-artifact exclusion")
	}
}

func TestStaleForUsesReaderClock(t *testing.T) {
	m := Machine{Note: &Note{LastSeen: "2026-08-01"}}
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
	d, ok := m.StaleFor(now)
	if !ok || d < 72*time.Hour || d > 96*time.Hour {
		t.Fatalf("StaleFor = %v, %v", d, ok)
	}
	// A note stamped in the future (a peer whose clock is ahead) must read as
	// fresh, not as a negative age.
	future := Machine{Note: &Note{LastSeen: "2026-08-09"}}
	if d, ok := future.StaleFor(now); !ok || d != 0 {
		t.Errorf("future note: StaleFor = %v, %v; want 0, true", d, ok)
	}
	if _, ok := (Machine{}).StaleFor(now); ok {
		t.Error("a note-less machine reported a staleness")
	}
}

// --- repo tips ------------------------------------------------------------

func initRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "trunk")
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("add", "f.txt")
	run("commit", "-q", "-m", "init")
}

func TestReadRepoTipWithoutGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	repo := filepath.Join(root, "alpha")
	initRepo(t, repo)

	branch, sha, ok := ReadRepoTip(repo)
	if !ok {
		t.Fatal("ReadRepoTip reported no tip for a real repo")
	}
	if branch != "trunk" {
		t.Errorf("branch = %q, want trunk", branch)
	}
	if !isHex(sha) || len(sha) < 40 {
		t.Errorf("sha = %q", sha)
	}

	// Packed refs must resolve identically — the loose ref is gone afterwards.
	cmd := exec.Command("git", "pack-refs", "--all")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git pack-refs: %v\n%s", err, out)
	}
	packedBranch, packedSHA, ok := ReadRepoTip(repo)
	if !ok || packedBranch != branch || packedSHA != sha {
		t.Errorf("packed-refs tip = %q/%q, want %q/%q", packedBranch, packedSHA, branch, sha)
	}

	if _, _, ok := ReadRepoTip(root); ok {
		t.Error("a non-repository reported a tip")
	}
}

func TestCollectRepoTipsScansRootAndChildren(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	initRepo(t, root)
	initRepo(t, filepath.Join(root, "beta"))
	initRepo(t, filepath.Join(root, "alpha"))
	if err := os.MkdirAll(filepath.Join(root, "notarepo"), 0o755); err != nil {
		t.Fatal(err)
	}

	tips := CollectRepoTips(map[string]string{"eco": root})
	var paths []string
	for _, tip := range tips {
		paths = append(paths, tip.Path)
		if tip.Root != "eco" {
			t.Errorf("wrong root on %+v", tip)
		}
	}
	want := []string{".", "alpha", "beta"}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Errorf("tips = %v, want %v", paths, want)
	}
}
