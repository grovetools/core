package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sandbox redirects every state/config path this package can reach into the
// test's own tmpdir. Every test here MUST call it: the rev cache resolves
// through paths.StateDir(), so an unsandboxed run would write into the
// developer's real ~/.local/state/grove — the exact class of accident that
// made a stray machine.json appear on the owner's laptop during job 11.
func sandbox(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GROVE_HOME", "")
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	return home
}

// writeNote materializes one machine note under a fake registry workspace root.
func writeNote(t *testing.T, root string, n *Note) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(NotePath(n.MachineID)))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, n.Render(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReadMachinesReportsPeersAndSelf(t *testing.T) {
	sandbox(t)
	root := t.TempDir()
	writeNote(t, root, &Note{MachineID: "01SELF", Name: "mbp", Rev: 3, LastSeen: "2026-08-02"})
	writeNote(t, root, &Note{
		MachineID: "01PEER", Name: "solm4", Rev: 1, LastSeen: "2026-07-20",
		Ecosystems: []NoteEcosystem{
			{Name: "grovetools", Path: "/code/grovetools", State: StateDeclaredMissing, Enabled: true},
			{Name: "present", Path: "/code/present", State: StatePresent, Enabled: true},
			{Name: "off", Path: "/code/off", State: StateDeclaredMissing, Enabled: false},
		},
	})

	machines, err := ReadMachines(root, "01SELF")
	if err != nil {
		t.Fatalf("ReadMachines: %v", err)
	}
	if len(machines) != 2 {
		t.Fatalf("got %d machines, want 2: %+v", len(machines), machines)
	}
	// Sorted by label: "mbp (01SELF)" < "solm4 (01PEER)".
	if machines[0].PathID != "01SELF" || !machines[0].Self {
		t.Errorf("first row is not self: %+v", machines[0])
	}
	if machines[1].Self {
		t.Errorf("peer marked as self: %+v", machines[1])
	}
	for _, m := range machines {
		if m.Suspicious() {
			t.Errorf("clean note flagged: %v", m.Suspect)
		}
	}

	missing := machines[1].DeclaredMissing()
	if len(missing) != 1 || missing[0].Name != "grovetools" {
		t.Errorf("DeclaredMissing = %+v; want only the enabled missing one", missing)
	}
}

func TestReadMachinesFlagsPathIDMismatch(t *testing.T) {
	sandbox(t)
	root := t.TempDir()
	// A note whose document claims a different identity than its own path:
	// either a botched copy or a forgery, and the path is what the
	// single-writer rule is keyed on, so the path wins.
	n := &Note{MachineID: "01LIAR", Name: "impostor", Rev: 1, LastSeen: "2026-08-02"}
	path := filepath.Join(root, MachinesDir, "01VICTIM.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, n.Render(), 0o600); err != nil {
		t.Fatal(err)
	}

	machines, err := ReadMachines(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(machines) != 1 || !machines[0].Suspicious() {
		t.Fatalf("mismatch not flagged: %+v", machines)
	}
	if !strings.Contains(machines[0].Suspect[0], "tampered") {
		t.Errorf("Suspect = %v", machines[0].Suspect)
	}
	if machines[0].PathID != "01VICTIM" {
		t.Errorf("identity taken from the document, not the path: %+v", machines[0])
	}
}

func TestReadMachinesFlagsRevRegression(t *testing.T) {
	sandbox(t)
	root := t.TempDir()
	writeNote(t, root, &Note{MachineID: "01PEER", Name: "solm4", Rev: 9, LastSeen: "2026-08-02"})

	if machines, err := ReadMachines(root, ""); err != nil || machines[0].Suspicious() {
		t.Fatalf("first read should be clean: %+v %v", machines, err)
	}

	// The note is replaced by an older one — a restore, or someone else
	// writing this machine's note.
	writeNote(t, root, &Note{MachineID: "01PEER", Name: "solm4", Rev: 4, LastSeen: "2026-08-02"})
	machines, err := ReadMachines(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if !machines[0].Suspicious() || !strings.Contains(machines[0].Suspect[0], "rev regressed") {
		t.Fatalf("regression not flagged: %+v", machines[0].Suspect)
	}

	// The watermark must NOT drop to the regressed value, or the finding would
	// evaporate on the very next read.
	again, err := ReadMachines(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if !again[0].Suspicious() {
		t.Error("the regression finding did not persist across reads")
	}
}

func TestReadMachinesSurfacesUnparseableNotes(t *testing.T) {
	sandbox(t)
	root := t.TempDir()
	dir := filepath.Join(root, MachinesDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "01BROKEN.md"), []byte("not a note"), 0o600); err != nil {
		t.Fatal(err)
	}
	machines, err := ReadMachines(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(machines) != 1 || machines[0].Note != nil || !machines[0].Suspicious() {
		t.Fatalf("unparseable note not surfaced: %+v", machines)
	}
	if machines[0].Err == nil {
		t.Error("Err not set on an unparseable note")
	}
}

func TestReadMachinesToleratesAnEmptyRegistry(t *testing.T) {
	sandbox(t)
	// A machine that has joined but never pulled has no machines/ dir at all;
	// that is "nothing known yet", not a failure.
	machines, err := ReadMachines(t.TempDir(), "")
	if err != nil || len(machines) != 0 {
		t.Fatalf("ReadMachines on an empty root = %+v, %v", machines, err)
	}
	if _, err := ReadMachines("", ""); err == nil {
		t.Error("an unresolvable root should be an error")
	}
}

func TestRevCacheLivesInState(t *testing.T) {
	home := sandbox(t)
	root := t.TempDir()
	writeNote(t, root, &Note{MachineID: "01PEER", Rev: 2, LastSeen: "2026-08-02"})
	if _, err := ReadMachines(root, ""); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, "state", "grove", "registry", RevCacheFileName)
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("rev cache not written to %s: %v", want, err)
	}
	cache, err := LoadRevCache()
	if err != nil {
		t.Fatal(err)
	}
	if cache.Revs["01PEER"] != 2 {
		t.Errorf("cache = %+v", cache.Revs)
	}
}
