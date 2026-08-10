package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/pelletier/go-toml/v2"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	"github.com/grovetools/core/errors"
	"github.com/grovetools/core/pkg/coderoot"
	"github.com/grovetools/core/pkg/paths"
	"github.com/grovetools/core/pkg/plugin"
)

// ResetLoadCache clears both config caches: LoadFromWithLogger's per-startDir
// memo (see loadfromcache.go) and Load's per-file memo (see filecache.go).
// Both revalidate by (mtime, size), so real edits are caught without a reset;
// what a reset covers is the stamp's blind spot — a same-size rewrite landing
// inside one mtime tick on a coarse-timestamp filesystem. Tests that mutate
// config files across sub-cases should call this between them.
//
// Production code needs it in exactly one shape: a command that WRITES config
// and then reads the result back inside the same process (`grove subscribe`,
// `grove ecosystem materialize`, `grove join` — all of which write
// machine.toml or sync.toml and immediately resolve against it). Their own
// write is exactly the same-tick rewrite the stamps can miss. Nothing else
// should call it.
func ResetLoadCache() {
	resetLoadFromCache()
	resetFileCache()
}

var envVarRegex = regexp.MustCompile(`\$\{([^}]+)\}`)

var (
	sharedValidator     *SchemaValidator
	sharedValidatorErr  error
	sharedValidatorOnce sync.Once
)

// getSharedValidator compiles the embedded-schema validator once and reuses it.
// Compiling the JSONSchema is the ~15ms cost the load cache exists to amortize,
// so the warn-only load-path validation must not pay it on every call.
func getSharedValidator() (*SchemaValidator, error) {
	sharedValidatorOnce.Do(func() {
		sharedValidator, sharedValidatorErr = NewSchemaValidator()
	})
	return sharedValidator, sharedValidatorErr
}

// validateAndWarn validates a loaded config against the embedded schema and
// reports any violation through logger at Warn level. It is deliberately never
// fatal: the embedded schema is advisory. It can lag real struct fields (e.g.
// tui leader_key / shortcuts) and it does not model extension namespaces, so a
// violation must never block loading — forward-compat keys and config fragments
// have to keep working. Extensions serialize inline at the top level, where the
// schema permits additional properties, so legitimate namespaces do not warn.
//
// Delivery goes through reportSchemaWarning: deduped per process (identical
// warnings would otherwise repeat once per fragment per load), routed to the
// logging pipeline's sink when available, and never written to an interactive
// stderr — see config_warnings.go.
func validateAndWarn(cfg *Config, logger *logrus.Logger, source string) {
	if cfg == nil || logger == nil {
		return
	}
	validator, err := getSharedValidator()
	if err != nil {
		logger.WithError(err).Debug("config schema validator unavailable; skipping validation")
		return
	}
	if err := validator.Validate(cfg); err != nil {
		reportSchemaWarning(logger, source, err)
	}
}

// ConfigMeta holds metadata about the config file itself.
// This is parsed from the [_grove] section and stripped from the final config.
type ConfigMeta struct {
	Priority int `toml:"priority" yaml:"priority"` // Loading priority (higher loads later, default: 50)
}

// DefaultPriority is the default priority for config fragments.
const DefaultPriority = 50

// configFragment holds a config file path and its priority for sorting.
type configFragment struct {
	path     string
	priority int
}

// extractConfigMeta reads the [_grove] section from a config file to get metadata.
// Returns default values if the section doesn't exist.
func extractConfigMeta(data []byte, path string) ConfigMeta {
	meta := ConfigMeta{Priority: DefaultPriority}

	if strings.HasSuffix(path, ".toml") {
		var raw struct {
			Grove ConfigMeta `toml:"_grove"`
		}
		if err := toml.Unmarshal(data, &raw); err == nil && raw.Grove.Priority != 0 {
			meta.Priority = raw.Grove.Priority
		}
	} else {
		var raw struct {
			Grove ConfigMeta `yaml:"_grove"`
		}
		if err := yaml.Unmarshal(data, &raw); err == nil && raw.Grove.Priority != 0 {
			meta.Priority = raw.Grove.Priority
		}
	}

	return meta
}

// stripGroveMeta removes the [_grove] section from Extensions after loading.
func stripGroveMeta(cfg *Config) {
	if cfg.Extensions != nil {
		delete(cfg.Extensions, "_grove")
	}
}

// coreConfigKeys lists the known top-level keys that are part of the core Config struct.
// These are excluded from Extensions when loading TOML files.
var coreConfigKeys = map[string]bool{
	"name":              true,
	"version":           true,
	"workspaces":        true,
	"build_cmd":         true,
	"build_after":       true,
	"notebooks":         true,
	"tui":               true,
	"context":           true,
	"daemon":            true,
	"environment":       true,
	"environments":      true,
	"explicit_projects": true,
	"commands":          true,
	"test_scopes":       true,
	"worktree":          true,
	"onboarding":        true,
	"security":          true,
	"ecosystem":         true, // Repo-side ecosystem identity card (EcosystemCard)
	"_grove":            true, // Meta section for config metadata (priority, etc.)
}

// rejectLegacyTopology prevents removed topology declarations from being
// silently swallowed by permissive decoders. The frozen parser behind
// `grove migrate` is the only remaining reader for these spellings.
func rejectLegacyTopology(path string, data []byte, tomlDialect, consultRecorded bool) error {
	raw := make(map[string]interface{})
	var err error
	if tomlDialect {
		err = toml.Unmarshal(data, &raw)
	} else {
		err = yaml.Unmarshal(data, &raw)
	}
	if err != nil {
		return nil
	}
	for _, table := range []string{"groves", "search_paths"} {
		if _, exists := raw[table]; !exists {
			continue
		}
		if consultRecorded {
			if _, statErr := os.Stat(coderoot.RootsPath()); statErr == nil {
				return fmt.Errorf("forbidden mixed state: %s contains legacy [%s] while %s exists; run 'grove migrate'", path, table, coderoot.RootsFileName)
			}
			return fmt.Errorf("legacy config %s contains [%s] and %s is absent; run 'grove migrate'", path, table, coderoot.RootsFileName)
		}
		return fmt.Errorf("legacy config %s contains [%s]; run 'grove migrate'", path, table)
	}
	return nil
}

// unmarshalConfig parses config data based on file extension (TOML or YAML).
// For TOML files, it also captures extension fields into Extensions to emulate YAML inline behavior.
func unmarshalConfig(path string, data []byte) (*Config, error) {
	var cfg Config
	if err := rejectLegacyTopology(path, data, strings.HasSuffix(path, ".toml"), true); err != nil {
		return nil, err
	}

	if strings.HasSuffix(path, ".toml") {
		if err := toml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		// Capture extension fields (non-core keys) into Extensions
		var raw map[string]interface{}
		if err := toml.Unmarshal(data, &raw); err == nil {
			extensions := make(map[string]interface{})
			for k, v := range raw {
				if !coreConfigKeys[k] {
					extensions[k] = v
				}
			}
			if len(extensions) > 0 {
				cfg.Extensions = extensions
			}
		}
		// Post-process TOML keybindings to handle simplified path format
		postProcessTOMLKeybindings(&cfg, data)
		// Post-process notebook sync configs (the field is toml:"-")
		postProcessTOMLNotebookSync(&cfg, data)
	} else {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}

	return &cfg, nil
}

// postProcessTOMLKeybindings handles the simplified keybinding path format for TOML.
// It parses [tui.keybindings.package.tui] sections and populates TUIOverrides.
func postProcessTOMLKeybindings(cfg *Config, data []byte) {
	// Parse raw TOML to find keybinding sections
	var raw map[string]interface{}
	if err := toml.Unmarshal(data, &raw); err != nil {
		return
	}

	tuiRaw, ok := raw["tui"].(map[string]interface{})
	if !ok {
		return
	}

	kbRaw, ok := tuiRaw["keybindings"].(map[string]interface{})
	if !ok {
		return
	}

	// Known section names that apply globally (not package names)
	sectionNames := map[string]bool{
		"navigation": true, "selection": true, "actions": true,
		"search": true, "view": true, "fold": true, "system": true,
		"overrides": true, // Also skip the legacy overrides key
	}

	// Ensure TUI and Keybindings structs exist
	if cfg.TUI == nil {
		cfg.TUI = &TUIConfig{}
	}
	if cfg.TUI.Keybindings == nil {
		cfg.TUI.Keybindings = &KeybindingsConfig{}
	}

	// Process non-section keys as package names
	for pkgName, pkgValue := range kbRaw {
		if sectionNames[pkgName] {
			continue
		}

		// This should be a package name with TUI sub-keys
		pkgMap, ok := pkgValue.(map[string]interface{})
		if !ok {
			continue
		}

		for tuiName, tuiValue := range pkgMap {
			tuiMap, ok := tuiValue.(map[string]interface{})
			if !ok {
				continue
			}

			// Convert to KeybindingSectionConfig
			sectionConfig := make(KeybindingSectionConfig)
			for action, keys := range tuiMap {
				if arr, ok := keys.([]interface{}); ok {
					var strKeys []string
					for _, k := range arr {
						if s, ok := k.(string); ok {
							strKeys = append(strKeys, s)
						}
					}
					sectionConfig[action] = strKeys
				}
			}

			// Add to TUIOverrides
			if cfg.TUI.Keybindings.TUIOverrides == nil {
				cfg.TUI.Keybindings.TUIOverrides = make(map[string]map[string]KeybindingSectionConfig)
			}
			if cfg.TUI.Keybindings.TUIOverrides[pkgName] == nil {
				cfg.TUI.Keybindings.TUIOverrides[pkgName] = make(map[string]KeybindingSectionConfig)
			}
			cfg.TUI.Keybindings.TUIOverrides[pkgName][tuiName] = sectionConfig
		}
	}
}

// Load reads and parses one explicit Grove configuration file. It is hermetic:
// ambient roots/notebooks and machine metadata are not consulted. Use
// LoadWithTopology for an explicit routing pair, or LoadFrom/LoadLayered for
// the canonical hierarchy.
//
// An unchanged file is parsed and schema-validated once per process, not once
// per call: the result is memoized per absolute path and revalidated with a
// stat. See filecache.go for what "unchanged" covers and for the
// immutability contract the returned *Config carries.
func Load(path string) (*Config, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		// Unresolvable path: skip the cache entirely rather than key it on
		// something ambiguous. loadUncached still returns the real error.
		return loadUncached(path)
	}

	info, statErr := os.Stat(abs)
	if statErr != nil || info.IsDir() {
		// Missing, unreadable, or a directory. Fall through to the original
		// path so ENOENT and permission errors are produced by exactly the
		// code that produced them before this cache existed.
		return loadUncached(path)
	}
	self := fileStamp{exists: true, modTime: info.ModTime(), size: info.Size()}

	raw, _ := fileCache.LoadOrStore(abs, &fileCacheEntry{})
	entry := raw.(*fileCacheEntry)

	// Per-entry lock: concurrent Loads of the same path collapse onto one
	// parse, Loads of different paths never contend.
	entry.mu.Lock()
	defer entry.mu.Unlock()

	if entry.fresh(self) {
		return entry.cfg, nil
	}

	// Read AFTER stamping. If the file changes between the two, we cache newer
	// content under an older stamp — the next call stats, sees the newer mtime
	// and reloads. Stamping after the read would have the opposite, unsafe
	// skew: older content pinned to a stamp nothing will ever invalidate.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, readConfigError(path, err)
	}

	cfg, err := parseConfigBytes(path, data)
	if err != nil {
		// Parse failures are not cached. They are rare, loud (workspace
		// classification surfaces them), and re-deriving one costs a parse of
		// a file the operator is actively fixing.
		return nil, err
	}

	entry.store(self, cfg, envRefs(string(data)))
	return cfg, nil
}

// loadUncached is Load without memoization: the original read-then-parse body,
// kept intact so uncacheable paths behave identically to before.
func loadUncached(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, readConfigError(path, err)
	}
	return parseConfigBytes(path, data)
}

// readConfigError maps a failed read of a config file to the error Load has
// always returned for it.
func readConfigError(path string, err error) error {
	if os.IsNotExist(err) {
		return errors.ConfigNotFound(path)
	}
	return errors.Wrap(err, errors.ErrCodeConfigInvalid, fmt.Sprintf("failed to read config file %s", path)).
		WithDetail("path", path)
}

// parseConfigBytes dispatches on extension the way Load always has: TOML for
// .toml, YAML for everything else.
func parseConfigBytes(path string, data []byte) (*Config, error) {
	if strings.HasSuffix(path, ".toml") {
		return loadFromTOMLBytes(path, data, false)
	}
	return loadFromYAMLBytes(path, data, false)
}

// LoadWithTopology parses one explicit config file and compiles one explicit
// recorded routing pair into it. Unlike hierarchical loaders, it never resolves
// ambient roots/notebooks or machine metadata.
func LoadWithTopology(path, rootsPath, notebooksPath string) (*Config, error) {
	cfg, err := Load(path)
	if err != nil {
		return nil, err
	}
	return compileCodeRootsFromPaths(cfg, rootsPath, notebooksPath)
}

// LoadDefault finds and loads the configuration with hierarchical merging:
// 1. Global config (~/.config/grove/grove.toml) - base layer
// 2. Project config (grove.toml) - overrides global
// 3. Local override (grove.override.toml) - overrides all
func LoadDefault() (*Config, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeConfigInvalid, "failed to get current directory")
	}

	return LoadFrom(cwd)
}

// LoadFrom loads configuration with hierarchical merging starting from the given directory
func LoadFrom(startDir string) (*Config, error) {
	return LoadFromWithLogger(startDir, logrus.New())
}

// LoadFromWithLogger loads configuration with hierarchical merging and logging.
//
// An unchanged hierarchy is loaded once per process, not once per call: the
// result is memoized per absolute startDir and revalidated against every input
// the previous load consulted — file stamps, glob membership, discovery
// lookups, and the env vars the files referenced. See loadfromcache.go for
// what "unchanged" covers and for the immutability contract the returned
// *Config carries.
func LoadFromWithLogger(startDir string, logger *logrus.Logger) (*Config, error) {
	cacheKey, _ := filepath.Abs(startDir)
	if cacheKey == "" {
		cacheKey = startDir
	}
	raw, _ := loadFromCache.LoadOrStore(cacheKey, &loadFromEntry{})
	entry := raw.(*loadFromEntry)

	// Per-entry lock: concurrent loads of the same startDir collapse onto one
	// hierarchy walk, loads of different directories never contend.
	entry.mu.Lock()
	defer entry.mu.Unlock()

	if entry.fresh() {
		return entry.cfg, nil
	}

	trace := &loadFromTrace{}
	cfg, err := loadFromHierarchy(startDir, logger, trace)
	if err != nil {
		// Failures are never cached: an error path takes exactly the code path
		// it took before the memo existed.
		return nil, err
	}
	entry.store(cfg, trace)
	return cfg, nil
}

// loadFromHierarchy is the full hierarchical load. It records every file,
// glob, discovery result, and env reference it consults into trace, which is
// what lets LoadFromWithLogger serve the result until one of them changes.
func loadFromHierarchy(startDir string, logger *logrus.Logger, trace *loadFromTrace) (*Config, error) {
	// machine.toml is display metadata, not a merge layer, but canonical loads
	// must reject its removed topology tables and cache its presence/absence.
	if err := validateCanonicalMachineConfig(trace); err != nil {
		return nil, err
	}

	// Find project config file first
	projectPath, err := FindConfigFile(startDir)
	if err != nil {
		// If it's any error other than not found, we fail.
		if !errors.Is(err, errors.ErrCodeConfigNotFound) {
			return nil, err
		}
		projectPath = "" // No project file found, proceed without it.
	}
	// A config file appearing closer to startDir (or the found one vanishing)
	// re-anchors the whole cascade; re-running the search is the only cheap
	// way to notice either. Content changes of the found file are carried by
	// its read stamp below.
	trace.lookup(projectPath, func() string {
		found, err := FindConfigFile(startDir)
		if err != nil {
			return ""
		}
		return found
	})

	// Start with an empty config
	var finalConfig *Config

	// 1. Load global config if it exists (optional)
	globalPath := getXDGConfigPath()
	// The XDG resolution depends on the config-dir env and on which of
	// grove.toml/grove.yml exists there.
	trace.lookup(globalPath, getXDGConfigPath)
	if globalPath != "" {
		if globalData, err := trace.readFile(globalPath); err == nil {
			logger.WithField("path", globalPath).Debug("Loading global configuration")
			// Load global config without validation/defaults (raw load)
			expanded := expandEnvVars(string(globalData))
			globalConfig, parseErr := unmarshalConfig(globalPath, []byte(expanded))
			if parseErr == nil {
				finalConfig = globalConfig
			} else {
				return nil, parseErr
			}
		} else if !os.IsNotExist(err) {
			return nil, readConfigError(globalPath, err)
		}

		// Glob and merge additional modular TOML files from config directory
		// Files are sorted by priority ([_grove].priority), then alphabetically within same priority
		globalDir := filepath.Dir(globalPath)
		pattern := filepath.Join(globalDir, "*.toml")
		if files, err := filepath.Glob(pattern); err == nil {
			trace.glob(pattern, files)
			// First pass: collect fragments with their priorities
			var fragments []configFragment
			for _, file := range files {
				baseName := filepath.Base(file)
				// Skip main config, override files, and the dedicated sync
				// and machine client configs (parsed separately via
				// LoadSyncConfig / LoadMachineConfig)
				if isExcludedGlobalFragment(baseName) {
					continue
				}

				fragmentData, err := trace.readFile(file)
				if err != nil {
					return nil, readConfigError(file, err)
				}

				meta := extractConfigMeta(fragmentData, file)
				fragments = append(fragments, configFragment{path: file, priority: meta.Priority})
			}

			// Sort by priority (stable sort maintains alphabetical order within same priority)
			sort.SliceStable(fragments, func(i, j int) bool {
				return fragments[i].priority < fragments[j].priority
			})

			// Second pass: merge in priority order
			for _, frag := range fragments {
				logger.WithFields(logrus.Fields{
					"path":     frag.path,
					"priority": frag.priority,
				}).Debug("Loading global config fragment")

				fragmentData, err := os.ReadFile(frag.path)
				if err != nil {
					return nil, readConfigError(frag.path, err)
				}

				expanded := expandEnvVars(string(fragmentData))
				fragmentConfig, parseErr := unmarshalConfig(frag.path, []byte(expanded))
				if parseErr != nil {
					return nil, parseErr
				}

				// Strip _grove meta section
				stripGroveMeta(fragmentConfig)

				if finalConfig == nil {
					finalConfig = fragmentConfig
				} else {
					finalConfig = mergeConfigs(finalConfig, fragmentConfig)
				}
			}
		} else {
			return nil, fmt.Errorf("failed to enumerate config fragments %s: %w", pattern, err)
		}

		// Also glob ~/.config/grove/plugins/*.toml for per-user plugin manifests
		pluginPattern := plugin.FragmentPattern(globalDir)
		if pluginFiles, err := filepath.Glob(pluginPattern); err == nil {
			trace.glob(pluginPattern, pluginFiles)
			for _, file := range pluginFiles {
				logger.WithField("path", file).Debug("Loading plugin config fragment")

				fragmentData, err := trace.readFile(file)
				if err != nil {
					return nil, readConfigError(file, err)
				}

				expanded := expandEnvVars(string(fragmentData))
				fragmentConfig, parseErr := unmarshalConfig(file, []byte(expanded))
				if parseErr != nil {
					return nil, parseErr
				}

				stripGroveMeta(fragmentConfig)

				if finalConfig == nil {
					finalConfig = fragmentConfig
				} else {
					finalConfig = mergeConfigs(finalConfig, fragmentConfig)
				}
			}
		} else {
			return nil, fmt.Errorf("failed to enumerate plugin config fragments %s: %w", pluginPattern, err)
		}
	}

	// Load global override if it exists
	if globalPath != "" {
		globalDir := filepath.Dir(globalPath)
		overrideFiles := globalOverrideFiles(globalDir)

		for _, overridePath := range overrideFiles {
			if _, err := os.Stat(overridePath); err != nil {
				// An absent candidate must stay absent: creating it would
				// change which override file wins.
				trace.absent(overridePath)
				if !os.IsNotExist(err) {
					return nil, readConfigError(overridePath, err)
				}
				continue
			}
			logger.WithField("path", overridePath).Debug("Loading global override configuration")
			overrideData, err := trace.readFile(overridePath)
			if err != nil {
				return nil, readConfigError(overridePath, err)
			}
			expanded := expandEnvVars(string(overrideData))
			overrideConfig, parseErr := unmarshalConfig(overridePath, []byte(expanded))
			if parseErr != nil {
				return nil, parseErr
			}
			if finalConfig == nil {
				finalConfig = overrideConfig
			} else {
				finalConfig = mergeConfigs(finalConfig, overrideConfig)
			}
			break // Only load one
		}
	}

	// Load GROVE_CONFIG_OVERLAY if set (for demo/testing environments)
	// Any field present in the overlay replaces the corresponding field in base config.
	// One lookup covers both the env var appearing/changing and its expansion.
	trace.lookup(overlayLookup(), overlayLookup)
	if overlayPath := os.Getenv("GROVE_CONFIG_OVERLAY"); overlayPath != "" {
		overlayPath = expandPath(overlayPath)
		if _, err := os.Stat(overlayPath); err == nil {
			logger.WithField("path", overlayPath).Debug("Loading config overlay from GROVE_CONFIG_OVERLAY")
			overlayData, err := trace.readFile(overlayPath)
			if err != nil {
				return nil, errors.Wrap(err, errors.ErrCodeConfigInvalid, "failed to read config overlay").
					WithDetail("path", overlayPath)
			}
			expanded := expandEnvVars(string(overlayData))
			overlayConfig, parseErr := unmarshalConfig(overlayPath, []byte(expanded))
			if parseErr != nil {
				return nil, errors.Wrap(parseErr, errors.ErrCodeConfigInvalid, "failed to parse config overlay").
					WithDetail("path", overlayPath)
			}
			if finalConfig == nil {
				finalConfig = overlayConfig
			} else {
				// Replace any non-zero field from overlay
				applyOverlay(finalConfig, overlayConfig)
			}
		} else if os.IsNotExist(err) {
			// If GROVE_CONFIG_OVERLAY is set but file doesn't exist, that's an error
			return nil, errors.ConfigNotFound(overlayPath).
				WithDetail("reason", "GROVE_CONFIG_OVERLAY path does not exist")
		} else {
			return nil, errors.Wrap(err, errors.ErrCodeConfigInvalid, "failed to access config overlay").
				WithDetail("path", overlayPath)
		}
	}

	// Everything merged so far comes from user-controlled files: the global
	// config, its fragments and plugin manifests, the global override, and
	// the GROVE_CONFIG_OVERLAY. Snapshot it before any repo-controlled layer
	// lands so the exec-trust policy is read from the user's own config only
	// — a workspace grove.toml must not be able to set exec_trust = "off"
	// and disable the gate that contains it.
	gate := newExecGateRun(finalConfig, logger)

	// Detect when FindConfigFile fell through to the global config (no local project config).
	// In this case, skip loading it again as the "project" layer — it's already loaded as global.
	isGlobalFallback := projectPath != "" && globalPath != "" && projectPath == globalPath

	if projectPath != "" {
		logger.WithField("path", projectPath).Debug("Loading project configuration")
		// 2. Load and merge project config - also without defaults/validation
		projectData, err := trace.readFile(projectPath)
		if err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeConfigInvalid, "failed to read project config").
				WithDetail("path", projectPath)
		}

		expanded := expandEnvVars(string(projectData))
		projectConfig, parseErr := unmarshalConfig(projectPath, []byte(expanded))
		if parseErr != nil {
			return nil, errors.Wrap(parseErr, errors.ErrCodeConfigInvalid, "failed to parse project config").
				WithDetail("path", projectPath)
		}

		// Check if this is a workspace config (has no workspaces field) and look for ecosystem config
		ecosystemPath := ""
		if !isGlobalFallback && len(projectConfig.Workspaces) == 0 {
			// This appears to be a workspace config, look for ecosystem config.
			// The traced walk records every candidate it consulted, so an
			// ecosystem config appearing in a nearer directory — or the found
			// one changing — invalidates by stamp alone.
			ecosystemPath, err = findEcosystemConfigForLoad(filepath.Dir(projectPath), trace)
			if err != nil {
				return nil, err
			}
			if ecosystemPath != "" {
				logger.WithField("path", ecosystemPath).Debug("Loading ecosystem configuration")
				// Already stamped by the walk; a plain read avoids a duplicate dep.
				ecosystemData, err := os.ReadFile(ecosystemPath)
				if err == nil {
					expandedEco := expandEnvVars(string(ecosystemData))
					ecosystemConfig, ecoParseErr := unmarshalConfig(ecosystemPath, []byte(expandedEco))
					if ecoParseErr == nil {
						gate.apply(ecosystemConfig, SourceEcosystem, ecosystemPath)
						// Merge ecosystem config after global but before project
						if finalConfig == nil {
							finalConfig = ecosystemConfig
						} else {
							logger.Debug("Merging ecosystem configuration over global configuration")
							finalConfig = mergeConfigs(finalConfig, ecosystemConfig)
						}
					} else {
						return nil, ecoParseErr
					}
				} else {
					return nil, readConfigError(ecosystemPath, err)
				}
			}
		}

		// Load notebook config (after ecosystem, before project local)
		//
		// Determine the correct project root for notebook resolution.
		// When FindConfigFile traversed up to the global/XDG config (no local
		// grove.toml exists), or found an ecosystem config with workspaces,
		// use startDir's git root as the project root. The notebook config is
		// keyed by the actual project directory name, not the config file's parent.
		projectRoot := filepath.Dir(projectPath)
		absStart, _ := filepath.Abs(startDir)
		absProjectRoot, _ := filepath.Abs(projectRoot)
		if absStart != absProjectRoot {
			// projectPath is not in startDir — it's an ancestor config (global/ecosystem).
			// Use git root or startDir as the actual project root for notebook lookup.
			if gitRoot, gitErr := getGitRoot(startDir); gitErr == nil && gitRoot != "" {
				projectRoot = gitRoot
			} else {
				projectRoot = startDir
			}
			// The derivation shells out to git, so it is only replayed for
			// loads that actually took this branch.
			trace.lookup(projectRoot, func() string {
				if gitRoot, gitErr := getGitRoot(startDir); gitErr == nil && gitRoot != "" {
					return gitRoot
				}
				return startDir
			})
		}
		// Snapshot the cascade the notebook lookup resolves against: later
		// layers reassign finalConfig, and the replay must see the same inputs
		// the load saw. Re-running the lookup catches a notebook config
		// appearing, vanishing, or rebinding (the ecosystem-card probe it rides
		// on revalidates by stat); content changes of the found file are
		// carried by its read stamp.
		nbBase, compileErr := compileCodeRoots(finalConfig)
		if compileErr != nil {
			return nil, compileErr
		}
		notebookConfigPath := findNotebookConfigPath(projectRoot, nbBase)
		trace.lookup(notebookConfigPath, func() string {
			return findNotebookConfigPath(projectRoot, nbBase)
		})
		if notebookConfigPath != "" {
			logger.WithField("path", notebookConfigPath).Debug("Loading project notebook configuration")
			nbData, err := trace.readFile(notebookConfigPath)
			if err != nil {
				return nil, readConfigError(notebookConfigPath, err)
			}
			expandedNb := expandEnvVars(string(nbData))
			nbConfig, parseErr := unmarshalConfig(notebookConfigPath, []byte(expandedNb))
			if parseErr != nil {
				return nil, parseErr
			}
			stripGroveMeta(nbConfig)
			gate.apply(nbConfig, SourceProjectNotebook, notebookConfigPath)
			if finalConfig == nil {
				finalConfig = nbConfig
			} else {
				finalConfig = mergeConfigs(finalConfig, nbConfig)
			}
		}

		if !isGlobalFallback {
			// The project layer is the repo's own grove.toml — the layer the
			// gate exists for. (When isGlobalFallback is set, projectPath IS
			// the global config, which is user-controlled and already merged.)
			gate.apply(projectConfig, SourceProject, projectPath)
			if finalConfig == nil {
				finalConfig = projectConfig
			} else {
				logger.Debug("Merging project configuration over global/ecosystem/notebook configuration")
				finalConfig = mergeConfigs(finalConfig, projectConfig)
			}
		}

		// 3. Load and merge override files if they exist (optional)
		projectDir := filepath.Dir(projectPath)
		overrideFiles := projectOverrideFiles(projectDir)

		for _, overridePath := range overrideFiles {
			if _, err := os.Stat(overridePath); err != nil {
				// Every existing candidate is merged, so each absent one must
				// stay absent.
				trace.absent(overridePath)
				if !os.IsNotExist(err) {
					return nil, readConfigError(overridePath, err)
				}
				continue
			}
			logger.WithField("path", overridePath).Debug("Loading local override configuration")

			overrideData, err := trace.readFile(overridePath)
			if err != nil {
				return nil, readConfigError(overridePath, err)
			}

			// Expand environment variables
			expanded := expandEnvVars(string(overrideData))
			overrideConfig, parseErr := unmarshalConfig(overridePath, []byte(expanded))
			if parseErr != nil {
				return nil, parseErr
			}

			// Only gate when these are the REPO's override files. Under
			// isGlobalFallback, projectDir is the global config dir, so
			// projectOverrideFiles() resolves to ~/.config/grove/grove.override.*
			// — a user-controlled file already merged ungated above. Gating
			// it here would quarantine the user's own hooks/plugins and warn
			// about their own config.
			if !isGlobalFallback {
				gate.apply(overrideConfig, SourceOverride, overridePath)
			}
			finalConfig = mergeConfigs(finalConfig, overrideConfig)
		}
	}

	// If no project config was found, still try loading notebook config
	if projectPath == "" && finalConfig != nil {
		projectRoot := startDir
		if gitRoot, err := getGitRoot(startDir); err == nil && gitRoot != "" {
			projectRoot = gitRoot
		}
		trace.lookup(projectRoot, func() string {
			if gitRoot, err := getGitRoot(startDir); err == nil && gitRoot != "" {
				return gitRoot
			}
			return startDir
		})
		// Same snapshot-and-replay as the project branch above.
		nbBase, compileErr := compileCodeRoots(finalConfig)
		if compileErr != nil {
			return nil, compileErr
		}
		notebookConfigPath := findNotebookConfigPath(projectRoot, nbBase)
		trace.lookup(notebookConfigPath, func() string {
			return findNotebookConfigPath(projectRoot, nbBase)
		})
		if notebookConfigPath != "" {
			logger.WithField("path", notebookConfigPath).Debug("Loading project notebook configuration (no local project config)")
			nbData, err := trace.readFile(notebookConfigPath)
			if err != nil {
				return nil, readConfigError(notebookConfigPath, err)
			}
			expandedNb := expandEnvVars(string(nbData))
			nbConfig, parseErr := unmarshalConfig(notebookConfigPath, []byte(expandedNb))
			if parseErr != nil {
				return nil, parseErr
			}
			stripGroveMeta(nbConfig)
			gate.apply(nbConfig, SourceProjectNotebook, notebookConfigPath)
			finalConfig = mergeConfigs(finalConfig, nbConfig)
		}
	}

	// If no configs were found at all, create an empty one to avoid nil pointers
	if finalConfig == nil {
		finalConfig = &Config{}
	}

	// Compile the authoritative recorded routing pair before defaults.
	rootsPath, notebooksPath := coderoot.RootsPath(), coderoot.NotebooksPath()
	trace.lookup(rootsPath, coderoot.RootsPath)
	_, _ = trace.readFile(rootsPath) // compileCodeRoots reports any real read error
	trace.lookup(notebooksPath, coderoot.NotebooksPath)
	_, _ = trace.readFile(notebooksPath) // also records env expansion dependencies
	finalConfig, err = compileCodeRoots(finalConfig)
	if err != nil {
		return nil, err
	}

	// Set defaults
	finalConfig.SetDefaults()

	// Attach the exec-provenance report AFTER all merging: mergeConfigs
	// rebuilds the config from declared fields and would drop it otherwise.
	// Warn loudly about anything the gate ignored.
	finalConfig.ExecGate = gate.report()
	warnExecGate(finalConfig.ExecGate, logger)

	// Warn-only schema check over the merged config. Never fatal — see
	// validateAndWarn. This is the report F2 called for: the "real" load path
	// now surfaces schema violations instead of validating nothing.
	validateAndWarn(finalConfig, logger, "merged config")

	logger.Debug("Configuration loaded and validated successfully")

	// Log the merged config at debug level
	if logger.IsLevelEnabled(logrus.DebugLevel) {
		configData, err := yaml.Marshal(finalConfig)
		if err == nil {
			logger.Debugf("Merged configuration:\n%s", string(configData))
		}
	}

	return finalConfig, nil
}

// overlayLookup is GROVE_CONFIG_OVERLAY's contribution to the load trace: the
// expanded overlay path when the variable is set, "" otherwise.
func overlayLookup() string {
	if raw := os.Getenv("GROVE_CONFIG_OVERLAY"); raw != "" {
		return expandPath(raw)
	}
	return ""
}

// LoadFromBytes parses configuration from byte array
func LoadFromBytes(data []byte) (*Config, error) {
	return loadFromYAMLBytes("<YAML bytes>", data, false)
}

func loadFromYAMLBytes(source string, data []byte, consultRecorded bool) (*Config, error) {
	// Expand environment variables
	expanded := expandEnvVars(string(data))

	if err := rejectLegacyTopology(source, []byte(expanded), false, consultRecorded); err != nil {
		return nil, err
	}
	var config Config
	if err := yaml.Unmarshal([]byte(expanded), &config); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeConfigInvalid, "failed to parse YAML configuration")
	}

	// Warn-only schema check. Never fatal: now that validation actually
	// compares snake_case keys (see schema.Validator.Validate), the embedded
	// schema's drift from real struct fields and its lack of extension coverage
	// must not turn config.Load — used across the whole ecosystem — into a hard
	// failure on otherwise-usable configs.
	validateAndWarn(&config, logrus.StandardLogger(), "config bytes")

	// Byte-only parsing is deliberately hermetic. Canonical file and hierarchy
	// loaders compile recorded topology explicitly after parsing.
	config.SetDefaults()

	return &config, nil
}

// LoadFromTOMLBytes parses configuration from TOML byte array
func LoadFromTOMLBytes(data []byte) (*Config, error) {
	return loadFromTOMLBytes("<TOML bytes>", data, false)
}

func loadFromTOMLBytes(source string, data []byte, consultRecorded bool) (*Config, error) {
	// Expand environment variables
	expanded := expandEnvVars(string(data))

	if err := rejectLegacyTopology(source, []byte(expanded), true, consultRecorded); err != nil {
		return nil, err
	}
	var config Config
	if err := toml.Unmarshal([]byte(expanded), &config); err != nil {
		return nil, errors.Wrap(err, errors.ErrCodeConfigInvalid, "failed to parse TOML configuration")
	}

	// Capture extension fields (non-core keys) into Extensions
	// TOML doesn't support inline like YAML, so we unmarshal again to a raw map
	var raw map[string]interface{}
	if err := toml.Unmarshal([]byte(expanded), &raw); err == nil {
		extensions := make(map[string]interface{})
		for k, v := range raw {
			if !coreConfigKeys[k] {
				extensions[k] = v
			}
		}
		if len(extensions) > 0 {
			config.Extensions = extensions
		}
	}

	// Post-process notebook sync configs (the field is toml:"-")
	postProcessTOMLNotebookSync(&config, []byte(expanded))

	// Warn-only schema check. Never fatal — see the note in LoadFromBytes.
	validateAndWarn(&config, logrus.StandardLogger(), "config TOML bytes")

	// Byte-only parsing is deliberately hermetic. Canonical file and hierarchy
	// loaders compile recorded topology explicitly after parsing.
	config.SetDefaults()

	return &config, nil
}

// FindConfigFile searches for grove configuration files with the following precedence:
// 1. Current directory up to filesystem root
// 2. Git repository root (if in a git repo)
// 3. XDG config directory (~/.config/grove/grove.toml)
//
// Within each directory, TOML is preferred over YAML (read compatibility for
// grove.yml/grove.yaml is preserved).
func FindConfigFile(startDir string) (string, error) {
	configNames := []string{
		"grove.toml",
		"grove.yml",
		"grove.yaml",
		".grove.toml",
		".grove.yml",
		".grove.yaml",
		"docker-compose.grove.toml",
		"docker-compose.grove.yml",
		"docker-compose.grove.yaml",
	}

	// 1. Search from current directory up to filesystem root
	dir := startDir
	for {
		// Check each possible config name
		for _, name := range configNames {
			path := filepath.Join(dir, name)
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				return path, nil
			}
		}

		// Move to parent directory
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// 2. Check git repository root if we're in a git repo
	if gitRoot, err := getGitRoot(startDir); err == nil && gitRoot != "" {
		for _, name := range configNames {
			path := filepath.Join(gitRoot, name)
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				return path, nil
			}
		}
	}

	// 3. Check XDG config directory
	if xdgConfigPath := getXDGConfigPath(); xdgConfigPath != "" {
		if info, err := os.Stat(xdgConfigPath); err == nil && !info.IsDir() {
			return xdgConfigPath, nil
		}
	}

	return "", errors.ConfigNotFound(startDir).WithDetail("searchPath", startDir)
}

// projectOverrideFiles returns the override file paths recognized next to a
// project config, in merge order (every existing file is merged, later
// entries win). This is the single source of truth shared by
// LoadFromWithLogger, LoadLayered and LoadWithOverrides so they all read the
// same set of files. The .grove-work.* names are legacy spellings kept for
// read compatibility.
func projectOverrideFiles(projectDir string) []string {
	names := []string{
		"grove.override.yml",
		"grove.override.yaml",
		"grove.override.toml",
		".grove.override.yml",
		".grove.override.yaml",
		".grove.override.toml",
		// Legacy names (previously .grove-work.*), still read for compat.
		".grove-work.yml",
		".grove-work.yaml",
		".grove-work.toml",
	}
	paths := make([]string, len(names))
	for i, name := range names {
		paths[i] = filepath.Join(projectDir, name)
	}
	return paths
}

// globalOverrideFiles returns the override file paths recognized next to the
// global config, in lookup order (only the first existing file is loaded).
func globalOverrideFiles(globalDir string) []string {
	return []string{
		filepath.Join(globalDir, "grove.override.yml"),
		filepath.Join(globalDir, "grove.override.yaml"),
		filepath.Join(globalDir, "grove.override.toml"),
	}
}

// expandPath expands ~ to home directory and environment variables in a path
func expandPath(path string) string {
	// First expand environment variables
	path = os.ExpandEnv(path)

	// Then handle ~ expansion
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}

	return path
}

// applyOverlay replaces fields in base with non-zero fields from overlay.
func applyOverlay(base, overlay *Config) {
	if overlay.Notebooks != nil && overlay.Notebooks.Definitions != nil {
		if base.Notebooks == nil {
			base.Notebooks = &NotebooksConfig{}
		}
		base.Notebooks.Definitions = overlay.Notebooks.Definitions
	}
	if len(overlay.Extensions) > 0 {
		// Route through mergeExtensions (not a wholesale replace) so an
		// accumulating extension (e.g. "claude") keeps the arrays it has
		// already unioned from lower cascade layers — a global
		// grove.override.toml must not wipe that accumulation.
		if base.Extensions == nil {
			base.Extensions = make(map[string]interface{})
		}
		base.Extensions = mergeExtensions(base.Extensions, overlay.Extensions)
	}
}

// expandEnvVars replaces ${VAR} with environment variable values
func expandEnvVars(content string) string {
	return envVarRegex.ReplaceAllStringFunc(content, func(match string) string {
		varName := envVarRegex.FindStringSubmatch(match)[1]

		// Handle default values: ${VAR:-default}
		parts := strings.SplitN(varName, ":-", 2)
		varName = parts[0]
		defaultValue := ""
		if len(parts) > 1 {
			defaultValue = parts[1]
		}

		if value := os.Getenv(varName); value != "" {
			return value
		}

		return defaultValue
	})
}

// getGitRoot attempts to find the git repository root
func getGitRoot(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// getXDGConfigPath returns the XDG config path for Grove
func getXDGConfigPath() string {
	configDir := paths.ConfigDir()
	if configDir == "" {
		return ""
	}

	// Check TOML first (preferred format)
	tomlPath := filepath.Join(configDir, "grove.toml")
	if _, err := os.Stat(tomlPath); err == nil {
		return tomlPath
	}

	// Check YAML second (read compatibility)
	yamlPath := filepath.Join(configDir, "grove.yml")
	if _, err := os.Stat(yamlPath); err == nil {
		return yamlPath
	}

	// Default to TOML if neither exists (for callers that might create it)
	return tomlPath
}

// FindEcosystemConfig searches upward from the given directory for a grove
// config that has a 'workspaces' field (indicating it's an ecosystem config).
// TOML is preferred over YAML when both exist with workspaces.
func FindEcosystemConfig(startDir string) string {
	return findEcosystemConfigTraced(startDir, nil)
}

// findEcosystemConfigTraced is FindEcosystemConfig recording every candidate
// it consulted — absent or read-stamped — into trace. The walk's decisions are
// pure functions of those files, so unchanged stamps mean an unchanged answer:
// revalidating the walk costs stats, never a re-parse.
func findEcosystemConfigTraced(startDir string, trace *loadFromTrace) string {
	configNames := []string{
		"grove.toml",
		"grove.yml",
		"grove.yaml",
		".grove.toml",
		".grove.yml",
		".grove.yaml",
	}

	dir := startDir // Start from the given directory itself
	for {
		// Check for grove.yml with workspaces in this directory
		// Note: We check even inside .grove-worktrees because ecosystem worktrees
		// contain a full copy of the ecosystem including grove.yml with workspaces
		for _, name := range configNames {
			path := filepath.Join(dir, name)
			if info, err := os.Stat(path); err != nil || info.IsDir() {
				trace.absent(path)
				continue
			}
			// Check if this config has workspaces field
			data, err := trace.readFile(path)
			if err != nil {
				continue
			}
			expanded := expandEnvVars(string(data))
			var cfg Config
			if strings.HasSuffix(name, ".toml") {
				if err := toml.Unmarshal([]byte(expanded), &cfg); err != nil {
					continue
				}
			} else {
				if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
					continue
				}
			}
			// An ecosystem config is identified by having a non-empty 'workspaces' field.
			if len(cfg.Workspaces) > 0 {
				return path
			}
		}

		// Move to parent directory
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return ""
}

// findEcosystemConfigForLoad is the fail-loud variant used by canonical load
// cascades. Discovery-only callers retain FindEcosystemConfig's best-effort
// behavior, but a canonical load must not silently reinterpret an unreadable or
// malformed candidate as "not an ecosystem layer".
func findEcosystemConfigForLoad(startDir string, trace *loadFromTrace) (string, error) {
	configNames := []string{
		"grove.toml", "grove.yml", "grove.yaml",
		".grove.toml", ".grove.yml", ".grove.yaml",
	}

	for dir := startDir; ; dir = filepath.Dir(dir) {
		for _, name := range configNames {
			path := filepath.Join(dir, name)
			info, err := os.Stat(path)
			if err != nil {
				trace.absent(path)
				if !os.IsNotExist(err) {
					return "", readConfigError(path, err)
				}
				continue
			}
			if info.IsDir() {
				trace.absent(path)
				continue
			}
			data, err := trace.readFile(path)
			if err != nil {
				return "", readConfigError(path, err)
			}
			cfg, err := unmarshalConfig(path, []byte(expandEnvVars(string(data))))
			if err != nil {
				return "", err
			}
			if len(cfg.Workspaces) > 0 {
				return path, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
	}
}

// notebookContext holds the resolved notebook workspace information for a project.
type notebookContext struct {
	notebookRootDir string
	workspaceName   string
}

// notebookWorkspaceContext finds the notebook workspace directory and name for a
// project root. The notebook itself comes from the unified resolver
// (ResolveNotebook, notebook_resolver.go); what this adds is the part only this
// caller needs — the WORKSPACE NAME, which is the project's path relative to
// its grove root. Returns nil if the project is not in a grove, has no name of
// its own inside it, or resolves to a notebook with no root_dir.
func notebookWorkspaceContext(projectRoot string, cfg *Config) *notebookContext {
	if cfg == nil || len(cfg.Groves) == 0 {
		return nil
	}

	// No owner paths: this caller answers "where is THIS project's config
	// stored", and a path's config lives under its own grove or nowhere. The
	// worktree→owner walk is the workspace layer's business (it is the layer
	// that can compute ownership at all).
	binding := ResolveNotebook(NotebookQuery{Path: projectRoot}, cfg)
	if binding.groveRootMatch == "" {
		return nil
	}

	wsName := relativeWorkspaceName(binding.groveRootMatch, projectRoot)
	if wsName == "" {
		return nil
	}

	// The historical last resort: a grove with no notebook, no default, and no
	// other binding still resolves to "nb". It lives here rather than in the
	// resolver because it is this caller's policy — the workspace-side caller
	// deliberately reports "no notebook" instead of guessing one.
	notebookName := binding.Notebook
	if notebookName == "" {
		notebookName = "nb"
	}

	// Resolve the notebook root directory
	var notebookRootDir string
	if cfg.Notebooks != nil && cfg.Notebooks.Definitions != nil {
		if nb, ok := cfg.Notebooks.Definitions[notebookName]; ok && nb != nil {
			notebookRootDir = expandPath(nb.RootDir)
		}
	}

	if notebookRootDir == "" {
		return nil
	}

	return &notebookContext{
		notebookRootDir: notebookRootDir,
		workspaceName:   wsName,
	}
}

// findNotebookConfigPath resolves the path to a project's configuration file
// stored in its notebook directory. It uses the global config to find the grove
// the project belongs to, determine the notebook name, and construct the path.
func findNotebookConfigPath(projectRoot string, globalCfg *Config) string {
	ctx := notebookWorkspaceContext(projectRoot, globalCfg)
	if ctx == nil {
		return ""
	}

	configNames := []string{"grove.toml", "grove.yml", "grove.yaml"}
	for _, name := range configNames {
		configPath := filepath.Join(ctx.notebookRootDir, "workspaces", ctx.workspaceName, name)
		if info, err := os.Stat(configPath); err == nil && !info.IsDir() {
			return configPath
		}
	}

	return ""
}

// ResolveNotebookDir returns the notebook workspace directory for a project.
// Unlike findNotebookConfigPath (which checks for existing files), this returns
// the directory where a notebook config *should* be placed, even if it doesn't
// exist yet. Returns the directory path and the workspace name, or empty strings
// if the project is not in a grove or has no notebook configured.
func ResolveNotebookDir(projectRoot string) (dir, workspaceName string, err error) {
	cfg, loadErr := LoadDefault()
	if loadErr != nil {
		return "", "", fmt.Errorf("failed to load config: %w", loadErr)
	}
	return resolveNotebookDirWithConfig(projectRoot, cfg)
}

func resolveNotebookDirWithConfig(projectRoot string, cfg *Config) (string, string, error) {
	ctx := notebookWorkspaceContext(projectRoot, cfg)
	if ctx == nil {
		if cfg == nil || len(cfg.Groves) == 0 {
			return "", "", fmt.Errorf("no groves configured")
		}
		return "", "", fmt.Errorf("project is not inside a configured grove or has no notebook configured")
	}

	return filepath.Join(ctx.notebookRootDir, "workspaces", ctx.workspaceName), ctx.workspaceName, nil
}

// LoadLayered finds and loads all configuration layers (global, project, overrides)
// without merging them, for analysis purposes. It also computes the final merged config.
func LoadLayered(startDir string) (*LayeredConfig, error) {
	if err := validateCanonicalMachineConfig(nil); err != nil {
		return nil, err
	}

	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel) // Suppress debug logs for this loader

	layeredConfig := &LayeredConfig{
		Overrides: make([]OverrideSource, 0),
		FilePaths: make(map[ConfigSource]string),
	}

	// 1. Determine Default layer
	defaultCfg := &Config{}
	defaultCfg.SetDefaults()
	// We don't run InferDefaults here as it depends on project structure which we haven't analyzed yet.
	// It will be part of the final merged config.
	layeredConfig.Default = defaultCfg

	// 2. Load Global layer (optional)
	globalPath := getXDGConfigPath()
	if globalPath != "" {
		if _, err := os.Stat(globalPath); err == nil {
			globalData, err := os.ReadFile(globalPath)
			if err != nil {
				return nil, readConfigError(globalPath, err)
			}
			expanded := expandEnvVars(string(globalData))
			globalConfig, parseErr := unmarshalConfig(globalPath, []byte(expanded))
			if parseErr != nil {
				return nil, parseErr
			}
			layeredConfig.Global = globalConfig
			layeredConfig.FilePaths[SourceGlobal] = globalPath
		} else if !os.IsNotExist(err) {
			return nil, readConfigError(globalPath, err)
		}

		// 2.25. Load Global Fragment layers (modular *.toml files)
		// Files are sorted by priority ([_grove].priority), then alphabetically within same priority
		globalDir := filepath.Dir(globalPath)
		pattern := filepath.Join(globalDir, "*.toml")
		if files, err := filepath.Glob(pattern); err == nil {
			// First pass: collect fragments with their priorities
			var fragments []configFragment
			for _, file := range files {
				baseName := filepath.Base(file)
				// Skip main config, override files, and the dedicated sync
				// and machine client configs (parsed separately via
				// LoadSyncConfig / LoadMachineConfig)
				if isExcludedGlobalFragment(baseName) {
					continue
				}

				fragmentData, err := os.ReadFile(file)
				if err != nil {
					return nil, readConfigError(file, err)
				}

				meta := extractConfigMeta(fragmentData, file)
				fragments = append(fragments, configFragment{path: file, priority: meta.Priority})
			}

			// Sort by priority (stable sort maintains alphabetical order within same priority)
			sort.SliceStable(fragments, func(i, j int) bool {
				return fragments[i].priority < fragments[j].priority
			})

			// Second pass: load in priority order
			for _, frag := range fragments {
				fragmentData, err := os.ReadFile(frag.path)
				if err != nil {
					return nil, readConfigError(frag.path, err)
				}

				expanded := expandEnvVars(string(fragmentData))
				fragmentConfig, parseErr := unmarshalConfig(frag.path, []byte(expanded))
				if parseErr != nil {
					return nil, parseErr
				}
				stripGroveMeta(fragmentConfig)
				layeredConfig.GlobalFragments = append(layeredConfig.GlobalFragments, OverrideSource{
					Path:   frag.path,
					Config: fragmentConfig,
				})
			}
		} else {
			return nil, fmt.Errorf("failed to enumerate config fragments %s: %w", pattern, err)
		}

		// 2.3. Load the plugin drop-in directory, ~/.config/grove/plugins/*.toml.
		//
		// These are global-layer fragments like the ones above, kept in their
		// own directory because they are MACHINE-written: `grove plugin
		// install` puts one [tui.plugins.<name>] file per installed panel
		// there, so an uninstall is a file removal and a hand-disabled plugin
		// is one file moved aside.
		//
		// LoadFromWithLogger has globbed this directory since the drop-in
		// mechanism shipped; LoadLayered did not, which meant the one consumer
		// of [tui.plugins] (treemux, via LoadLayered) never saw a drop-in.
		// They are loaded after the ordinary fragments in both loaders, so the
		// merge order is identical.
		//
		// The pattern comes from core/pkg/plugin, which is where the installer
		// derives the paths it writes: the glob and the files it must find (and
		// the lockfile it must not) are one fact, stated once.
		pluginPattern := plugin.FragmentPattern(globalDir)
		if pluginFiles, err := filepath.Glob(pluginPattern); err == nil {
			sort.Strings(pluginFiles)
			for _, file := range pluginFiles {
				fragmentData, err := os.ReadFile(file)
				if err != nil {
					return nil, readConfigError(file, err)
				}
				expanded := expandEnvVars(string(fragmentData))
				fragmentConfig, parseErr := unmarshalConfig(file, []byte(expanded))
				if parseErr != nil {
					return nil, parseErr
				}
				stripGroveMeta(fragmentConfig)
				layeredConfig.GlobalFragments = append(layeredConfig.GlobalFragments, OverrideSource{
					Path:   file,
					Config: fragmentConfig,
				})
			}
		} else {
			return nil, fmt.Errorf("failed to enumerate plugin config fragments %s: %w", pluginPattern, err)
		}
	}

	// 2.5. Load Global Override layer (optional)
	if globalPath != "" {
		globalDir := filepath.Dir(globalPath)
		overrideFiles := globalOverrideFiles(globalDir)
		for _, overridePath := range overrideFiles {
			if _, err := os.Stat(overridePath); err == nil {
				overrideData, err := os.ReadFile(overridePath)
				if err != nil {
					return nil, readConfigError(overridePath, err)
				}
				expanded := expandEnvVars(string(overrideData))
				overrideConfig, parseErr := unmarshalConfig(overridePath, []byte(expanded))
				if parseErr != nil {
					return nil, parseErr
				}
				layeredConfig.GlobalOverride = &OverrideSource{Path: overridePath, Config: overrideConfig}
				layeredConfig.FilePaths[SourceGlobalOverride] = overridePath
				break // Only load the first one found
			} else if !os.IsNotExist(err) {
				return nil, readConfigError(overridePath, err)
			}
		}
	}

	// 2.75. Load GROVE_CONFIG_OVERLAY layer (optional)
	if overlayPath := os.Getenv("GROVE_CONFIG_OVERLAY"); overlayPath != "" {
		overlayPath = expandPath(overlayPath)
		if _, err := os.Stat(overlayPath); err == nil {
			overlayData, err := os.ReadFile(overlayPath)
			if err != nil {
				return nil, readConfigError(overlayPath, err)
			}
			expanded := expandEnvVars(string(overlayData))
			overlayConfig, parseErr := unmarshalConfig(overlayPath, []byte(expanded))
			if parseErr != nil {
				return nil, parseErr
			}
			layeredConfig.EnvOverlay = &OverrideSource{Path: overlayPath, Config: overlayConfig}
			layeredConfig.FilePaths[SourceEnvOverlay] = overlayPath
		} else if os.IsNotExist(err) {
			return nil, errors.ConfigNotFound(overlayPath).WithDetail("reason", "GROVE_CONFIG_OVERLAY path does not exist")
		} else {
			return nil, readConfigError(overlayPath, err)
		}
	}

	// The exec-provenance gate, driven exactly as in LoadFromWithLogger: the
	// policy comes from the user-controlled layers loaded above, never from
	// the repo-controlled layers it is about to gate. LoadLayered keeps the
	// RAW per-layer configs for analysis, so the quarantine must happen here
	// too — otherwise `grove config` would display values the real loader
	// dropped.
	gate := newExecGateRun(userLayerConfig(layeredConfig), logger)

	// 3. Load Project layer (optional)
	projectPath, err := FindConfigFile(startDir)
	if err != nil {
		// If config not found, it's not a fatal error. We can proceed with just global/defaults.
		if !errors.Is(err, errors.ErrCodeConfigNotFound) {
			return nil, errors.Wrap(err, errors.ErrCodeConfigInvalid, "error while finding project config file")
		}
		projectPath = "" // No project file found, proceed without it.
	}

	// FindConfigFile falls through to the XDG global config when no project
	// config exists anywhere above startDir. That file is user-controlled and
	// already loaded as the Global layer, so it must not be gated as if it
	// were a repo's grove.toml — mirrors LoadFromWithLogger's isGlobalFallback.
	isGlobalFallback := projectPath != "" && globalPath != "" && projectPath == globalPath

	if projectPath != "" {
		projectData, err := os.ReadFile(projectPath)
		if err != nil {
			return nil, errors.Wrap(err, errors.ErrCodeConfigInvalid, "failed to read project config").WithDetail("path", projectPath)
		}
		expandedProject := expandEnvVars(string(projectData))
		projectConfig, parseErr := unmarshalConfig(projectPath, []byte(expandedProject))
		if parseErr != nil {
			return nil, errors.Wrap(parseErr, errors.ErrCodeConfigInvalid, "failed to parse project config").WithDetail("path", projectPath)
		}
		if !isGlobalFallback {
			gate.apply(projectConfig, SourceProject, projectPath)
		}
		layeredConfig.Project = projectConfig
		layeredConfig.FilePaths[SourceProject] = projectPath

		// 3.5. Load Ecosystem layer (optional, only if this is a workspace config)
		if len(projectConfig.Workspaces) == 0 {
			ecosystemPath, findErr := findEcosystemConfigForLoad(filepath.Dir(projectPath), nil)
			if findErr != nil {
				return nil, findErr
			}
			if ecosystemPath != "" {
				ecosystemData, err := os.ReadFile(ecosystemPath)
				if err != nil {
					return nil, readConfigError(ecosystemPath, err)
				}
				expandedEco := expandEnvVars(string(ecosystemData))
				ecosystemConfig, ecoParseErr := unmarshalConfig(ecosystemPath, []byte(expandedEco))
				if ecoParseErr != nil {
					return nil, ecoParseErr
				}
				gate.apply(ecosystemConfig, SourceEcosystem, ecosystemPath)
				layeredConfig.Ecosystem = ecosystemConfig
				layeredConfig.FilePaths[SourceEcosystem] = ecosystemPath
			}
		}
	}

	// 3.75. Load Project Notebook layer (optional)
	// Build a lookup config from global + ecosystem layers to resolve notebook paths.
	// Ecosystem config is included because notebooks may be defined there.
	lookupConfig := &Config{}
	if layeredConfig.Global != nil {
		lookupConfig = layeredConfig.Global
	}
	for _, fragment := range layeredConfig.GlobalFragments {
		lookupConfig = mergeConfigs(lookupConfig, fragment.Config)
	}
	if layeredConfig.GlobalOverride != nil {
		lookupConfig = mergeConfigs(lookupConfig, layeredConfig.GlobalOverride.Config)
	}
	if layeredConfig.Ecosystem != nil {
		lookupConfig = mergeConfigs(lookupConfig, layeredConfig.Ecosystem)
	}
	if layeredConfig.Project != nil {
		lookupConfig = mergeConfigs(lookupConfig, layeredConfig.Project)
	}
	// Notebook-layer resolution consumes the authoritative compiled roots.
	lookupConfig, err = compileCodeRoots(lookupConfig)
	if err != nil {
		return nil, err
	}

	projectRoot := startDir
	if projectPath != "" {
		projectRoot = filepath.Dir(projectPath)
		// When the found config is not in startDir (global/ecosystem ancestor),
		// use startDir's git root as the actual project root for notebook lookup.
		absStart, _ := filepath.Abs(startDir)
		absProjectRoot, _ := filepath.Abs(projectRoot)
		if absStart != absProjectRoot {
			if gitRoot, gitErr := getGitRoot(startDir); gitErr == nil && gitRoot != "" {
				projectRoot = gitRoot
			} else {
				projectRoot = startDir
			}
		}
	} else if gitRoot, err := getGitRoot(startDir); err == nil && gitRoot != "" {
		projectRoot = gitRoot
	}
	notebookConfigPath := findNotebookConfigPath(projectRoot, lookupConfig)
	if notebookConfigPath != "" {
		nbData, err := os.ReadFile(notebookConfigPath)
		if err != nil {
			return nil, readConfigError(notebookConfigPath, err)
		}
		expandedNb := expandEnvVars(string(nbData))
		nbConfig, parseErr := unmarshalConfig(notebookConfigPath, []byte(expandedNb))
		if parseErr != nil {
			return nil, parseErr
		}
		stripGroveMeta(nbConfig)
		gate.apply(nbConfig, SourceProjectNotebook, notebookConfigPath)
		layeredConfig.ProjectNotebook = nbConfig
		layeredConfig.FilePaths[SourceProjectNotebook] = notebookConfigPath
	}

	// 4. Load Override layers (optional)
	if projectPath != "" {
		projectDir := filepath.Dir(projectPath)
		overrideFiles := projectOverrideFiles(projectDir)
		for _, overridePath := range overrideFiles {
			if _, err := os.Stat(overridePath); err == nil {
				overrideData, err := os.ReadFile(overridePath)
				if err != nil {
					return nil, readConfigError(overridePath, err)
				}
				expandedOverride := expandEnvVars(string(overrideData))
				overrideConfig, parseErr := unmarshalConfig(overridePath, []byte(expandedOverride))
				if parseErr != nil {
					return nil, parseErr
				}
				// Under isGlobalFallback these resolve to the user's own
				// ~/.config/grove/grove.override.* — never gate those.
				if !isGlobalFallback {
					gate.apply(overrideConfig, SourceOverride, overridePath)
				}
				layeredConfig.Overrides = append(layeredConfig.Overrides, OverrideSource{
					Path:   overridePath,
					Config: overrideConfig,
				})
			} else if !os.IsNotExist(err) {
				return nil, readConfigError(overridePath, err)
			}
		}
	}

	// 5. Compute Final merged config
	// This logic is duplicated from LoadFrom, but necessary to build the final config for analysis.
	finalConfig := &Config{}

	// Start with global if it exists
	if layeredConfig.Global != nil {
		finalConfig = layeredConfig.Global
	}

	// Merge global fragments (modular *.toml files)
	for _, fragment := range layeredConfig.GlobalFragments {
		finalConfig = mergeConfigs(finalConfig, fragment.Config)
	}

	// Merge global override
	if layeredConfig.GlobalOverride != nil {
		finalConfig = mergeConfigs(finalConfig, layeredConfig.GlobalOverride.Config)
	}

	// Apply env overlay (GROVE_CONFIG_OVERLAY) - REPLACES groves/workspaces for isolation
	if layeredConfig.EnvOverlay != nil {
		applyOverlay(finalConfig, layeredConfig.EnvOverlay.Config)
	}

	// Merge ecosystem config (after global, before notebook)
	if layeredConfig.Ecosystem != nil {
		finalConfig = mergeConfigs(finalConfig, layeredConfig.Ecosystem)
	}

	// Merge project notebook config (after ecosystem, before project local)
	if layeredConfig.ProjectNotebook != nil {
		finalConfig = mergeConfigs(finalConfig, layeredConfig.ProjectNotebook)
	}

	// Merge project config
	if layeredConfig.Project != nil {
		finalConfig = mergeConfigs(finalConfig, layeredConfig.Project)
	}

	// Merge overrides (skip when overlay is active for full isolation)
	if layeredConfig.EnvOverlay == nil {
		for _, override := range layeredConfig.Overrides {
			finalConfig = mergeConfigs(finalConfig, override.Config)
		}
	}

	// Compile the authoritative recorded routing tables before defaults.
	finalConfig, err = compileCodeRoots(finalConfig)
	if err != nil {
		return nil, err
	}

	// Set defaults for the final merged config
	finalConfig.SetDefaults()

	finalConfig.ExecGate = gate.report()
	warnExecGate(finalConfig.ExecGate, logger)

	layeredConfig.Final = finalConfig

	// Warn-only schema check over the merged view (never fatal; see
	// validateAndWarn). logger is WarnLevel, so this only surfaces on an actual
	// violation.
	validateAndWarn(finalConfig, logger, "layered config")

	return layeredConfig, nil
}
