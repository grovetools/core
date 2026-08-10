package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadWithOverrides loads configuration with override files
func LoadWithOverrides(baseFile string) (*Config, error) {
	// Load the base UNCACHED. mergeConfigs shallow-copies its base and then
	// writes through the copy's still-shared nested pointers (Worktree, TUI,
	// …), so handing it a memoized *Config would let an override file edit
	// the cached entry every other Load caller sees. This path is cold — a
	// full parse here costs nothing worth defending.
	config, err := loadUncached(baseFile)
	if err != nil {
		return nil, err
	}

	// Look for override files
	dir := filepath.Dir(baseFile)
	overrides := projectOverrideFiles(dir)

	for _, overrideFile := range overrides {
		if _, err := os.Stat(overrideFile); err == nil {
			// Load override without validation
			data, err := os.ReadFile(overrideFile)
			if err != nil {
				return nil, fmt.Errorf("read override %s: %w", overrideFile, err)
			}

			// Expand environment variables
			expanded := expandEnvVars(string(data))
			override, parseErr := unmarshalConfig(overrideFile, []byte(expanded))
			if parseErr != nil {
				return nil, fmt.Errorf("parse override %s: %w", overrideFile, parseErr)
			}

			config = mergeConfigs(config, override)
		}
	}

	return config, nil
}

// mergeKeybindingSection merges override keybindings into base.
// Override values replace base values for the same action key.
func mergeKeybindingSection(base, override KeybindingSectionConfig) KeybindingSectionConfig {
	if override == nil {
		return base
	}
	if base == nil {
		result := make(KeybindingSectionConfig)
		for k, v := range override {
			result[k] = v
		}
		return result
	}
	result := make(KeybindingSectionConfig)
	for k, v := range base {
		result[k] = v
	}
	for k, v := range override {
		result[k] = v
	}
	return result
}

// deepMergeMaps recursively merges two maps, with src values overriding dst values.
// When both dst and src have the same key pointing to maps, they are merged recursively.
//
// Delete sentinel: if a src value is a map containing `_delete = true`, the
// corresponding key is removed from the merged result instead of merged. This
// lets a profile drop an inherited entry it doesn't want — e.g. a hybrid env
// dropping the default `services.clickhouse` block. Without this, deepMergeMaps
// has no way to express deletion and profiles have to resort to empty-command
// short-circuit hacks or `$VAR` indirection.
func deepMergeMaps(dst, src map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{})
	for k, v := range dst {
		out[k] = v
	}
	for k, vSrc := range src {
		// Delete sentinel: `_delete = true` in src drops the key entirely.
		if mapSrc, ok := vSrc.(map[string]interface{}); ok {
			if del, _ := mapSrc["_delete"].(bool); del {
				delete(out, k)
				continue
			}
		}
		if vDst, ok := out[k]; ok {
			if mapDst, okDst := vDst.(map[string]interface{}); okDst {
				if mapSrc, okSrc := vSrc.(map[string]interface{}); okSrc {
					out[k] = deepMergeMaps(mapDst, mapSrc)
					continue
				}
			}
		}
		out[k] = vSrc
	}
	return out
}

// ExtensionMergePolicy controls how a single [extension] block merges across
// the grove config cascade. By default extension blocks merge like any other
// config (deepMergeMaps: array/scalar leaves whole-replace, maps recurse). A
// policy with AccumulateArrays=true switches array leaves under that extension
// to UNION (accumulate down the cascade) instead, with an opt-out keyed by
// InheritKey: a block whose `<InheritKey> = false` resets accumulation beneath
// it (clean slate).
//
// This reproduces ClaudeConfig.Merge's semantics (core/pkg/claudenotebook/
// config.go) in raw-map form, because core/config must NOT import the leaf
// claudenotebook package. The two impls are kept behaviorally in sync — see the
// drift note on unionRawArrays below.
type ExtensionMergePolicy struct {
	// AccumulateArrays unions array leaves down the cascade instead of
	// whole-replacing them.
	AccumulateArrays bool
	// InheritKey is the bool key (e.g. "inherit") read at the top of an
	// extension block: when explicitly false, that layer's subtree replaces
	// the accumulated-below subtree wholesale.
	InheritKey string
}

// extensionMergePolicies maps an extension key to its merge policy. Keys absent
// from this map use the default whole-replace deepMergeMaps semantics, so
// existing extensions (skills, notify, settings, …) are unaffected.
var extensionMergePolicies = map[string]ExtensionMergePolicy{
	"claude": {AccumulateArrays: true, InheritKey: "inherit"},
}

// RegisterExtensionMergePolicy registers (or overrides) the merge policy for an
// extension key. Intended for downstream packages that want accumulate-down
// semantics for their own [extension] block.
func RegisterExtensionMergePolicy(key string, p ExtensionMergePolicy) {
	extensionMergePolicies[key] = p
}

// mergeExtensions merges the override Extensions map into dst, dispatching per
// extension key: keys WITH an accumulate policy union their array leaves (and
// honor the per-block inherit opt-out); keys WITHOUT a policy fall back to the
// existing whole-replace deepMergeMaps semantics. This is the cascade analog of
// ClaudeConfig.Merge for the raw-map (merge-then-decode) path.
func mergeExtensions(dst, src map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{})
	for k, v := range dst {
		out[k] = v
	}
	for k, vSrc := range src {
		// Delete sentinel parity with deepMergeMaps: `_delete = true` drops
		// the key entirely.
		if mapSrc, ok := vSrc.(map[string]interface{}); ok {
			if del, _ := mapSrc["_delete"].(bool); del {
				delete(out, k)
				continue
			}
		}

		policy, hasPolicy := extensionMergePolicies[k]
		if hasPolicy && policy.AccumulateArrays {
			if vDst, ok := out[k]; ok {
				if mapDst, okDst := vDst.(map[string]interface{}); okDst {
					if mapSrc, okSrc := vSrc.(map[string]interface{}); okSrc {
						out[k] = deepMergeMapsUnionWithInherit(mapDst, mapSrc, policy)
						continue
					}
				}
			}
			out[k] = vSrc
			continue
		}

		// No accumulate policy: replicate deepMergeMaps' per-key behavior
		// (maps recurse, everything else whole-replaces).
		if vDst, ok := out[k]; ok {
			if mapDst, okDst := vDst.(map[string]interface{}); okDst {
				if mapSrc, okSrc := vSrc.(map[string]interface{}); okSrc {
					out[k] = deepMergeMaps(mapDst, mapSrc)
					continue
				}
			}
		}
		out[k] = vSrc
	}
	return out
}

// deepMergeMapsUnionWithInherit merges src into dst with array-union leaf
// semantics, honoring the per-block inherit opt-out. It is invoked at the top
// of an accumulating extension block (e.g. the `claude` map). If
// src[policy.InheritKey] is the bool false, src REPLACES dst wholesale (clears
// accumulation from lower cascade layers); if absent or true, array leaves of
// dst and src are unioned. The inherit key governs only THIS layer's reset; it
// does not propagate into nested recursion.
func deepMergeMapsUnionWithInherit(dst, src map[string]interface{}, policy ExtensionMergePolicy) map[string]interface{} {
	if policy.InheritKey != "" {
		if inherit, ok := src[policy.InheritKey].(bool); ok && !inherit {
			// Clean slate: src subtree replaces the accumulated-below subtree.
			return deepMergeMapsUnion(map[string]interface{}{}, src)
		}
	}
	return deepMergeMapsUnion(dst, src)
}

// deepMergeMapsUnion recursively merges src into dst like deepMergeMaps, except
// that two array leaves at the same key are UNIONED (order-preserving, deduped)
// instead of whole-replaced. Nested maps recurse; scalars and other non-array
// leaves keep highest-wins. The `_delete = true` sentinel is preserved.
func deepMergeMapsUnion(dst, src map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{})
	for k, v := range dst {
		out[k] = v
	}
	for k, vSrc := range src {
		// Delete sentinel: `_delete = true` in src drops the key entirely.
		if mapSrc, ok := vSrc.(map[string]interface{}); ok {
			if del, _ := mapSrc["_delete"].(bool); del {
				delete(out, k)
				continue
			}
		}
		if vDst, ok := out[k]; ok {
			// Both maps: recurse.
			if mapDst, okDst := vDst.(map[string]interface{}); okDst {
				if mapSrc, okSrc := vSrc.(map[string]interface{}); okSrc {
					out[k] = deepMergeMapsUnion(mapDst, mapSrc)
					continue
				}
			}
			// Both arrays: union.
			if isRawArray(vDst) && isRawArray(vSrc) {
				out[k] = unionRawArrays(vDst, vSrc)
				continue
			}
		}
		// Scalar / non-array leaf / type mismatch: highest-wins.
		out[k] = vSrc
	}
	return out
}

// isRawArray reports whether v is a config array leaf. Raw cascade maps decoded
// from YAML/TOML use []interface{}, but hand-built maps (and some test
// fixtures) may use []string — both count.
func isRawArray(v interface{}) bool {
	switch v.(type) {
	case []interface{}, []string:
		return true
	default:
		return false
	}
}

// unionRawArrays returns the order-preserving, deduped union of two raw config
// array leaves (a's elements first, then b's new elements). It handles both
// []interface{} and []string element containers, coercing each element to its
// fmt "%v" form for the dedupe key.
//
// DRIFT NOTE: this is the raw-map analog of unionStrings in
// core/pkg/claudenotebook/config.go. The two implement one semantics across a
// package boundary that forbids sharing (core/config must not import the leaf
// claudenotebook); keep them behaviorally in sync.
func unionRawArrays(a, b interface{}) []interface{} {
	seen := make(map[string]struct{})
	out := make([]interface{}, 0)
	add := func(arr interface{}) {
		switch v := arr.(type) {
		case []interface{}:
			for _, e := range v {
				key := fmt.Sprintf("%v", e)
				if _, ok := seen[key]; !ok {
					seen[key] = struct{}{}
					out = append(out, e)
				}
			}
		case []string:
			for _, e := range v {
				if _, ok := seen[e]; !ok {
					seen[e] = struct{}{}
					out = append(out, e)
				}
			}
		}
	}
	add(a)
	add(b)
	return out
}

// deepMergeMapsWithProvenance mirrors deepMergeMaps but also records which
// layer contributed each leaf value. `sourceLabel` identifies the current src
// layer (e.g. "project (environments.hybrid-api)"). `prefix` is the dotted
// path prefix of the map being merged (e.g. "config" for an env Config map).
// `prov` accumulates leaf path → sourceLabel; `deleted` accumulates paths
// removed via the `_delete = true` sentinel, mapped to the layer that removed
// them.
//
// This is additive: existing callers of deepMergeMaps remain unchanged.
func deepMergeMapsWithProvenance(dst, src map[string]interface{}, sourceLabel, prefix string, prov, deleted map[string]string) map[string]interface{} {
	out := make(map[string]interface{})
	for k, v := range dst {
		out[k] = v
	}
	for k, vSrc := range src {
		currentPath := k
		if prefix != "" {
			currentPath = prefix + "." + k
		}

		// Delete sentinel: `_delete = true` drops the key and its subtree.
		if mapSrc, ok := vSrc.(map[string]interface{}); ok {
			if del, _ := mapSrc["_delete"].(bool); del {
				delete(out, k)
				prunePathAndDescendants(prov, deleted, currentPath)
				deleted[currentPath] = sourceLabel
				continue
			}
		}

		if vDst, okDst := out[k]; okDst {
			if mapDst, dstIsMap := vDst.(map[string]interface{}); dstIsMap {
				if mapSrc, srcIsMap := vSrc.(map[string]interface{}); srcIsMap {
					out[k] = deepMergeMapsWithProvenance(mapDst, mapSrc, sourceLabel, currentPath, prov, deleted)
					continue
				}
			}
		}

		// Scalar, array, or map replacing a non-map (or unset). Prune any
		// stale provenance under currentPath — we just replaced the subtree.
		prunePathAndDescendants(prov, deleted, currentPath)
		out[k] = vSrc

		if mapSrc, ok := vSrc.(map[string]interface{}); ok {
			recordMapProvenance(mapSrc, sourceLabel, currentPath, prov)
		} else {
			prov[currentPath] = sourceLabel
		}
	}
	return out
}

// prunePathAndDescendants removes entries at `path` and any child of `path`
// (i.e. keys beginning with `path + "."`) from both provenance maps.
func prunePathAndDescendants(prov, deleted map[string]string, path string) {
	delete(prov, path)
	delete(deleted, path)
	childPrefix := path + "."
	for k := range prov {
		if strings.HasPrefix(k, childPrefix) {
			delete(prov, k)
		}
	}
	for k := range deleted {
		if strings.HasPrefix(k, childPrefix) {
			delete(deleted, k)
		}
	}
}

// recordMapProvenance walks a fresh map and records leaf provenance for every
// scalar or array value it contains at `sourceLabel`. Used when a whole
// subtree is freshly introduced by a merge layer.
func recordMapProvenance(m map[string]interface{}, sourceLabel, prefix string, prov map[string]string) {
	for k, v := range m {
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}
		if inner, ok := v.(map[string]interface{}); ok {
			recordMapProvenance(inner, sourceLabel, path, prov)
			continue
		}
		prov[path] = sourceLabel
	}
}

func cloneDrawerNode(node *DrawerNodeConfig) *DrawerNodeConfig {
	if node == nil {
		return nil
	}
	cloned := *node
	cloned.First = cloneDrawerNode(node.First)
	cloned.Second = cloneDrawerNode(node.Second)
	return &cloned
}

func cloneDrawerPage(page *DrawerPageConfig) *DrawerPageConfig {
	if page == nil {
		return nil
	}
	cloned := *page
	cloned.Layout = cloneDrawerNode(page.Layout)
	return &cloned
}

func cloneDrawerViews(drawer *DrawerViewsConfig) *DrawerViewsConfig {
	if drawer == nil {
		return nil
	}
	cloned := *drawer
	if drawer.PageOrder != nil {
		cloned.PageOrder = make([]string, len(drawer.PageOrder))
		copy(cloned.PageOrder, drawer.PageOrder)
	}
	if drawer.Pages != nil {
		cloned.Pages = make(map[string]*DrawerPageConfig, len(drawer.Pages))
		for name, page := range drawer.Pages {
			cloned.Pages[name] = cloneDrawerPage(page)
		}
	}
	if drawer.Files != nil {
		files := *drawer.Files
		cloned.Files = &files
	}
	if drawer.Panes != nil {
		cloned.Panes = make(map[string]*DrawerPaneConfig, len(drawer.Panes))
		for name, pane := range drawer.Panes {
			cloned.Panes[name] = cloneDrawerPane(pane)
		}
	}
	return &cloned
}

// cloneDrawerPane deep-copies one pane declaration. The slices and the settings
// map are copied rather than shared because a merged result is owned by the
// caller: a layer that appended to a retained Args would otherwise edit the
// layer it inherited from.
func cloneDrawerPane(pane *DrawerPaneConfig) *DrawerPaneConfig {
	if pane == nil {
		return nil
	}
	cloned := *pane
	if pane.Args != nil {
		cloned.Args = append([]string(nil), pane.Args...)
	}
	if pane.Env != nil {
		cloned.Env = append([]string(nil), pane.Env...)
	}
	if pane.Keys != nil {
		cloned.Keys = append([]PluginKey(nil), pane.Keys...)
	}
	if pane.Views != nil {
		cloned.Views = append([]PluginView(nil), pane.Views...)
	}
	if pane.Settings != nil {
		cloned.Settings = make(map[string]interface{}, len(pane.Settings))
		for k, v := range pane.Settings {
			cloned.Settings[k] = v
		}
	}
	return &cloned
}

// mergeConfigs merges override configuration into base
func mergeConfigs(base, override *Config) *Config {
	result := *base

	// Merge simple string fields
	if override.Name != "" {
		result.Name = override.Name
	}
	if override.Version != "" {
		result.Version = override.Version
	}
	if override.BuildCmd != "" {
		result.BuildCmd = override.BuildCmd
	}

	// Merge slice fields (replace if present)
	if override.Workspaces != nil {
		result.Workspaces = override.Workspaces
	}
	if override.BuildAfter != nil {
		result.BuildAfter = override.BuildAfter
	}
	if override.ExplicitProjects != nil {
		result.ExplicitProjects = override.ExplicitProjects
	}
	// A repo's [[test_scopes]] live in its own grove.toml, which is the
	// project layer — i.e. always an override here whenever any earlier layer
	// (global config, ecosystem grove.toml) exists. Without this clause the
	// array parsed fine and was then dropped on the floor, so every repo that
	// declared scopes still ran its whole tend suite on every turn.
	if override.TestScopes != nil {
		result.TestScopes = override.TestScopes
	}

	// Merge worktree configuration
	if override.Worktree != nil {
		if result.Worktree == nil {
			result.Worktree = &WorktreeConfig{}
		}
		if override.Worktree.Layout != "" {
			result.Worktree.Layout = override.Worktree.Layout
		}
	}

	// Merge onboarding state. Or-style: a later layer can mark the flow
	// completed or move the resume marker, but a zero-value overlay never
	// un-completes it (same idiom as the bool merges in the TUI block).
	if override.Onboarding != nil {
		if result.Onboarding == nil {
			result.Onboarding = &OnboardingConfig{}
		}
		if override.Onboarding.Completed {
			result.Onboarding.Completed = true
		}
		if override.Onboarding.LastStep != "" {
			result.Onboarding.LastStep = override.Onboarding.LastStep
		}
	}

	// Merge security policy. Only user-controlled layers ever reach this
	// merge with a meaningful value — execTrustMode reads the knob from the
	// user-layer merge specifically, so a workspace setting it is inert.
	if override.Security != nil {
		if result.Security == nil {
			result.Security = &SecurityConfig{}
		} else {
			// result starts as a shallow copy of base; detach Security before
			// writing into it so merging does not mutate the base config.
			copied := *result.Security
			result.Security = &copied
		}
		if override.Security.ExecTrust != "" {
			result.Security.ExecTrust = override.Security.ExecTrust
		}
		// Pointer-valued: nil means "not set at this layer", which must fall
		// through to the layer below rather than overwrite it with false.
		if override.Security.InheritWorktreeTrust != nil {
			result.Security.InheritWorktreeTrust = override.Security.InheritWorktreeTrust
		}
	}

	// Merge the ecosystem identity card. Whole-card replacement, deliberately:
	// a card is one repo's identity, and field-wise merging two cards would
	// produce a chimera (one ecosystem's id wearing another's remotes). The
	// nearest layer that declares a card wins outright.
	if override.Ecosystem != nil {
		result.Ecosystem = override.Ecosystem
	}

	// Merge TUI configuration
	if override.TUI != nil {
		if result.TUI == nil {
			result.TUI = &TUIConfig{}
		} else {
			// result starts as a shallow copy of base; detach the TUI before
			// applying nested overrides so mergeConfigs does not mutate base.
			copied := *result.TUI
			result.TUI = &copied
		}
		if override.TUI.Icons != "" {
			result.TUI.Icons = override.TUI.Icons
		}
		if override.TUI.Theme != "" {
			result.TUI.Theme = override.TUI.Theme
		}
		if override.TUI.Preset != "" {
			result.TUI.Preset = override.TUI.Preset
		}
		if override.TUI.LeaderKey != "" {
			result.TUI.LeaderKey = override.TUI.LeaderKey
		}
		if override.TUI.ActionKey != "" {
			result.TUI.ActionKey = override.TUI.ActionKey
		}
		if override.TUI.NvimEmbed != nil {
			result.TUI.NvimEmbed = override.TUI.NvimEmbed
		}
		// drawer_size is an ordinary last-non-empty-wins scalar, matching the
		// drawer scalars below. Unset in an override layer means "no opinion",
		// not "back to the built-in default".
		if override.TUI.DrawerSize != "" {
			result.TUI.DrawerSize = override.TUI.DrawerSize
		}
		// drawer_orientation and drawer_expanded are the same two spellings of
		// "no opinion" the rest of this block uses — empty string, false — and
		// they need clauses for the same reason every other key does. Their
		// x-layer=global tag is a HINT to the config editor about where a key
		// usually belongs, not a rule about where it works: drawer_size carries
		// the identical tag and has always merged from any layer. Without these
		// two, an ecosystem grove.toml setting either one was silently ignored.
		if override.TUI.DrawerOrientation != "" {
			result.TUI.DrawerOrientation = override.TUI.DrawerOrientation
		}
		if override.TUI.DrawerExpanded {
			result.TUI.DrawerExpanded = true
		}

		// Drawer scalars use last-non-empty-wins, page_order uses
		// last-non-nil-wins, and pages are replaced as whole definitions per
		// key. Field-wise inheritance of built-in pages happens later in the
		// TUI host, not between configuration layers.
		if override.TUI.Drawer != nil {
			if result.TUI.Drawer == nil {
				result.TUI.Drawer = &DrawerViewsConfig{}
			} else {
				// Drawer values are owned by the merged result: clone retained
				// pages, order, and recursive layouts before applying overlays.
				result.TUI.Drawer = cloneDrawerViews(result.TUI.Drawer)
			}
			if override.TUI.Drawer.CycleKey != "" {
				result.TUI.Drawer.CycleKey = override.TUI.Drawer.CycleKey
			}
			if override.TUI.Drawer.DefaultPage != "" {
				result.TUI.Drawer.DefaultPage = override.TUI.Drawer.DefaultPage
			}
			if override.TUI.Drawer.PageOrder != nil {
				result.TUI.Drawer.PageOrder = make([]string, len(override.TUI.Drawer.PageOrder))
				copy(result.TUI.Drawer.PageOrder, override.TUI.Drawer.PageOrder)
			}
			if override.TUI.Drawer.Pages != nil {
				if result.TUI.Drawer.Pages == nil {
					result.TUI.Drawer.Pages = make(map[string]*DrawerPageConfig, len(override.TUI.Drawer.Pages))
				}
				for name, page := range override.TUI.Drawer.Pages {
					result.TUI.Drawer.Pages[name] = cloneDrawerPage(page)
				}
			}
			// The drawer's behaviour booleans are three-state (*bool), so unlike
			// the or-style plain bools elsewhere in this function they merge on
			// nil rather than on false — which is the whole point of the pointer.
			// A layer can therefore turn one OFF again, not just on: an ecosystem
			// that wants `responsive = false` under a global true has no other way
			// to say so, and an or-merge would make that config a silent no-op.
			if override.TUI.Drawer.Responsive != nil {
				result.TUI.Drawer.Responsive = override.TUI.Drawer.Responsive
			}
			if override.TUI.Drawer.HideInapplicablePages != nil {
				result.TUI.Drawer.HideInapplicablePages = override.TUI.Drawer.HideInapplicablePages
			}
			if override.TUI.Drawer.PageMapLongForm != nil {
				result.TUI.Drawer.PageMapLongForm = override.TUI.Drawer.PageMapLongForm
			}
			// Pane settings merge field-wise (last-non-empty-wins), unlike whole
			// page definitions: a layer that only names a files view must not have
			// to restate anything else the pane grows later.
			if override.TUI.Drawer.Files != nil && override.TUI.Drawer.Files.View != "" {
				if result.TUI.Drawer.Files == nil {
					result.TUI.Drawer.Files = &DrawerFilesConfig{}
				}
				result.TUI.Drawer.Files.View = override.TUI.Drawer.Files.View
			}
			// Pane DECLARATIONS replace wholesale, like page definitions and
			// unlike the field-wise Files block above: a declaration says what a
			// pane IS, and half of one layer's command with half of another's
			// args is a process nobody wrote down.
			if override.TUI.Drawer.Panes != nil {
				if result.TUI.Drawer.Panes == nil {
					result.TUI.Drawer.Panes = make(map[string]*DrawerPaneConfig, len(override.TUI.Drawer.Panes))
				}
				for name, pane := range override.TUI.Drawer.Panes {
					result.TUI.Drawer.Panes[name] = cloneDrawerPane(pane)
				}
			}
		}
		// Bool fields need explicit clauses or an override layer's value is
		// silently dropped (a false in an override can never un-set a true
		// in the base, matching the other or-style bool merges here).
		if override.TUI.HideSplashOnStartup {
			result.TUI.HideSplashOnStartup = true
		}
		if override.TUI.VimControlHjklPaneNav {
			result.TUI.VimControlHjklPaneNav = true
		}
		if override.TUI.Panels != nil {
			result.TUI.Panels = override.TUI.Panels
		}
		if len(override.TUI.PluginOrder) > 0 {
			result.TUI.PluginOrder = append([]string(nil), override.TUI.PluginOrder...)
		}
		// experimental_pages is a layer-local opt-in list, so the nearest
		// explicitly configured layer replaces the earlier list. In particular,
		// ~/.config/grove/tui.toml is a global fragment (an override here), not
		// the base global config; without this clause its parsed value was
		// silently discarded. Preserve nil as "not configured", while allowing
		// an explicit empty list to clear an inherited opt-in.
		if override.TUI.ExperimentalPages != nil {
			result.TUI.ExperimentalPages = append([]string{}, override.TUI.ExperimentalPages...)
		}

		// Merge [tui.plugins] per entry rather than wholesale. Plugin panels
		// arrive one file at a time — `grove plugin install` writes one
		// ~/.config/grove/plugins/<name>.toml per installed panel — so a
		// layer that declares one panel must not erase the panels declared by
		// the layers under it. Same-named entries are replaced, which is what
		// lets a user's own config override an installed panel's command.
		//
		// Without this clause [tui.plugins] survived only in the base layer
		// (mergeConfigs shallow-copies base and then rebuilds TUI field by
		// field), so a panel declared anywhere but ~/.config/grove/grove.toml
		// silently never appeared.
		if len(override.TUI.Plugins) > 0 {
			merged := make(map[string]*PluginConfig, len(result.TUI.Plugins)+len(override.TUI.Plugins))
			for name, plugin := range result.TUI.Plugins {
				merged[name] = plugin
			}
			for name, plugin := range override.TUI.Plugins {
				merged[name] = plugin
			}
			result.TUI.Plugins = merged
		}

		// Merge [tui.rail] field-wise (last-non-empty-wins), like the Focus
		// block below: a layer that only pins the shortcut policy must not
		// also have to restate max_shortcuts.
		if override.TUI.Rail != nil {
			if result.TUI.Rail == nil {
				result.TUI.Rail = &RailConfig{}
			} else {
				// Owned by the merged result — detach before overlaying so
				// mergeConfigs never writes through into the base layer.
				copied := *result.TUI.Rail
				result.TUI.Rail = &copied
			}
			if override.TUI.Rail.Shortcuts != "" {
				result.TUI.Rail.Shortcuts = override.TUI.Rail.Shortcuts
			}
			if override.TUI.Rail.MaxShortcuts != 0 {
				result.TUI.Rail.MaxShortcuts = override.TUI.Rail.MaxShortcuts
			}
		}

		// Merge Focus config
		if override.TUI.Focus != nil {
			if result.TUI.Focus == nil {
				result.TUI.Focus = &FocusConfig{}
			}
			if override.TUI.Focus.Style != "" {
				result.TUI.Focus.Style = override.TUI.Focus.Style
			}
			if override.TUI.Focus.ActiveColor != "" {
				result.TUI.Focus.ActiveColor = override.TUI.Focus.ActiveColor
			}
			if override.TUI.Focus.InactiveColor != "" {
				result.TUI.Focus.InactiveColor = override.TUI.Focus.InactiveColor
			}
			if override.TUI.Focus.Thickness != 0 {
				result.TUI.Focus.Thickness = override.TUI.Focus.Thickness
			}
			if override.TUI.Focus.DimInactive {
				result.TUI.Focus.DimInactive = true
			}
		}

		// Merge Keybindings
		if override.TUI.Keybindings != nil {
			if result.TUI.Keybindings == nil {
				result.TUI.Keybindings = &KeybindingsConfig{}
			}

			// Merge standard sections
			result.TUI.Keybindings.Navigation = mergeKeybindingSection(result.TUI.Keybindings.Navigation, override.TUI.Keybindings.Navigation)
			result.TUI.Keybindings.Selection = mergeKeybindingSection(result.TUI.Keybindings.Selection, override.TUI.Keybindings.Selection)
			result.TUI.Keybindings.Actions = mergeKeybindingSection(result.TUI.Keybindings.Actions, override.TUI.Keybindings.Actions)
			result.TUI.Keybindings.Search = mergeKeybindingSection(result.TUI.Keybindings.Search, override.TUI.Keybindings.Search)
			result.TUI.Keybindings.View = mergeKeybindingSection(result.TUI.Keybindings.View, override.TUI.Keybindings.View)
			result.TUI.Keybindings.Fold = mergeKeybindingSection(result.TUI.Keybindings.Fold, override.TUI.Keybindings.Fold)
			result.TUI.Keybindings.System = mergeKeybindingSection(result.TUI.Keybindings.System, override.TUI.Keybindings.System)

			// Merge TUIOverrides (per-TUI overrides) - these have yaml:"-" toml:"-" tags
			// so they must be manually merged to preserve them across config merges
			if override.TUI.Keybindings.TUIOverrides != nil {
				if result.TUI.Keybindings.TUIOverrides == nil {
					result.TUI.Keybindings.TUIOverrides = make(map[string]map[string]KeybindingSectionConfig)
				}
				for pkgName, pkgOverrides := range override.TUI.Keybindings.TUIOverrides {
					if result.TUI.Keybindings.TUIOverrides[pkgName] == nil {
						result.TUI.Keybindings.TUIOverrides[pkgName] = make(map[string]KeybindingSectionConfig)
					}
					for tuiName, tuiOverrides := range pkgOverrides {
						result.TUI.Keybindings.TUIOverrides[pkgName][tuiName] = mergeKeybindingSection(
							result.TUI.Keybindings.TUIOverrides[pkgName][tuiName],
							tuiOverrides,
						)
					}
				}
			}

			// Merge legacy Overrides map for backward compatibility
			if override.TUI.Keybindings.Overrides != nil {
				if result.TUI.Keybindings.Overrides == nil {
					result.TUI.Keybindings.Overrides = make(map[string]map[string]KeybindingSectionConfig)
				}
				for pkgName, pkgOverrides := range override.TUI.Keybindings.Overrides {
					if result.TUI.Keybindings.Overrides[pkgName] == nil {
						result.TUI.Keybindings.Overrides[pkgName] = make(map[string]KeybindingSectionConfig)
					}
					for tuiName, tuiOverrides := range pkgOverrides {
						result.TUI.Keybindings.Overrides[pkgName][tuiName] = mergeKeybindingSection(
							result.TUI.Keybindings.Overrides[pkgName][tuiName],
							tuiOverrides,
						)
					}
				}
			}
		}
	}

	// Merge Notebooks configuration (now nested under NotebooksConfig)
	if override.Notebooks != nil {
		if result.Notebooks == nil {
			result.Notebooks = &NotebooksConfig{}
		}

		// Merge Definitions
		if override.Notebooks.Definitions != nil {
			if result.Notebooks.Definitions == nil {
				result.Notebooks.Definitions = make(map[string]*Notebook)
			}
			for k, v := range override.Notebooks.Definitions {
				if v != nil {
					// Deep merge notebook fields instead of replacing
					if existing, exists := result.Notebooks.Definitions[k]; exists && existing != nil {
						merged := *existing // Copy existing
						// Override non-empty fields
						if v.RootDir != "" {
							merged.RootDir = v.RootDir
						}
						if v.NotesPathTemplate != "" {
							merged.NotesPathTemplate = v.NotesPathTemplate
						}
						if v.PlansPathTemplate != "" {
							merged.PlansPathTemplate = v.PlansPathTemplate
						}
						if v.ChatsPathTemplate != "" {
							merged.ChatsPathTemplate = v.ChatsPathTemplate
						}
						if v.TemplatesPathTemplate != "" {
							merged.TemplatesPathTemplate = v.TemplatesPathTemplate
						}
						if v.RecipesPathTemplate != "" {
							merged.RecipesPathTemplate = v.RecipesPathTemplate
						}
						if v.Types != nil {
							if merged.Types == nil {
								merged.Types = make(map[string]*NoteTypeConfig)
							}
							for typeKey, typeVal := range v.Types {
								merged.Types[typeKey] = typeVal
							}
						}
						result.Notebooks.Definitions[k] = &merged
					} else {
						// No existing notebook, just use the override
						result.Notebooks.Definitions[k] = v
					}
				}
			}
		}

		// Merge Rules
		if override.Notebooks.Rules != nil {
			if result.Notebooks.Rules == nil {
				result.Notebooks.Rules = &NotebookRules{}
			}
			if override.Notebooks.Rules.Default != "" {
				result.Notebooks.Rules.Default = override.Notebooks.Rules.Default
			}
			if override.Notebooks.Rules.Global != nil && override.Notebooks.Rules.Global.RootDir != "" {
				if result.Notebooks.Rules.Global == nil {
					result.Notebooks.Rules.Global = &GlobalNotebookConfig{}
				}
				result.Notebooks.Rules.Global.RootDir = override.Notebooks.Rules.Global.RootDir
			}
		}
	}

	// Merge Context configuration
	// keep this list in sync with the ContextConfig struct in types.go
	if override.Context != nil {
		if result.Context == nil {
			result.Context = &ContextConfig{}
		}
		if override.Context.DefaultRules != "" {
			result.Context.DefaultRules = override.Context.DefaultRules
		}
		if override.Context.DefaultRulesPath != "" {
			result.Context.DefaultRulesPath = override.Context.DefaultRulesPath
		}
		if override.Context.ReposDir != nil {
			result.Context.ReposDir = override.Context.ReposDir
		}
		if override.Context.AllowedPaths != nil {
			result.Context.AllowedPaths = override.Context.AllowedPaths
		}
		if override.Context.IncludedWorkspaces != nil {
			result.Context.IncludedWorkspaces = override.Context.IncludedWorkspaces
		}
		if override.Context.ExcludedWorkspaces != nil {
			result.Context.ExcludedWorkspaces = override.Context.ExcludedWorkspaces
		}
	}

	// Merge Environment configuration (deep merge)
	if override.Environment != nil {
		if result.Environment == nil {
			result.Environment = &EnvironmentConfig{}
		}
		if override.Environment.Provider != "" {
			result.Environment.Provider = override.Environment.Provider
		}
		if override.Environment.Command != "" {
			result.Environment.Command = override.Environment.Command
		}
		if override.Environment.Config != nil {
			if result.Environment.Config == nil {
				result.Environment.Config = make(map[string]interface{})
			}
			result.Environment.Config = deepMergeMaps(result.Environment.Config, override.Environment.Config)
		}
		if override.Environment.Commands != nil {
			if result.Environment.Commands == nil {
				result.Environment.Commands = make(map[string]interface{})
			}
			for k, v := range override.Environment.Commands {
				result.Environment.Commands[k] = v
			}
		}
	}

	// Merge Named Environments
	if override.Environments != nil {
		if result.Environments == nil {
			result.Environments = make(map[string]*EnvironmentConfig)
		}
		for name, envOverride := range override.Environments {
			existing, exists := result.Environments[name]
			if !exists || existing == nil {
				// Deep copy to avoid pointer pollution
				envCopy := *envOverride
				if envOverride.Config != nil {
					envCopy.Config = deepMergeMaps(nil, envOverride.Config)
				}
				if envOverride.Commands != nil {
					envCopy.Commands = make(map[string]interface{})
					for k, v := range envOverride.Commands {
						envCopy.Commands[k] = v
					}
				}
				result.Environments[name] = &envCopy
			} else {
				if envOverride.Provider != "" {
					existing.Provider = envOverride.Provider
				}
				if envOverride.Command != "" {
					existing.Command = envOverride.Command
				}
				if envOverride.Config != nil {
					if existing.Config == nil {
						existing.Config = make(map[string]interface{})
					}
					existing.Config = deepMergeMaps(existing.Config, envOverride.Config)
				}
				if envOverride.Commands != nil {
					if existing.Commands == nil {
						existing.Commands = make(map[string]interface{})
					}
					for k, v := range envOverride.Commands {
						existing.Commands[k] = v
					}
				}
			}
		}
	}

	// Merge extensions with recursive deep merge. mergeExtensions dispatches
	// per extension key: keys with an accumulate policy (e.g. "claude") union
	// their array leaves down the cascade; all other keys keep the existing
	// whole-replace deepMergeMaps semantics.
	if override.Extensions != nil {
		if result.Extensions == nil {
			result.Extensions = make(map[string]interface{})
		}
		result.Extensions = mergeExtensions(result.Extensions, override.Extensions)
	}

	return &result
}
