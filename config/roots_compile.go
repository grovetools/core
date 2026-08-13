package config

import (
	"github.com/grovetools/core/pkg/coderoot"
)

// compileCodeRoots projects the canonical recorded routing pair into the
// compatibility views consumed by the rest of core. The recorded files are
// authoritative for names they contain; legacy views remain only for the
// migration window and are compiled first by callers.
func compileCodeRoots(cfg *Config) (*Config, error) {
	table, err := loadCodeRootsForCompile()
	if err != nil {
		return nil, err
	}
	return compileCodeRootTable(cfg, table), nil
}

// loadCodeRootsForCompile is the single canonical read used by config loads.
func loadCodeRootsForCompile() (coderoot.Table, error) { return coderoot.Load() }

// compileCodeRootsFromPaths is the explicit-path test hook. Tests must not
// depend on a developer's ambient ~/.config/grove routing files.
func compileCodeRootsFromPaths(cfg *Config, rootsPath, notebooksPath string) (*Config, error) {
	table, err := coderoot.LoadFrom(rootsPath, notebooksPath)
	if err != nil {
		return nil, err
	}
	return compileCodeRootTable(cfg, table), nil
}

func compileCodeRootTable(cfg *Config, table coderoot.Table) *Config {
	if len(table.Roots) == 0 && table.NotebooksFilePath == "" {
		return resolveNotebookRoots(cfg)
	}
	if cfg == nil {
		cfg = &Config{}
	}

	// Never mutate a merged layer in place: LoadLayered may use its Global
	// pointer as the starting Final view. Runtime compilation belongs only in
	// the returned view, not in source-attributed layers.
	out := *cfg
	out.Groves = make(map[string]GroveSourceConfig, len(cfg.Groves)+len(table.Roots))
	for name, grove := range cfg.Groves {
		out.Groves[name] = grove
	}
	cfg = &out
	for _, name := range table.SortedRootNames() {
		r := table.Roots[name]
		notebook := table.RootNotebook(name)
		cfg.Groves[name] = GroveSourceConfig{
			Path:         expandPath(r.Path),
			Enabled:      r.Enabled,
			Description:  r.Description,
			Notebook:     notebook,
			NotebookRoot: table.NotebookRoot(notebook),
			Depth:        r.Depth,
			IncludeRepos: append([]string(nil), r.Repos...),
			ExcludeRepos: append([]string(nil), r.Exclude...),
			Scan:         r.Scan,
		}
	}

	// The existence of notebooks.toml is the authority marker. An explicitly
	// empty recorded table replaces legacy global definitions just as a
	// populated one does.
	if table.NotebooksFilePath != "" {
		if cfg.Notebooks == nil {
			cfg.Notebooks = &NotebooksConfig{}
		} else {
			notebooksCopy := *cfg.Notebooks
			cfg.Notebooks = &notebooksCopy
		}
		// notebooks.toml owns membership and routing roots. During the migration
		// window, same-name global definitions still own the orthogonal notebook
		// behavior that the recorded schema cannot express (templates, types,
		// sync and Obsidian settings). Preserve those fields while overriding the
		// root; definitions absent from the recorded file remain removed.
		//
		// Shared is projected the same way and for the same reason as RootDir:
		// the recorded pair owns `[notebooks.<name>.sync] share`, and the
		// daemon's containment rule ("a notespace is shared because its
		// notebook is") needs that bit in the view it already reads rather
		// than a second read of notebooks.toml from inside the daemon. It is
		// assigned unconditionally — a legacy same-name definition can carry
		// no share state, so there is nothing to preserve and nothing to leak.
		legacyDefinitions := cfg.Notebooks.Definitions
		cfg.Notebooks.Definitions = make(map[string]*Notebook, len(table.Notebooks))
		for _, name := range table.SortedNotebookNames() {
			notebook := &Notebook{}
			if legacy := legacyDefinitions[name]; legacy != nil {
				copy := *legacy
				notebook = &copy
			}
			notebook.RootDir = table.NotebookRoot(name)
			notebook.Shared = table.Notebooks[name].Shared()
			cfg.Notebooks.Definitions[name] = notebook
		}
		if cfg.Notebooks.Rules == nil {
			cfg.Notebooks.Rules = &NotebookRules{}
		} else {
			rulesCopy := *cfg.Notebooks.Rules
			cfg.Notebooks.Rules = &rulesCopy
		}
		cfg.Notebooks.Rules.Default = table.Default
	}
	return resolveNotebookRoots(cfg)
}

// resolveNotebookRoots guarantees the compiled view's notebook roots are
// USABLE paths, never declared spellings.
//
// The recorded branch above already resolves every root through
// Table.NotebookRoot. This closes the other branch: when notebooks.toml is
// absent — a pre-migration machine, a sandbox, a satellite seeded without it —
// Definitions still carries whatever the legacy layer merged in, and
// notebooks.legacy-compat.toml writes that as root_dir = '~/notebooks/<name>'.
// A consumer holding one of those gets no error from filepath.Join or
// os.Stat, only a wrong answer about a directory that cannot exist.
//
// Consumers may still expand defensively — expansion is idempotent — but after
// this they no longer have to remember to.
func resolveNotebookRoots(cfg *Config) *Config {
	if cfg == nil || cfg.Notebooks == nil {
		return cfg
	}
	declared := func(path string) bool { return path != "" && coderoot.ExpandPath(path) != path }
	needed := false
	for _, definition := range cfg.Notebooks.Definitions {
		if definition != nil && declared(definition.RootDir) {
			needed = true
			break
		}
	}
	if rules := cfg.Notebooks.Rules; !needed && rules != nil && rules.Global != nil && declared(rules.Global.RootDir) {
		needed = true
	}
	// Copy-on-write, for the same reason compileCodeRootTable does: cfg may be
	// a source-attributed layer that LoadLayered still hands out separately.
	if !needed {
		return cfg
	}

	out := *cfg
	notebooks := *cfg.Notebooks
	out.Notebooks = &notebooks
	if cfg.Notebooks.Definitions != nil {
		definitions := make(map[string]*Notebook, len(cfg.Notebooks.Definitions))
		for name, definition := range cfg.Notebooks.Definitions {
			if definition == nil {
				definitions[name] = nil
				continue
			}
			resolved := *definition
			resolved.RootDir = coderoot.ExpandPath(resolved.RootDir)
			definitions[name] = &resolved
		}
		notebooks.Definitions = definitions
	}
	if cfg.Notebooks.Rules != nil {
		rules := *cfg.Notebooks.Rules
		if rules.Global != nil {
			global := *rules.Global
			global.RootDir = coderoot.ExpandPath(global.RootDir)
			rules.Global = &global
		}
		notebooks.Rules = &rules
	}
	return &out
}
