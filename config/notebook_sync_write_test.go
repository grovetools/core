package config

import (
	"os"
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

// The loader accepts `sync = { share = true }`, so the writer has to be able to
// maintain it. A writer that could only append [notebooks.<n>.sync] would make
// `notebook unshare` permanently impossible on a file the loader blessed — the
// re-parse would reject the duplicate key and refuse the write.
func TestWriteNotebooksMaintainsInlineSyncTable(t *testing.T) {
	rootsPath, nbPath := tmpPair(t)
	seed := "default = \"nb\"\n\n[notebooks.nb]\nroot = \"/notebooks/nb\"\nsync = { share = true }\n"
	if err := os.WriteFile(nbPath, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if !loadNotebooks(t, rootsPath, nbPath).NotebookShared("nb") {
		t.Fatalf("fixture must load as shared:\n%s", seed)
	}

	changed, err := WriteNotebooks(nbPath, NotebookEdits{SyncShare: map[string]bool{"nb": false}})
	if err != nil || !changed {
		t.Fatalf("unshare of an inline sync table: changed=%t err=%v", changed, err)
	}
	table := loadNotebooks(t, rootsPath, nbPath)
	if table.NotebookShared("nb") {
		t.Fatalf("unshare did not take:\n%s", mustRead(t, nbPath))
	}
	if !table.Notebooks["nb"].SyncRecorded() {
		t.Fatalf("the recorded fact must survive as `share = false`:\n%s", mustRead(t, nbPath))
	}
	// The operator chose the inline spelling; maintaining the file is not an
	// occasion to restyle it.
	content := mustRead(t, nbPath)
	if !strings.Contains(content, "sync = { share = false }") || strings.Contains(content, "[notebooks.nb.sync]") {
		t.Fatalf("inline spelling was not maintained in place:\n%s", content)
	}

	// And back again: re-sharing rewrites the same key rather than growing a
	// second declaration.
	if _, err := WriteNotebooks(nbPath, NotebookEdits{SyncShare: map[string]bool{"nb": true}}); err != nil {
		t.Fatalf("re-share: %v", err)
	}
	if !loadNotebooks(t, rootsPath, nbPath).NotebookShared("nb") {
		t.Fatalf("re-share did not take:\n%s", mustRead(t, nbPath))
	}
	if strings.Count(mustRead(t, nbPath), "sync") != 1 {
		t.Fatalf("the sync fact is recorded more than once:\n%s", mustRead(t, nbPath))
	}
}

// A comment block sitting above a table header documents THAT table. Neither
// inserting a child table nor rewriting the table before it may move or delete
// it — the worst case is a "# do not sync." comment landing immediately above
// an inserted `share = true`.
func TestWriteNotebooksKeepsCommentsWithTheirTable(t *testing.T) {
	rootsPath, nbPath := tmpPair(t)
	seed := "default = \"work\"\n\n[notebooks.work]\nroot = \"/n/work\"\n\n" +
		"# Personal notes live here; do not sync.\n[notebooks.personal]\nroot = \"/n/personal\"\n"
	if err := os.WriteFile(nbPath, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := WriteNotebooks(nbPath, NotebookEdits{SyncShare: map[string]bool{"work": true}}); err != nil {
		t.Fatal(err)
	}
	content := mustRead(t, nbPath)
	if strings.Index(content, "# Personal notes live here") > strings.Index(content, "[notebooks.personal]") {
		t.Fatalf("the comment was separated from the table it documents:\n%s", content)
	}
	if strings.Index(content, "[notebooks.work.sync]") > strings.Index(content, "# Personal notes live here") {
		t.Fatalf("the child table was inserted past its parent's neighbourhood:\n%s", content)
	}

	// An unrelated root rewrite goes through the same table editor; it must not
	// consume the following table's comment either.
	if _, err := WriteNotebooks(nbPath, NotebookEdits{
		Upserts: map[string]coderoot.Notebook{"work": {Root: "/n/moved"}},
	}); err != nil {
		t.Fatal(err)
	}
	content = mustRead(t, nbPath)
	if !strings.Contains(content, "# Personal notes live here; do not sync.") {
		t.Fatalf("a root rewrite deleted an operator's comment:\n%s", content)
	}
	table := loadNotebooks(t, rootsPath, nbPath)
	if table.NotebookRoot("work") != "/n/moved" || !table.NotebookShared("work") {
		t.Fatalf("rewrite lost state: root=%q shared=%t\n%s", table.NotebookRoot("work"), table.NotebookShared("work"), content)
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
