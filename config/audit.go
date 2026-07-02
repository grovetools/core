package config

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

// AuditClass classifies a config key found in a layer file.
type AuditClass string

const (
	// AuditKnownCore: the key maps to a Config struct field (or a nested
	// field reachable through one) — the core decoder reads it.
	AuditKnownCore AuditClass = "known-core"
	// AuditKnownExtension: a top-level key registered in the extension
	// registry. Nested keys under it are owned by the consuming code and
	// are not classified further.
	AuditKnownExtension AuditClass = "known-extension"
	// AuditDeprecated: the key maps to a struct field carrying deprecation
	// tags (e.g. search_paths). Still read, but should be migrated.
	AuditDeprecated AuditClass = "deprecated"
	// AuditUnknownNested: a nested key under a known core struct that
	// matches no field tag — the decoder silently drops it.
	AuditUnknownNested AuditClass = "unknown-nested"
	// AuditOrphan: a top-level key that is neither a core field, a
	// registered extension, nor _grove — nothing reads it.
	AuditOrphan AuditClass = "orphan"
)

// AuditFinding reports the classification of one key path set in one config
// layer file.
type AuditFinding struct {
	Key   string       `json:"key"`   // Dot-joined key path (e.g. "tui.theme").
	Class AuditClass   `json:"class"` // Classification of the key.
	Layer ConfigSource `json:"layer"` // Layer the file belongs to.
	File  string       `json:"file"`  // Absolute path of the file that sets the key.
}

// auditFreeFormPaths lists key paths whose subtrees are intentionally
// free-form: user-defined keys under them are consumed by post-processing or
// generic map access, so the reflection walk must not flag them. Descent
// stops here and the path is reported as known-core.
var auditFreeFormPaths = map[string]bool{
	// [tui.keybindings.<package>.<tui>] uses arbitrary package/TUI names,
	// decoded by postProcessTOMLKeybindings into TUIOverrides.
	"tui.keybindings": true,
}

// Audit loads the layered configuration starting from startDir and classifies
// every key set in every layer file present. It is report-only: parse
// failures on individual layers are returned as errors, but no classification
// is ever fatal.
func Audit(startDir string) ([]AuditFinding, error) {
	layered, err := LoadLayered(startDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load layered config: %w", err)
	}
	return AuditLayered(layered)
}

// AuditLayered classifies every key set in every layer file recorded on an
// already-loaded LayeredConfig. Files are audited in cascade order.
func AuditLayered(layered *LayeredConfig) ([]AuditFinding, error) {
	var findings []AuditFinding
	for _, layer := range auditLayerFiles(layered) {
		fileFindings, err := auditFile(layer.path, layer.source)
		if err != nil {
			return nil, err
		}
		findings = append(findings, fileFindings...)
	}
	return findings, nil
}

// auditLayerFile pairs a layer file path with the ConfigSource it belongs to.
type auditLayerFile struct {
	source ConfigSource
	path   string
}

// auditLayerFiles enumerates the layer files present on a LayeredConfig in
// cascade order. FilePaths covers the single-file layers; fragments and
// overrides carry their paths on their OverrideSource entries.
func auditLayerFiles(layered *LayeredConfig) []auditLayerFile {
	var files []auditLayerFile
	add := func(source ConfigSource, path string) {
		if path != "" {
			files = append(files, auditLayerFile{source: source, path: path})
		}
	}

	add(SourceGlobal, layered.FilePaths[SourceGlobal])
	for _, frag := range layered.GlobalFragments {
		add(SourceGlobalFragment, frag.Path)
	}
	if layered.GlobalOverride != nil {
		add(SourceGlobalOverride, layered.GlobalOverride.Path)
	}
	if layered.EnvOverlay != nil {
		add(SourceEnvOverlay, layered.EnvOverlay.Path)
	}
	add(SourceProject, layered.FilePaths[SourceProject])
	add(SourceEcosystem, layered.FilePaths[SourceEcosystem])
	add(SourceProjectNotebook, layered.FilePaths[SourceProjectNotebook])
	for _, ov := range layered.Overrides {
		add(SourceOverride, ov.Path)
	}
	return files
}

// auditFile re-parses one layer file RAW (bypassing the typed decoder, the
// same second-pass approach unmarshalConfig uses) and classifies every key in
// the tree.
func auditFile(path string, source ConfigSource) ([]AuditFinding, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config layer %s: %w", path, err)
	}

	// Match the loader: env vars are expanded before parsing.
	expanded := []byte(expandEnvVars(string(data)))

	raw := make(map[string]interface{})
	if strings.HasSuffix(path, ".toml") {
		if err := toml.Unmarshal(expanded, &raw); err != nil {
			return nil, fmt.Errorf("failed to parse config layer %s: %w", path, err)
		}
	} else {
		if err := yaml.Unmarshal(expanded, &raw); err != nil {
			return nil, fmt.Errorf("failed to parse config layer %s: %w", path, err)
		}
	}

	w := &auditWalker{source: source, file: path}
	w.classifyTopLevel(raw)
	return w.findings, nil
}

// auditWalker accumulates findings for a single layer file while walking its
// raw key tree against the Config struct via reflection.
type auditWalker struct {
	source   ConfigSource
	file     string
	findings []AuditFinding
}

func (w *auditWalker) emit(key string, class AuditClass) {
	w.findings = append(w.findings, AuditFinding{
		Key:   key,
		Class: class,
		Layer: w.source,
		File:  w.file,
	})
}

// classifyTopLevel dispatches each top-level key: core struct fields walk
// into the reflection classifier, registry keys report as known-extension
// (without descending — code owns those shapes), _grove is core metadata, and
// everything else is an orphan.
func (w *auditWalker) classifyTopLevel(raw map[string]interface{}) {
	fields := structFieldsByConfigKey(reflect.TypeOf(Config{}))
	for _, key := range sortedRawKeys(raw) {
		if key == "_grove" {
			// Meta section (priority etc.), read by extractConfigMeta.
			w.emit(key, AuditKnownCore)
			continue
		}
		if sf, ok := fields[key]; ok {
			if fieldDeprecated(sf) {
				w.emit(key, AuditDeprecated)
				continue
			}
			w.walkValue(sf.Type, raw[key], key)
			continue
		}
		if _, ok := KnownExtension(key); ok {
			w.emit(key, AuditKnownExtension)
			continue
		}
		w.emit(key, AuditOrphan)
	}
}

// walkValue classifies the raw value at path against the expected Go type.
// Structs check each raw key against the field tags (mismatches are
// unknown-nested); typed maps descend into their element type per key;
// free-form values (interface{} leaves, map[string]interface{} elements)
// terminate as known-core since code owns those shapes.
func (w *auditWalker) walkValue(t reflect.Type, v interface{}, path string) {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if auditFreeFormPaths[path] {
		w.emit(path, AuditKnownCore)
		return
	}

	switch t.Kind() {
	case reflect.Struct:
		m, ok := v.(map[string]interface{})
		if !ok {
			// Scalar where a table is expected — a decode problem, not a
			// relevance one; the key itself is known.
			w.emit(path, AuditKnownCore)
			return
		}
		if len(m) == 0 {
			w.emit(path, AuditKnownCore)
			return
		}
		fields := structFieldsByConfigKey(t)
		for _, key := range sortedRawKeys(m) {
			childPath := path + "." + key
			sf, ok := fields[key]
			if !ok {
				w.emit(childPath, AuditUnknownNested)
				continue
			}
			if fieldDeprecated(sf) {
				w.emit(childPath, AuditDeprecated)
				continue
			}
			w.walkValue(sf.Type, m[key], childPath)
		}

	case reflect.Map:
		elem := t.Elem()
		for elem.Kind() == reflect.Ptr {
			elem = elem.Elem()
		}
		// map[string]interface{} values are free-form by design
		// (environment.config, commands, …) — stop descending.
		if elem.Kind() == reflect.Interface {
			w.emit(path, AuditKnownCore)
			return
		}
		m, ok := v.(map[string]interface{})
		if !ok || len(m) == 0 {
			w.emit(path, AuditKnownCore)
			return
		}
		// Map keys are user-defined (grove names, env profiles, …); only
		// the element shape is checked.
		for _, key := range sortedRawKeys(m) {
			w.walkValue(elem, m[key], path+"."+key)
		}

	case reflect.Slice, reflect.Array:
		elem := t.Elem()
		for elem.Kind() == reflect.Ptr {
			elem = elem.Elem()
		}
		arr, ok := v.([]interface{})
		if elem.Kind() != reflect.Struct || !ok || len(arr) == 0 {
			// Scalar arrays (workspaces, build_after, …) are leaves.
			w.emit(path, AuditKnownCore)
			return
		}
		// Array of tables (explicit_projects, test_scopes): check each
		// element's keys against the element struct.
		for i, item := range arr {
			w.walkValue(elem, item, fmt.Sprintf("%s[%d]", path, i))
		}

	default:
		// Scalars, interface{} leaves: a matched, readable key.
		w.emit(path, AuditKnownCore)
	}
}

// structFieldsByConfigKey maps a struct's config key names (from yaml/toml
// tags) to their fields. Fields hidden from both codecs (yaml:"-" toml:"-",
// like the inline Extensions catch-all) are omitted.
func structFieldsByConfigKey(t reflect.Type) map[string]reflect.StructField {
	fields := make(map[string]reflect.StructField)
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.PkgPath != "" {
			continue // unexported
		}
		if key := fieldConfigKey(sf); key != "" {
			fields[key] = sf
		}
	}
	return fields
}

// fieldConfigKey returns the config file key for a struct field, preferring
// the yaml tag over the toml tag (they match everywhere except fields decoded
// by post-processing, where yaml still carries the real key). Returns "" for
// fields hidden from both codecs.
func fieldConfigKey(sf reflect.StructField) string {
	for _, tag := range []string{sf.Tag.Get("yaml"), sf.Tag.Get("toml")} {
		name := strings.Split(tag, ",")[0]
		if name != "" && name != "-" {
			return name
		}
	}
	return ""
}

// fieldDeprecated reports whether a struct field carries deprecation markers
// in its schema tags (e.g. search_paths' `deprecated=true` /
// `x-deprecated=true`).
func fieldDeprecated(sf reflect.StructField) bool {
	return strings.Contains(sf.Tag.Get("jsonschema"), "deprecated=true") ||
		strings.Contains(sf.Tag.Get("jsonschema_extras"), "x-deprecated=true")
}

// sortedRawKeys returns the map's keys sorted for deterministic output.
func sortedRawKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
