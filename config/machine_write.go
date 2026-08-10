package config

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
)

// WriteMachineName sets [machine] name in machine.toml at the given path,
// creating the file when absent.
//
// The edit is surgical, not a re-render: an existing file keeps every other
// byte — comments, key order, tables this minimal schema does not yet know
// about (the subscriptions and bare roots the machine-config phase adds).
// That matters because machine.toml is dotfiles-portable and hand-authored;
// round-tripping it through a marshaller would silently eat the parts of the
// user's intent this phase cannot model yet.
//
// It reports whether the file changed.
func WriteMachineName(path, name string) (bool, error) {
	if path == "" {
		return false, fmt.Errorf("machine config path is not resolvable")
	}
	if name == "" {
		return false, fmt.Errorf("machine name must not be empty")
	}
	probe := MachineConfig{Machine: MachineSettings{Name: name}}
	if err := probe.Validate(); err != nil {
		return false, err
	}

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("failed to read machine config %s: %w", path, err)
	}

	updated := setMachineName(string(existing), name)
	if err == nil && updated == string(existing) {
		return false, nil
	}

	verify := func(candidate string) error {
		_, err := ParseMachineConfigContent(path, candidate)
		return err
	}
	if err := atomicWriteVerified(path, updated, verify); err != nil {
		return false, err
	}
	return true, nil
}

// MachineSubscriptions is a batch of machine.toml entries to write. Both maps
// are keyed by subscription name; an entry replaces the table of the same name
// and leaves every other byte of the file alone.
type MachineSubscriptions struct {
	Ecosystems map[string]MachineEcosystem
	Roots      map[string]MachineRoot
	// Header is an optional comment block written above the first table this
	// call appends to a file that does not have one yet. It exists so
	// `grove machine migrate` can explain, in the file, where its content came
	// from. Lines are written verbatim; callers include their own `#`.
	Header []string
}

// WriteMachineSubscriptions upserts ecosystem subscriptions and bare roots
// into machine.toml, creating the file when absent. It reports whether the
// file changed.
//
// Like WriteMachineName, the edit is surgical: only the tables named here are
// replaced. Comments, key order, the [machine] name, and tables this schema
// does not model survive byte-for-byte — machine.toml is hand-authored and
// dotfiles-portable, so a marshaller round-trip would quietly rewrite the
// user's file.
func WriteMachineSubscriptions(path string, subs MachineSubscriptions) (bool, error) {
	if path == "" {
		return false, fmt.Errorf("machine config path is not resolvable")
	}
	if len(subs.Ecosystems) == 0 && len(subs.Roots) == 0 {
		return false, nil
	}
	// Validate the batch before touching the file: a rejected entry must not
	// leave a half-written config behind.
	probe := MachineConfig{Machine: MachineSettings{Ecosystems: subs.Ecosystems, Roots: subs.Roots}}
	if err := probe.Validate(); err != nil {
		return false, err
	}

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("failed to read machine config %s: %w", path, err)
	}

	updated := string(existing)
	if len(subs.Header) > 0 && strings.TrimSpace(updated) == "" {
		updated = strings.Join(subs.Header, "\n") + "\n"
	}
	for _, name := range sortedKeys(subs.Ecosystems) {
		updated = setTOMLTableParts(updated, []string{"machine", "ecosystems", name}, renderMachineEcosystem(name, subs.Ecosystems[name]))
	}
	for _, name := range sortedKeys(subs.Roots) {
		updated = setTOMLTableParts(updated, []string{"machine", "roots", name}, renderMachineRoot(name, subs.Roots[name]))
	}

	if err == nil && updated == string(existing) {
		return false, nil
	}

	verify := func(candidate string) error {
		_, err := ParseMachineConfigContent(path, candidate)
		return err
	}
	if err := atomicWriteVerified(path, updated, verify); err != nil {
		return false, err
	}
	return true, nil
}

func renderMachineEcosystem(name string, eco MachineEcosystem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[machine.ecosystems.%s]\n", tomlKey(name))
	fmt.Fprintf(&b, "path = %s\n", strconv.Quote(eco.Path))
	if eco.Notebook != "" {
		fmt.Fprintf(&b, "notebook = %s\n", strconv.Quote(eco.Notebook))
	}
	if eco.Description != "" {
		fmt.Fprintf(&b, "description = %s\n", strconv.Quote(eco.Description))
	}
	if len(eco.Repos) > 0 {
		fmt.Fprintf(&b, "repos = %s\n", renderTOMLStringArray(eco.Repos))
	}
	if len(eco.Exclude) > 0 {
		fmt.Fprintf(&b, "exclude = %s\n", renderTOMLStringArray(eco.Exclude))
	}
	if eco.Enabled != nil {
		fmt.Fprintf(&b, "enabled = %t\n", *eco.Enabled)
	}
	return b.String()
}

func renderTOMLStringArray(values []string) string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = strconv.Quote(value)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func renderMachineRoot(name string, root MachineRoot) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[machine.roots.%s]\n", tomlKey(name))
	fmt.Fprintf(&b, "path = %s\n", strconv.Quote(root.Path))
	if root.Notebook != "" {
		fmt.Fprintf(&b, "notebook = %s\n", strconv.Quote(root.Notebook))
	}
	if root.Description != "" {
		fmt.Fprintf(&b, "description = %s\n", strconv.Quote(root.Description))
	}
	if root.Enabled != nil {
		fmt.Fprintf(&b, "enabled = %t\n", *root.Enabled)
	}
	return b.String()
}

// setTOMLTable replaces the table named by key (header line through the line
// before the next table header) with block, or appends block when the document
// declares no such table. Every line outside that region is untouched — the
// same surgical discipline setTOMLEcosystemCard uses, generalized to one
// table.
func setTOMLTable(content, key, block string) string {
	return setTOMLTableParts(content, strings.Split(key, "."), block)
}

// setTOMLTableParts is the segment-aware form used when a table name may
// itself contain dots. Comparing parsed segments keeps [roots."work.notes"]
// distinct from [roots.work.notes].
func setTOMLTableParts(content string, key []string, block string) string {
	lines := strings.Split(content, "\n")
	start, end := -1, len(lines)
	for i, line := range lines {
		parts, ok := tomlTableKeyParts(line)
		if !ok {
			continue
		}
		if slices.Equal(parts, key) {
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
	// belong to the document, not the table.
	for end > start+1 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}

	out := make([]string, 0, len(lines)+8)
	out = append(out, lines[:start]...)
	out = append(out, strings.Split(strings.TrimSuffix(block, "\n"), "\n")...)
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n")
}

// setMachineName returns content with [machine] name = "<name>" in place:
// rewriting an existing name key inside the [machine] table, inserting one at
// the top of that table, or appending a whole [machine] table when none
// exists. Every other line is preserved verbatim.
func setMachineName(content, name string) string {
	encoded := fmt.Sprintf("name = %q", name)

	if strings.TrimSpace(content) == "" {
		return "[machine]\n" + encoded + "\n"
	}

	lines := strings.Split(content, "\n")
	inMachine := false
	machineHeader := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			// A new table header ends the [machine] table. Any nested table
			// ([machine.ecosystems.x]) is NOT the [machine] table itself, so
			// a bare `name` under it must not be mistaken for ours.
			inMachine = trimmed == "[machine]"
			if inMachine {
				machineHeader = i
			}
			continue
		}
		if !inMachine {
			continue
		}
		if key, _, ok := strings.Cut(trimmed, "="); ok && strings.TrimSpace(key) == "name" {
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			lines[i] = indent + encoded
			return strings.Join(lines, "\n")
		}
	}

	if machineHeader >= 0 {
		// [machine] exists but declares no name — insert directly beneath the
		// header so the key stays inside its own table.
		out := append([]string{}, lines[:machineHeader+1]...)
		out = append(out, encoded)
		out = append(out, lines[machineHeader+1:]...)
		return strings.Join(out, "\n")
	}

	sep := "\n"
	if strings.HasSuffix(content, "\n") {
		sep = ""
	}
	return content + sep + "\n[machine]\n" + encoded + "\n"
}
