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
// byte — comments, key order, and all notes-plane routing tables. That matters
// because machine.toml is dotfiles-portable and hand-authored;
// round-tripping it through a marshaller would silently eat the parts of the
// user's intent this phase cannot model yet.
//
// It reports whether the file changed.
func WriteMachineName(path, name string) (bool, error) {
	if path == "" {
		return false, fmt.Errorf("machine config path is not resolvable")
	}
	unlock, err := lockMachineFile(path)
	if err != nil {
		return false, err
	}
	defer unlock()
	if name == "" {
		return false, fmt.Errorf("machine name must not be empty")
	}
	probe := MachineConfig{Machine: MachineSettings{Name: name}}
	if err := probe.Validate(); err != nil {
		return false, err
	}
	if err := reviewConfigWritePath(path); err != nil {
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

func renderTOMLStringArray(values []string) string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = strconv.Quote(value)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
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

	// Blank lines trailing the old table — and the comment block introducing
	// the next one — separate it from what follows; they belong to the
	// document, not the table. Rewriting a root must not delete the comment an
	// operator wrote above the NEXT table.
	end = trimTableTail(lines, start, end)

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
