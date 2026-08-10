package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/grovetools/core/pkg/coderoot"
)

// The writer tests use explicit paths under t.TempDir() only — never the
// canonical config dir — per the fleet-lab isolation rule: a config test that
// reads ambient state passes only on a clean machine.

func tmpPair(t *testing.T) (rootsPath, nbPath string) {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, coderoot.RootsFileName), filepath.Join(dir, coderoot.NotebooksFileName)
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestWriteNotebooksCreatesFile(t *testing.T) {
	_, nbPath := tmpPair(t)
	def := "nb"
	changed, err := WriteNotebooks(nbPath, NotebookEdits{
		Default: &def,
		Upserts: map[string]coderoot.Notebook{"nb": {Root: "/notebooks/nb"}},
		Header:  []string{"# Recorded notebooks. Written by grove.", ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected a change")
	}
	content := mustRead(t, nbPath)
	if !strings.Contains(content, "# Recorded notebooks.") {
		t.Fatalf("header missing:\n%s", content)
	}
	nf, err := coderoot.ParseNotebooks(nbPath, []byte(content))
	if err != nil {
		t.Fatalf("written file must reload: %v", err)
	}
	if nf.Default != "nb" || nf.Notebooks["nb"].Root != "/notebooks/nb" {
		t.Fatalf("decoded wrong: %+v", nf)
	}
}

func TestWriteCodeRootsRequiresResolvableNotebook(t *testing.T) {
	rootsPath, nbPath := tmpPair(t)

	// With no notebooks.toml sibling, a root binding a notebook must be
	// refused: the writer never persists a state that does not reload.
	_, err := WriteCodeRoots(rootsPath, CodeRootEdits{
		Upserts: map[string]coderoot.Root{"code": {Path: "/code", Scan: true, Notebook: "nb"}},
	})
	if err == nil || !strings.Contains(err.Error(), "would not reload") {
		t.Fatalf("expected reload refusal, got %v", err)
	}
	if _, statErr := os.Stat(rootsPath); !os.IsNotExist(statErr) {
		t.Fatal("refused write must leave no file behind")
	}

	// Record the notebook first; then the same edit lands.
	def := "nb"
	if _, err := WriteNotebooks(nbPath, NotebookEdits{
		Default: &def,
		Upserts: map[string]coderoot.Notebook{"nb": {Root: "/notebooks/nb"}},
	}); err != nil {
		t.Fatal(err)
	}
	changed, err := WriteCodeRoots(rootsPath, CodeRootEdits{
		Upserts: map[string]coderoot.Root{"code": {Path: "/code", Scan: true, Notebook: "nb"}},
	})
	if err != nil || !changed {
		t.Fatalf("write after recording notebook: changed=%v err=%v", changed, err)
	}
}

func TestWriteCodeRootsSurgicalUpsertAndDelete(t *testing.T) {
	rootsPath, nbPath := tmpPair(t)
	def := "nb"
	if _, err := WriteNotebooks(nbPath, NotebookEdits{
		Default: &def,
		Upserts: map[string]coderoot.Notebook{"nb": {Root: "/notebooks/nb"}},
	}); err != nil {
		t.Fatal(err)
	}

	// Hand-authored file: comments and an unrelated table must survive edits
	// byte-for-byte.
	seed := `# my roots — hand comment
[roots.keep]
path = "/keep"
notebook = "nb"

[roots.gone]
path = "/gone"
notebook = "nb"
`
	if err := os.WriteFile(rootsPath, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	depth := 3
	changed, err := WriteCodeRoots(rootsPath, CodeRootEdits{
		Upserts: map[string]coderoot.Root{
			"code": {Path: "/code", Scan: true, Notebook: "nb", Exclude: []string{"vendor-fork"}, Depth: &depth, Description: "scan root"},
		},
		Deletes: []string{"gone"},
	})
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}

	content := mustRead(t, rootsPath)
	if !strings.Contains(content, "# my roots — hand comment") {
		t.Fatalf("hand comment must survive:\n%s", content)
	}
	if strings.Contains(content, "[roots.gone]") {
		t.Fatalf("deleted table still present:\n%s", content)
	}
	rf, err := coderoot.ParseRoots(rootsPath, []byte(content))
	if err != nil {
		t.Fatalf("written file must reload: %v", err)
	}
	if _, ok := rf.Roots["keep"]; !ok {
		t.Fatal("untouched table lost")
	}
	code := rf.Roots["code"]
	if !code.Scan || code.Depth == nil || *code.Depth != 3 || len(code.Exclude) != 1 {
		t.Fatalf("upserted root decoded wrong: %+v", code)
	}

	// Idempotence: the identical upsert is a no-op.
	changed, err = WriteCodeRoots(rootsPath, CodeRootEdits{
		Upserts: map[string]coderoot.Root{
			"code": {Path: "/code", Scan: true, Notebook: "nb", Exclude: []string{"vendor-fork"}, Depth: &depth, Description: "scan root"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("identical upsert must report no change")
	}
}

func TestRecordedWritersHandleQuotedNamesContainingDots(t *testing.T) {
	rootsPath, nbPath := tmpPair(t)
	def := "main"
	if _, err := WriteNotebooks(nbPath, NotebookEdits{
		Default: &def,
		Upserts: map[string]coderoot.Notebook{
			"main":       {Root: "/notebooks/main"},
			"work.notes": {Root: "/notebooks/old"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteCodeRoots(rootsPath, CodeRootEdits{
		Upserts: map[string]coderoot.Root{"work.notes": {Path: "/code/old", Notebook: "main"}},
	}); err != nil {
		t.Fatal(err)
	}

	// Upsert must replace the quoted table rather than append a duplicate.
	if _, err := WriteNotebooks(nbPath, NotebookEdits{
		Upserts: map[string]coderoot.Notebook{"work.notes": {Root: "/notebooks/new"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteCodeRoots(rootsPath, CodeRootEdits{
		Upserts: map[string]coderoot.Root{"work.notes": {Path: "/code/new", Notebook: "main"}},
	}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(mustRead(t, nbPath), `[notebooks."work.notes"]`); got != 1 {
		t.Fatalf("quoted notebook table count = %d, want 1:\n%s", got, mustRead(t, nbPath))
	}
	if got := strings.Count(mustRead(t, rootsPath), `[roots."work.notes"]`); got != 1 {
		t.Fatalf("quoted root table count = %d, want 1:\n%s", got, mustRead(t, rootsPath))
	}
	nf, err := coderoot.ParseNotebooks(nbPath, []byte(mustRead(t, nbPath)))
	if err != nil || nf.Notebooks["work.notes"].Root != "/notebooks/new" {
		t.Fatalf("quoted notebook upsert decoded incorrectly: nf=%+v err=%v", nf, err)
	}
	rf, err := coderoot.ParseRoots(rootsPath, []byte(mustRead(t, rootsPath)))
	if err != nil || rf.Roots["work.notes"].Path != "/code/new" {
		t.Fatalf("quoted root upsert decoded incorrectly: rf=%+v err=%v", rf, err)
	}

	// Delete must identify the same logical quoted segment.
	if _, err := WriteCodeRoots(rootsPath, CodeRootEdits{Deletes: []string{"work.notes"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteNotebooks(nbPath, NotebookEdits{Deletes: []string{"work.notes"}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(mustRead(t, rootsPath), "work.notes") || strings.Contains(mustRead(t, nbPath), "work.notes") {
		t.Fatalf("quoted tables survived delete:\nroots:\n%s\nnotebooks:\n%s", mustRead(t, rootsPath), mustRead(t, nbPath))
	}
}

func TestWriteNotebooksRewritesDefaultInPlace(t *testing.T) {
	_, nbPath := tmpPair(t)
	seed := `# hand header
default = "nb"

[notebooks.nb]
root = "/notebooks/nb"

[notebooks.personal]
root = "/notebooks/personal"
`
	if err := os.WriteFile(nbPath, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	def := "personal"
	changed, err := WriteNotebooks(nbPath, NotebookEdits{Default: &def})
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	content := mustRead(t, nbPath)
	if !strings.Contains(content, "# hand header") {
		t.Fatalf("header must survive:\n%s", content)
	}
	if strings.Count(content, "default =") != 1 {
		t.Fatalf("default key must be rewritten in place, not duplicated:\n%s", content)
	}
	nf, err := coderoot.ParseNotebooks(nbPath, []byte(content))
	if err != nil {
		t.Fatal(err)
	}
	if nf.Default != "personal" {
		t.Fatalf("Default = %q", nf.Default)
	}
}

func TestWriteNotebooksRefusesDanglingDefault(t *testing.T) {
	_, nbPath := tmpPair(t)
	def := "ghost"
	_, err := WriteNotebooks(nbPath, NotebookEdits{
		Default: &def,
		Upserts: map[string]coderoot.Notebook{"nb": {Root: "/n"}},
	})
	if err == nil || !strings.Contains(err.Error(), "would not reload") {
		t.Fatalf("expected reload refusal, got %v", err)
	}
	if _, statErr := os.Stat(nbPath); !os.IsNotExist(statErr) {
		t.Fatal("refused write must leave no file behind")
	}
}

func TestWriteNotebooksRefusesBreakingSiblingRoots(t *testing.T) {
	rootsPath, nbPath := tmpPair(t)
	def := "nb"
	if _, err := WriteNotebooks(nbPath, NotebookEdits{
		Default: &def,
		Upserts: map[string]coderoot.Notebook{"nb": {Root: "/n"}, "extra": {Root: "/e"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteCodeRoots(rootsPath, CodeRootEdits{
		Upserts: map[string]coderoot.Root{"code": {Path: "/code", Notebook: "extra"}},
	}); err != nil {
		t.Fatal(err)
	}

	// Deleting the notebook a recorded root routes to must be refused: the
	// full recorded set is re-parsed before anything is persisted.
	before := mustRead(t, nbPath)
	_, err := WriteNotebooks(nbPath, NotebookEdits{Deletes: []string{"extra"}})
	if err == nil || !strings.Contains(err.Error(), "would not reload") {
		t.Fatalf("expected cross-file refusal, got %v", err)
	}
	if got := mustRead(t, nbPath); got != before {
		t.Fatal("refused write must leave the file untouched")
	}
}
