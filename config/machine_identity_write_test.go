package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const (
	testNotespaceID  = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	testOtherID      = "01BX5ZZKBKACTAV9WEVGEMMVRZ"
	testLocalSubject = "local:01ARZ3NDEKTSV4RRFFQ69G5FAV"
)

func TestMachineIdentityExactShapeAndSurgicalWrite(t *testing.T) {
	dir := sandboxConfig(t)
	path := filepath.Join(dir, MachineConfigFileName)
	original := "# retained\n[machine]\nname = \"laptop\"\n\n[unowned]\nvalue = 1\n"
	writeFile(t, path, original)
	_, changed, err := EditMachineConfig(path, MachineEditOptions{}, func(cfg *MachineConfig) error {
		cfg.Sync.Registry = &SyncRegistry{Notebook: "grovetools", NotespaceID: testNotespaceID}
		cfg.Primaries = map[string]string{"github.com/grovetools/core": testNotespaceID}
		cfg.Subjects = map[string]string{filepath.Join(t.TempDir(), "repo"): testLocalSubject}
		return nil
	})
	if err != nil || !changed {
		t.Fatalf("EditMachineConfig changed=%t err=%v", changed, err)
	}
	got := readFile(t, path)
	for _, want := range []string{"# retained", "[unowned]", "[sync.registry]", "notebook = \"grovetools\"", "notespace_id = \"" + testNotespaceID + "\"", "[primaries]", "\"github.com/grovetools/core\"", "[subjects]"} {
		if !strings.Contains(got, want) {
			t.Fatalf("machine.toml missing %q:\n%s", want, got)
		}
	}
	cfg, err := LoadMachineConfigFrom(path)
	if err != nil || cfg.Sync.Registry == nil || cfg.Primaries["github.com/grovetools/core"] != testNotespaceID {
		t.Fatalf("reloaded machine config = %+v, %v", cfg, err)
	}
}

func TestMachineEditCASAndPrimaryCrossCheck(t *testing.T) {
	path := filepath.Join(sandboxConfig(t), MachineConfigFileName)
	writeFile(t, path, "[machine]\nname = \"one\"\n")
	rev, err := MachineRevision(path)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, "[machine]\nname = \"two\"\n")
	if _, _, err := EditMachineConfig(path, MachineEditOptions{ExpectedRevision: rev}, func(*MachineConfig) error { return nil }); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("stale CAS not rejected: %v", err)
	}
	before := readFile(t, path)
	_, _, err = EditMachineConfig(path, MachineEditOptions{KnownNotespaceIDs: map[string]struct{}{}}, func(cfg *MachineConfig) error {
		cfg.Primaries = map[string]string{"github.com/grovetools/core": testNotespaceID}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "absent from supplied stamp index") {
		t.Fatalf("missing stamp id not rejected: %v", err)
	}
	if got := readFile(t, path); got != before {
		t.Fatal("rejected edit changed machine.toml")
	}
}

func TestConcurrentMachineEditsSerializeWithoutLostUpdates(t *testing.T) {
	path := filepath.Join(sandboxConfig(t), MachineConfigFileName)
	var wg sync.WaitGroup
	errs := make(chan error, 3)
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, err := WriteMachineName(path, "laptop")
		errs <- err
	}()
	for subject, id := range map[string]string{"github.com/a/one": testNotespaceID, "github.com/b/two": testOtherID} {
		subject, id := subject, id
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := EditMachineConfig(path, MachineEditOptions{}, func(cfg *MachineConfig) error {
				if cfg.Primaries == nil {
					cfg.Primaries = make(map[string]string)
				}
				cfg.Primaries[subject] = id
				return nil
			})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := LoadMachineConfigFrom(path)
	if err != nil || len(cfg.Primaries) != 2 || cfg.Machine.Name != "laptop" {
		t.Fatalf("serialized machine edits = %+v, %v", cfg, err)
	}
}

func TestRerecordSubjectMovesPrimaryAtomically(t *testing.T) {
	path := filepath.Join(sandboxConfig(t), MachineConfigFileName)
	oldSubject, newSubject := "github.com/old/Core", "github.com/new/Core"
	_, _, err := EditMachineConfig(path, MachineEditOptions{}, func(cfg *MachineConfig) error {
		cfg.Primaries = map[string]string{oldSubject: testNotespaceID}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	rev, _ := MachineRevision(path)
	known := map[string]struct{}{testNotespaceID: {}}
	if _, changed, err := RerecordSubject(path, rev, oldSubject, newSubject, testNotespaceID, known); err != nil || !changed {
		t.Fatalf("RerecordSubject changed=%t err=%v", changed, err)
	}
	cfg, err := LoadMachineConfigFrom(path)
	if err != nil || cfg.Primaries[oldSubject] != "" || cfg.Primaries[newSubject] != testNotespaceID {
		t.Fatalf("re-recorded primaries = %+v, %v", cfg.Primaries, err)
	}
	if _, err := os.Stat(path + ".lock"); !os.IsNotExist(err) {
		t.Fatalf("lock residue: %v", err)
	}
}
