package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Ecosystem layout values (EcosystemCard.Layout).
const (
	// LayoutSuperrepo: the primary remote is a superrepo whose submodules are
	// the member repos. A peer clones it and runs `git submodule update`,
	// which is what gives worktree/plan tooling full parity with the source.
	LayoutSuperrepo = "superrepo"
	// LayoutFlat: the remotes enumerate independent repos, cloned side by
	// side. Correct only for ecosystems that never were superrepos.
	LayoutFlat = "flat"
)

// ecosystemManifestNames are the manifest basenames that may carry a card, in
// the order the card writer prefers them. It intentionally matches the
// import-mode probe in grove's setup package: a directory "is an ecosystem"
// when one of these exists.
var ecosystemManifestNames = []string{"grove.toml", "grove.yml"}

// Validate checks a card for self-consistency before it is written or trusted.
// It does NOT mint anything and does not require an ID: an un-minted card is a
// legitimate intermediate the writer fills in.
func (c *EcosystemCard) Validate() error {
	if c == nil {
		return nil
	}
	if strings.TrimSpace(c.ID) != c.ID {
		return fmt.Errorf("ecosystem id %q has leading or trailing whitespace", c.ID)
	}
	switch c.Layout {
	case "", LayoutSuperrepo, LayoutFlat:
	default:
		return fmt.Errorf("ecosystem layout %q must be %q or %q", c.Layout, LayoutSuperrepo, LayoutFlat)
	}

	seen := make(map[string]bool, len(c.Remotes))
	for _, r := range c.Remotes {
		if strings.TrimSpace(r.Name) == "" {
			return fmt.Errorf("ecosystem remote has an empty name")
		}
		if strings.TrimSpace(r.URL) == "" {
			return fmt.Errorf("ecosystem remote %q has an empty url", r.Name)
		}
		if seen[r.Name] {
			return fmt.Errorf("ecosystem remote %q is declared twice", r.Name)
		}
		seen[r.Name] = true
	}

	defaults := 0
	for name, nb := range c.Notebooks {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("ecosystem notebook has an empty name")
		}
		if nb.Default {
			defaults++
		}
	}
	if defaults > 1 {
		return fmt.Errorf("ecosystem declares %d default notebooks; at most one may be default", defaults)
	}
	return nil
}

// DefaultNotebookName returns the name of the notebook marked default, or ""
// when the card binds none. Ties cannot happen — Validate rejects them — but a
// hand-edited card that slipped through resolves deterministically by name.
func (c *EcosystemCard) DefaultNotebookName() string {
	if c == nil {
		return ""
	}
	best := ""
	for name, nb := range c.Notebooks {
		if nb.Default && (best == "" || name < best) {
			best = name
		}
	}
	return best
}

// FindEcosystemManifest returns the path of dir's grove manifest, or "" when
// dir carries none. grove.toml wins over grove.yml when both exist, matching
// the load path's precedence.
func FindEcosystemManifest(dir string) string {
	for _, name := range ecosystemManifestNames {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
	}
	return ""
}

// LoadEcosystemCard reads just the card out of a manifest. A manifest without
// an `[ecosystem]` table returns (nil, nil) — "no card yet", the adopt case.
func LoadEcosystemCard(path string) (*EcosystemCard, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}
	cfg, err := unmarshalConfig(path, data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	return cfg.Ecosystem, nil
}

// WriteEcosystemCard persists card into the manifest at path, creating the
// manifest only if it is already absent (callers that scaffold write their own
// skeleton first). It reports whether the file changed.
//
// Two properties are load-bearing:
//
//  1. **The id is minted once.** If the manifest already carries a card with an
//     id, that id is kept and a mismatching card.ID is an error rather than an
//     overwrite. The guard lives here, at the only write path, instead of being
//     a rule every caller has to remember.
//  2. **Everything outside the card survives byte-for-byte.** The edit is
//     surgical — the `[ecosystem]` table (and its subtables) is the only region
//     replaced. Comments, key order, and tables this struct does not model are
//     preserved, because an ecosystem manifest is hand-authored and committed;
//     round-tripping it through a marshaller would quietly rewrite the user's
//     file.
//
// An unchanged rendering writes nothing at all, which is what makes re-running
// `grove ecosystem adopt` a true no-op.
func WriteEcosystemCard(path string, card EcosystemCard) (bool, error) {
	if path == "" {
		return false, fmt.Errorf("ecosystem manifest path is empty")
	}

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("failed to read %s: %w", path, err)
	}
	missing := err != nil

	if !missing {
		current, cErr := LoadEcosystemCard(path)
		if cErr != nil {
			return false, cErr
		}
		if current != nil && current.ID != "" {
			if card.ID != "" && card.ID != current.ID {
				return false, fmt.Errorf("%s already has ecosystem id %s; an ecosystem id is minted once and never rewritten", path, current.ID)
			}
			card.ID = current.ID
		}
	}

	if err := card.Validate(); err != nil {
		return false, err
	}

	var updated string
	if strings.HasSuffix(path, ".toml") {
		updated = setTOMLEcosystemCard(string(existing), card)
	} else {
		updated = setYAMLEcosystemCard(string(existing), card)
	}
	if !missing && updated == string(existing) {
		return false, nil
	}

	if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr != nil {
		return false, fmt.Errorf("failed to create directory for %s: %w", path, mkErr)
	}
	if wErr := os.WriteFile(path, []byte(updated), 0o600); wErr != nil {
		return false, fmt.Errorf("failed to write %s: %w", path, wErr)
	}

	// Re-parse what we just persisted: a surgical edit that produced an
	// unparseable manifest must fail here, not at the next config load.
	if _, pErr := LoadEcosystemCard(path); pErr != nil {
		return true, fmt.Errorf("%s is invalid after writing the ecosystem card: %w", path, pErr)
	}
	return true, nil
}

// setTOMLEcosystemCard replaces the `[ecosystem]` table (header through its
// last subtable) with a freshly rendered block, or appends one when the
// manifest declares no card. Every line outside that region is untouched.
func setTOMLEcosystemCard(content string, card EcosystemCard) string {
	block := renderTOMLEcosystemCard(card)

	lines := strings.Split(content, "\n")
	start, end := -1, len(lines)
	for i, line := range lines {
		key, ok := tomlTableKey(line)
		if !ok {
			continue
		}
		inCard := key == "ecosystem" || strings.HasPrefix(key, "ecosystem.")
		if inCard {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			end = i
			break
		}
	}

	if start < 0 {
		if strings.TrimSpace(content) == "" {
			return block
		}
		sep := "\n"
		if strings.HasSuffix(content, "\n") {
			sep = ""
		}
		return content + sep + "\n" + block
	}

	// Blank lines trailing the old table separate it from what follows; they
	// belong to the document, not the card, so hand them back rather than
	// swallowing the separator.
	for end > start+1 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}

	out := make([]string, 0, len(lines)+8)
	out = append(out, lines[:start]...)
	out = append(out, strings.Split(strings.TrimSuffix(block, "\n"), "\n")...)
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n")
}

// tomlTableKey extracts the dotted key of a TOML table header line
// (`[a.b]` or `[[a.b]]`), reporting false for any other line.
func tomlTableKey(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "[") {
		return "", false
	}
	// Drop a trailing comment; a `#` inside a quoted key would be misread,
	// which no grove manifest has ever contained.
	if idx := strings.Index(trimmed, "#"); idx >= 0 {
		trimmed = strings.TrimSpace(trimmed[:idx])
	}
	trimmed = strings.TrimSuffix(trimmed, "]")
	trimmed = strings.TrimPrefix(trimmed, "[")
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		trimmed = strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]")
	}
	key := strings.TrimSpace(trimmed)
	if key == "" {
		return "", false
	}
	// Normalize quoted segments so `["ecosystem"]` matches `[ecosystem]`.
	parts := strings.Split(key, ".")
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if unquoted, err := strconv.Unquote(p); err == nil {
			p = unquoted
		}
		parts[i] = p
	}
	return strings.Join(parts, "."), true
}

// renderTOMLEcosystemCard renders the card deterministically: fixed field
// order, remotes in card order (callers sort at collection time), notebooks
// sorted by name. Determinism is what lets an unchanged card skip the write.
func renderTOMLEcosystemCard(card EcosystemCard) string {
	var b strings.Builder
	b.WriteString("[ecosystem]\n")
	if card.ID != "" {
		fmt.Fprintf(&b, "id = %s\n", strconv.Quote(card.ID))
	}
	if card.Layout != "" {
		fmt.Fprintf(&b, "layout = %s\n", strconv.Quote(card.Layout))
	}
	for _, r := range card.Remotes {
		b.WriteString("\n[[ecosystem.remotes]]\n")
		fmt.Fprintf(&b, "name = %s\n", strconv.Quote(r.Name))
		fmt.Fprintf(&b, "url = %s\n", strconv.Quote(r.URL))
	}
	for _, name := range sortedNotebookNames(card.Notebooks) {
		nb := card.Notebooks[name]
		fmt.Fprintf(&b, "\n[ecosystem.notebooks.%s]\n", tomlKey(name))
		if nb.Default {
			b.WriteString("default = true\n")
		}
		if nb.Audience != "" {
			fmt.Fprintf(&b, "audience = %s\n", strconv.Quote(nb.Audience))
		}
	}
	return b.String()
}

// setYAMLEcosystemCard is the YAML dialect of setTOMLEcosystemCard: it
// replaces the top-level `ecosystem:` mapping (its key line plus every
// following indented or blank line) and leaves the rest of the document
// verbatim. `grove ecosystem init` scaffolds TOML now, but every ecosystem
// created before that — and any created with `--format yaml` — carries a
// grove.yml, so adopt must be able to backfill a card there.
func setYAMLEcosystemCard(content string, card EcosystemCard) string {
	block := renderYAMLEcosystemCard(card)

	lines := strings.Split(content, "\n")
	start, end := -1, len(lines)
	for i, line := range lines {
		if start < 0 {
			if isYAMLTopLevelKey(line, "ecosystem") {
				start = i
			}
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Any line back at column zero ends the mapping.
		if line[0] != ' ' && line[0] != '\t' {
			end = i
			break
		}
	}

	if start < 0 {
		if strings.TrimSpace(content) == "" {
			return block
		}
		sep := "\n"
		if strings.HasSuffix(content, "\n") {
			sep = ""
		}
		return content + sep + block
	}

	// Trailing blank lines swept up by the scan belong to the document, not
	// the card; give them back so the write stays minimal.
	for end > start+1 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}

	out := make([]string, 0, len(lines)+8)
	out = append(out, lines[:start]...)
	out = append(out, strings.Split(strings.TrimSuffix(block, "\n"), "\n")...)
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n")
}

// isYAMLTopLevelKey reports whether line declares the given key at column
// zero.
func isYAMLTopLevelKey(line, key string) bool {
	if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '#' {
		return false
	}
	name, _, ok := strings.Cut(line, ":")
	if !ok {
		return false
	}
	if unquoted, err := strconv.Unquote(strings.TrimSpace(name)); err == nil {
		name = unquoted
	}
	return strings.TrimSpace(name) == key
}

func renderYAMLEcosystemCard(card EcosystemCard) string {
	var b strings.Builder
	b.WriteString("ecosystem:\n")
	if card.ID != "" {
		fmt.Fprintf(&b, "  id: %s\n", strconv.Quote(card.ID))
	}
	if card.Layout != "" {
		fmt.Fprintf(&b, "  layout: %s\n", strconv.Quote(card.Layout))
	}
	if len(card.Remotes) > 0 {
		b.WriteString("  remotes:\n")
		for _, r := range card.Remotes {
			fmt.Fprintf(&b, "    - name: %s\n", strconv.Quote(r.Name))
			fmt.Fprintf(&b, "      url: %s\n", strconv.Quote(r.URL))
		}
	}
	if len(card.Notebooks) > 0 {
		b.WriteString("  notebooks:\n")
		for _, name := range sortedNotebookNames(card.Notebooks) {
			nb := card.Notebooks[name]
			if !nb.Default && nb.Audience == "" {
				// An empty mapping still has to be spelled out, or the key
				// would decode as null and drop the binding.
				fmt.Fprintf(&b, "    %s: {}\n", strconv.Quote(name))
				continue
			}
			fmt.Fprintf(&b, "    %s:\n", strconv.Quote(name))
			if nb.Default {
				b.WriteString("      default: true\n")
			}
			if nb.Audience != "" {
				fmt.Fprintf(&b, "      audience: %s\n", strconv.Quote(nb.Audience))
			}
		}
	}
	return b.String()
}

func sortedNotebookNames(notebooks map[string]EcosystemNotebook) []string {
	names := make([]string, 0, len(notebooks))
	for name := range notebooks {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// tomlKey renders a table-key segment bare when it is a TOML bare key and
// quoted otherwise, so a notebook named "work notes" still produces a valid
// header.
func tomlKey(name string) string {
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return strconv.Quote(name)
		}
	}
	if name == "" {
		return `""`
	}
	return name
}
