package notespace

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

const testSubject = "github.com/Me/Core"

func TestMintLoadAndMutableUpdate(t *testing.T) {
	root := t.TempDir()
	stamp, err := MintNotespace(root, NotespaceMutable{Name: "core", Subject: testSubject, Kind: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadNotespace(root)
	if err != nil || *loaded != *stamp {
		t.Fatalf("LoadNotespace = %+v, %v; want %+v", loaded, err, stamp)
	}
	updated, err := UpdateNotespace(root, stamp.ID, NotespaceMutable{Name: "core-personal", Subject: testSubject, Kind: "repo"})
	if err != nil || updated.ID != stamp.ID || updated.Name != "core-personal" {
		t.Fatalf("UpdateNotespace = %+v, %v", updated, err)
	}
	if _, err := UpdateNotespace(root, "01ARZ3NDEKTSV4RRFFQ69G5FAV", NotespaceMutable{Name: "x", Subject: testSubject, Kind: "repo"}); err == nil {
		t.Fatal("immutable id mismatch was accepted")
	}
}

func TestConcurrentMintAdoptsSingleWinner(t *testing.T) {
	root := t.TempDir()
	const workers = 24
	ids := make(chan string, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stamp, err := MintNotespace(root, NotespaceMutable{Name: "core", Subject: testSubject, Kind: "repo"})
			if err != nil {
				errs <- err
				return
			}
			ids <- stamp.ID
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	var winner string
	for id := range ids {
		if winner == "" {
			winner = id
		} else if id != winner {
			t.Fatalf("concurrent mint returned ids %q and %q", winner, id)
		}
	}
}

func TestMalformedStampNeverRemints(t *testing.T) {
	root := t.TempDir()
	path := NotespaceStampPath(root)
	if err := os.WriteFile(path, []byte("id = [broken\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	if _, err := MintNotespace(root, NotespaceMutable{Name: "core", Subject: testSubject, Kind: "repo"}); err == nil {
		t.Fatal("malformed stamp was silently reminted")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatal("malformed stamp changed")
	}
}

func TestNotebookStampAndIdentityDotfiles(t *testing.T) {
	root := t.TempDir()
	stamp, err := MintNotebook(root, "grovetools")
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadNotebook(root)
	if err != nil || *loaded != *stamp {
		t.Fatalf("LoadNotebook = %+v, %v", loaded, err)
	}
	updated, err := UpdateNotebook(root, stamp.ID, "renamed")
	if err != nil || updated.ID != stamp.ID || updated.Name != "renamed" {
		t.Fatalf("UpdateNotebook = %+v, %v", updated, err)
	}
	if _, err := UpdateNotebook(root, "01ARZ3NDEKTSV4RRFFQ69G5FAV", "wrong"); err == nil {
		t.Fatal("notebook immutable id mismatch was accepted")
	}
	for _, path := range []string{NotespaceStampName, filepath.Join("nested", NotebookStampName)} {
		if !IsIdentityStamp(path) {
			t.Fatalf("IsIdentityStamp(%q) = false", path)
		}
	}
	if IsIdentityStamp("ordinary.md") {
		t.Fatal("ordinary file classified as identity stamp")
	}
}

func TestIndexReportsDuplicateIDsAndAllowsRepeatedSubjects(t *testing.T) {
	id := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	roots := []string{t.TempDir(), t.TempDir()}
	for i, root := range roots {
		_, err := InstallNotespace(root, NotespaceStamp{ID: id, Name: []string{"a", "b"}[i], Subject: testSubject, Kind: "repo"})
		if err != nil {
			t.Fatal(err)
		}
	}
	idx, err := BuildIndex(roots)
	if err != nil {
		t.Fatal(err)
	}
	if got := idx.BySubject(testSubject); len(got) != 2 {
		t.Fatalf("repeated subject records = %d, want 2", len(got))
	}
	if _, err := idx.ByID(id); err == nil {
		t.Fatal("duplicate physical id was not reported")
	}
}
