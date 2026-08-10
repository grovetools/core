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
		return cfg
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
		cfg.Notebooks.Definitions = make(map[string]*Notebook, len(table.Notebooks))
		for _, name := range table.SortedNotebookNames() {
			// A recorded definition replaces the same-name legacy definition;
			// stale templates, note types, and sync integrations must not leak
			// beneath notebooks.toml's intentionally smaller schema.
			cfg.Notebooks.Definitions[name] = &Notebook{RootDir: table.NotebookRoot(name)}
		}
		if cfg.Notebooks.Rules == nil {
			cfg.Notebooks.Rules = &NotebookRules{}
		} else {
			rulesCopy := *cfg.Notebooks.Rules
			cfg.Notebooks.Rules = &rulesCopy
		}
		cfg.Notebooks.Rules.Default = table.Default
	}
	return cfg
}
