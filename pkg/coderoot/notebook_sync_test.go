package coderoot

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseNotebooksSyncShare(t *testing.T) {
	cases := []struct {
		name       string
		content    string
		wantShared bool
		wantTable  bool
	}{
		{
			name:      "no sync table is not shared and not recorded",
			content:   "[notebooks.nb]\nroot = \"/x\"\n",
			wantTable: false,
		},
		{
			name:       "share true",
			content:    "[notebooks.nb]\nroot = \"/x\"\n[notebooks.nb.sync]\nshare = true\n",
			wantShared: true,
			wantTable:  true,
		},
		{
			// An unshared notebook is a recorded fact, not an absence: D9's
			// forward-only unshare has to be distinguishable from a notebook
			// nobody ever considered sharing.
			name:      "share false is recorded, not shared",
			content:   "[notebooks.nb]\nroot = \"/x\"\n[notebooks.nb.sync]\nshare = false\n",
			wantTable: true,
		},
		{
			name:      "empty sync table stays accepted",
			content:   "[notebooks.nb]\nroot = \"/x\"\n[notebooks.nb.sync]\n",
			wantTable: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nf, err := ParseNotebooks("notebooks.toml", []byte(tc.content))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			nb, ok := nf.Notebooks["nb"]
			if !ok {
				t.Fatal("notebook lost")
			}
			if nb.Shared() != tc.wantShared {
				t.Fatalf("Shared() = %t, want %t", nb.Shared(), tc.wantShared)
			}
			if nb.SyncRecorded() != tc.wantTable {
				t.Fatalf("SyncRecorded() = %t, want %t", nb.SyncRecorded(), tc.wantTable)
			}
		})
	}
}

func TestParseNotebooksSyncRejectsUnrecordedKeysByNameAndTable(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "unknown key",
			content: "[notebooks.nb]\nroot = \"/x\"\n[notebooks.nb.sync]\nshare = true\nserver = \"https://example\"\n",
			want:    []string{"notebooks.toml", "[notebooks.nb.sync]", "server"},
		},
		{
			name:    "several unknown keys are all named, ordered",
			content: "[notebooks.nb]\nroot = \"/x\"\n[notebooks.nb.sync]\nzeta = 1\nalpha = 2\n",
			want:    []string{"[notebooks.nb.sync]", "alpha, zeta"},
		},
		{
			name:    "share must be a boolean",
			content: "[notebooks.nb]\nroot = \"/x\"\n[notebooks.nb.sync]\nshare = \"yes\"\n",
			want:    []string{"[notebooks.nb.sync]", "share must be a boolean"},
		},
		{
			name:    "sync must be a table",
			content: "[notebooks.nb]\nroot = \"/x\"\nsync = true\n",
			want:    []string{"[notebooks.nb.sync]", "must be a table"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseNotebooks("notebooks.toml", []byte(tc.content))
			if err == nil {
				t.Fatal("expected a hard error")
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error %q must name %q", err, want)
				}
			}
		})
	}
}

func TestTableSharedNotebookQueries(t *testing.T) {
	dir := t.TempDir()
	rootsPath := filepath.Join(dir, RootsFileName)
	nbPath := filepath.Join(dir, NotebooksFileName)
	writeFile(t, nbPath, `default = "nb"

[notebooks.nb]
root = "/notebooks/nb"

[notebooks.nb.sync]
share = true

[notebooks.personal]
root = "/notebooks/personal"

[notebooks.retired]
root = "/notebooks/retired"

[notebooks.retired.sync]
share = false
`)
	writeFile(t, rootsPath, "[roots.code]\npath = \"/code\"\nnotebook = \"nb\"\n")

	table, err := LoadFrom(rootsPath, nbPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := table.SharedNotebookNames(); len(got) != 1 || got[0] != "nb" {
		t.Fatalf("SharedNotebookNames() = %v, want [nb]", got)
	}
	for name, want := range map[string]bool{"nb": true, "personal": false, "retired": false, "ghost": false} {
		if got := table.NotebookShared(name); got != want {
			t.Fatalf("NotebookShared(%q) = %t, want %t", name, got, want)
		}
	}
	if table.Notebooks["retired"].SyncRecorded() == table.Notebooks["personal"].SyncRecorded() {
		t.Fatal("an unshared notebook must be distinguishable from one with no recorded sync table")
	}
}
