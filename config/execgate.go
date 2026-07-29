package config

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/grovetools/core/pkg/exectrust"
	"github.com/sirupsen/logrus"
)

// The exec-provenance gate.
//
// The 9-layer config cascade merges grove.toml files that come out of cloned
// repositories — ecosystem, project-notebook, project, and project-local
// override layers — into the effective config. Several config keys carry
// shell commands grove or one of its satellites executes: [[hooks.on_stop]]
// runs when an agent session stops, [tui.plugins] panels are spawned when
// treemux boots, [daemon.hooks.on_skill_sync] runs on a daemon skill sync,
// and so on. Honoring those unconditionally means cloning a repository and
// starting a session inside it is enough to give the repo's author code
// execution.
//
// This file is the single choke point that stops that. Every field whose
// value is executed is declared in execFields below; on load, values reaching
// the merged config from a repo-controlled layer are quarantined (stripped
// before merge) unless the user has explicitly trusted that config file via
// `grove config trust`. Values from user-controlled layers — the global
// config, ~/.config/grove fragments and plugins, the global override, and the
// GROVE_CONFIG_OVERLAY — are always honored: the user owns those files.
//
// Every consumer benefits without changing: the value never reaches the
// merged Config the consumer reads.

// ExecRisk classifies how a field's command comes to be executed, which sets
// how aggressively the default policy gates it.
type ExecRisk string

const (
	// RiskImplicit: the command runs without the user asking for it — a
	// session ending, a TUI booting, a keypress bound by the config itself,
	// an API key being resolved. This is the attack the gate exists for, and
	// the default policy quarantines these from untrusted layers.
	RiskImplicit ExecRisk = "implicit"
	// RiskExplicit: the command runs only because the user invoked the verb
	// that runs it (`grove build`, `grove env up`, `grove satellite up`). A
	// malicious value still executes, but the user chose to run the repo's
	// build. The default policy warns rather than strips, because
	// build_cmd/commands are the overwhelmingly common workspace keys and
	// stripping them by default would break every existing repo. Set
	// [security] exec_trust = "strict" to gate these too.
	RiskExplicit ExecRisk = "explicit"
)

// ExecTrustMode is the enforcement policy, from [security] exec_trust (read
// only from user-controlled layers — see execTrustMode) or the
// GROVE_EXEC_TRUST environment variable, which wins.
type ExecTrustMode string

const (
	// ExecTrustModeDefault quarantines implicit-risk exec values from
	// untrusted layers and warns about explicit-risk ones. The default.
	ExecTrustModeDefault ExecTrustMode = "default"
	// ExecTrustModeStrict quarantines every exec value from untrusted layers.
	ExecTrustModeStrict ExecTrustMode = "strict"
	// ExecTrustModeWarn never strips; it only reports. For diagnosing a
	// broken setup, or for users who accept the risk but want visibility.
	ExecTrustModeWarn ExecTrustMode = "warn"
	// ExecTrustModeOff disables the gate entirely, including its warnings.
	ExecTrustModeOff ExecTrustMode = "off"
)

// EnvExecTrust overrides the configured mode. Set it to a mode name; an
// unrecognized value is ignored (fail toward the configured policy, never
// toward "off" on a typo).
const EnvExecTrust = "GROVE_EXEC_TRUST"

// ExecField declares one config key whose value is executed.
//
// Path is a dotted key path in the config document namespace, using the same
// key names that appear in grove.toml. Two wildcards are understood:
//
//   - matches every key of a map (tui.plugins.*, environments.*)
//     []  matches every element of an array (hooks.on_stop[] — only needed
//     when the gate should reach INTO elements rather than drop the array)
//
// The path names the unit that gets quarantined, which is deliberately
// coarser than "the string that gets passed to sh -c". [tui.panels.bindings]
// entries are dropped whole, not just their `command`, because a binding that
// keeps its `args` would still launch the [tui.panels] default binary (or
// $EDITOR) with attacker-chosen arguments.
type ExecField struct {
	// Path is the dotted config key path (see above).
	Path string
	// Risk sets how the default policy treats the field.
	Risk ExecRisk
	// Consumer names what executes the value, for the warning text.
	Consumer string
	// Description is a short human-readable summary of what runs.
	Description string
	// SafeValues, when non-empty, exempts these exact scalar values from the
	// gate. Used for environment.provider, where "native"/"docker"/"cloud"
	// select a built-in provider and only other values resolve to a
	// grove-env-<name> binary on PATH.
	SafeValues []string
}

// execFields is the registry: every config key whose value grove or one of
// its satellites executes.
//
// Verified against the ecosystem source, not just the ticket's enumeration:
// a sweep of exec.Command/exec.CommandContext call sites reachable from a
// config value, plus a sweep of config struct fields whose yaml/toml key
// matches command/cmd/script/exec. Extension namespaces (keys under
// Config.Extensions) are covered too — [hooks], [keys], [anthropic] and
// friends are read via UnmarshalExtension, so their exec keys ride the same
// cascade as core fields.
//
// Downstream repos linked into one binary can add their own entries at init
// via RegisterExecField, mirroring RegisterExtension.
var execFields = []ExecField{
	// --- core Config fields -------------------------------------------------
	{
		Path: "hooks.on_stop", Risk: RiskImplicit, Consumer: "hooks",
		Description: "shell commands run when an agent session stops",
	},
	{
		Path: "daemon.hooks.on_skill_sync", Risk: RiskImplicit, Consumer: "groved",
		Description: "shell commands run after the daemon syncs skills",
	},
	{
		Path: "tui.plugins.*", Risk: RiskImplicit, Consumer: "treemux",
		Description: "process spawned in its own PTY rail panel at TUI startup",
	},
	{
		Path: "tui.panels.command", Risk: RiskImplicit, Consumer: "treemux",
		Description: "default binary spawned by panel keybindings",
	},
	{
		Path: "tui.panels.bindings.*", Risk: RiskImplicit, Consumer: "treemux",
		Description: "process spawned on a keypress (command/args/args_command)",
	},
	{
		Path: "notebooks.definitions.*.sync.token_command", Risk: RiskImplicit, Consumer: "core/sync",
		Description: "shell command run to resolve the notebook sync token",
	},
	{
		Path: "build_cmd", Risk: RiskExplicit, Consumer: "grove build",
		Description: "command run by the build orchestrator",
	},
	{
		Path: "commands", Risk: RiskExplicit, Consumer: "grove orchestrator",
		Description: "per-verb command overrides run by grove <verb>",
	},
	{
		Path: "environment.provider", Risk: RiskExplicit, Consumer: "grove env",
		Description: "custom provider resolved to a grove-env-<name> binary",
		SafeValues:  []string{"native", "docker", "cloud"},
	},
	{
		Path: "environment.command", Risk: RiskExplicit, Consumer: "grove env",
		Description: "path to the environment provider binary",
	},
	{
		Path: "environment.commands", Risk: RiskExplicit, Consumer: "grove env",
		Description: "named commands run in the environment (startup=true auto-runs)",
	},
	{
		Path: "environments.*.provider", Risk: RiskExplicit, Consumer: "grove env",
		Description: "custom provider resolved to a grove-env-<name> binary",
		SafeValues:  []string{"native", "docker", "cloud"},
	},
	{
		Path: "environments.*.command", Risk: RiskExplicit, Consumer: "grove env",
		Description: "path to the environment provider binary",
	},
	{
		Path: "environments.*.commands", Risk: RiskExplicit, Consumer: "grove env",
		Description: "named commands run in the environment (startup=true auto-runs)",
	},

	// --- extension namespaces ----------------------------------------------
	{
		Path: "keys.tmux.popups.*", Risk: RiskImplicit, Consumer: "grove keys",
		Description: "command bound to a tmux popup keybinding",
	},
	{
		Path: "keys.shell.bindings", Risk: RiskImplicit, Consumer: "grove keys",
		Description: "commands bound into the user's shell keybindings",
	},
	{
		Path: "keys.nvim.bindings.*.command", Risk: RiskImplicit, Consumer: "grove keys",
		Description: "vim command or Lua code bound into the user's neovim config",
	},
	{
		Path: "anthropic.api_key_command", Risk: RiskImplicit, Consumer: "grove-anthropic",
		Description: "shell command run to resolve the Anthropic API key",
	},
	{
		Path: "gemini.api_key_command", Risk: RiskImplicit, Consumer: "grove-gemini",
		Description: "shell command run to resolve the Gemini API key",
	},
	{
		Path: "openrouter.api_key_command", Risk: RiskImplicit, Consumer: "grove-openrouter",
		Description: "shell command run to resolve the OpenRouter API key",
	},
	{
		Path: "notifications.home_assistant.token_command", Risk: RiskImplicit, Consumer: "notify",
		Description: "shell command run to resolve the Home Assistant token",
	},
	{
		Path: "notifications.home_assistant.webhook_secret_command", Risk: RiskImplicit, Consumer: "notify",
		Description: "shell command run to resolve the Home Assistant webhook secret",
	},
	{
		Path: "flow.recipes.get_recipe_cmd", Risk: RiskExplicit, Consumer: "flow plan init",
		Description: "command run to produce dynamic recipe definitions as JSON",
	},
	{
		Path: "satellites.*.provision.gh_token_cmd", Risk: RiskExplicit, Consumer: "grove satellite up",
		Description: "local command run to mint the GitHub token piped to VM bootstrap",
	},
	{
		Path: "satellites.*.provision.claude_token_cmd", Risk: RiskExplicit, Consumer: "grove satellite up",
		Description: "local command run to mint the Claude OAuth token piped to VM bootstrap",
	},
}

// RegisterExecField registers (or replaces, by Path) an exec-bearing field.
// Intended for downstream packages that own an extension namespace carrying
// commands, mirroring RegisterExtension. Call from init so registration
// happens before any config load.
func RegisterExecField(f ExecField) {
	for i := range execFields {
		if execFields[i].Path == f.Path {
			execFields[i] = f
			return
		}
	}
	execFields = append(execFields, f)
}

// ExecFields returns the registry sorted by path.
func ExecFields() []ExecField {
	out := append([]ExecField(nil), execFields...)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// ExecFinding reports one exec-bearing value found in one config layer.
type ExecFinding struct {
	// Key is the concrete key path, wildcards resolved (e.g.
	// "tui.plugins.btop").
	Key string `json:"key"`
	// Value is a display rendering of what would be executed.
	Value string `json:"value"`
	// Risk is the declaring field's risk class.
	Risk ExecRisk `json:"risk"`
	// Consumer names what executes the value.
	Consumer string `json:"consumer"`
	// Description is the declaring field's summary.
	Description string `json:"description"`
	// Layer is the config layer the value came from.
	Layer ConfigSource `json:"layer"`
	// File is the config file that set it.
	File string `json:"file"`
	// Quarantined is true when the value was stripped before merge. False
	// means it is live in the merged config — either because the file is
	// trusted, or because the policy only warned about it.
	Quarantined bool `json:"quarantined"`
}

// ExecGateFile is the per-file trust verdict, and what `grove config trust`
// acts on.
type ExecGateFile struct {
	// Path is the config file.
	Path string `json:"path"`
	// Layer is the cascade layer it was loaded as.
	Layer ConfigSource `json:"layer"`
	// Digest identifies the exact exec values this file carries; trusting
	// the file records this digest, so a later edit re-closes the gate.
	Digest string `json:"digest"`
	// Trusted reports whether the user has trusted this file at this digest.
	Trusted bool `json:"trusted"`
}

// ExecGateReport is the outcome of the gate for one config load. It hangs off
// the loaded Config (Config.ExecGate) so any consumer — and `grove config
// trust` — can see what was ignored and why.
type ExecGateReport struct {
	// Mode is the policy that was applied.
	Mode ExecTrustMode `json:"mode"`
	// Files are the repo-controlled layer files that carried exec values.
	Files []ExecGateFile `json:"files"`
	// Findings are the individual exec values, in cascade order.
	Findings []ExecFinding `json:"findings"`
}

// Quarantined returns only the findings that were stripped.
func (r *ExecGateReport) Quarantined() []ExecFinding {
	if r == nil {
		return nil
	}
	var out []ExecFinding
	for _, f := range r.Findings {
		if f.Quarantined {
			out = append(out, f)
		}
	}
	return out
}

// HasQuarantined reports whether anything was stripped.
func (r *ExecGateReport) HasQuarantined() bool { return len(r.Quarantined()) > 0 }

// execTrustMode resolves the enforcement policy. GROVE_EXEC_TRUST wins;
// otherwise [security] exec_trust is read from userCfg, which MUST be the
// merge of user-controlled layers only. Reading the knob from the merged
// config would let the very layer being gated set exec_trust = "off".
func execTrustMode(userCfg *Config) ExecTrustMode {
	if env := strings.ToLower(strings.TrimSpace(os.Getenv(EnvExecTrust))); env != "" {
		switch ExecTrustMode(env) {
		case ExecTrustModeDefault, ExecTrustModeStrict, ExecTrustModeWarn, ExecTrustModeOff:
			return ExecTrustMode(env)
		}
	}
	if userCfg != nil && userCfg.Security != nil {
		switch mode := ExecTrustMode(strings.ToLower(strings.TrimSpace(userCfg.Security.ExecTrust))); mode {
		case ExecTrustModeDefault, ExecTrustModeStrict, ExecTrustModeWarn, ExecTrustModeOff:
			return mode
		}
	}
	return ExecTrustModeDefault
}

// stripsRisk reports whether mode quarantines a value of this risk class.
func (m ExecTrustMode) stripsRisk(risk ExecRisk) bool {
	switch m {
	case ExecTrustModeStrict:
		return true
	case ExecTrustModeDefault:
		return risk == RiskImplicit
	default: // warn, off
		return false
	}
}

// ScanExecValues reports every exec-bearing value present in cfg WITHOUT
// modifying it. Used to compute a file's trust digest and to preview what
// trusting a config would enable.
func ScanExecValues(cfg *Config) []ExecFinding {
	if cfg == nil {
		return nil
	}
	w := &execWalker{}
	for _, field := range ExecFields() {
		w.field = field
		w.walk(reflect.ValueOf(cfg).Elem(), strings.Split(field.Path, "."), field.Path, false)
	}
	return w.findings
}

// ExecDigest is the trust digest for the exec values cfg carries: the same
// value `grove config trust` records and IsTrusted compares against. An empty
// string means the config carries no exec values at all.
func ExecDigest(cfg *Config) string {
	findings := ScanExecValues(cfg)
	parts := make([]string, 0, len(findings))
	for _, f := range findings {
		parts = append(parts, f.Key+"="+f.Value)
	}
	return exectrust.Digest(parts)
}

// applyExecGate enforces the gate on ONE repo-controlled layer, in place,
// before it is merged. It returns the findings (annotated with layer/file and
// whether they were stripped) and the layer's trust verdict.
//
// trusted is passed in rather than looked up here so the caller can load the
// trust store once per config load instead of once per layer.
func applyExecGate(cfg *Config, source ConfigSource, file string, store *exectrust.Store, mode ExecTrustMode) ([]ExecFinding, ExecGateFile) {
	digest := ExecDigest(cfg)
	verdict := ExecGateFile{Path: file, Layer: source, Digest: digest, Trusted: store.IsTrusted(file, digest)}

	if mode == ExecTrustModeOff || digest == "" {
		return nil, verdict
	}

	w := &execWalker{}
	for _, field := range ExecFields() {
		w.field = field
		// A trusted file keeps everything; an untrusted one keeps whatever
		// the policy does not gate. Either way we still walk, so the report
		// lists what is live as well as what was dropped.
		w.strip = !verdict.Trusted && mode.stripsRisk(field.Risk)
		w.walk(reflect.ValueOf(cfg).Elem(), strings.Split(field.Path, "."), field.Path, w.strip)
	}

	for i := range w.findings {
		w.findings[i].Layer = source
		w.findings[i].File = file
	}
	return w.findings, verdict
}

// execGateRun carries the per-load gate state: the resolved policy and the
// trust store (both loaded once), plus the findings accumulated across
// layers. Both loaders drive it the same way — newExecGateRun as soon as the
// user-controlled layers are merged, gate.apply on each repo-controlled layer
// before it is merged, gate.report once at the end.
type execGateRun struct {
	mode     ExecTrustMode
	store    *exectrust.Store
	files    []ExecGateFile
	findings []ExecFinding
}

// newExecGateRun resolves the policy from userCfg — which MUST be the merge
// of user-controlled layers only — and loads the trust store.
func newExecGateRun(userCfg *Config, logger *logrus.Logger) *execGateRun {
	mode := execTrustMode(userCfg)
	run := &execGateRun{mode: mode}
	if mode != ExecTrustModeOff {
		run.store = exectrust.Load()
		if logger != nil && run.store != nil && exectrust.StorePath() == "" {
			logger.Debug("exec-trust store path unresolvable; treating every workspace as untrusted")
		}
	}
	return run
}

// userLayerConfig merges the user-controlled layers of a partially-populated
// LayeredConfig — everything the user owns on this machine, and nothing that
// came out of a repository. LoadLayered calls it at the same point in the
// cascade where LoadFromWithLogger snapshots finalConfig, so both loaders
// resolve the exec-trust policy from an identical view.
func userLayerConfig(layered *LayeredConfig) *Config {
	if layered == nil {
		return nil
	}
	out := &Config{}
	if layered.Global != nil {
		out = layered.Global
	}
	for _, fragment := range layered.GlobalFragments {
		out = mergeConfigs(out, fragment.Config)
	}
	if layered.GlobalOverride != nil {
		out = mergeConfigs(out, layered.GlobalOverride.Config)
	}
	if layered.EnvOverlay != nil {
		out = mergeConfigs(out, layered.EnvOverlay.Config)
	}
	return out
}

// apply gates one repo-controlled layer in place, before it is merged.
func (g *execGateRun) apply(cfg *Config, source ConfigSource, file string) {
	if g == nil || g.mode == ExecTrustModeOff || cfg == nil {
		return
	}
	findings, verdict := applyExecGate(cfg, source, file, g.store, g.mode)
	if verdict.Digest != "" {
		g.files = append(g.files, verdict)
	}
	g.findings = append(g.findings, findings...)
}

// report returns the accumulated report, or nil when no repo-controlled layer
// carried any exec-bearing config (the overwhelmingly common case — keep the
// loaded Config clean rather than hanging an empty struct off every load).
func (g *execGateRun) report() *ExecGateReport {
	if g == nil || len(g.findings) == 0 && len(g.files) == 0 {
		return nil
	}
	return &ExecGateReport{Mode: g.mode, Files: g.files, Findings: g.findings}
}

// execWarnOnce dedupes the gate's warnings per process. LoadFrom is called on
// hot paths — the daemon's fsnotify handlers, the nav ticker, cx rendering —
// and the 2s load cache does not span them. Keyed by file+digest, so an edit
// to a quarantined config warns again.
var execWarnOnce sync.Map // map[string]struct{}

// warnExecGate emits the loud warning the gate owes the user: what was
// ignored, where it came from, and how to allow it. Warnings are emitted once
// per file+digest per process (see execWarnOnce).
func warnExecGate(report *ExecGateReport, logger *logrus.Logger) {
	if report == nil || logger == nil || len(report.Findings) == 0 {
		return
	}
	fresh := false
	for _, f := range report.Files {
		if _, seen := execWarnOnce.LoadOrStore(f.Path+"\x00"+f.Digest, struct{}{}); !seen {
			fresh = true
		}
	}
	if !fresh {
		return
	}
	quarantined := report.Quarantined()
	if len(quarantined) == 0 {
		// Nothing stripped. In warn mode, untrusted-but-honored values are
		// still worth a single line; in default/strict mode a live value
		// means the file is trusted, which needs no warning.
		if report.Mode != ExecTrustModeWarn {
			return
		}
		for _, f := range report.Findings {
			logger.WithFields(logrus.Fields{
				"key":   f.Key,
				"file":  f.File,
				"layer": string(f.Layer),
			}).Warnf("exec-bearing config from an untrusted layer is being honored (exec_trust=warn): %s", logLine(f.Value))
		}
		return
	}

	for _, f := range quarantined {
		logger.WithFields(logrus.Fields{
			"key":      f.Key,
			"file":     f.File,
			"layer":    string(f.Layer),
			"consumer": f.Consumer,
		}).Warnf("ignored exec-bearing config from an untrusted workspace: %s = %s", f.Key, logLine(f.Value))
	}
	logger.Warnf("%d exec-bearing config value(s) were ignored because the workspace is not trusted. "+
		"Review them with `grove config trust`, and allow them with `grove config trust --yes`.", len(quarantined))
}

// execWalker resolves one ExecField path against a *Config (including its
// Extensions map) and records — and optionally strips — what it finds.
type execWalker struct {
	field    ExecField
	strip    bool
	findings []ExecFinding
}

// walk descends v along segs. v must be addressable whenever strip is set;
// the map and interface cases below establish that by copying into an
// addressable temporary and writing the result back.
func (w *execWalker) walk(v reflect.Value, segs []string, disp string, strip bool) {
	if !v.IsValid() {
		return
	}

	switch v.Kind() {
	case reflect.Ptr:
		if v.IsNil() {
			return
		}
		w.walk(v.Elem(), segs, disp, strip)
		return
	case reflect.Interface:
		if v.IsNil() {
			return
		}
		inner := v.Elem()
		tmp := reflect.New(inner.Type()).Elem()
		tmp.Set(inner)
		w.walk(tmp, segs, disp, strip)
		if strip && v.CanSet() {
			v.Set(tmp)
		}
		return
	}

	if len(segs) == 0 {
		w.record(v, disp)
		if strip && v.CanSet() {
			v.SetZero()
		}
		return
	}

	seg, rest := segs[0], segs[1:]

	switch v.Kind() {
	case reflect.Struct:
		if sf, ok := structFieldsByConfigKey(v.Type())[seg]; ok {
			w.walk(v.FieldByIndex(sf.Index), rest, disp, strip)
			return
		}
		// Not a declared field. On Config itself, unknown top-level keys
		// live in the Extensions catch-all, which is how extension
		// namespaces ([hooks], [keys], [anthropic]) are reached.
		if ext := v.FieldByName("Extensions"); ext.IsValid() && ext.Kind() == reflect.Map {
			w.mapChild(ext, seg, rest, disp, strip)
		}

	case reflect.Map:
		if v.IsNil() || v.Len() == 0 {
			return
		}
		if seg != "*" {
			w.mapChild(v, seg, rest, disp, strip)
			return
		}
		for _, key := range sortedMapKeys(v) {
			w.mapChild(v, key, rest, replaceLastWildcard(disp, key), strip)
		}

	case reflect.Slice, reflect.Array:
		if seg != "[]" {
			return
		}
		for i := 0; i < v.Len(); i++ {
			w.walk(v.Index(i), rest, fmt.Sprintf("%s[%d]", strings.TrimSuffix(disp, ".[]"), i), strip)
		}
	}
}

// mapChild descends into m[key]. Map entries are not addressable, so the
// value is copied into an addressable temporary, walked, and written back —
// which also makes "quarantine the whole entry" a key deletion rather than a
// zero value left behind.
func (w *execWalker) mapChild(m reflect.Value, key string, rest []string, disp string, strip bool) {
	kv := reflect.ValueOf(key)
	if !kv.Type().AssignableTo(m.Type().Key()) {
		if !kv.Type().ConvertibleTo(m.Type().Key()) {
			return
		}
		kv = kv.Convert(m.Type().Key())
	}
	ev := m.MapIndex(kv)
	if !ev.IsValid() {
		return
	}

	if len(rest) == 0 {
		w.record(ev, disp)
		if strip {
			m.SetMapIndex(kv, reflect.Value{}) // delete the entry outright
		}
		return
	}

	tmp := reflect.New(ev.Type()).Elem()
	tmp.Set(ev)
	w.walk(tmp, rest, disp, strip)
	if strip {
		m.SetMapIndex(kv, tmp)
	}
}

// record appends a finding for a non-zero value, unless the declaring field
// exempts it via SafeValues.
func (w *execWalker) record(v reflect.Value, disp string) {
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if !v.IsValid() || v.IsZero() {
		return
	}
	value := formatExecValue(v)
	if value == "" {
		return
	}
	for _, safe := range w.field.SafeValues {
		if value == safe {
			return
		}
	}
	w.findings = append(w.findings, ExecFinding{
		Key:         disp,
		Value:       value,
		Risk:        w.field.Risk,
		Consumer:    w.field.Consumer,
		Description: w.field.Description,
		Quarantined: w.strip,
	})
}

// formatExecValue renders a config value for the warning and the digest.
// Composite values (a plugin table, an array of hook commands) are rendered
// as a compact, deterministic summary of their command-bearing leaves so the
// digest changes whenever any of them changes.
func formatExecValue(v reflect.Value) string {
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return fmt.Sprintf("%v", v.Interface())
	case reflect.Slice, reflect.Array:
		parts := make([]string, 0, v.Len())
		composite := false
		for i := 0; i < v.Len(); i++ {
			s := formatExecValue(v.Index(i))
			if s == "" {
				continue
			}
			composite = composite || strings.HasPrefix(s, "{")
			parts = append(parts, s)
		}
		// An array of tables — [[hooks.on_stop]] — gets one entry per line so
		// the review UX can show each command on its own. Scalar arrays
		// (args) stay inline.
		if composite {
			return "[\n" + strings.Join(parts, "\n") + "\n]"
		}
		return "[" + strings.Join(parts, " ") + "]"
	case reflect.Map:
		parts := make([]string, 0, v.Len())
		for _, key := range sortedMapKeys(v) {
			kv := reflect.ValueOf(key)
			if kv.Type() != v.Type().Key() && kv.Type().ConvertibleTo(v.Type().Key()) {
				kv = kv.Convert(v.Type().Key())
			}
			if s := formatExecValue(v.MapIndex(kv)); s != "" {
				parts = append(parts, key+"="+s)
			}
		}
		return "{" + strings.Join(parts, " ") + "}"
	case reflect.Struct:
		parts := make([]string, 0, v.NumField())
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			sf := t.Field(i)
			key := fieldConfigKey(sf)
			if key == "" {
				continue
			}
			fv := v.Field(i)
			if fv.IsZero() {
				continue
			}
			if s := formatExecValue(fv); s != "" {
				parts = append(parts, key+"="+s)
			}
		}
		return "{" + strings.Join(parts, " ") + "}"
	default:
		return ""
	}
}

// logLine flattens a multi-line rendered value into one log line. The review
// UX wants one hook per line; a log record wants one record per line.
func logLine(value string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(value, "\n", " ")), " ")
}

// sortedMapKeys returns a map's keys as sorted strings. Non-string keys are
// rendered with %v; every config map in the registry is string-keyed.
func sortedMapKeys(v reflect.Value) []string {
	keys := make([]string, 0, v.Len())
	for _, k := range v.MapKeys() {
		if k.Kind() == reflect.String {
			keys = append(keys, k.String())
			continue
		}
		keys = append(keys, fmt.Sprintf("%v", k.Interface()))
	}
	sort.Strings(keys)
	return keys
}

// replaceLastWildcard substitutes the concrete key for the first "*" segment
// still present in the display path, so "tui.plugins.*" reports as
// "tui.plugins.btop" while the remaining segments stay as declared.
func replaceLastWildcard(disp, key string) string {
	segs := strings.Split(disp, ".")
	for i, s := range segs {
		if s == "*" {
			segs[i] = key
			return strings.Join(segs, ".")
		}
	}
	return disp + "." + key
}
