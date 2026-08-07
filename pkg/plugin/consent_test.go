package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A changed default is a changed behavior, so an update diff has to name the
// setting and both values rather than saying "settings changed".
func TestDiffNamesTheSettingThatMoved(t *testing.T) {
	old := ConsentFacts{Settings: []string{"break_minutes = 5", "work_minutes = 25"}}
	next := ConsentFacts{Settings: []string{"break_minutes = 5", "chime = bell", "work_minutes = 50"}}

	changes := Diff(old, next)
	byField := map[string]FactChange{}
	for _, c := range changes {
		byField[c.Field] = c
	}
	if c, ok := byField["settings.work_minutes"]; !ok || c.Old != "25" || c.New != "50" {
		t.Errorf("work_minutes change = %+v, want 25 → 50", c)
	}
	if c, ok := byField["settings.chime"]; !ok || c.Old != "" || c.New != "bell" {
		t.Errorf("added setting = %+v, want an addition of bell", c)
	}
	if _, ok := byField["settings.break_minutes"]; ok {
		t.Error("an unchanged setting appeared in the diff")
	}
}

// The digest binds the approval, so a retuned default must re-open the prompt.
func TestDigestCoversSettingsAndLabel(t *testing.T) {
	base := ConsentFacts{Name: "timer", Settings: []string{"work_minutes = 25"}}
	retuned := ConsentFacts{Name: "timer", Settings: []string{"work_minutes = 50"}}
	if base.Digest() == retuned.Digest() {
		t.Error("changing a settings default did not change the approval digest")
	}
	relabeled := ConsentFacts{Name: "timer", Settings: []string{"work_minutes = 25"}, Label: "Timer"}
	if base.Digest() == relabeled.Digest() {
		t.Error("changing the label did not change the approval digest")
	}
}

// ViewFacts and KeyFacts render the lines an approval is hashed over, so their
// exact wording is a recorded fact rather than presentation: a reworded marker
// would change every digest and orphan every approval already on disk.
func TestViewAndKeyFactsRenderTheApprovedLines(t *testing.T) {
	m, err := ParseManifest([]byte(viewManifest))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	views := ViewFacts(&m.Panel)
	want := []string{
		"full — clock, history and help",
		"compact — one line: state and time remaining (what a drawer pane gets by default)",
	}
	if len(views) != len(want) {
		t.Fatalf("view facts = %v, want %v", views, want)
	}
	for i := range want {
		if views[i] != want[i] {
			t.Errorf("view fact[%d] = %q, want %q", i, views[i], want[i])
		}
	}

	// A second drawer-suitable view is marked too, but as an alternative rather
	// than as the default — only the first one is what a bare drawer pane gets.
	both, err := ParseManifest([]byte(strings.Replace(viewManifest, "drawer      = false", "drawer      = true", 1)))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	lines := ViewFacts(&both.Panel)
	if !strings.HasSuffix(lines[0], "(what a drawer pane gets by default)") {
		t.Errorf("first declared drawer view = %q, want it marked as the default", lines[0])
	}
	if !strings.HasSuffix(lines[1], "(also offered to a drawer pane)") {
		t.Errorf("second drawer view = %q, want it marked as an alternative", lines[1])
	}

	hello, err := ParseManifest([]byte(validManifest))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	keys := KeyFacts(&hello.Panel)
	if len(keys) != 1 || keys[0] != "ctrl+f — jump to the notebook" {
		t.Errorf("key facts = %v, want the manifest's one claim", keys)
	}
}

// A fragment is approved under the path it will be written to, and that path is
// canonicalized HERE rather than by the store: exectrust resolves symlinks,
// which only works for a path that exists, and a fragment does not exist when
// the install is approved nor after it is removed. Filing under one path and
// checking under another would read as "edited" for every plugin on a machine
// whose temp or home directory is a symlink (macOS: /var -> /private/var).
func TestApprovalSurvivesAFragmentThatDoesNotExistYet(t *testing.T) {
	isolate(t)

	dir, err := ConfigPluginsDir()
	if err != nil {
		t.Fatalf("ConfigPluginsDir: %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	fragment := filepath.Join(dir, "demo.toml")

	// Approved before the file exists, exactly as an install does it.
	if err := RecordApproval(fragment, "sha256:demo", time.Unix(1700000000, 0)); err != nil {
		t.Fatalf("RecordApproval: %v", err)
	}
	if !IsApproved(fragment, "sha256:demo") {
		t.Fatal("an approval recorded before the fragment exists must still verify")
	}
	if err := os.WriteFile(fragment, []byte("# written\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !IsApproved(fragment, "sha256:demo") {
		t.Error("writing the fragment invalidated the approval recorded for it")
	}
	if IsApproved(fragment, "sha256:other") {
		t.Error("a different digest must not be covered by this approval")
	}

	// And revocation reaches the same record, after the file is gone again.
	if err := os.Remove(fragment); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := RevokeApproval(fragment); err != nil {
		t.Fatalf("RevokeApproval: %v", err)
	}
	if IsApproved(fragment, "sha256:demo") {
		t.Error("the approval survived its revocation")
	}
}
