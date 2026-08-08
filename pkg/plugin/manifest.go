// Package plugin is the READ side of grove's plugin distribution layer: the
// manifest a plugin repository ships, the lockfile that pins exactly what is
// installed, where all of it lives on disk, and whether the user's approval
// still covers it.
//
// It is here rather than in grove because the two ends of the pipeline are in
// different modules. `grove plugin` writes this state; treemux reads it, to
// answer questions about panels it is already running — which commit is
// installed, what the manifest declared, whether the fragment still matches the
// approval. A host reaching that state through a grove import or a subprocess
// would be paying for the whole install pipeline to read four files.
//
// So the split is by direction, not by file:
//
//	Read     manifest, lockfile, locations, approval check      (this package)
//	Write    clone, build, install, declare, record, remove     (grove/pkg/plugin)
//
// Nothing here runs a program, clones a repository or writes to the lockfile.
// The one thing it writes is the exec-trust record (consent.go), which is the
// same MAC'd store `grove config trust` uses rather than a second trust store
// of its own.
package plugin

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// ManifestFile is the file a plugin repository ships at its root.
const ManifestFile = "grove-plugin.toml"

// SchemaVersion is the manifest schema this grove understands. A manifest
// declaring a higher version is refused rather than guessed at: its build or
// panel section may mean something this binary would get wrong.
const SchemaVersion = 1

// ProtocolEmbedV1 is the sidecar control-plane protocol treemux speaks
// (treemux/docs/panel-protocol-v1.md). The empty string is the other legal
// value: a plain PTY panel.
const ProtocolEmbedV1 = "embed/v1"

// Manifest is a parsed grove-plugin.toml.
//
//	schema_version = 1
//
//	[plugin]
//	name        = "hello"
//	description = "A hello-world sidecar panel"
//	homepage    = "https://github.com/grovetools/grove-panel-hello"
//
//	[build]
//	command = ["go", "build", "-o", "bin/grove-panel-hello", "."]
//	binary  = "bin/grove-panel-hello"
//
//	[panel]
//	label            = "Break timer"
//	icon             = ""
//	protocol         = "embed/v1"
//	protocol_timeout = "2s"
//	args             = []
//	env              = []
//	restart          = true
//
//	[panel.settings]
//	work_minutes  = 25
//	break_minutes = 5
//
//	[[panel.setting_options]]
//	setting     = "palette"
//	description = "which colors the panel draws in"
//	options     = ["hn", "host", "auto"]
//
//	[panel.notebook]
//	subtree     = "hn/clippings"
//	description = "stories you clip from the feed"
//
//	[panel.digest]
//	description = "the current state and how long is left of it"
//
//	[[panel.keys]]
//	key         = "ctrl+f"
//	description = "jump to the notebook"
//
//	[panel.views.full]
//	description = "clock, history and help"
//	drawer      = false
//
//	[panel.views.compact]
//	description = "one line: state and time remaining"
//	drawer      = true
//
// A manifest may instead declare a TOOL — a binary grove dispatches to rather
// than a pane treemux runs. Exactly one of [panel]/[tool]:
//
//	[tool]
//	binary   = "forge"
//	provides = ["forge up", "forge status"]
type Manifest struct {
	SchemaVersion int    `toml:"schema_version"`
	Plugin        Plugin `toml:"plugin"`
	Build         Build  `toml:"build"`
	Panel         Panel  `toml:"panel"`
	// Tool is the manifest's other kind — see [Tool]. A pointer where Panel is
	// a value because presence IS the declaration: the kind is inferred from
	// which section the document contains, not from a field that could
	// disagree with it.
	Tool    *Tool    `toml:"tool"`
	Unknown []string `toml:"-"` // keys this grove does not understand
	// PanelDeclared reports that the document literally contains a [panel]
	// section, recovered from the bytes (see panelDeclared) because Panel is a
	// value and an absent section decodes identically to an empty one. It
	// exists for exactly one check: a manifest declaring [tool] alongside
	// [panel] is refused, while a manifest declaring NEITHER stays what it has
	// always been — a plain PTY panel.
	PanelDeclared bool `toml:"-"`
}

// Plugin is the identity section.
type Plugin struct {
	Name        string `toml:"name"`
	Description string `toml:"description"`
	Homepage    string `toml:"homepage"`
}

// Build says how to turn the checkout into a binary. Command is argv, never a
// shell string: the consent screen has to show the user exactly what will run,
// and "sh -c ..." would hide it behind a shell. Command may be empty for a
// plugin that ships an interpreted program — then Binary must already exist in
// the checkout, which is also the no-toolchain-required path.
type Build struct {
	Command []string `toml:"command"`
	Binary  string   `toml:"binary"`
}

// Tool is the manifest's other kind — `[tool]` in place of `[panel]`. A tool
// is not a pane treemux runs; it is a binary grove installs and dispatches to
// when the user types one of the verbs it declares (`grove forge up`). Same
// checkout, same pin, same consent gate — only the host differs.
type Tool struct {
	// Binary is the bare command name the tool is installed as. Optional: it
	// defaults to the basename of build.binary (see Manifest.BinaryName). Held
	// to namePattern rather than to build.binary's path rules, because it is a
	// NAME and never a path — it becomes a file in grove's bin dir and the
	// command dispatch resolves, and a separator here would be a claim about
	// directories the install does not manage.
	Binary string `toml:"binary"`
	// Provides is the CLI phrases the tool answers, exactly as the user would
	// type them after `grove` — "forge up", "forge status". Required and
	// non-empty: a tool that answers nothing is not installable as one.
	//
	// The set of distinct first tokens is the tool's dispatch verb set,
	// usually one. Phrases beyond the verb are a DISCLOSURE like a panel's
	// keys: the consent screen renders each as the command it enables, and an
	// update that grows the list re-opens the prompt. Dispatch keys on the
	// verb — nothing checks the tool's real argv surface against this list.
	Provides []string `toml:"provides"`
}

// Panel is what the installer turns into a [tui.plugins.<name>] fragment.
type Panel struct {
	// Label is the human-readable name the rail shows. Empty falls back to
	// plugin.name, which is constrained to a bare key and is often not what
	// the author would choose to display.
	Label string `toml:"label"`
	// Icon is either a core theme icon NAME ("rss" — mode-aware, degrades to
	// its ASCII form) or the plugin's own LITERAL glyph ("", an emoji, a
	// short ASCII mark) — a third-party panel is not limited to the names the
	// host compiled in. The host resolves it with theme.ResolveIconOr: an
	// unknown name (or a literal glyph under ASCII icon mode, which has no
	// ASCII form to degrade to) falls back to a generic mark.
	Icon            string   `toml:"icon"`
	Protocol        string   `toml:"protocol"`
	ProtocolTimeout string   `toml:"protocol_timeout"`
	Args            []string `toml:"args"`
	Env             []string `toml:"env"`
	Restart         bool     `toml:"restart"`
	Keys            []Key    `toml:"keys"`
	// Settings are the panel's DEFAULT settings: the free-form table the host
	// delivers to it over the control plane, seeded from the manifest so a
	// freshly installed panel works before the user has configured anything.
	//
	// The user's own [tui.plugins.<name>.settings] is the same key in a config
	// layer they own, and later layers replace earlier ones wholesale, so
	// editing the fragment is how a user overrides these.
	//
	// They are shown on the consent screen and bound into the approval digest
	// like everything else in the manifest — not because grove will run them
	// (it will not: the host forwards this table and never interprets it) but
	// because a value the user has approved is one they should have read. An
	// update that changes a default re-opens the prompt with a diff.
	Settings map[string]any `toml:"settings"`
	// SettingOptions are the CHOICES an author declares for individual settings
	// — `[[panel.setting_options]]`, one entry per setting that has a closed
	// vocabulary rather than a free-form value.
	//
	// Optional, and absent for most settings: a work interval in minutes is a
	// number and nothing here would improve it. What the table is for is the
	// setting whose legal values the author already knows and the user has to
	// guess — a browser to open links with, a palette, a layout — where a config
	// UI drawing a text box is asking a question it already has the answer to.
	//
	// Like every other declaration in this manifest it is READ AND REPORTED ON,
	// never obeyed. `grove plugin set` still writes a value outside the list
	// (the user may know about a browser the author did not), the host still
	// delivers whatever the settings table says, and the panel remains the only
	// party that decides what an unrecognized value means. What the declaration
	// buys is a UI that can offer the list, and a consent screen that shows the
	// user which values they are approving a panel to be pointed at.
	SettingOptions []SettingOptions `toml:"setting_options"`
	// Views are the panel's own named layouts — `[panel.views.<name>]`, keyed by
	// the name the panel answers to.
	//
	// The names are an OPEN SET the panel defines. They are not checked against
	// any list this package holds and no host branches on one: a panel may have
	// two views or six and call them `compact`, `graph`, `tree` or `tiny`,
	// because only the panel knows what its own layouts are. What the host reads
	// is one bool per view (see View.Drawer), and it reads it off the
	// DECLARATION rather than off the name.
	//
	// Declaring none is normal and stays working: `view` was carried on the wire
	// before this table existed, and a panel that never declares a view still
	// gets whatever the user asks for.
	Views map[string]View `toml:"views"`
	// ViewOrder is the order the manifest declares the views in, recovered from
	// the document because a TOML table of tables decodes into an unordered map.
	// Filled by ParseManifest; see manifest_views.go for why the order matters
	// and Panel.ViewNames for what happens when it is absent.
	ViewOrder []string `toml:"-"`
	// Notebook is the panel's declared notebook subtree — `[panel.notebook]`,
	// present only when the panel saves content into the user's notebook. Like
	// Keys and Views it is reported and bound, never enforced; see [Notebook].
	Notebook *Notebook `toml:"notebook"`
	// Digest is the panel's declaration that it publishes a digest —
	// `[panel.digest]`, present only when it does. See [Digest].
	Digest *Digest `toml:"digest"`
}

// View is one of the panel's own layouts, declared so the host can default and
// report rather than guess.
//
// Like [Key] it is a DECLARATION and grants nothing. A `view` in the user's
// config naming something absent from this table still reaches the panel
// verbatim: the host cannot know the name was wrong, and a panel that receives a
// name it does not implement renders its default and may say so. Nothing here is
// enforcement — the manifest is read and reported on, never obeyed.
type View struct {
	// Description is what the view shows, in the author's words. Required: it is
	// the only thing that tells a user what they would be asking for, both on the
	// consent screen and in the fragment they edit to choose one.
	Description string `toml:"description"`
	// Drawer says whether the author means this view for a drawer pane — a narrow
	// column a few rows tall. It is the one field a host acts on, in exactly two
	// ways: a drawer pane naming no view gets the first view declared `true`, and
	// a view declared `false` mounted in a drawer warns and mounts anyway.
	//
	// `false` is a real answer rather than an absent one. A panel whose full
	// layout is a title, a clock, a history list and a footer has no drawer width
	// at which mounting it is right, and saying so is information the host can
	// use.
	Drawer bool `toml:"drawer"`
}

// Key is a host hotkey the panel intends to claim over the control plane. It
// is a DECLARATION, not a grant: the host filters claims at handshake time
// (see welcome.rejected_keys in the protocol spec). It lives in the manifest
// so the user reads it before approving an install, not because the installer
// enforces it.
type Key struct {
	Key         string `toml:"key"`
	Description string `toml:"description"`
}

// SettingOptions is one setting's declared vocabulary — one entry of
// `[[panel.setting_options]]`.
//
// An ARRAY rather than `[panel.setting_options.<key>]` tables, unlike
// `[panel.views.<name>]`, because the thing being keyed is a SETTING PATH:
// `timer.work_minutes` is a legal setting and an illegal bare TOML key, and a
// declaration whose commonest form has to be written in quotes is one authors
// will get wrong. Naming the setting in a field also lets this type be the same
// type in the manifest, in the fragment and in the config — the translation
// views need (a map, ordered by a recovered declaration order) has nothing to do
// here.
type SettingOptions struct {
	// Setting is the dotted path of the setting these are the options for, in
	// the form the consent screen and `grove plugin set` already use —
	// "palette", "timer.work_minutes". It must name a setting `[panel.settings]`
	// declares, because options for a setting the panel has no default for are
	// options for nothing.
	Setting string `toml:"setting"`
	// Description is what the setting decides, in the author's words. Optional:
	// a list of browser names says most of what there is to say, and a
	// declaration that forced a sentence out of every author would get
	// sentences that repeat the key.
	Description string `toml:"description"`
	// Options is the vocabulary, in the order a UI should offer it. Required —
	// an entry declaring none declares nothing — and the values are compared
	// against the setting's value as text, so an option for a numeric setting is
	// still written as a string here ("24", not 24).
	Options []string `toml:"options"`
	// AllowCustom says the list is a set of SUGGESTIONS rather than the whole
	// vocabulary: a UI offering these should also let the user type a value of
	// their own into this same setting.
	//
	// It changes nothing about what may be written — nothing here is enforcement
	// — it says which of the two a UI should draw, and it is what keeps a
	// declaration from turning "the seven browsers I tested" into "the only
	// seven browsers".
	AllowCustom bool `toml:"allow_custom"`
	// CustomOption and CustomSetting are the other shape of the same escape
	// hatch, and the one a panel picks when the custom value needs a home of its
	// own: choosing CustomOption from the list means the setting named by
	// CustomSetting is the one that decides.
	//
	// It exists because a panel that takes a browser NAME and a panel that takes
	// an executable PATH are reading two different things, and a single setting
	// holding either would have to guess which it was given. Two settings, one
	// of which nominates the other, lets both be typed and both be declared.
	//
	// They are meaningless apart, so a manifest declaring one must declare both.
	// CustomOption must be one of Options — the choice a user makes has to be
	// choosable — and CustomSetting must be a setting the panel declares, which
	// is the same rule Setting is held to.
	CustomOption  string `toml:"custom_option"`
	CustomSetting string `toml:"custom_setting"`
}

// Notebook is the panel's declared notebook subtree — `[panel.notebook]`.
//
// A panel that saves content into the user's notebook (clippings, captures,
// logs) says WHERE here, so the user reads it before approving the install.
// Like [Key] and [View] it is a DECLARATION, not a grant: the host never
// resolves the path, never creates it and never fences the panel into it — a
// process the user approved writes wherever its own authority reaches, and
// pretending otherwise would dress a disclosure up as a sandbox. What the
// declaration buys is honesty at the one moment it matters: the consent
// screen names the subtree, the approval digest binds it, and an update that
// moves it re-opens the prompt.
type Notebook struct {
	// Subtree is the notebook-relative path the panel writes under, e.g.
	// "hn/clippings". Held to build.binary's path rules — relative, no `..`
	// escapes, printable — not because the host walks it (it never does) but
	// because it is rendered on the consent screen, and a path that escapes or
	// pretends to be absolute reads as a claim about directories the notebook
	// does not contain.
	Subtree string `toml:"subtree"`
	// Description is what the panel saves there, in the author's words.
	// Required: it is the only thing that tells the user what kind of content
	// would appear in their notebook.
	Description string `toml:"description"`
}

// Digest is the panel's declaration that it publishes a digest —
// `[panel.digest]`.
//
// Nothing to do with the hashes this package computes: a DIGEST here is the
// panelproto frame a panel pushes so a host can draw a one-line projection of it
// in a slot the panel itself cannot run in — a drawer column, a roster row.
//
// It exists because until now that capability was only ever a RUNTIME fact. The
// frame arrives over a live control plane, so "does this panel publish a digest"
// could not be answered about a panel that had not been opened yet — and both
// places that want the answer ask it cold: a roster listing panels nobody has
// focused, and a user writing `backend = "digest"` into their drawer config for
// a panel they have just installed.
//
// Like [Key], [View] and [Notebook] it is a DISCLOSURE, not a grant and not a
// contract. Declaring it obliges no panel to push anything, and a panel that
// pushes one without declaring it is drawn exactly the same — the host reads the
// live frame, never this. What the declaration buys is the cold answer, and the
// consent screen sentence that goes with it: a projection of the panel will be
// visible in surfaces the panel is not running in, and that is worth reading
// before approving.
//
// Absence means "declares no digest", which is deliberately weaker than "does
// not publish one": every fragment written before this table existed lacks it,
// as does every hand-written [tui.plugins] entry. So the declaration is only
// ever read in the affirmative.
type Digest struct {
	// Description is what the projection says, in the author's words — the
	// content of the line, not the fact that there is one. Required, for the
	// reason [View]'s is: "publishes a digest" tells a user nothing they can
	// decide on, and this is the only thing that says what would appear in the
	// drawer column they are about to give it.
	Description string `toml:"description"`
}

var (
	// namePattern keeps a plugin name usable as a filename, a TOML bare key
	// and a [tui.plugins.<name>] table name all at once.
	namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	// envPattern is the KEY=VALUE form core/config's PluginConfig.Env expects.
	envPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
)

// LoadManifest reads and validates the manifest at the root of a checkout.
// It returns the parsed manifest and the exact bytes it parsed, which the
// consent digest binds to (see ConsentFacts).
func LoadManifest(repoDir string) (*Manifest, []byte, error) {
	path := filepath.Join(repoDir, ManifestFile)
	data, err := os.ReadFile(path) //nolint:gosec // G304: path is grove's own managed checkout
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("%s is not a grove plugin: no %s at its root", repoDir, ManifestFile)
		}
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	m, err := ParseManifest(data)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, data, nil
}

// ParseManifest decodes and validates manifest bytes.
//
// Decoding is strict about unknown keys, but a strict decode is a WARNING, not
// a failure: a manifest written for a later schema must still be readable
// enough to say so, and a plugin author's typo is worth surfacing without
// breaking installs that otherwise work. Unrecognized keys land in
// Manifest.Unknown and the consent screen prints them.
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		var strict *toml.StrictMissingError
		if !errors.As(err, &strict) {
			return nil, fmt.Errorf("parse %s: %w", ManifestFile, err)
		}
		for _, e := range strict.Errors {
			m.Unknown = append(m.Unknown, strings.Join(e.Key(), "."))
		}
	}
	// The decode has already accepted the bytes, so these are second reads of a
	// document known to parse — for the two facts the decoder throws away.
	m.Panel.ViewOrder = viewOrder(data)
	m.PanelDeclared = panelDeclared(data)
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Validate reports the first thing wrong with a manifest. Every message names
// the key, because the reader is a plugin author debugging their own repo.
func (m *Manifest) Validate() error {
	switch {
	case m.SchemaVersion == 0:
		return fmt.Errorf("schema_version is required (this grove understands %d)", SchemaVersion)
	case m.SchemaVersion > SchemaVersion:
		return fmt.Errorf("schema_version %d is newer than this grove understands (%d) — upgrade grove", m.SchemaVersion, SchemaVersion)
	case m.SchemaVersion < SchemaVersion:
		return fmt.Errorf("schema_version %d is no longer supported (this grove understands %d)", m.SchemaVersion, SchemaVersion)
	}

	if m.Plugin.Name == "" {
		return errors.New("plugin.name is required")
	}
	if !namePattern.MatchString(m.Plugin.Name) {
		return fmt.Errorf("plugin.name %q must be lowercase letters, digits and dashes, starting with a letter or digit", m.Plugin.Name)
	}
	if strings.TrimSpace(m.Plugin.Description) == "" {
		// The description is not decoration: it is one of the few things the
		// user reads before approving an install.
		return errors.New("plugin.description is required — it is shown at the install consent prompt")
	}

	if err := validateArgv("build.command", m.Build.Command); err != nil {
		return err
	}
	if m.Build.Binary == "" {
		return errors.New("build.binary is required — name the file the build produces")
	}
	if err := validateRelPath("build.binary", m.Build.Binary); err != nil {
		return err
	}

	// A manifest is exactly one kind, and the kind is which section it wrote.
	// Declaring both is refused rather than ranked: two hosts each honoring
	// half a manifest is not a state the user could have approved. Declaring
	// NEITHER stays what it has always been — a plain PTY panel — because
	// every manifest written before [tool] existed is one.
	if m.Tool != nil {
		if m.PanelDeclared {
			return errors.New("[tool] and [panel] are both declared — a plugin is one kind: a tool grove dispatches to, or a panel treemux runs")
		}
		return validateTool(m.Tool)
	}

	switch m.Panel.Protocol {
	case "", ProtocolEmbedV1:
	default:
		return fmt.Errorf("panel.protocol %q is not a protocol this host speaks (use %q or leave it empty for a plain PTY panel)", m.Panel.Protocol, ProtocolEmbedV1)
	}
	if m.Panel.ProtocolTimeout != "" {
		if _, err := time.ParseDuration(m.Panel.ProtocolTimeout); err != nil {
			return fmt.Errorf("panel.protocol_timeout %q is not a Go duration (e.g. \"2s\")", m.Panel.ProtocolTimeout)
		}
	}
	if err := validateArgv("panel.args", m.Panel.Args); err != nil {
		return err
	}
	for _, e := range m.Panel.Env {
		if !envPattern.MatchString(e) {
			return fmt.Errorf("panel.env entry %q must be KEY=VALUE", e)
		}
		if err := printable("panel.env", e); err != nil {
			return err
		}
	}
	if err := printable("panel.icon", m.Panel.Icon); err != nil {
		return err
	}
	if err := printable("panel.label", m.Panel.Label); err != nil {
		return err
	}
	if err := validateSettings("panel.settings", m.Panel.Settings); err != nil {
		return err
	}
	if err := validateSettingOptions(&m.Panel); err != nil {
		return err
	}
	for i, k := range m.Panel.Keys {
		if strings.TrimSpace(k.Key) == "" {
			return fmt.Errorf("panel.keys[%d].key is required", i)
		}
		if strings.TrimSpace(k.Description) == "" {
			return fmt.Errorf("panel.keys[%d].description is required — the user reads it before approving the claim", i)
		}
		if err := printable(fmt.Sprintf("panel.keys[%d].key", i), k.Key); err != nil {
			return err
		}
	}
	// Views are validated for the two things a host and a reader need of them: a
	// name that can be compared verbatim against the user's `view`, and a
	// sentence explaining what asking for it would get you. Nothing here checks
	// the name against a vocabulary, because there is no vocabulary to check it
	// against — that is the point of the field.
	//
	// A manifest declaring no views, or declaring none for the drawer, is VALID.
	// The first is every panel written before views existed; the second is a
	// panel stating that none of its layouts belongs in a narrow column, which is
	// an answer rather than an omission.
	for _, name := range m.Panel.ViewNames() {
		key := "panel.views." + name
		if strings.TrimSpace(name) == "" {
			return errors.New("panel.views has a view with a blank name")
		}
		if name != strings.TrimSpace(name) {
			return fmt.Errorf("panel.views name %q must not begin or end with whitespace — it is compared verbatim against the user's `view`", name)
		}
		if err := printable(key, name); err != nil {
			return err
		}
		if strings.TrimSpace(m.Panel.Views[name].Description) == "" {
			return fmt.Errorf("%s.description is required — it is the only thing that says what asking for this view would get", key)
		}
		if err := printable(key+".description", m.Panel.Views[name].Description); err != nil {
			return err
		}
	}
	// The notebook section is optional — most panels save nothing — but a panel
	// that declares one must declare it whole: a subtree without a description
	// is a path the user cannot judge, and a description without a subtree is a
	// promise about nowhere.
	if nb := m.Panel.Notebook; nb != nil {
		if strings.TrimSpace(nb.Subtree) == "" {
			return errors.New("panel.notebook.subtree is required — name the notebook subtree the panel writes under")
		}
		if err := validateRelPath("panel.notebook.subtree", nb.Subtree); err != nil {
			return err
		}
		// build.binary tolerates a trailing slash because Clean eats it before
		// anything reads the path; here nothing ever Cleans it — the string goes
		// to the consent screen verbatim — so the manifest has to write the
		// canonical form itself.
		if strings.HasPrefix(nb.Subtree, "/") || strings.HasSuffix(nb.Subtree, "/") {
			return fmt.Errorf("panel.notebook.subtree %q must not begin or end with a slash — it names a subtree relative to the notebook root", nb.Subtree)
		}
		if len(nb.Subtree) > maxNotebookSubtreeLen {
			return fmt.Errorf("panel.notebook.subtree is %d characters — longer than a path the consent screen can show legibly (%d)", len(nb.Subtree), maxNotebookSubtreeLen)
		}
		if strings.TrimSpace(nb.Description) == "" {
			return errors.New("panel.notebook.description is required — it is shown at the install consent prompt")
		}
		if err := printable("panel.notebook.description", nb.Description); err != nil {
			return err
		}
	}
	// The digest section is optional and has exactly one key, so declaring it is
	// declaring the description: an empty `[panel.digest]` is a panel claiming a
	// surface without saying what would appear there, which is the one thing the
	// user reading the consent screen is deciding about.
	if d := m.Panel.Digest; d != nil {
		if strings.TrimSpace(d.Description) == "" {
			return errors.New("panel.digest.description is required — it is the only thing that says what the projection would show")
		}
		if err := printable("panel.digest.description", d.Description); err != nil {
			return err
		}
	}
	return nil
}

// maxNotebookSubtreeLen bounds the declared subtree the way maxSettingsDepth
// bounds nesting: not as attack surface, but as legibility. The subtree is
// rendered inline in one consent-screen sentence, and a path longer than this
// has outgrown what a user can meaningfully read there.
const maxNotebookSubtreeLen = 128

// ViewNames returns the declared view names in the order the manifest declares
// them, which is the order preference is read from.
//
// Any name the recovered order missed is appended, sorted, rather than dropped:
// the order is a refinement of the map and never its authority, so a hand-built
// Manifest (a test, a caller assembling one in memory) and a document whose
// order could not be read both still validate and still render every view they
// declare. Sorted so that fallback is deterministic — the fragment's contents
// feed a digest an approval is bound to.
func (p *Panel) ViewNames() []string {
	if len(p.Views) == 0 {
		return nil
	}
	names := make([]string, 0, len(p.Views))
	seen := make(map[string]bool, len(p.Views))
	for _, name := range p.ViewOrder {
		if _, ok := p.Views[name]; ok && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	rest := make([]string, 0, len(p.Views)-len(names))
	for name := range p.Views {
		if !seen[name] {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	return append(names, rest...)
}

// PreferredDrawerView is the first view the author declared drawer-suitable, or
// empty when they declared none.
//
// It is what a drawer pane mounts when the user names no view, and the reason
// declaration order is recovered at all. The host resolves this from the config
// it was copied into (config.DrawerPaneConfig.EffectiveView); here it is what the
// consent screen reports, so a user can see which view an install is offering a
// drawer before they approve it.
func (p *Panel) PreferredDrawerView() string {
	for _, name := range p.ViewNames() {
		if p.Views[name].Drawer {
			return name
		}
	}
	return ""
}

// Kind is which kind of plugin the manifest declares: "tool", or "panel" —
// inferred from section presence rather than stated in a field, so the answer
// can never disagree with the document. "panel" covers a manifest with no
// [panel] section at all, because that has always meant a plain PTY panel.
func (m *Manifest) Kind() string {
	if m.Tool != nil {
		return "tool"
	}
	return "panel"
}

// BinaryName is the name the built binary is installed under. A tool may pick
// its own (tool.binary — the command `grove <verb>` resolves is worth naming
// deliberately); everything else installs as the basename of what the build
// produces.
func (m *Manifest) BinaryName() string {
	if m.Tool != nil && m.Tool.Binary != "" {
		return m.Tool.Binary
	}
	return filepath.Base(filepath.Clean(m.Build.Binary))
}

// validateTool holds a [tool] declaration to what dispatch and the consent
// screen need of it: an installable command name, and at least one phrase a
// user could actually type. Nothing else about the manifest tightens for a
// tool — the panel checks are skipped because there is no panel to check.
func validateTool(t *Tool) error {
	if t.Binary != "" && !namePattern.MatchString(t.Binary) {
		return fmt.Errorf("tool.binary %q must be a bare command name — lowercase letters, digits and dashes, no path", t.Binary)
	}
	if len(t.Provides) == 0 {
		return errors.New("tool.provides is required — name at least one `grove <verb>` phrase the tool answers")
	}
	for i, phrase := range t.Provides {
		key := fmt.Sprintf("tool.provides[%d]", i)
		if strings.TrimSpace(phrase) == "" {
			return fmt.Errorf("%s is empty", key)
		}
		if phrase != strings.TrimSpace(phrase) {
			return fmt.Errorf("%s %q must not begin or end with whitespace — it is rendered verbatim as the command it enables", key, phrase)
		}
		if err := printable(key, phrase); err != nil {
			return err
		}
		tokens := strings.Fields(phrase)
		for _, tok := range tokens {
			if strings.HasPrefix(tok, "-") {
				return fmt.Errorf("%s %q contains a flag-shaped token %q — provides declares verbs, not options", key, phrase, tok)
			}
		}
		// The first token is the dispatch verb, so it is held to the same
		// shape a plugin name is: it has to survive as a subcommand name.
		if !namePattern.MatchString(tokens[0]) {
			return fmt.Errorf("%s %q must begin with a bare verb — lowercase letters, digits and dashes", key, phrase)
		}
	}
	return nil
}

// validateArgv rejects empty and non-printable argv elements. An empty element
// is almost always a TOML mistake, and it would silently pass an empty
// argument to the command.
func validateArgv(key string, argv []string) error {
	for i, a := range argv {
		if a == "" {
			return fmt.Errorf("%s[%d] is empty", key, i)
		}
		if err := printable(fmt.Sprintf("%s[%d]", key, i), a); err != nil {
			return err
		}
	}
	return nil
}

// validateRelPath keeps a manifest from naming a file outside its own
// checkout — the manifest is untrusted input until the user approves it, and
// even after approval "build.binary = /etc/passwd" is not a thing to honor.
func validateRelPath(key, p string) error {
	if filepath.IsAbs(p) {
		return fmt.Errorf("%s %q must be relative to the plugin repo root", key, p)
	}
	clean := filepath.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s %q must stay inside the plugin repo", key, p)
	}
	return printable(key, p)
}

// validateSettings walks a free-form settings table and rejects anything that
// could not be shown honestly on the consent screen.
//
// grove deliberately does not constrain the SHAPE — it cannot know what a
// third-party panel's options mean, and a schema that guessed would reject
// valid ones. What it does constrain is renderability: every key and every
// string value ends up printed on a screen the user's approval depends on, and
// a value carrying an escape sequence could redraw that screen to say something
// other than what will be installed. Tables and arrays are walked so nesting
// cannot smuggle one past.
//
// The depth bound is not about attack surface; it is about the consent screen
// staying legible. A manifest that needs more than a few levels of nesting to
// express its defaults has outgrown what a user can meaningfully approve.
func validateSettings(key string, settings map[string]any) error {
	return validateSettingsAt(key, settings, 0)
}

const maxSettingsDepth = 8

func validateSettingsAt(key string, v any, depth int) error {
	if depth > maxSettingsDepth {
		return fmt.Errorf("%s nests more than %d levels deep", key, maxSettingsDepth)
	}
	switch v := v.(type) {
	case map[string]any:
		for name, value := range v {
			if err := printable(key+" key "+name, name); err != nil {
				return err
			}
			if err := validateSettingsAt(key+"."+name, value, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for i, value := range v {
			if err := validateSettingsAt(fmt.Sprintf("%s[%d]", key, i), value, depth+1); err != nil {
				return err
			}
		}
	case string:
		return printable(key, v)
	}
	return nil
}

// validateSettingOptions holds a `[[panel.setting_options]]` declaration to the
// two things that make it useful: it names a setting the panel actually
// declares, and the setting's own default is one of the values it offers.
//
// Both are author errors that are otherwise invisible. Options hung on a
// mistyped setting path silently do nothing; a default outside its own option
// list means the panel ships in a state its own declaration says is not
// available, and a UI offering the list would have no entry to show as current.
// Neither is a policy about what the USER may set — a value outside the list is
// still writable, because the list is a declaration and not a gate.
func validateSettingOptions(p *Panel) error {
	values := settingValues(p.Settings)
	seen := make(map[string]bool, len(p.SettingOptions))
	for i, o := range p.SettingOptions {
		key := fmt.Sprintf("panel.setting_options[%d]", i)
		if strings.TrimSpace(o.Setting) == "" {
			return fmt.Errorf("%s.setting is required — name the setting these are the options for", key)
		}
		if err := printable(key+".setting", o.Setting); err != nil {
			return err
		}
		if seen[o.Setting] {
			return fmt.Errorf("%s.setting %q is declared twice — one setting has one set of options", key, o.Setting)
		}
		seen[o.Setting] = true
		if err := printable(key+".description", o.Description); err != nil {
			return err
		}
		value, declared := values[o.Setting]
		if !declared {
			return fmt.Errorf("%s.setting %q names no setting in [panel.settings] — options for a setting the panel has no default for are options for nothing", key, o.Setting)
		}
		if len(o.Options) == 0 {
			return fmt.Errorf("%s.options is required — an entry that offers no values declares nothing about %s", key, o.Setting)
		}
		offered := make(map[string]bool, len(o.Options))
		for j, opt := range o.Options {
			if strings.TrimSpace(opt) == "" {
				return fmt.Errorf("%s.options[%d] is empty", key, j)
			}
			if err := printable(fmt.Sprintf("%s.options[%d]", key, j), opt); err != nil {
				return err
			}
			if offered[opt] {
				return fmt.Errorf("%s.options lists %q twice", key, opt)
			}
			offered[opt] = true
		}
		// The default is compared as TEXT, which is how every other comparison
		// in this package reads a setting: the consent screen, the update diff
		// and the host's own editor all hold "24" rather than 24, so an option
		// list for a numeric setting is written as strings and still matches.
		if !offered[value] && !o.AllowCustom {
			return fmt.Errorf("[panel.settings].%s is %s, which is not one of the options %s declares (%s) — set allow_custom if the list is only a suggestion", o.Setting, value, key, strings.Join(o.Options, ", "))
		}
		switch {
		case o.CustomOption == "" && o.CustomSetting == "":
		case o.CustomOption == "":
			return fmt.Errorf("%s.custom_setting names %q but %s.custom_option does not say which choice selects it", key, o.CustomSetting, key)
		case o.CustomSetting == "":
			return fmt.Errorf("%s.custom_option is %q but %s.custom_setting does not say which setting that choice hands over to", key, o.CustomOption, key)
		case !offered[o.CustomOption]:
			return fmt.Errorf("%s.custom_option %q is not one of the options it declares (%s)", key, o.CustomOption, strings.Join(o.Options, ", "))
		case o.CustomSetting == o.Setting:
			return fmt.Errorf("%s.custom_setting is %s itself — a setting cannot hand over to itself; allow_custom is how a setting takes a typed value of its own", key, o.Setting)
		default:
			if _, ok := values[o.CustomSetting]; !ok {
				return fmt.Errorf("%s.custom_setting %q names no setting in [panel.settings]", key, o.CustomSetting)
			}
		}
	}
	return nil
}

// OptionsFor returns the declaration for one setting path, or nil.
func (p *Panel) OptionsFor(setting string) *SettingOptions {
	for i := range p.SettingOptions {
		if p.SettingOptions[i].Setting == setting {
			return &p.SettingOptions[i]
		}
	}
	return nil
}

// printable rejects control characters. Everything here is rendered into a
// terminal consent screen the user's decision depends on, so a value that can
// move the cursor or clear the line is refused rather than displayed.
func printable(key, v string) error {
	for _, r := range v {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("%s contains a control character", key)
		}
	}
	return nil
}
