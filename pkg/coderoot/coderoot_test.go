package coderoot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadFromMissingFilesIsEmptyNotError(t *testing.T) {
	dir := t.TempDir()
	table, err := LoadFrom(filepath.Join(dir, RootsFileName), filepath.Join(dir, NotebooksFileName))
	if err != nil {
		t.Fatalf("missing files must not error: %v", err)
	}
	if table.HasRoots() {
		t.Fatal("HasRoots must be false when roots.toml does not exist")
	}
	if len(table.Roots) != 0 || len(table.Notebooks) != 0 || table.Default != "" {
		t.Fatalf("expected empty table, got %+v", table)
	}
}

func TestLoadFromValidPair(t *testing.T) {
	dir := t.TempDir()
	rootsPath := filepath.Join(dir, RootsFileName)
	nbPath := filepath.Join(dir, NotebooksFileName)
	writeFile(t, nbPath, `default = "nb"

[notebooks.nb]
root = "`+dir+`/notebooks/nb"

[notebooks.grovetools]
root = "`+dir+`/notebooks/grovetools"
`)
	writeFile(t, rootsPath, `[roots.code]
path = "`+dir+`/code"
scan = true
notebook = "nb"
exclude = ["vendor-fork"]
depth = 2

[roots.grovetools]
path = "`+dir+`/code/grovetools"
notebook = "grovetools"
`)

	table, err := LoadFrom(rootsPath, nbPath)
	if err != nil {
		t.Fatal(err)
	}
	if !table.HasRoots() {
		t.Fatal("HasRoots must be true")
	}
	if got := table.Default; got != "nb" {
		t.Fatalf("Default = %q, want nb", got)
	}
	code := table.Roots["code"]
	if !code.Scan || code.Notebook != "nb" || code.Depth == nil || *code.Depth != 2 {
		t.Fatalf("code root decoded wrong: %+v", code)
	}
	if got := table.NotebookRoot("grovetools"); got != dir+"/notebooks/grovetools" {
		t.Fatalf("NotebookRoot(grovetools) = %q", got)
	}
	if got := table.NotebookRoot(""); got != dir+"/notebooks/nb" {
		t.Fatalf("NotebookRoot(default) = %q", got)
	}
	if got := table.RootNotebook("grovetools"); got != "grovetools" {
		t.Fatalf("RootNotebook(grovetools) = %q", got)
	}
}

func TestParseRootsRejections(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "duplicate table",
			content: "[roots.a]\npath = \"/x\"\n[roots.a]\npath = \"/y\"\n",
			want:    "already exists",
		},
		{
			name:    "duplicate key",
			content: "[roots.a]\npath = \"/x\"\npath = \"/y\"\n",
			want:    "already defined",
		},
		{
			name:    "missing path",
			content: "[roots.a]\nnotebook = \"nb\"\n",
			want:    "[roots.a] has no path",
		},
		{
			name:    "repos and exclude together",
			content: "[roots.a]\npath = \"/x\"\nrepos = [\"m\"]\nexclude = [\"n\"]\n",
			want:    "cannot set both repos and exclude",
		},
		{
			name:    "unknown root field",
			content: "[roots.a]\npath = \"/x\"\nnotebok = \"nb\"\n",
			want:    "strict mode",
		},
		{
			name:    "unknown top-level table",
			content: "[roots.a]\npath = \"/x\"\n[stray]\nvalue = true\n",
			want:    "strict mode",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseRoots("roots.toml", []byte(tc.content))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestParseNotebooksRejections(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "missing root",
			content: "[notebooks.nb]\n",
			want:    "[notebooks.nb] has no root",
		},
		{
			name:    "reserved sync table",
			content: "[notebooks.nb]\nroot = \"/x\"\n[notebooks.nb.sync]\nshare = true\n",
			want:    "[notebooks.nb.sync] is reserved",
		},
		{
			name:    "duplicate notebook table",
			content: "[notebooks.nb]\nroot = \"/x\"\n[notebooks.nb]\nroot = \"/y\"\n",
			want:    "already exists",
		},
		{
			name:    "unknown notebook field",
			content: "[notebooks.nb]\nroot = \"/x\"\nrot = \"/typo\"\n",
			want:    "strict mode",
		},
		{
			name:    "unknown top-level table",
			content: "[notebooks.nb]\nroot = \"/x\"\n[stray]\nvalue = true\n",
			want:    "strict mode",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseNotebooks("notebooks.toml", []byte(tc.content))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestParseNotebooksAllowsEmptyReservedSyncTable(t *testing.T) {
	nf, err := ParseNotebooks("notebooks.toml", []byte("[notebooks.nb]\nroot = \"/x\"\n[notebooks.nb.sync]\n"))
	if err != nil {
		t.Fatalf("empty reserved sync table must remain accepted: %v", err)
	}
	if _, ok := nf.Notebooks["nb"]; !ok {
		t.Fatal("notebook containing an empty reserved sync table was lost")
	}
}

func TestValidateCrossChecks(t *testing.T) {
	dir := t.TempDir()
	rootsPath := filepath.Join(dir, RootsFileName)
	nbPath := filepath.Join(dir, NotebooksFileName)

	t.Run("unresolvable notebook ref", func(t *testing.T) {
		writeFile(t, nbPath, "default = \"nb\"\n[notebooks.nb]\nroot = \"/n\"\n")
		writeFile(t, rootsPath, "[roots.a]\npath = \"/x\"\nnotebook = \"ghost\"\n")
		_, err := LoadFrom(rootsPath, nbPath)
		if err == nil || !strings.Contains(err.Error(), `notebook = "ghost" names no`) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("default names no definition", func(t *testing.T) {
		writeFile(t, nbPath, "default = \"ghost\"\n[notebooks.nb]\nroot = \"/n\"\n")
		writeFile(t, rootsPath, "[roots.a]\npath = \"/x\"\nnotebook = \"nb\"\n")
		_, err := LoadFrom(rootsPath, nbPath)
		if err == nil || !strings.Contains(err.Error(), `default = "ghost" names no`) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("two roots one path", func(t *testing.T) {
		writeFile(t, nbPath, "default = \"nb\"\n[notebooks.nb]\nroot = \"/n\"\n")
		writeFile(t, rootsPath, "[roots.a]\npath = \"/same\"\n[roots.b]\npath = \"/same\"\n")
		_, err := LoadFrom(rootsPath, nbPath)
		if err == nil || !strings.Contains(err.Error(), "both declare path") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("unroutable root: no binding and no default", func(t *testing.T) {
		writeFile(t, nbPath, "[notebooks.nb]\nroot = \"/n\"\n")
		writeFile(t, rootsPath, "[roots.a]\npath = \"/x\"\n")
		_, err := LoadFrom(rootsPath, nbPath)
		if err == nil || !strings.Contains(err.Error(), "records no default notebook") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("nested roots are legal", func(t *testing.T) {
		writeFile(t, nbPath, "default = \"nb\"\n[notebooks.nb]\nroot = \"/n\"\n")
		writeFile(t, rootsPath, "[roots.code]\npath = \"/code\"\nscan = true\n[roots.eco]\npath = \"/code/eco\"\n")
		if _, err := LoadFrom(rootsPath, nbPath); err != nil {
			t.Fatalf("nested roots must validate: %v", err)
		}
	})
}

func TestDeepestRootWins(t *testing.T) {
	off := false
	table := Table{
		Default: "nb",
		Roots: map[string]Root{
			"code":     {Path: "/code", Scan: true},
			"eco":      {Path: "/code/eco"},
			"disabled": {Path: "/code/eco/member", Enabled: &off},
		},
		Notebooks: map[string]Notebook{"nb": {Root: "/n"}},
	}

	if got := table.DeepestRootFor("/code/other/repo"); got != "code" {
		t.Fatalf("DeepestRootFor(/code/other/repo) = %q, want code", got)
	}
	if got := table.DeepestRootFor("/code/eco/member/file.go"); got != "eco" {
		t.Fatalf("deepest enabled root must win over shallower and over disabled deeper: got %q", got)
	}
	if got := table.DeepestRootFor("/code/eco"); got != "eco" {
		t.Fatalf("a root contains its own path: got %q", got)
	}
	if got := table.DeepestRootFor("/elsewhere"); got != "" {
		t.Fatalf("uncovered path must resolve to no root, got %q", got)
	}
}

func TestIncludesRepo(t *testing.T) {
	if !(Root{}).IncludesRepo("anything") {
		t.Fatal("no narrowing includes everything")
	}
	r := Root{Repos: []string{"core", "grove"}}
	if !r.IncludesRepo("core") || r.IncludesRepo("nb") {
		t.Fatal("repos narrowing wrong")
	}
	r = Root{Exclude: []string{"vendor-fork"}}
	if r.IncludesRepo("vendor-fork") || !r.IncludesRepo("core") {
		t.Fatal("exclude narrowing wrong")
	}
}

func TestNotebookRootExpandsTilde(t *testing.T) {
	table := Table{
		Default:   "nb",
		Notebooks: map[string]Notebook{"nb": {Root: "~/notebooks/nb"}},
	}
	got := table.NotebookRoot("nb")
	if strings.HasPrefix(got, "~") {
		t.Fatalf("NotebookRoot must expand ~: %q", got)
	}
	home, err := os.UserHomeDir()
	if err == nil && got != filepath.Join(home, "notebooks/nb") {
		t.Fatalf("NotebookRoot = %q, want under %q", got, home)
	}
}
