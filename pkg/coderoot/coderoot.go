// Package coderoot owns the two recorded config files of the code plane:
//
//	~/.config/grove/roots.toml      [roots.<name>] — where code lives
//	~/.config/grove/notebooks.toml  [notebooks.<name>] — where notes live
//
// Together they form the machine's routing table: every code root names the
// notebook its projects write into, and every notebook records its root
// directory. Nothing in this package guesses — a name that resolves to no
// definition, a duplicate, or an overlapping declaration is a hard error
// naming the file and table, never a fallback.
//
// The two files are loaded standalone (excluded from the global config
// fragment glob, like machine.toml) and compile into the internal
// cfg.Groves / cfg.Notebooks views at config load time; see
// core/config.compileCodeRoots.
package coderoot

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/util/pathutil"
)

// File basenames, exported so the config loader can exclude them from the
// fragment glob and the satellite seeder can denylist them.
const (
	RootsFileName     = "roots.toml"
	NotebooksFileName = "notebooks.toml"
)

// RootsPath returns the canonical path of roots.toml, or "" when no config
// directory can be resolved.
func RootsPath() string {
	configDir := paths.ConfigDir()
	if configDir == "" {
		return ""
	}
	return filepath.Join(configDir, RootsFileName)
}

// NotebooksPath returns the canonical path of notebooks.toml, or "" when no
// config directory can be resolved.
func NotebooksPath() string {
	configDir := paths.ConfigDir()
	if configDir == "" {
		return ""
	}
	return filepath.Join(configDir, NotebooksFileName)
}

// Root is one [roots.<name>] table: a declaration of where code lives.
// A scan root is a directory repos are discovered beneath; a specific tree
// (scan absent/false) is an ecosystem (manifest present) or a single repo.
type Root struct {
	// Path is where the root lives on this machine (~ and ${VAR} expanded at
	// load time; the declared spelling is preserved here).
	Path string `toml:"path" jsonschema:"description=Path to this code root on this machine"`
	// Scan marks a scan root: repos are discovered beneath Path. False or
	// absent means a specific tree — an ecosystem or a single repo.
	Scan bool `toml:"scan,omitempty" jsonschema:"description=Scan root: repos are discovered beneath the path (default: false — the path is one specific tree)"`
	// Notebook routes this root's projects to a notebook NAME, resolved
	// literally against notebooks.toml. Empty falls back to the recorded
	// default notebook.
	Notebook string `toml:"notebook,omitempty" jsonschema:"description=Notebook name this root's projects write into (resolved against notebooks.toml)"`
	// Repos narrows a specific tree to the named member repositories; empty
	// means every member. Carried from the machine-subscription model so a
	// machine can still materialize a subset of an ecosystem.
	Repos []string `toml:"repos,omitempty" jsonschema:"description=Member repositories to include (empty means all)"`
	// Exclude omits repo directory names: discovered repos under a scan root,
	// member repos of a specific tree.
	Exclude []string `toml:"exclude,omitempty" jsonschema:"description=Repository directory names to omit"`
	// Depth bounds how many directory levels a scan root is scanned.
	Depth *int `toml:"depth,omitempty" jsonschema:"description=Directory levels to scan below a scan root"`
	// Enabled disables a root without deleting it. Nil = enabled.
	Enabled *bool `toml:"enabled,omitempty" jsonschema:"description=Whether this root is active (default: true)"`
	// Description is a human label.
	Description string `toml:"description,omitempty" jsonschema:"description=Human-readable description"`
}

// IncludesRepo reports whether a member/discovered repo name belongs to this
// root, honoring the repos/exclude narrowing.
func (r Root) IncludesRepo(name string) bool {
	if len(r.Repos) > 0 {
		return containsString(r.Repos, name)
	}
	return !containsString(r.Exclude, name)
}

// Notebook is one [notebooks.<name>] table: a recorded notebook definition.
type Notebook struct {
	// Root is the notebook's root directory.
	Root string `toml:"root" jsonschema:"description=Absolute path to the notebook root directory"`
	// Sync is [notebooks.<name>.sync] — the notebook-grained sync scope model.
	// Absent means "never recorded"; present means the answer was written down.
	Sync *NotebookSync `toml:"sync,omitempty" jsonschema:"-"`
}

// NotebookSync is one [notebooks.<name>.sync] table.
//
// The notebook is the ONLY sync knob: there is no per-notespace toggle and no
// derived share state anywhere. A notespace is shared because the notebook
// containing it is shared — containment is consent — so this table has exactly
// one recorded key, and any other key is a hard error naming file and table.
//
// The table is tri-state on purpose, and the third state is not a nuance:
// absent means "never recorded", `share = true` means shared, and an explicit
// `share = false` means recorded-as-unshared. Unshare is forward-only (D9) —
// the server retains history and copies pulled elsewhere are never retracted —
// so "we deliberately stopped sharing this" is a fact the file has to be able
// to state, not one a reader may infer from silence.
type NotebookSync struct {
	// Share records whether this notebook's notespaces are shared with the
	// recorded sync server.
	Share bool `toml:"share" jsonschema:"-"`
}

// notebookSyncKeys is the closed key set of [notebooks.<name>.sync]. Parsing
// checks raw TOML against it so the error names the file and table rather than
// surfacing the decoder's generic strict-mode complaint.
var notebookSyncKeys = map[string]struct{}{"share": {}}

// Shared reports whether this notebook is recorded as shared. A notebook with
// no recorded sync table is not shared.
func (n Notebook) Shared() bool { return n.Sync != nil && n.Sync.Share }

// SyncRecorded reports whether [notebooks.<name>.sync] exists at all, which is
// distinct from being shared: a notebook unshared per D9 records
// `share = false` and must not read as "never considered".
func (n Notebook) SyncRecorded() bool { return n.Sync != nil }

// RootsFile is the typed shape of roots.toml.
type RootsFile struct {
	Roots map[string]Root `toml:"roots"`
}

// NotebooksFile is the typed shape of notebooks.toml.
type NotebooksFile struct {
	// Default names the notebook used when nothing more specific routes a
	// path. It must name a defined notebook.
	Default   string              `toml:"default"`
	Notebooks map[string]Notebook `toml:"notebooks"`
}

// Table is the loaded, cross-validated pair. Zero value means "nothing
// recorded" (neither file exists).
type Table struct {
	Roots     map[string]Root
	Notebooks map[string]Notebook
	// Default is notebooks.toml's default key.
	Default string
	// RootsFilePath / NotebooksFilePath are the files the table was loaded
	// from ("" when the file does not exist).
	RootsFilePath     string
	NotebooksFilePath string
}

// HasRoots reports whether roots.toml exists (the marker the loader guard
// keys on: legacy config with no roots.toml means `grove migrate` has not
// run).
func (t Table) HasRoots() bool { return t.RootsFilePath != "" }

// NotebookRoot resolves a notebook name to its recorded, expanded root
// directory. Empty name resolves the default. Returns "" when the name (or
// the default) names no definition — the caller decides whether that is
// fatal.
func (t Table) NotebookRoot(name string) string {
	if name == "" {
		name = t.Default
	}
	if name == "" {
		return ""
	}
	nb, ok := t.Notebooks[name]
	if !ok {
		return ""
	}
	return expandPath(nb.Root)
}

// RootNotebook returns the notebook name a root routes to: its own binding,
// else the recorded default.
func (t Table) RootNotebook(name string) string {
	if r, ok := t.Roots[name]; ok && r.Notebook != "" {
		return r.Notebook
	}
	return t.Default
}

// SortedRootNames returns the root names in deterministic order.
func (t Table) SortedRootNames() []string {
	return sortedKeys(t.Roots)
}

// SortedNotebookNames returns the notebook names in deterministic order.
func (t Table) SortedNotebookNames() []string {
	return sortedKeys(t.Notebooks)
}

// NotebookShared reports whether the named notebook is recorded as shared.
// An unknown name is not shared — the caller that needs the difference between
// "not shared" and "not defined" asks Notebooks directly.
func (t Table) NotebookShared(name string) bool {
	nb, ok := t.Notebooks[name]
	return ok && nb.Shared()
}

// SharedNotebookNames returns, in deterministic order, the notebooks recorded
// as shared. This is the containment boundary: everything inside one of these
// notebooks is in scope for sync, and nothing outside them is.
func (t Table) SharedNotebookNames() []string {
	out := []string{}
	for _, name := range sortedKeys(t.Notebooks) {
		if t.Notebooks[name].Shared() {
			out = append(out, name)
		}
	}
	return out
}

// DeepestRootFor returns the name of the most specific enabled root whose
// path contains the given path — "deepest root wins" — or "" when no root
// covers it. Nested roots are legal; the deepest declaration decides.
func (t Table) DeepestRootFor(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(expandPath(path))
	if err != nil {
		abs = path
	}
	best, bestLen := "", -1
	for _, name := range sortedKeys(t.Roots) {
		r := t.Roots[name]
		if r.Enabled != nil && !*r.Enabled {
			continue
		}
		rootAbs, err := filepath.Abs(expandPath(r.Path))
		if err != nil {
			continue
		}
		if abs != rootAbs && !strings.HasPrefix(abs, rootAbs+string(filepath.Separator)) {
			continue
		}
		if len(rootAbs) > bestLen {
			bestLen = len(rootAbs)
			best = name
		}
	}
	return best
}

// Load reads and cross-validates the canonical roots.toml + notebooks.toml
// pair. Missing files are not errors — the corresponding maps are empty and
// the path fields are "" — because "nothing recorded" is a legitimate state
// (fresh machine, pre-migration sandbox). Everything else fails loudly.
func Load() (Table, error) {
	return LoadFrom(RootsPath(), NotebooksPath())
}

// LoadFrom loads the pair from explicit paths (tests and sandboxed loads).
func LoadFrom(rootsPath, notebooksPath string) (Table, error) {
	table := Table{
		Roots:     map[string]Root{},
		Notebooks: map[string]Notebook{},
	}

	if rootsPath != "" {
		data, err := os.ReadFile(rootsPath)
		switch {
		case err == nil:
			rf, err := ParseRoots(rootsPath, data)
			if err != nil {
				return Table{}, err
			}
			table.Roots = rf.Roots
			table.RootsFilePath = rootsPath
		case !os.IsNotExist(err):
			return Table{}, fmt.Errorf("failed to read %s: %w", rootsPath, err)
		}
	}

	if notebooksPath != "" {
		data, err := os.ReadFile(notebooksPath)
		switch {
		case err == nil:
			nf, err := ParseNotebooks(notebooksPath, data)
			if err != nil {
				return Table{}, err
			}
			table.Notebooks = nf.Notebooks
			table.Default = nf.Default
			table.NotebooksFilePath = notebooksPath
		case !os.IsNotExist(err):
			return Table{}, fmt.Errorf("failed to read %s: %w", notebooksPath, err)
		}
	}

	if err := table.Validate(); err != nil {
		return Table{}, err
	}
	return table, nil
}

// ParseRoots decodes roots.toml content. path is used in error messages only.
// Duplicate tables/keys are rejected by the TOML parser itself.
func ParseRoots(path string, data []byte) (RootsFile, error) {
	var rf RootsFile
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&rf); err != nil {
		return RootsFile{}, fmt.Errorf("%s: %w", displayName(path, RootsFileName), err)
	}
	if rf.Roots == nil {
		rf.Roots = map[string]Root{}
	}
	for _, name := range sortedKeys(rf.Roots) {
		if strings.TrimSpace(name) == "" {
			return RootsFile{}, fmt.Errorf("%s: [roots] has an entry with an empty name", displayName(path, RootsFileName))
		}
		if strings.TrimSpace(rf.Roots[name].Path) == "" {
			return RootsFile{}, fmt.Errorf("%s: [roots.%s] has no path", displayName(path, RootsFileName), name)
		}
		r := rf.Roots[name]
		if len(r.Repos) > 0 && len(r.Exclude) > 0 {
			return RootsFile{}, fmt.Errorf("%s: [roots.%s] cannot set both repos and exclude", displayName(path, RootsFileName), name)
		}
	}
	return rf, nil
}

// ParseNotebooks decodes notebooks.toml content. path is used in error
// messages only.
func ParseNotebooks(path string, data []byte) (NotebooksFile, error) {
	// The sync table is checked against its closed key set first so a stray
	// key is reported as "[notebooks.x.sync] accepts only share" rather than
	// as the decoder's generic strict-mode message, which names neither the
	// file nor the table an operator has to go fix.
	if err := checkNotebookSyncKeys(path, data); err != nil {
		return NotebooksFile{}, err
	}
	var nf NotebooksFile
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&nf); err != nil {
		return NotebooksFile{}, fmt.Errorf("%s: %w", displayName(path, NotebooksFileName), err)
	}
	if nf.Notebooks == nil {
		nf.Notebooks = map[string]Notebook{}
	}
	for _, name := range sortedKeys(nf.Notebooks) {
		if strings.TrimSpace(name) == "" {
			return NotebooksFile{}, fmt.Errorf("%s: [notebooks] has an entry with an empty name", displayName(path, NotebooksFileName))
		}
		nb := nf.Notebooks[name]
		if strings.TrimSpace(nb.Root) == "" {
			return NotebooksFile{}, fmt.Errorf("%s: [notebooks.%s] has no root", displayName(path, NotebooksFileName), name)
		}
	}
	return nf, nil
}

// checkNotebookSyncKeys validates every [notebooks.<name>.sync] table against
// the closed key set, before the typed decode, so the diagnostic names the
// file and the table.
func checkNotebookSyncKeys(path string, data []byte) error {
	var loose struct {
		Notebooks map[string]map[string]any `toml:"notebooks"`
	}
	if err := toml.Unmarshal(data, &loose); err != nil {
		// A syntax error is the typed decode's to report, with its position.
		return nil
	}
	for _, name := range sortedKeys(loose.Notebooks) {
		raw, ok := loose.Notebooks[name]["sync"]
		if !ok {
			continue
		}
		table, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: [notebooks.%s.sync] must be a table", displayName(path, NotebooksFileName), name)
		}
		var unknown []string
		for key := range table {
			if _, known := notebookSyncKeys[key]; !known {
				unknown = append(unknown, key)
			}
		}
		if len(unknown) > 0 {
			sort.Strings(unknown)
			return fmt.Errorf("%s: [notebooks.%s.sync] accepts only share (found %s)",
				displayName(path, NotebooksFileName), name, strings.Join(unknown, ", "))
		}
		if share, present := table["share"]; present {
			if _, isBool := share.(bool); !isBool {
				return fmt.Errorf("%s: [notebooks.%s.sync] share must be a boolean, got %T", displayName(path, NotebooksFileName), name, share)
			}
		}
	}
	return nil
}

// Validate cross-checks the pair: every notebook reference resolves, the
// default names a definition, and no two roots claim the same path
// (nested roots are legal — deepest wins — but an identical path declared
// twice has no deepest).
func (t Table) Validate() error {
	nbFile := t.NotebooksFilePath
	if nbFile == "" {
		nbFile = NotebooksFileName
	}
	rootsFile := t.RootsFilePath
	if rootsFile == "" {
		rootsFile = RootsFileName
	}

	if t.Default != "" {
		if _, ok := t.Notebooks[t.Default]; !ok {
			return fmt.Errorf("%s: default = %q names no [notebooks.%s] definition", nbFile, t.Default, t.Default)
		}
	}

	byPath := map[string]string{}
	for _, name := range sortedKeys(t.Roots) {
		r := t.Roots[name]
		if r.Notebook != "" {
			if _, ok := t.Notebooks[r.Notebook]; !ok {
				return fmt.Errorf("%s: [roots.%s] notebook = %q names no [notebooks.%s] definition in %s",
					rootsFile, name, r.Notebook, r.Notebook, nbFile)
			}
		}
		abs, err := filepath.Abs(expandPath(r.Path))
		if err != nil {
			abs = expandPath(r.Path)
		}
		if prev, dup := byPath[abs]; dup {
			return fmt.Errorf("%s: [roots.%s] and [roots.%s] both declare path %s; a path may have one owning root",
				rootsFile, prev, name, abs)
		}
		byPath[abs] = name
	}

	// A root with no binding of its own and no recorded default cannot route
	// anywhere; recording the answer now beats guessing one later.
	if t.Default == "" {
		for _, name := range sortedKeys(t.Roots) {
			if t.Roots[name].Notebook == "" {
				return fmt.Errorf("%s: [roots.%s] declares no notebook and %s records no default notebook", rootsFile, name, nbFile)
			}
		}
	}

	return nil
}

// ExpandPath resolves a DECLARED path — the spelling as written in roots.toml
// or notebooks.toml — into a path this process can use: ${VAR} references and a
// leading ~ expanded. It is the one expander for the recorded routing files, so
// a consumer that needs a usable path has a single obvious call rather than a
// hand-rolled copy.
//
// The declared spelling is preserved in the Root/Notebook structs and in the
// files themselves; nothing here writes back. Expansion is idempotent, so a
// path that is already absolute passes through untouched and calling it twice
// is harmless.
//
// It is exported because forgetting it is a whole class of silent bug:
// filepath.Abs("~/code/x") does not fail, it yields "<cwd>/~/code/x", and every
// stat, EvalSymlinks and string comparison downstream then quietly answers
// about a directory that does not exist.
func ExpandPath(path string) string {
	if path == "" {
		return ""
	}
	path = os.ExpandEnv(path)
	if expanded, err := pathutil.Expand(path); err == nil {
		return expanded
	}
	return path
}

// expandPath is the package-internal spelling of ExpandPath.
func expandPath(path string) string { return ExpandPath(path) }

// displayName renders path for error messages, falling back to the canonical
// basename when the caller had no real path.
func displayName(path, fallback string) string {
	if strings.TrimSpace(path) == "" {
		return fallback
	}
	return path
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
