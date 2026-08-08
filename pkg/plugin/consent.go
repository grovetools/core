package plugin

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/grovetools/core/pkg/exectrust"
)

// Install-time trust is the consent moment.
//
// An installed plugin is a process treemux spawns on your machine every time
// it boots. The decision to allow that is made once, against a screen showing
// what will run — and it is recorded in core/pkg/exectrust, the SAME MAC'd
// store `grove config trust` writes, keyed by the manifest fragment the
// installer is about to write. There is deliberately no second trust store:
// the provenance work already learned that lesson, and a plugin the user
// forgot about should show up in one `grove config trust --list`, not two.
//
// The digest binds the decision to the pinned commit, so an approval covers
// that ref and nothing else. `update` recomputes it, finds a different digest,
// and asks again with a diff.
//
// The facts, the digest, the diff and the lookup are here rather than beside the
// installer because the digest is a FORMAT, not an implementation detail: every
// approval already on disk was hashed by the code below, and a reworded line or
// a reordered part would read as "edited" for every plugin on every machine. The
// screen that renders these facts, and the moment a user answers it, stay in
// grove — that is a CLI's job, and it is the half a host never needs.

// ConsentFacts is everything the user is shown before an install proceeds, and
// everything the approval is bound to.
type ConsentFacts struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Homepage    string `json:"homepage,omitempty"`

	// Source is the display form of what is being installed: url@ref, or
	// "<path> (working tree)" for a development install.
	Source string `json:"source"`
	Commit string `json:"commit"`
	// Dev reports that this approval covers a working tree rather than a
	// commit. It is a consent fact in its own right, not a display detail: it
	// is the difference between approving something fixed and approving
	// whatever that directory contains the next time the panel is rebuilt.
	Dev bool `json:"dev,omitempty"`
	// ManifestDigest hashes the grove-plugin.toml bytes, so an edit anywhere
	// in the manifest re-opens the question even if no field below changed.
	ManifestDigest string `json:"manifest_digest"`

	// Build is the argv that runs in the checkout. Empty means no build step.
	Build []string `json:"build,omitempty"`
	// Run is the argv treemux will spawn.
	Run []string `json:"run"`
	// Env is the extra environment the panel is spawned with.
	Env []string `json:"env,omitempty"`

	Protocol string   `json:"protocol,omitempty"`
	Icon     string   `json:"icon,omitempty"`
	Label    string   `json:"label,omitempty"`
	Keys     []string `json:"keys,omitempty"`
	// Views is the panel's view declaration, one line per view in declaration
	// order, with the one a drawer pane would default to marked.
	//
	// Flattened to lines for the reason Settings is: this struct is compared and
	// digested as text. Ordered rather than sorted, because the order IS one of
	// the facts — it decides which view an installed panel offers a drawer.
	//
	// Like the keys above it grants nothing, so it is on the screen for the same
	// reason: an update that stops offering `compact` to the drawer, or starts
	// offering something else, changes what the user will see in their drawer and
	// should not pass silently.
	Views []string `json:"views,omitempty"`
	// NotebookSubtree and NotebookDescription carry the panel's
	// [panel.notebook] declaration: the notebook subtree it says it writes,
	// and what it saves there. Two fields rather than one prejoined line
	// because the consent screen renders them into a sentence with words
	// between them; the digest and Diff join them themselves (notebookFact).
	//
	// Like Keys and Views it grants nothing — the host never resolves the
	// path — and it is on the screen for the same reason: content appearing in
	// the user's notebook is a fact about their data, and an update that moves
	// the subtree should not pass silently.
	NotebookSubtree     string `json:"notebook_subtree,omitempty"`
	NotebookDescription string `json:"notebook_description,omitempty"`
	// DigestDescription carries the panel's [panel.digest] declaration: what its
	// projection says, in the author's words. Empty means the manifest declares
	// none.
	//
	// One field rather than a bool and a sentence, because the sentence IS the
	// declaration — a panel saying only "I publish a digest" gives the user
	// nothing to decide on. Named for the field rather than for the section
	// because ConsentFacts.Digest is already the approval hash, and a struct
	// where `f.Digest` and `f.Digest()` meant different things would be a trap
	// laid for every later reader.
	//
	// Like Keys, Views and the notebook it grants nothing: the host draws the
	// live frame and reads this nowhere. It is on the screen because a digest is
	// the panel appearing in surfaces it is not running in, and an update that
	// starts doing that should not pass silently.
	DigestDescription string `json:"digest_description,omitempty"`
	// Settings is the manifest's default settings table, flattened to sorted
	// "dotted.key = value" lines.
	//
	// Flattened rather than carried as a map because everything in ConsentFacts
	// is compared and digested as text: a map's iteration order would make the
	// digest unstable, and Diff would have nothing to show a user but "the
	// settings changed". A line per leaf means an update diff names the setting
	// that moved and both of its values.
	//
	// It is here despite grove never executing any of it. A panel's defaults
	// decide what it does on first run, an update that changes one changes the
	// behavior the user approved, and the manifest digest already re-opens the
	// prompt for it — showing the values is what makes that prompt answerable.
	Settings []string `json:"settings,omitempty"`
	// SettingOptions is the manifest's `[[panel.setting_options]]`, flattened to
	// the same "key = value" line shape Settings uses so an update diff can name
	// the setting whose vocabulary moved rather than saying "the options
	// changed".
	//
	// It is a fact worth recording for the reason the settings themselves are.
	// The options are what a config UI will offer, so they decide the states the
	// panel can be put into WITHOUT the user going back to a text editor — and
	// an update that adds `custom` to a list of browser names is an update that
	// starts offering to point the panel at an executable. Nothing here is
	// enforcement (the list is a declaration, and a value outside it is still
	// writable), which is exactly why it is on the screen: what it changes is
	// what gets offered, and the only place to notice that is here.
	SettingOptions []string `json:"setting_options,omitempty"`
}

// ViewFacts renders a panel's view declaration the way the consent screen and
// the digest read it: one line per view in declaration order, with the one a
// drawer pane would default to marked.
//
// It is the whole of what ConsentFacts.Views is built from, and it lives beside
// the digest rather than beside the installer because the digest is what an
// approval is bound to — the line format IS part of the recorded decision, and a
// reworded marker would orphan every approval already on disk.
func ViewFacts(p *Panel) []string {
	preferred := p.PreferredDrawerView()
	views := make([]string, 0, len(p.Views))
	for _, name := range p.ViewNames() {
		line := fmt.Sprintf("%s — %s", name, p.Views[name].Description)
		switch {
		case name == preferred:
			line += " (what a drawer pane gets by default)"
		case p.Views[name].Drawer:
			line += " (also offered to a drawer pane)"
		}
		views = append(views, line)
	}
	return views
}

// KeyFacts renders a panel's key declaration the way the consent screen and the
// digest read it. Here for the same reason ViewFacts is: the line format is part
// of what an approval hashes.
func KeyFacts(p *Panel) []string {
	keys := make([]string, 0, len(p.Keys))
	for _, k := range p.Keys {
		keys = append(keys, fmt.Sprintf("%s — %s", k.Key, k.Description))
	}
	return keys
}

// SettingOptionFacts renders a panel's setting-option declarations the way the
// consent screen and the digest read them: one line per declaration, in the
// author's order, in the "key = value" shape the settings lines already use so
// the update diff keys on the setting name.
//
// Here beside ViewFacts and KeyFacts for the same reason both of those are: the
// line format is part of what an approval hashes, and rewording it would orphan
// every approval already on disk.
func SettingOptionFacts(p *Panel) []string {
	out := make([]string, 0, len(p.SettingOptions))
	for _, o := range p.SettingOptions {
		line := o.Setting + " = " + strings.Join(o.Options, ", ")
		if o.AllowCustom {
			line += ", or a value you type"
		}
		if o.CustomSetting != "" {
			line += fmt.Sprintf(" (%s hands over to %s)", o.CustomOption, o.CustomSetting)
		}
		if o.Description != "" {
			line += " — " + o.Description
		}
		out = append(out, line)
	}
	return out
}

// settingValues is a settings table as leaf path -> the value's flattened text.
//
// Through FlattenSettings rather than beside it, so "the value of
// timer.work_minutes" means one thing in this package: what the consent screen
// prints, what the update diff compares, and what an option list is matched
// against are the same string.
func settingValues(settings map[string]any) map[string]string {
	lines := FlattenSettings(settings)
	out := make(map[string]string, len(lines))
	for _, line := range lines {
		if key, value, ok := strings.Cut(line, " = "); ok {
			out[key] = value
		}
	}
	return out
}

// FlattenSettings renders a settings table as sorted "dotted.key = value"
// lines. Sorted so the result is stable across runs — the approval digest is
// computed over it, and a map's iteration order would make an unchanged
// manifest hash differently every time.
//
// Exported because the fragment writer and the consent screen must agree on
// what a setting is called; a user reading "timer.work_minutes = 25" at the
// prompt should find the same path in the file grove writes.
func FlattenSettings(settings map[string]any) []string {
	var out []string
	flattenSettingsInto(&out, "", settings)
	sort.Strings(out)
	return out
}

func flattenSettingsInto(out *[]string, prefix string, v any) {
	switch v := v.(type) {
	case map[string]any:
		for name, value := range v {
			key := name
			if prefix != "" {
				key = prefix + "." + name
			}
			flattenSettingsInto(out, key, value)
		}
	case []any:
		// Arrays render whole rather than one line per element: an ordered
		// list is one decision, and splitting it would let a reordering read
		// as several unrelated changes in an update diff.
		parts := make([]string, 0, len(v))
		for _, e := range v {
			parts = append(parts, fmt.Sprintf("%v", e))
		}
		*out = append(*out, fmt.Sprintf("%s = [%s]", prefix, strings.Join(parts, ", ")))
	default:
		*out = append(*out, fmt.Sprintf("%s = %v", prefix, v))
	}
}

// Digest is the value recorded in the exec-trust store. It uses
// exectrust.Digest so a plugin approval hashes the same way a `grove config
// trust` approval does.
func (f ConsentFacts) Digest() string {
	parts := []string{
		"source=" + f.Source,
		"commit=" + f.Commit,
		"manifest=" + f.ManifestDigest,
		"build=" + strings.Join(f.Build, "\x1f"),
		"run=" + strings.Join(f.Run, "\x1f"),
		"env=" + strings.Join(f.Env, "\x1f"),
		"protocol=" + f.Protocol,
		"keys=" + strings.Join(f.Keys, "\x1f"),
		"label=" + f.Label,
		"settings=" + strings.Join(f.Settings, "\x1f"),
	}
	// Appended only when there are views, so a manifest that declares none
	// hashes exactly as it did before views existed. Every plugin installed
	// before this field hashes the same way it did when it was approved, and an
	// unconditional line would have re-opened the prompt for all of them to ask
	// about something none of them says.
	if len(f.Views) > 0 {
		parts = append(parts, "views="+strings.Join(f.Views, "\x1f"))
	}
	// Appended only when there are any, for the reason views are: a manifest
	// that declares no options hashes exactly as it did before the section
	// existed, so no plugin already approved re-opens its prompt to be asked
	// about something it never said.
	if len(f.SettingOptions) > 0 {
		parts = append(parts, "setting_options="+strings.Join(f.SettingOptions, "\x1f"))
	}
	// Appended only when declared, for the reason views are. It sits with the
	// other manifest declarations and ahead of `dev=`, which is not one — but
	// the position is a readability choice and nothing more: what makes an
	// existing approval survive is that a manifest declaring no digest appends
	// no part at all, at any position.
	if f.DigestDescription != "" {
		parts = append(parts, "digest="+f.DigestDescription)
	}
	// Appended only when declared, for the reason views are: a manifest without
	// [panel.notebook] must hash byte-identically to how it did before the
	// section existed, so no previously-approved plugin re-opens its prompt to
	// ask about something it never said.
	if f.NotebookSubtree != "" {
		parts = append(parts, "notebook="+notebookFact(f))
	}
	// Appended only when set, for the same round-tripping reason as views: no
	// previously-approved plugin re-opens its prompt. When it IS set it must be
	// in the digest, so an approval granted to a pinned commit can never be
	// reused to run a mutable working tree — the exec-trust store would
	// otherwise see the same value for two materially different things.
	if f.Dev {
		parts = append(parts, "dev=true")
	}
	return exectrust.Digest(parts)
}

// notebookFact renders the notebook declaration as one comparable
// "subtree — description" line, empty when the manifest declares none. The
// digest and Diff both go through it, so the two can never disagree about
// what the declaration says.
func notebookFact(f ConsentFacts) string {
	if f.NotebookSubtree == "" {
		return ""
	}
	return f.NotebookSubtree + " — " + f.NotebookDescription
}

// FactChange is one line of an update diff.
type FactChange struct {
	Field string
	Old   string
	New   string
}

// Diff reports what changed between an approved install and a proposed one,
// in the order the consent screen shows the fields. Only the fields the
// approval is bound to are compared; a new description or homepage is shown by
// the consent screen anyway and is not what the user is deciding about.
func Diff(old, next ConsentFacts) []FactChange {
	var out []FactChange
	add := func(field, a, b string) {
		if a != b {
			out = append(out, FactChange{Field: field, Old: a, New: b})
		}
	}
	add("source", old.Source, next.Source)
	add("commit", shortCommit(old.Commit), shortCommit(next.Commit))
	add("build", strings.Join(old.Build, " "), strings.Join(next.Build, " "))
	add("run", strings.Join(old.Run, " "), strings.Join(next.Run, " "))
	add("env", strings.Join(old.Env, " "), strings.Join(next.Env, " "))
	add("protocol", old.Protocol, next.Protocol)
	add("keys", strings.Join(old.Keys, ", "), strings.Join(next.Keys, ", "))
	// One row rather than one per view: the order is part of what changed (it
	// decides the drawer default), and a per-line diff keyed on the view name
	// would report a reordering as nothing at all.
	add("views", strings.Join(old.Views, ", "), strings.Join(next.Views, ", "))
	// Beside the views, and in the same order the consent screen prints them:
	// both are declarations about what the panel draws and where, and the digest
	// is the one of them that draws somewhere the panel is not.
	add("digest", old.DigestDescription, next.DigestDescription)
	// The subtree and its description diff as one row: what the panel writes
	// into the notebook is one fact, and a moved subtree with a reworded
	// description is one change rather than two unrelated ones.
	add("notebook", notebookFact(old), notebookFact(next))
	add("label", old.Label, next.Label)
	// One line per changed setting rather than one "settings" row: an update
	// that retunes a default should say which one and from what.
	out = append(out, diffLines("settings", old.Settings, next.Settings)...)
	// Beside the settings and keyed the same way, because a changed vocabulary
	// is a change to that setting: "options.palette" reads as one more thing
	// that moved about `palette`, next to the row saying its default moved.
	out = append(out, diffLines("options", old.SettingOptions, next.SettingOptions)...)
	// The icon is cosmetic, but the manifest digest covers it, so a changed
	// icon alone re-opens the prompt. Showing it keeps the screen from saying
	// "nothing you approved has changed" while asking about something.
	add("icon", old.Icon, next.Icon)
	return out
}

// trustKey is the path an approval is filed under.
//
// exectrust canonicalizes its keys through EvalSymlinks, which only resolves
// for a path that EXISTS. A fragment does not exist when an install is
// approved and no longer exists when one is removed, so letting the store
// canonicalize would file the record under one path and look it up under
// another (on macOS, /var/... versus /private/var/...). Resolving the
// directory — which always exists at both moments — and rejoining the basename
// gives the same key at every point in a plugin's life.
func trustKey(fragmentPath string) string {
	dir, base := filepath.Split(fragmentPath)
	resolved, err := filepath.EvalSymlinks(filepath.Clean(dir))
	if err != nil {
		return fragmentPath
	}
	return filepath.Join(resolved, base)
}

// RecordApproval writes the install decision into the exec-trust store,
// keyed by the manifest fragment path.
func RecordApproval(fragmentPath, digest string, now time.Time) error {
	store := exectrust.Load()
	store.Trust(trustKey(fragmentPath), digest, now)
	if err := store.Save(); err != nil {
		return fmt.Errorf("record the install approval: %w", err)
	}
	return nil
}

// RevokeApproval drops the trust record for a fragment. Used by remove, so an
// uninstall does not leave a decision behind about a file that no longer
// exists.
func RevokeApproval(fragmentPath string) error {
	store := exectrust.Load()
	if !store.Revoke(trustKey(fragmentPath)) {
		return nil
	}
	if err := store.Save(); err != nil {
		return fmt.Errorf("drop the install approval: %w", err)
	}
	return nil
}

// IsApproved reports whether the fragment is still trusted at the digest that
// was approved. A false here means the fragment or the pin was edited outside
// `grove plugin`, which `grove plugin list` surfaces rather than repairing.
func IsApproved(fragmentPath, digest string) bool {
	return exectrust.Load().IsTrusted(trustKey(fragmentPath), digest)
}

// shortCommit abbreviates a commit for display, leaving anything that is not a
// full hash alone.
func shortCommit(c string) string {
	if len(c) >= 12 {
		return c[:12]
	}
	return c
}

// diffLines reports per-line changes between two flattened key = value lists,
// keyed by the part before the first "=" so a changed VALUE reads as a change
// rather than as one removal plus one addition.
func diffLines(field string, old, next []string) []FactChange {
	index := func(lines []string) map[string]string {
		m := make(map[string]string, len(lines))
		for _, l := range lines {
			key, value, found := strings.Cut(l, " = ")
			if !found {
				key = l
			}
			m[key] = value
		}
		return m
	}
	oldByKey, nextByKey := index(old), index(next)

	keys := make([]string, 0, len(oldByKey)+len(nextByKey))
	seen := make(map[string]bool, len(keys))
	for _, m := range []map[string]string{oldByKey, nextByKey} {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	sort.Strings(keys)

	var out []FactChange
	for _, k := range keys {
		before, hadBefore := oldByKey[k]
		after, hasAfter := nextByKey[k]
		if hadBefore && hasAfter && before == after {
			continue
		}
		out = append(out, FactChange{Field: field + "." + k, Old: before, New: after})
	}
	return out
}
