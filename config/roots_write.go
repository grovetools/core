package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/grovetools/core/pkg/coderoot"
)

// The shared writer for the recorded config files: roots.toml,
// notebooks.toml, machine.toml. One discipline for all three:
//
//  1. surgical TOML edit — only the named tables/keys change; comments, key
//     order and unknown tables survive byte-for-byte (machine_write.go's
//     setTOMLTable pattern);
//  2. re-parse the full recorded set — the candidate content is parsed AND
//     cross-validated against its sibling file, so a state that does not
//     reload is never persisted;
//  3. atomic rename — the candidate is written to a temp file in the same
//     directory and renamed over the target.
//
// All verbs and the TUI go through these functions. sync_edit.go's
// additive-merge writer is deliberately NOT extended to these files: it
// cannot rewrite entries by design, and these files need upsert/delete.

// CodeRootEdits is a batch of roots.toml changes.
type CodeRootEdits struct {
	Upserts map[string]coderoot.Root
	Deletes []string
	// Header is an optional comment block written at the top of a file this
	// call creates. Lines are verbatim; callers include their own '#'.
	Header []string
}

// NotebookEdits is a batch of notebooks.toml changes.
type NotebookEdits struct {
	// Default, when non-nil, rewrites the top-level `default =` key.
	Default *string
	Upserts map[string]coderoot.Notebook
	Deletes []string
	Header  []string
	// SyncShare rewrites [notebooks.<name>.sync] share for the named
	// notebooks. It is a separate field from Upserts on purpose: share state
	// is edited by different verbs (`notebook share`/`unshare`) than the root
	// binding, and an Upsert that did not mention sync must never be read as
	// "unshare it". Recording false is meaningful — see coderoot.NotebookSync.
	SyncShare map[string]bool
}

// WriteCodeRoots applies edits to roots.toml at path, creating the file when
// absent. It reports whether the file changed. The candidate content is
// re-parsed and cross-validated against the sibling notebooks.toml before
// anything is persisted.
func WriteCodeRoots(path string, edits CodeRootEdits) (bool, error) {
	if path == "" {
		return false, fmt.Errorf("roots.toml path is not resolvable")
	}
	if len(edits.Upserts) == 0 && len(edits.Deletes) == 0 {
		return false, nil
	}
	if err := reviewConfigWritePath(path); err != nil {
		return false, err
	}

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("failed to read %s: %w", path, err)
	}

	updated := string(existing)
	if len(edits.Header) > 0 && strings.TrimSpace(updated) == "" {
		updated = strings.Join(edits.Header, "\n") + "\n"
	}
	for _, name := range sortedKeys(edits.Upserts) {
		updated = setTOMLTableParts(updated, []string{"roots", name}, renderCodeRoot(name, edits.Upserts[name]))
	}
	for _, name := range edits.Deletes {
		updated = deleteTOMLTableParts(updated, []string{"roots", name})
	}

	if err == nil && updated == string(existing) {
		return false, nil
	}

	verify := func(candidate string) error {
		rf, perr := coderoot.ParseRoots(path, []byte(candidate))
		if perr != nil {
			return perr
		}
		return crossValidateRecorded(path, rf.Roots, nil, filepath.Join(filepath.Dir(path), coderoot.NotebooksFileName), true)
	}
	return true, atomicWriteVerified(path, updated, verify)
}

// WriteNotebooks applies edits to notebooks.toml at path, creating the file
// when absent. It reports whether the file changed. The candidate content is
// re-parsed and cross-validated against the sibling roots.toml before
// anything is persisted.
func WriteNotebooks(path string, edits NotebookEdits) (bool, error) {
	if path == "" {
		return false, fmt.Errorf("notebooks.toml path is not resolvable")
	}
	if edits.Default == nil && len(edits.Upserts) == 0 && len(edits.Deletes) == 0 && len(edits.SyncShare) == 0 {
		return false, nil
	}
	if err := reviewConfigWritePath(path); err != nil {
		return false, err
	}

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Errorf("failed to read %s: %w", path, err)
	}

	updated := string(existing)
	if len(edits.Header) > 0 && strings.TrimSpace(updated) == "" {
		updated = strings.Join(edits.Header, "\n") + "\n"
	}
	if edits.Default != nil {
		updated = setTOMLTopLevelKey(updated, "default", strconv.Quote(*edits.Default))
	}
	// Upserts run before sync edits so a notebook created in the same call
	// already has its parent table when its [.sync] child is appended.
	//
	// An upsert rewrites the lines from [notebooks.<n>] to the next table
	// header, and [notebooks.<n>.sync] IS a table header — so a recorded share
	// state survives an unrelated root rewrite untouched. That is the whole
	// preservation contract; the test that pins it is TestWriteNotebooks
	// PreservesRecordedSyncTable.
	for _, name := range sortedKeys(edits.Upserts) {
		updated = setTOMLTableParts(updated, []string{"notebooks", name}, renderNotebookDef(name, edits.Upserts[name]))
	}
	for _, name := range sortedKeys(edits.SyncShare) {
		updated = setTOMLChildTable(updated,
			[]string{"notebooks", name},
			[]string{"notebooks", name, "sync"},
			renderNotebookSync(name, edits.SyncShare[name]))
	}
	for _, name := range edits.Deletes {
		// The child goes first: a [notebooks.<n>.sync] left behind by itself
		// would reload as a notebook with no root, and the writer's contract
		// is that what it persists reloads.
		updated = deleteTOMLTableParts(updated, []string{"notebooks", name, "sync"})
		updated = deleteTOMLTableParts(updated, []string{"notebooks", name})
	}

	if err == nil && updated == string(existing) {
		return false, nil
	}

	verify := func(candidate string) error {
		nf, perr := coderoot.ParseNotebooks(path, []byte(candidate))
		if perr != nil {
			return perr
		}
		return crossValidateRecorded(filepath.Join(filepath.Dir(path), coderoot.RootsFileName), nil, &nf, path, false)
	}
	return true, atomicWriteVerified(path, updated, verify)
}

// crossValidateRecorded builds the full recorded table from one candidate
// side plus the sibling file on disk, and validates the pair. candidateRoots
// XOR candidateNotebooks is set; siblingPath names the counterpart file.
func crossValidateRecorded(rootsPath string, candidateRoots map[string]coderoot.Root, candidateNotebooks *coderoot.NotebooksFile, siblingIsNotebooksAt string, candidateIsRoots bool) error {
	table := coderoot.Table{
		Roots:     map[string]coderoot.Root{},
		Notebooks: map[string]coderoot.Notebook{},
	}
	if candidateIsRoots {
		table.Roots = candidateRoots
		table.RootsFilePath = rootsPath
		data, err := os.ReadFile(siblingIsNotebooksAt)
		if err == nil {
			nf, perr := coderoot.ParseNotebooks(siblingIsNotebooksAt, data)
			if perr != nil {
				return fmt.Errorf("sibling %s does not reload: %w", siblingIsNotebooksAt, perr)
			}
			table.Notebooks = nf.Notebooks
			table.Default = nf.Default
			table.NotebooksFilePath = siblingIsNotebooksAt
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("failed to read sibling %s: %w", siblingIsNotebooksAt, err)
		}
	} else {
		table.Notebooks = candidateNotebooks.Notebooks
		table.Default = candidateNotebooks.Default
		table.NotebooksFilePath = siblingIsNotebooksAt
		data, err := os.ReadFile(rootsPath)
		if err == nil {
			rf, perr := coderoot.ParseRoots(rootsPath, data)
			if perr != nil {
				return fmt.Errorf("sibling %s does not reload: %w", rootsPath, perr)
			}
			table.Roots = rf.Roots
			table.RootsFilePath = rootsPath
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("failed to read sibling %s: %w", rootsPath, err)
		}
	}
	return table.Validate()
}

// atomicWriteVerified verifies candidate content, then writes it to a temp
// file in the target's directory and renames it into place. Refusing to
// persist a state that does not reload is the writer's whole contract.
//
// The rename lands on the reviewed target, not on the caller's logical path:
// when the logical path is a dotfiles symlink, the link survives and its
// resolved file is rewritten (see ResolveConfigWriteTarget). Staging inside the
// resolved file's own directory also keeps the rename on one filesystem, which
// a dotfiles checkout on a separate volume otherwise would not be.
func atomicWriteVerified(path, content string, verify func(string) error) error {
	// Reviewing the destination comes first: an unreviewable link is refused on
	// its own terms, not behind a content complaint that would send the operator
	// looking at the wrong problem.
	target, viaSymlink, err := ResolveConfigWriteTarget(path)
	if err != nil {
		return err
	}
	label := writeTargetLabel(path, target, viaSymlink)
	if verify != nil {
		if err := verify(content); err != nil {
			return fmt.Errorf("refusing to write %s: result would not reload: %w", path, err)
		}
	}
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory for %s: %w", label, err)
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(target)+".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to stage write of %s: %w", label, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("failed to stage write of %s: %w", label, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("failed to stage write of %s: %w", label, err)
	}
	// A file reached through a link belongs to the dotfiles checkout, which
	// chose its own permissions; a file written in place keeps this writer's
	// long-standing 0644.
	mode := os.FileMode(0o644)
	if viaSymlink {
		if info, statErr := os.Stat(target); statErr == nil {
			mode = info.Mode().Perm()
		}
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("failed to stage write of %s: %w", label, err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("failed to write %s: %w", label, err)
	}
	return nil
}

func renderCodeRoot(name string, r coderoot.Root) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[roots.%s]\n", tomlKey(name))
	fmt.Fprintf(&b, "path = %s\n", strconv.Quote(r.Path))
	if r.Scan {
		fmt.Fprintf(&b, "scan = true\n")
	}
	if r.Notebook != "" {
		fmt.Fprintf(&b, "notebook = %s\n", strconv.Quote(r.Notebook))
	}
	if len(r.Repos) > 0 {
		fmt.Fprintf(&b, "repos = %s\n", renderTOMLStringArray(r.Repos))
	}
	if len(r.Exclude) > 0 {
		fmt.Fprintf(&b, "exclude = %s\n", renderTOMLStringArray(r.Exclude))
	}
	if r.Depth != nil {
		fmt.Fprintf(&b, "depth = %d\n", *r.Depth)
	}
	if r.Enabled != nil {
		fmt.Fprintf(&b, "enabled = %t\n", *r.Enabled)
	}
	if r.Description != "" {
		fmt.Fprintf(&b, "description = %s\n", strconv.Quote(r.Description))
	}
	return b.String()
}

// renderNotebookDef renders [notebooks.<name>] alone. The sync child table is
// deliberately not rendered here: it is edited through NotebookEdits.SyncShare
// and preserved verbatim by every other write.
func renderNotebookDef(name string, nb coderoot.Notebook) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[notebooks.%s]\n", tomlKey(name))
	fmt.Fprintf(&b, "root = %s\n", strconv.Quote(nb.Root))
	return b.String()
}

func renderNotebookSync(name string, share bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[notebooks.%s.sync]\n", tomlKey(name))
	fmt.Fprintf(&b, "share = %t\n", share)
	return b.String()
}

// setTOMLChildTable sets a child table, and — when the child is absent —
// inserts it immediately after its parent's block instead of at end of file.
// Plain appending would be valid TOML but would strand [notebooks.b.sync]
// below [notebooks.z], and this is a file operators read and hand-edit.
func setTOMLChildTable(content string, parent, child []string, block string) string {
	if _, _, found := tomlTableBounds(content, child); found {
		return setTOMLTableParts(content, child, block)
	}
	_, parentEnd, parentFound := tomlTableBounds(content, parent)
	if !parentFound {
		return setTOMLTableParts(content, child, block)
	}
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines)+4)
	out = append(out, lines[:parentEnd]...)
	out = append(out, strings.Split(strings.TrimSuffix(block, "\n"), "\n")...)
	if parentEnd < len(lines) {
		out = append(out, "")
	}
	out = append(out, lines[parentEnd:]...)
	return strings.Join(out, "\n")
}

// tomlTableBounds locates the table named by key: start is its header line,
// end is one past its last content line (trailing blank lines excluded, since
// they separate it from what follows and belong to the document).
func tomlTableBounds(content string, key []string) (int, int, bool) {
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
		return 0, 0, false
	}
	for end > start+1 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return start, end, true
}

// deleteTOMLTable removes the table named by key (header line through the
// line before the next table header), leaving every other line untouched.
func deleteTOMLTable(content, key string) string {
	return deleteTOMLTableParts(content, strings.Split(key, "."))
}

func deleteTOMLTableParts(content string, key []string) string {
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
		return content
	}
	// Trailing blank lines between the removed table and the next belong to
	// the document; keep exactly one separator by trimming the tail.
	for end > start+1 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	out := make([]string, 0, len(lines))
	out = append(out, lines[:start]...)
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n")
}

// setTOMLTopLevelKey rewrites (or inserts) a top-level scalar key. Insertion
// lands before the first table header so the key stays top-level.
func setTOMLTopLevelKey(content, key, encodedValue string) string {
	encoded := key + " = " + encodedValue
	if strings.TrimSpace(content) == "" {
		return encoded + "\n"
	}
	lines := strings.Split(content, "\n")
	firstTable := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			firstTable = i
			break
		}
		if k, _, ok := strings.Cut(trimmed, "="); ok && strings.TrimSpace(k) == key {
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			lines[i] = indent + encoded
			return strings.Join(lines, "\n")
		}
	}
	if firstTable < 0 {
		sep := "\n"
		if strings.HasSuffix(content, "\n") {
			sep = ""
		}
		return content + sep + encoded + "\n"
	}
	out := make([]string, 0, len(lines)+2)
	out = append(out, lines[:firstTable]...)
	out = append(out, encoded, "")
	out = append(out, lines[firstTable:]...)
	return strings.Join(out, "\n")
}
