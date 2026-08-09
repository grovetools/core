// Package registry owns the machine-presence note: the one document a machine
// publishes about itself into the reserved `registry` sync workspace, at
// machines/<machine-id>.md.
//
// # Why a note and not a server table
//
// grove-syncd is a dumb rendezvous: it replicates documents and knows nothing
// about machines. Presence is therefore *replicated content* — the same
// substrate notes already use — rather than a new identity subsystem. One
// document per machine, keyed by the machine's state-held ULID and written by
// that machine alone, composes with per-document OCC by construction: two
// machines never write the same document, so a presence note can never
// conflict.
//
// # ID-keyed, never name-keyed
//
// The path segment is the ULID from core/pkg/machine, never the display name.
// Names come from machine.toml, which is dotfiles-portable on purpose, so two
// hosts restored from one dotfiles repo share a name and would otherwise share
// a note — silently overwriting each other. Renders pair the name with the
// short id (machine.Describe) for exactly the same reason.
//
// # Interim trust model (integrity-advisory, not authenticated)
//
// Every token grove-syncd issues today is the owner and can write any path
// (sync/pkg/server getUserPrefixes: user_id 1 bypasses filtering; CreateToken
// cannot assign a user). The registry is therefore a coordination surface
// among one user's mutually-trusting devices, NOT an authenticated inventory.
// Until device principals land, the posture is detection rather than
// prevention, in two places:
//
//   - the daemon's own-note pull guard drops inbound events for this machine's
//     own path and surfaces them as a registry_foreign_write conflict;
//   - readers validate at read time (Validate below): machine_id must equal
//     the id in the path, and rev must not regress against the local cache.
//
// Both are advisory. A determined writer with a token can still forge a note;
// what it cannot do is go unnoticed.
//
// # Rendering
//
// The frontmatter is rendered BY HAND rather than marshalled, so the daemon
// gains no note-tooling dependency and so byte stability is a property of this
// file rather than of a library's map ordering. Every collection is emitted in
// sorted order and every field in a fixed order, which is what lets the writer
// compare rendered bytes against disk and skip the write when nothing changed.
// Parsing goes through yaml.v3 against a mirror struct; the round-trip test is
// what keeps the two halves honest.
package registry

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// MachinesDir is the directory, relative to the registry workspace root, that
// holds one note per machine.
//
// It is deliberately not dot-prefixed: SyncHandler.MatchesEvent drops any path
// whose basename starts with a dot, and DocSpace excludes several dot
// directories outright, so a hidden directory here would never replicate.
const MachinesDir = "machines"

// NoteExt is the machine note's file extension.
const NoteExt = ".md"

// DateFormat is last_seen's day resolution (RFC3339 full-date). Staleness is
// an advisory read-side comparison against the reader's own clock, so a finer
// resolution would only add write churn: a timestamp that changes every second
// would defeat the byte-compare write suppression entirely.
const DateFormat = "2006-01-02"

// Note is one machine's presence document: identity, the observed state of
// its ecosystems and repos, and the intent it declared.
//
// Field order here is the rendered field order. Adding a field means adding it
// to Render, to noteFrontmatter, and to the round-trip test.
type Note struct {
	// MachineID is the state-held ULID (core/pkg/machine). It MUST equal the
	// id in the note's path; Validate treats a mismatch as tampering.
	MachineID string
	// Name is the config-held display name. Display only — never an identifier.
	Name string
	// Rev is a monotonic counter this machine bumps on every write. Readers
	// cache the highest rev they have seen per machine; a regression means
	// either a restored-from-backup note or a forgery, and renders as suspect.
	Rev int64
	// LastSeen is the day this note was last written, at DateFormat
	// resolution. Advisory: it is compared against the READER's clock, which
	// the sync protocol never does for ordering.
	LastSeen string
	// OriginID is the per-sync.db install id, which dies with that database.
	// Recorded so "same MachineID, new OriginID" is diagnosable as a wiped
	// sync.db rather than a new machine.
	OriginID string
	// GrovedVersion is the daemon build that wrote the note.
	GrovedVersion string
	// Ecosystems is every declared subscription reconciled against the disk,
	// sorted by name, each carrying a copy of the ecosystem's own card.
	Ecosystems []NoteEcosystem
	// Roots is every declared bare scan root, sorted by name. Roots never
	// carry a card and are never "declared-missing": nothing materializes one.
	Roots []NoteRoot
	// Subscriptions is this machine's sync subscriptions, sorted by name.
	// Tokens are never recorded — nothing token-shaped may enter a note, both
	// because the registry is world-readable to every device and because the
	// push path quarantines documents matching the secret heuristics.
	Subscriptions []NoteSubscription
	// Repos is the point-in-time tip of every repo under a declared root,
	// sorted by (root, path). Refreshed only inside a triggered write, never
	// per commit.
	Repos []NoteRepo
}

// NoteEcosystem is one reconciled ecosystem subscription.
type NoteEcosystem struct {
	Name string
	Path string
	// Notebook is the machine-side override, empty when the card decides.
	Notebook string
	// State is "present", "declared-missing", or "unmanifested" (the
	// config.MachineEcosystem* vocabulary). "declared-missing" is what makes
	// another machine able to say "you asked for this and it isn't there".
	State   string
	Enabled bool
	// Repos/Exclude copy subscriber-local partial membership intent. Empty
	// Repos and Exclude means all card members; they are mutually exclusive in
	// machine.toml. Publishing them is what lets peers distinguish an
	// intentionally excluded repo from an absent one.
	Repos   []string
	Exclude []string
	// Card is a COPY of the ecosystem's own repo-side card. There is
	// deliberately no shared ecosystems/<name>.md note: that would be a
	// multi-writer document, which the single-writer rule forbids. Readers
	// dedup identical cards at render time instead.
	Card *NoteCard
}

// NoteCard is the embedded copy of an ecosystem's identity card.
type NoteCard struct {
	ID        string
	Layout    string
	Remotes   []NoteRemote
	Notebooks []NoteCardNotebook
}

// NoteRemote is one git remote off the embedded card.
type NoteRemote struct {
	Name string
	URL  string
}

// NoteCardNotebook is one notebook binding off the embedded card. The map in
// config.EcosystemCard becomes a name-sorted slice here so the render is
// stable.
type NoteCardNotebook struct {
	Name     string
	Default  bool
	Audience string
}

// NoteRoot is one declared bare scan root.
type NoteRoot struct {
	Name     string
	Path     string
	Notebook string
	Enabled  bool
	Exists   bool
}

// NoteSubscription is one sync subscription, minus anything secret.
type NoteSubscription struct {
	Name string
	Role string
	Mode string
	Pull bool
}

// NoteRepo is one repository's tip at write time.
type NoteRepo struct {
	// Root is the declared ecosystem/root this repo was found under.
	Root string
	// Path is the repo's location relative to that root ("." for the root
	// repo itself), so the note stays readable and machine-path-independent.
	Path string
	// Branch is empty on a detached HEAD.
	Branch string
	SHA    string
}

// NotePath returns the workspace-relative path of a machine's note. It is the
// single place the layout is decided; the pull guard, the writer, and every
// reader all go through it.
func NotePath(machineID string) string {
	return path.Join(MachinesDir, machineID+NoteExt)
}

// MachineIDFromPath returns the machine id encoded in a workspace-relative
// note path, or "" when the path is not a machine note. Readers use it to
// check the id in the path against the id in the document.
func MachineIDFromPath(relPath string) string {
	rel := strings.TrimPrefix(path.Clean(strings.ReplaceAll(relPath, "\\", "/")), "./")
	dir, file := path.Split(rel)
	if strings.Trim(dir, "/") != MachinesDir {
		return ""
	}
	if !strings.HasSuffix(file, NoteExt) {
		return ""
	}
	return strings.TrimSuffix(file, NoteExt)
}

// Today returns the day stamp for a time, at last_seen resolution.
func Today(t time.Time) string { return t.UTC().Format(DateFormat) }

// Render produces the note's bytes. It is a pure function of the Note: the
// same Note always renders identically, which is the property the writer's
// byte-compare skip depends on. Collections are sorted here rather than
// assumed sorted, so a caller that builds them in map order still gets a
// stable document.
func (n *Note) Render() []byte {
	if n == nil {
		return nil
	}
	c := n.normalized()

	var b strings.Builder
	b.WriteString("---\n")
	writeScalar(&b, "", "machine_id", c.MachineID)
	writeScalar(&b, "", "name", c.Name)
	b.WriteString("rev: " + strconv.FormatInt(c.Rev, 10) + "\n")
	writeScalar(&b, "", "last_seen", c.LastSeen)
	writeScalar(&b, "", "origin_id", c.OriginID)
	writeScalar(&b, "", "groved_version", c.GrovedVersion)

	writeSeqKey(&b, "", "ecosystems", len(c.Ecosystems))
	for _, e := range c.Ecosystems {
		item := itemPrefix("")
		writeDashScalar(&b, "", "name", e.Name)
		writeScalar(&b, item, "path", e.Path)
		writeOptScalar(&b, item, "notebook", e.Notebook)
		writeScalar(&b, item, "state", e.State)
		writeBool(&b, item, "enabled", e.Enabled)
		writeStringSeq(&b, item, "repos", e.Repos)
		writeStringSeq(&b, item, "exclude", e.Exclude)
		if e.Card == nil {
			continue
		}
		b.WriteString(item + "card:\n")
		card := item + "  "
		writeScalar(&b, card, "id", e.Card.ID)
		writeOptScalar(&b, card, "layout", e.Card.Layout)

		writeSeqKey(&b, card, "remotes", len(e.Card.Remotes))
		for _, r := range e.Card.Remotes {
			writeDashScalar(&b, card, "name", r.Name)
			writeScalar(&b, itemPrefix(card), "url", r.URL)
		}
		writeSeqKey(&b, card, "notebooks", len(e.Card.Notebooks))
		for _, nb := range e.Card.Notebooks {
			writeDashScalar(&b, card, "name", nb.Name)
			writeBool(&b, itemPrefix(card), "default", nb.Default)
			writeOptScalar(&b, itemPrefix(card), "audience", nb.Audience)
		}
	}

	writeSeqKey(&b, "", "roots", len(c.Roots))
	for _, r := range c.Roots {
		item := itemPrefix("")
		writeDashScalar(&b, "", "name", r.Name)
		writeScalar(&b, item, "path", r.Path)
		writeOptScalar(&b, item, "notebook", r.Notebook)
		writeBool(&b, item, "enabled", r.Enabled)
		writeBool(&b, item, "exists", r.Exists)
	}

	writeSeqKey(&b, "", "subscriptions", len(c.Subscriptions))
	for _, s := range c.Subscriptions {
		item := itemPrefix("")
		writeDashScalar(&b, "", "name", s.Name)
		writeOptScalar(&b, item, "role", s.Role)
		writeOptScalar(&b, item, "mode", s.Mode)
		writeBool(&b, item, "pull", s.Pull)
	}

	writeSeqKey(&b, "", "repos", len(c.Repos))
	for _, r := range c.Repos {
		item := itemPrefix("")
		writeDashScalar(&b, "", "root", r.Root)
		writeScalar(&b, item, "path", r.Path)
		writeOptScalar(&b, item, "branch", r.Branch)
		writeScalar(&b, item, "sha", r.SHA)
	}
	b.WriteString("---\n\n")

	c.renderBody(&b)
	return []byte(b.String())
}

// renderBody writes the human-readable half. It carries no information the
// frontmatter lacks — readers parse the frontmatter — but a machine note is
// still a note, and someone will open it in an editor.
func (n *Note) renderBody(b *strings.Builder) {
	label := n.Name
	if label == "" {
		label = n.MachineID
	}
	fmt.Fprintf(b, "# %s\n\n", label)
	fmt.Fprintf(b, "Machine presence note, written by groved on %s.\n", n.LastSeen)
	b.WriteString("This document is single-writer: only the machine it describes updates it.\n\n")

	b.WriteString("## Ecosystems\n\n")
	if len(n.Ecosystems) == 0 {
		b.WriteString("None declared.\n\n")
	} else {
		for _, e := range n.Ecosystems {
			suffix := ""
			if e.State != "present" {
				suffix = " — " + e.State
			}
			if !e.Enabled {
				suffix += " (disabled)"
			}
			fmt.Fprintf(b, "- **%s** `%s`%s\n", e.Name, e.Path, suffix)
		}
		b.WriteString("\n")
	}

	if len(n.Roots) > 0 {
		b.WriteString("## Roots\n\n")
		for _, r := range n.Roots {
			fmt.Fprintf(b, "- **%s** `%s`\n", r.Name, r.Path)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Repositories\n\n")
	if len(n.Repos) == 0 {
		b.WriteString("None observed.\n")
		return
	}
	for _, r := range n.Repos {
		branch := r.Branch
		if branch == "" {
			branch = "(detached)"
		}
		fmt.Fprintf(b, "- `%s/%s` %s @ %s\n", r.Root, r.Path, branch, shortSHA(r.SHA))
	}
}

func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// normalized returns a copy with every collection sorted, so Render never
// depends on the order a caller happened to build things in.
func (n *Note) normalized() *Note {
	c := *n
	c.Ecosystems = append([]NoteEcosystem(nil), n.Ecosystems...)
	sort.Slice(c.Ecosystems, func(i, j int) bool { return c.Ecosystems[i].Name < c.Ecosystems[j].Name })
	for i := range c.Ecosystems {
		c.Ecosystems[i].Repos = append([]string(nil), c.Ecosystems[i].Repos...)
		sort.Strings(c.Ecosystems[i].Repos)
		c.Ecosystems[i].Exclude = append([]string(nil), c.Ecosystems[i].Exclude...)
		sort.Strings(c.Ecosystems[i].Exclude)
		if card := c.Ecosystems[i].Card; card != nil {
			cp := *card
			cp.Remotes = append([]NoteRemote(nil), card.Remotes...)
			sort.Slice(cp.Remotes, func(a, b int) bool { return cp.Remotes[a].Name < cp.Remotes[b].Name })
			cp.Notebooks = append([]NoteCardNotebook(nil), card.Notebooks...)
			sort.Slice(cp.Notebooks, func(a, b int) bool { return cp.Notebooks[a].Name < cp.Notebooks[b].Name })
			c.Ecosystems[i].Card = &cp
		}
	}
	c.Roots = append([]NoteRoot(nil), n.Roots...)
	sort.Slice(c.Roots, func(i, j int) bool { return c.Roots[i].Name < c.Roots[j].Name })
	c.Subscriptions = append([]NoteSubscription(nil), n.Subscriptions...)
	sort.Slice(c.Subscriptions, func(i, j int) bool { return c.Subscriptions[i].Name < c.Subscriptions[j].Name })
	c.Repos = append([]NoteRepo(nil), n.Repos...)
	sort.Slice(c.Repos, func(i, j int) bool {
		if c.Repos[i].Root != c.Repos[j].Root {
			return c.Repos[i].Root < c.Repos[j].Root
		}
		return c.Repos[i].Path < c.Repos[j].Path
	})
	return &c
}

// --- hand-rolled YAML emission -------------------------------------------
//
// Everything is prefix-driven: a mapping key at prefix P, a sequence under it
// whose "- " markers sit at P, and that sequence's item keys at P + 2 spaces
// (itemPrefix). Empty sequences are emitted as the flow form `key: []` so
// "declared nothing" and "field absent" stay distinguishable in the document.

// itemPrefix is where a sequence item's mapping keys live, given the prefix of
// the key that introduced the sequence. The "- " marker occupies the first two
// columns of the first key's line, so continuation keys line up two in.
func itemPrefix(prefix string) string { return prefix + "  " }

func writeScalar(b *strings.Builder, prefix, key, value string) {
	b.WriteString(prefix + key + ": " + yamlScalar(value) + "\n")
}

// writeOptScalar omits a key entirely when its value is empty, so an unset
// optional never renders as `key: ""`.
func writeOptScalar(b *strings.Builder, prefix, key, value string) {
	if value == "" {
		return
	}
	writeScalar(b, prefix, key, value)
}

func writeBool(b *strings.Builder, prefix, key string, value bool) {
	b.WriteString(prefix + key + ": " + strconv.FormatBool(value) + "\n")
}

// writeDashScalar emits a sequence item's FIRST key — the line carrying "- ".
func writeDashScalar(b *strings.Builder, prefix, key, value string) {
	b.WriteString(prefix + "- " + key + ": " + yamlScalar(value) + "\n")
}

func writeSeqKey(b *strings.Builder, prefix, key string, n int) {
	if n == 0 {
		b.WriteString(prefix + key + ": []\n")
		return
	}
	b.WriteString(prefix + key + ":\n")
}

func writeStringSeq(b *strings.Builder, prefix, key string, values []string) {
	writeSeqKey(b, prefix, key, len(values))
	for _, value := range values {
		b.WriteString(prefix + "  - " + yamlScalar(value) + "\n")
	}
}

// plainScalar matches the values safe to emit unquoted: no leading indicator
// character, no colon-space, no chance of being read as a number or a boolean.
var plainScalar = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._/@+ -]*$`)

// yamlReserved are the plain scalars YAML 1.1 readers coerce to non-strings.
var yamlReserved = map[string]bool{
	"true": true, "false": true, "yes": true, "no": true, "on": true, "off": true,
	"null": true, "Null": true, "NULL": true, "True": true, "False": true,
	"Yes": true, "No": true, "On": true, "Off": true, "~": true,
}

// yamlScalar renders a Go string as a YAML scalar, quoting only when a plain
// scalar would be ambiguous. Quoting is done with Go's %q, whose escapes are a
// subset of YAML's double-quoted style for the characters that can appear
// here.
func yamlScalar(s string) string {
	if s == "" {
		return `""`
	}
	if yamlReserved[s] || !plainScalar.MatchString(s) || strings.HasSuffix(s, " ") {
		return strconv.Quote(s)
	}
	// A value that parses as a number must be quoted, or it round-trips as an
	// int/float and the mirror struct's string field rejects it.
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return strconv.Quote(s)
	}
	return s
}
