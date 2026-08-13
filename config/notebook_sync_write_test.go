package config

import (
	"strings"
	"testing"

	"github.com/grovetools/core/pkg/coderoot"
)

// Share state is recorded through the same shared writer as everything else in
// notebooks.toml, so these tests hold it to the same contract: surgical edits,
// a candidate that re-parses, and an atomic rename.

func loadNotebooks(t *testing.T, rootsPath, nbPath string) coderoot.Table {
	t.Helper()
	table, err := coderoot.LoadFrom(rootsPath, nbPath)
	if err != nil {
		t.Fatalf("recorded pair must reload: %v", err)
	}
	return table
}

func TestWriteNotebooksRecordsShare(t *testing.T) {
	rootsPath, nbPath := tmpPair(t)
	def := "nb"
	if _, err := WriteNotebooks(nbPath, NotebookEdits{
		Default: &def,
		Upserts: map[string]coderoot.Notebook{"nb": {Root: "/notebooks/nb"}},
	}); err != nil {
		t.Fatal(err)
	}

	changed, err := WriteNotebooks(nbPath, NotebookEdits{SyncShare: map[string]bool{"nb": true}})
	if err != nil || !changed {
		t.Fatalf("share write: changed=%t err=%v", changed, err)
	}
	if !loadNotebooks(t, rootsPath, nbPath).NotebookShared("nb") {
		t.Fatalf("share was not recorded:\n%s", mustRead(t, nbPath))
	}

	// Unshare is forward-only, so the file states it rather than falling
	// silent: `share = false` is the recorded fact, not a deleted table.
	if _, err := WriteNotebooks(nbPath, NotebookEdits{SyncShare: map[string]bool{"nb": false}}); err != nil {
		t.Fatal(err)
	}
	table := loadNotebooks(t, rootsPath, nbPath)
	if table.NotebookShared("nb") {
		t.Fatal("unshare did not clear the share flag")
	}
	if !table.Notebooks["nb"].SyncRecorded() {
		t.Fatalf("unshare must leave the recorded sync table in place:\n%s", mustRead(t, nbPath))
	}
	if !strings.Contains(mustRead(t, nbPath), "share = false") {
		t.Fatalf("unshare must write the fact explicitly:\n%s", mustRead(t, nbPath))
	}
}

func TestWriteNotebooksPreservesRecordedSyncTable(t *testing.T) {
	rootsPath, nbPath := tmpPair(t)
	def := "nb"
	if _, err := WriteNotebooks(nbPath, NotebookEdits{
		Default: &def,
		Upserts: map[string]coderoot.Notebook{"nb": {Root: "/notebooks/nb"}, "personal": {Root: "/notebooks/personal"}},
		Header:  []string{"# Recorded notebooks.", ""},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteNotebooks(nbPath, NotebookEdits{SyncShare: map[string]bool{"nb": true}}); err != nil {
		t.Fatal(err)
	}

	// A later, unrelated edit to the notebook's root must not silently
	// unshare it — an upsert that never mentioned sync has no opinion on it.
	if _, err := WriteNotebooks(nbPath, NotebookEdits{
		Upserts: map[string]coderoot.Notebook{"nb": {Root: "/notebooks/moved"}},
	}); err != nil {
		t.Fatal(err)
	}
	table := loadNotebooks(t, rootsPath, nbPath)
	if !table.NotebookShared("nb") {
		t.Fatalf("root rewrite dropped the recorded share state:\n%s", mustRead(t, nbPath))
	}
	if table.NotebookRoot("nb") != "/notebooks/moved" {
		t.Fatalf("root = %q, want /notebooks/moved", table.NotebookRoot("nb"))
	}
	content := mustRead(t, nbPath)
	if !strings.Contains(content, "# Recorded notebooks.") {
		t.Fatalf("header lost:\n%s", content)
	}
	// The child table belongs next to its parent, not at end of file after an
	// unrelated notebook.
	if strings.Index(content, "[notebooks.nb.sync]") > strings.Index(content, "[notebooks.personal]") {
		t.Fatalf("sync table was stranded away from its notebook:\n%s", content)
	}
}

func TestWriteNotebooksDeleteRemovesSyncTableToo(t *testing.T) {
	rootsPath, nbPath := tmpPair(t)
	def := "nb"
	if _, err := WriteNotebooks(nbPath, NotebookEdits{
		Default: &def,
		Upserts: map[string]coderoot.Notebook{"nb": {Root: "/notebooks/nb"}, "personal": {Root: "/notebooks/personal"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteNotebooks(nbPath, NotebookEdits{SyncShare: map[string]bool{"personal": true}}); err != nil {
		t.Fatal(err)
	}
	// An orphaned [notebooks.personal.sync] would reload as a notebook with no
	// root, and the writer refuses to persist anything that does not reload —
	// so a delete that left one behind would fail loudly here.
	if _, err := WriteNotebooks(nbPath, NotebookEdits{Deletes: []string{"personal"}}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	table := loadNotebooks(t, rootsPath, nbPath)
	if _, ok := table.Notebooks["personal"]; ok {
		t.Fatalf("notebook survived deletion:\n%s", mustRead(t, nbPath))
	}
	if strings.Contains(mustRead(t, nbPath), "personal") {
		t.Fatalf("orphaned sync table left behind:\n%s", mustRead(t, nbPath))
	}
}

func TestWriteNotebooksShareRefusesUnrecordedNotebook(t *testing.T) {
	_, nbPath := tmpPair(t)
	// Sharing a notebook that has no definition would produce a file that
	// does not reload; the writer must refuse rather than persist it.
	if _, err := WriteNotebooks(nbPath, NotebookEdits{SyncShare: map[string]bool{"ghost": true}}); err == nil {
		t.Fatalf("expected a refusal; file is now:\n%s", mustRead(t, nbPath))
	}
}

func TestWriteNotebooksShareIsIdempotent(t *testing.T) {
	_, nbPath := tmpPair(t)
	def := "nb"
	if _, err := WriteNotebooks(nbPath, NotebookEdits{
		Default: &def,
		Upserts: map[string]coderoot.Notebook{"nb": {Root: "/notebooks/nb"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteNotebooks(nbPath, NotebookEdits{SyncShare: map[string]bool{"nb": true}}); err != nil {
		t.Fatal(err)
	}
	before := mustRead(t, nbPath)
	changed, err := WriteNotebooks(nbPath, NotebookEdits{SyncShare: map[string]bool{"nb": true}})
	if err != nil {
		t.Fatal(err)
	}
	if changed || mustRead(t, nbPath) != before {
		t.Fatalf("re-sharing rewrote the file: changed=%t\n%s", changed, mustRead(t, nbPath))
	}
}
