package config

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Writing a HAND-CONFIGURED [tui.plugins] entry back to the file that declares
// it.
//
// This is deliberately not a general config writer, and it is deliberately not
// the path a MANAGED plugin's settings take. A managed plugin lives in a
// fragment grove owns, its contents are hashed into the install approval, and
// editing it out from under that record flips its trust state to `edited` —
// which is why `grove plugin set` exists and re-records the approval. A
// hand-written entry has no pin, no fragment and no approval: it is a table in
// the user's own config, and the only thing standing between a UI and editing
// it is the file's other bytes.
//
// So the edit is surgical, the same discipline machine.toml and the ecosystem
// card are written with: the [tui.plugins.<name>] table is re-rendered in
// place and every other line — comments, key order, unrelated tables, the
// user's own formatting — is preserved verbatim. A marshaller round-trip would
// be simpler and would silently eat all of it.
//
// Two things are refused rather than guessed at:
//
//   - An entry the file does not declare as a [tui.plugins.<name>] TABLE.
//     TOML has several other ways to say the same thing (a dotted key under
//     [tui], an inline table), and appending a table for a name the document
//     already defines another way produces a duplicate-key parse error at the
//     next load. Refusing names the file and hands the edit back to the user.
//   - A write that leaves the file unparseable. The re-read below is what
//     turns a bug here into an error at the moment of the edit instead of a
//     config that stops loading later, with no obvious cause.

// WritePluginEntry rewrites the [tui.plugins.<name>] table in the TOML file at
// path to describe entry, preserving every other byte of the document.
//
// It reports whether the file changed. A subtable of the entry
// ([tui.plugins.<name>.settings]) ends the replaced region and is preserved:
// this writer speaks for the entry's own keys, not for tables nested under it.
//
// The limit is stated rather than hidden: the entry's OWN table is re-rendered,
// so a comment written between its keys does not survive. Everything outside it
// does — other tables, other entries, the file's leading comments, the user's
// blank lines. That is the same trade setTOMLEcosystemCard makes, and it is the
// most a line-oriented editor can offer without a comment-preserving TOML
// parser, which the ecosystem does not have.
func WritePluginEntry(path, name string, entry *PluginConfig) (bool, error) {
	if path == "" {
		return false, fmt.Errorf("no config file was named for [tui.plugins.%s]", name)
	}
	if entry == nil {
		return false, fmt.Errorf("[tui.plugins.%s] has nothing to write", name)
	}
	existing, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("failed to read %s: %w", path, err)
	}
	key := "tui.plugins." + name
	if !declaresTOMLTable(string(existing), key) {
		return false, fmt.Errorf(
			"%s does not declare [%s] as a table, so this edit cannot be made without guessing at the form it is written in — edit the file by hand",
			path, key)
	}

	updated := setTOMLTable(string(existing), key, renderPluginEntryTable(key, entry))
	if updated == string(existing) {
		return false, nil
	}
	if wErr := os.WriteFile(path, []byte(updated), pluginConfigFileMode(path)); wErr != nil {
		return false, fmt.Errorf("failed to write %s: %w", path, wErr)
	}
	if pErr := reparsePluginConfig(path); pErr != nil {
		return true, pErr
	}
	return true, nil
}

// WritePluginOrder sets [tui] plugin_order in the TOML file at path.
//
// Rail order is the one part of a plugin's presentation that is never the
// plugin's to state: `plugin_order` is a key in the user's own config naming
// config keys, so reordering a MANAGED plugin is still an edit to a user layer
// and needs no approval re-record. That is why this is here rather than behind
// `grove plugin`.
func WritePluginOrder(path string, order []string) (bool, error) {
	if path == "" {
		return false, fmt.Errorf("no config file was named for [tui].plugin_order")
	}
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("failed to read %s: %w", path, err)
	}
	encoded := "plugin_order = " + encodeTOMLStringArray(order)
	updated := setTOMLKeyInTable(string(existing), "tui", "plugin_order", encoded)
	if err == nil && updated == string(existing) {
		return false, nil
	}
	if wErr := os.WriteFile(path, []byte(updated), pluginConfigFileMode(path)); wErr != nil {
		return false, fmt.Errorf("failed to write %s: %w", path, wErr)
	}
	if pErr := reparsePluginConfig(path); pErr != nil {
		return true, pErr
	}
	return true, nil
}

// CommentOutPluginEntry comments out the [tui.plugins.<name>] table rather than
// deleting it, and returns the lines it commented.
//
// Commenting rather than deleting is the whole point: this runs after an
// "adopt" — `grove plugin install` of the same panel — and the two entries
// would otherwise both declare the rail item. The user's own words for how they
// ran the panel are the only record of what it was doing before grove owned it,
// and a UI that silently deleted them would be asking for trust it has not
// earned. The returned text is what the caller shows as the diff.
func CommentOutPluginEntry(path, name string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("no config file was named for [tui.plugins.%s]", name)
	}
	existing, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", path, err)
	}
	key := "tui.plugins." + name
	if !declaresTOMLTable(string(existing), key) {
		return "", fmt.Errorf("%s does not declare [%s] as a table", path, key)
	}

	lines := strings.Split(string(existing), "\n")
	start, end := tomlTableRegion(lines, key)
	if start < 0 {
		return "", fmt.Errorf("%s does not declare [%s] as a table", path, key)
	}
	var commented []string
	for i := start; i < end; i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "#") || strings.TrimSpace(lines[i]) == "" {
			continue
		}
		commented = append(commented, lines[i])
		lines[i] = "# " + lines[i]
	}
	if len(commented) == 0 {
		return "", nil
	}
	// A note above the block, because a commented-out table with no explanation
	// reads as something the user disabled and forgot, not as something a tool
	// retired on their behalf.
	note := "# retired by the Plugins panel — `grove plugin install` now declares " + name
	out := append([]string{}, lines[:start]...)
	out = append(out, note)
	out = append(out, lines[start:]...)

	if wErr := os.WriteFile(path, []byte(strings.Join(out, "\n")), pluginConfigFileMode(path)); wErr != nil {
		return "", fmt.Errorf("failed to write %s: %w", path, wErr)
	}
	if pErr := reparsePluginConfig(path); pErr != nil {
		return strings.Join(commented, "\n"), pErr
	}
	return strings.Join(commented, "\n"), nil
}

// renderPluginEntryTable renders one [tui.plugins.<name>] table.
//
// Only the fields a person edits are written, and an empty one is omitted
// rather than written as `key = ""`: the merged entry is assembled from this
// table alone (whole-entry replacement), so an omitted key and an empty one
// mean the same thing to the loader, and the shorter of the two is the one a
// hand-authored file looks like.
func renderPluginEntryTable(key string, entry *PluginConfig) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[%s]\n", key)
	fmt.Fprintf(&b, "command = %s\n", strconv.Quote(entry.Command))
	if len(entry.Args) > 0 {
		fmt.Fprintf(&b, "args = %s\n", encodeTOMLStringArray(entry.Args))
	}
	if entry.Icon != "" {
		fmt.Fprintf(&b, "icon = %s\n", strconv.Quote(entry.Icon))
	}
	if entry.Label != "" {
		fmt.Fprintf(&b, "label = %s\n", strconv.Quote(entry.Label))
	}
	if entry.Position != "" {
		fmt.Fprintf(&b, "position = %s\n", strconv.Quote(entry.Position))
	}
	if entry.Cwd != "" {
		fmt.Fprintf(&b, "cwd = %s\n", strconv.Quote(entry.Cwd))
	}
	if len(entry.Env) > 0 {
		fmt.Fprintf(&b, "env = %s\n", encodeTOMLStringArray(entry.Env))
	}
	if entry.Restart {
		b.WriteString("restart = true\n")
	}
	if entry.Protocol != "" {
		fmt.Fprintf(&b, "protocol = %s\n", strconv.Quote(entry.Protocol))
	}
	if entry.ProtocolTimeout != "" {
		fmt.Fprintf(&b, "protocol_timeout = %s\n", strconv.Quote(entry.ProtocolTimeout))
	}
	return b.String()
}

func encodeTOMLStringArray(items []string) string {
	quoted := make([]string, 0, len(items))
	for _, item := range items {
		quoted = append(quoted, strconv.Quote(item))
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// declaresTOMLTable reports whether the document has a `[key]` header line.
func declaresTOMLTable(content, key string) bool {
	for _, line := range strings.Split(content, "\n") {
		if k, ok := tomlTableKey(line); ok && k == key {
			return true
		}
	}
	return false
}

// tomlTableRegion is the half-open line range of the table named key: its
// header through the line before the next table header, with trailing blank
// lines handed back to the document.
func tomlTableRegion(lines []string, key string) (start, end int) {
	start, end = -1, len(lines)
	for i, line := range lines {
		k, ok := tomlTableKey(line)
		if !ok {
			continue
		}
		if k == key {
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
		return -1, -1
	}
	for end > start+1 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return start, end
}

// setTOMLKeyInTable rewrites `key = ...` inside the named table, inserts it
// directly under the header when the table exists without it, or appends the
// whole table. Every other line survives verbatim — the same shape
// setMachineName has, generalized off [machine].
func setTOMLKeyInTable(content, table, key, encoded string) string {
	if strings.TrimSpace(content) == "" {
		return "[" + table + "]\n" + encoded + "\n"
	}
	lines := strings.Split(content, "\n")
	inTable := false
	header := -1
	for i, line := range lines {
		if k, ok := tomlTableKey(line); ok {
			// A nested table ([tui.plugins.x]) is not [tui], so a bare key
			// under it must not be mistaken for ours.
			inTable = k == table
			if inTable {
				header = i
			}
			continue
		}
		if !inTable {
			continue
		}
		if k, _, ok := strings.Cut(strings.TrimSpace(line), "="); ok && strings.TrimSpace(k) == key {
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			lines[i] = indent + encoded
			return strings.Join(lines, "\n")
		}
	}
	if header >= 0 {
		out := append([]string{}, lines[:header+1]...)
		out = append(out, encoded)
		out = append(out, lines[header+1:]...)
		return strings.Join(out, "\n")
	}
	sep := "\n"
	if strings.HasSuffix(content, "\n") {
		sep = ""
	}
	return content + sep + "\n[" + table + "]\n" + encoded + "\n"
}

// reparsePluginConfig re-reads what was just written. A surgical edit that
// produced invalid TOML has to fail here, at the edit, rather than at the next
// config load with nothing to connect it to.
func reparsePluginConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to re-read %s after writing it: %w", path, err)
	}
	var probe Config
	if err := toml.Unmarshal(data, &probe); err != nil {
		return fmt.Errorf("%s is not valid TOML after the edit: %w", path, err)
	}
	return nil
}

// pluginConfigFileMode keeps an existing file's permissions. os.WriteFile
// honors its mode only on create, so this matters solely for the file that did
// not exist — but a config written 0644 where the rest of the user's dotfiles
// are 0600 is a surprise worth not causing.
func pluginConfigFileMode(path string) os.FileMode {
	if info, err := os.Stat(path); err == nil {
		return info.Mode().Perm()
	}
	return 0o644
}

// SortedPluginNames is the deterministic tail PluginOrder leaves: every
// configured plugin the order does not name, sorted by config key. Exported
// because a UI that reorders `plugin_order` has to write a COMPLETE list — a
// partial one silently re-sorts everything it omits.
func SortedPluginNames(plugins map[string]*PluginConfig) []string {
	out := make([]string, 0, len(plugins))
	for name := range plugins {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
