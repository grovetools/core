package config

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// This file is the structural guard against the bug class that produced it: a
// field added to Config (or to one of the structs it points at) with no clause
// in mergeConfigs. That failure is SILENT — the key parses, validates, passes
// the exec-provenance gate, and is then dropped by the merge, so the value
// survives only when it happens to be written in the base layer. The whole
// [daemon] table lived that way, [tui.plugins] before it, and [[test_scopes]]
// before that; each was found by a user wondering why their config did nothing.
//
// TestMergeConfigsCoversEveryConfigField walks Config by reflection, populates
// ONE top-level table at a time in an override layer, merges it over an empty
// base, and reports every leaf that did not arrive. Anything deliberately not
// merged has to be named in mergeCoverageExempt with the reason, so "this field
// does not merge" becomes a written decision instead of an oversight.
//
// What it does NOT cover: it merges over an EMPTY base, so it proves a value
// can reach the merged config at all. Field-wise merges that drop a subfield
// only when BOTH layers set the same key (Notebooks.Definitions is the live
// example) need their own tests — see TestMergeConfigsSameKeyCollision below
// for the ones that are pinned.

// mergeCoverageExempt names leaf paths that mergeConfigs deliberately does not
// carry from an override layer, with the reason. Paths are Go field paths
// rooted at Config ("Groves", "TUI.Drawer.Pages"), and a listed path exempts
// its whole subtree.
var mergeCoverageExempt = map[string]string{
	"Groves":   "not authorable: compiled from roots.toml/notebooks.toml by compileRoots AFTER the merge, so a layer can neither set nor override it",
	"ExecGate": "not authorable: the loaders write the exec-provenance report onto the merged config after merging; it is a result, not an input",
}

// TestMergeConfigsCoversEveryConfigField is the regression test the ticket for
// the dropped [daemon] table asked for: every field of Config either survives a
// merge from an override layer, or is named in mergeCoverageExempt.
func TestMergeConfigsCoversEveryConfigField(t *testing.T) {
	cfgType := reflect.TypeOf(Config{})
	for i := 0; i < cfgType.NumField(); i++ {
		field := cfgType.Field(i)
		if field.PkgPath != "" {
			continue // unexported
		}
		if reason, exempt := mergeCoverageExempt[field.Name]; exempt {
			if reason == "" {
				t.Errorf("%s is exempt from merge coverage with no reason given", field.Name)
			}
			continue
		}

		t.Run(field.Name, func(t *testing.T) {
			override := &Config{}
			populate(reflect.ValueOf(override).Elem().Field(i), field.Name, map[reflect.Type]bool{})

			merged := mergeConfigs(&Config{}, override)

			var missing []string
			compareMerged(
				reflect.ValueOf(override).Elem().Field(i),
				reflect.ValueOf(merged).Elem().Field(i),
				field.Name,
				&missing,
			)
			for _, path := range missing {
				t.Errorf("%s was dropped by mergeConfigs: an override layer setting it is silently ignored.\n"+
					"Add a merge clause in mergeConfigs, or name the path in mergeCoverageExempt with the reason.", path)
			}
		})
	}
}

// TestMergeCoverageExemptionsAreLive keeps the exemption list from outliving
// the fields it excuses: a stale entry would silently re-open the hole it was
// written to document.
func TestMergeCoverageExemptionsAreLive(t *testing.T) {
	for path := range mergeCoverageExempt {
		if !fieldPathExists(reflect.TypeOf(Config{}), path) {
			t.Errorf("mergeCoverageExempt names %q, which is not a field path on Config any more", path)
		}
	}
}

// populate fills v with distinctive non-zero values so that a value which fails
// to arrive is distinguishable from one that arrived zeroed. `onPath` guards the
// self-referential types (DrawerNodeConfig.First/Second): a type already being
// populated further up the path is left at its zero value.
func populate(v reflect.Value, path string, onPath map[reflect.Type]bool) {
	switch v.Kind() {
	case reflect.String:
		// Written through reflect.Value.SetString rather than Set so named
		// string types (DrawerSize) are populated too.
		v.SetString("v:" + path)
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(int64(len(path))%97 + 1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(uint64(len(path))%97 + 1)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(float64(len(path)%97 + 1))
	case reflect.Interface:
		// The raw-map corners (Extensions, Environment.Config) are
		// interface-valued; a string leaf is all the merge needs to carry.
		v.Set(reflect.ValueOf("v:" + path))
	case reflect.Ptr:
		if onPath[v.Type().Elem()] {
			return // self-referential type: stop here
		}
		elem := reflect.New(v.Type().Elem())
		populate(elem.Elem(), path, onPath)
		v.Set(elem)
	case reflect.Slice:
		elem := reflect.New(v.Type().Elem()).Elem()
		populate(elem, path+"[0]", onPath)
		v.Set(reflect.Append(reflect.MakeSlice(v.Type(), 0, 1), elem))
	case reflect.Map:
		key := reflect.New(v.Type().Key()).Elem()
		populate(key, path+".key", onPath)
		val := reflect.New(v.Type().Elem()).Elem()
		populate(val, path+".value", onPath)
		m := reflect.MakeMap(v.Type())
		m.SetMapIndex(key, val)
		v.Set(m)
	case reflect.Struct:
		onPath[v.Type()] = true
		defer delete(onPath, v.Type())
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			if field.PkgPath != "" {
				continue // unexported
			}
			populate(v.Field(i), path+"."+field.Name, onPath)
		}
	}
}

// compareMerged walks the populated override value beside the merged one and
// appends the path of every leaf the merge failed to carry. Exempt paths (and
// their subtrees) are skipped.
func compareMerged(want, got reflect.Value, path string, missing *[]string) {
	if _, exempt := mergeCoverageExempt[path]; exempt {
		return
	}

	switch want.Kind() {
	case reflect.Ptr:
		if want.IsNil() {
			return
		}
		if got.Kind() != reflect.Ptr || got.IsNil() {
			*missing = append(*missing, path)
			return
		}
		compareMerged(want.Elem(), got.Elem(), path, missing)
	case reflect.Struct:
		for i := 0; i < want.NumField(); i++ {
			field := want.Type().Field(i)
			if field.PkgPath != "" {
				continue
			}
			compareMerged(want.Field(i), got.Field(i), path+"."+field.Name, missing)
		}
	case reflect.Map:
		if want.Len() == 0 {
			return
		}
		for _, key := range want.MapKeys() {
			gotVal := got.MapIndex(key)
			if !gotVal.IsValid() {
				*missing = append(*missing, fmt.Sprintf("%s[%v]", path, key.Interface()))
				continue
			}
			compareMerged(want.MapIndex(key), gotVal, fmt.Sprintf("%s[%v]", path, key.Interface()), missing)
		}
	case reflect.Interface:
		if want.IsNil() {
			return
		}
		if got.Kind() == reflect.Interface && !got.IsNil() {
			compareMerged(want.Elem(), got.Elem(), path, missing)
			return
		}
		*missing = append(*missing, path)
	case reflect.Slice:
		if want.Len() == 0 {
			return
		}
		if got.Len() != want.Len() {
			*missing = append(*missing, path)
			return
		}
		for i := 0; i < want.Len(); i++ {
			compareMerged(want.Index(i), got.Index(i), fmt.Sprintf("%s[%d]", path, i), missing)
		}
	default:
		if !reflect.DeepEqual(want.Interface(), got.Interface()) {
			*missing = append(*missing, path)
		}
	}
}

// fieldPathExists resolves a dotted Go field path against a struct type,
// following pointers, slices and maps the way compareMerged's paths are built.
func fieldPathExists(t reflect.Type, path string) bool {
	for _, name := range strings.Split(path, ".") {
		for t.Kind() == reflect.Ptr || t.Kind() == reflect.Slice || t.Kind() == reflect.Map {
			t = t.Elem()
		}
		if t.Kind() != reflect.Struct {
			return false
		}
		field, ok := t.FieldByName(name)
		if !ok {
			return false
		}
		t = field.Type
	}
	return true
}
