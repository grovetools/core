package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/notespace"
	"github.com/grovetools/core/pkg/subject"
)

// NotespaceResolution is the complete routing answer. Name is intentionally
// absent: display names are never routing keys.
type NotespaceResolution struct {
	Subject     string
	NotespaceID string
	Root        string
}

// ResolveNotespace resolves a code path through one fail-closed chain:
// worktree -> base repository, repository -> recorded/local or canonical
// remote subject, subject -> recorded primary id, and id -> stamped root.
// It never chooses a sibling by name, sort order, or directory containment.
func ResolveNotespace(pathOrNode string, cfg *config.Config, machine *config.MachineConfig) (NotespaceResolution, error) {
	owner, err := BaseRepositoryOwner(pathOrNode)
	if err != nil {
		return NotespaceResolution{}, err
	}
	value, err := subjectForRepository(owner, machine)
	if err != nil {
		return NotespaceResolution{}, err
	}
	return resolvePrimary(value, cfg, machine)
}

// ResolveNotespaceName resolves a notes-plane display alias. It first reads
// stamps, then accepts the name only when it identifies exactly one recorded
// primary. This is deliberately separate from code-plane Provider.FindByName:
// a notespace name may differ from both its immutable id and repository name.
func ResolveNotespaceName(name string, cfg *config.Config, machine *config.MachineConfig) (NotespaceResolution, error) {
	if strings.TrimSpace(name) == "" {
		return NotespaceResolution{}, fmt.Errorf("notespace name is empty")
	}
	idx, records, err := configuredNotespaceIndex(cfg)
	if err != nil {
		return NotespaceResolution{}, err
	}
	var matches []notespace.Record
	for _, record := range records {
		if record.Stamp.Name != name {
			continue
		}
		primary := ""
		if machine != nil {
			primary = machine.Primaries[record.Stamp.Subject]
		}
		if primary == record.Stamp.ID {
			matches = append(matches, record)
		}
	}
	if len(matches) == 0 {
		return NotespaceResolution{}, fmt.Errorf("notespace %q is not a recorded primary", name)
	}
	if len(matches) > 1 {
		return NotespaceResolution{}, fmt.Errorf("notespace display name %q is ambiguous across %d primary roots", name, len(matches))
	}
	// Force duplicate-id validation even when the matching record happened to
	// be encountered first.
	if _, err := idx.ByID(matches[0].Stamp.ID); err != nil {
		return NotespaceResolution{}, err
	}
	return NotespaceResolution{Subject: matches[0].Stamp.Subject, NotespaceID: matches[0].Stamp.ID, Root: matches[0].Root}, nil
}

func resolvePrimary(value string, cfg *config.Config, machine *config.MachineConfig) (NotespaceResolution, error) {
	if machine == nil || machine.Primaries == nil || machine.Primaries[value] == "" {
		return NotespaceResolution{}, fmt.Errorf("subject %q has no recorded primary notespace", value)
	}
	id := machine.Primaries[value]
	idx, _, err := configuredNotespaceIndex(cfg)
	if err != nil {
		return NotespaceResolution{}, err
	}
	records, err := idx.ByID(id)
	if err != nil {
		return NotespaceResolution{}, err
	}
	if len(records) == 0 {
		return NotespaceResolution{}, fmt.Errorf("primary notespace id %q for subject %q has no stamped root", id, value)
	}
	if records[0].Stamp.Subject != value {
		return NotespaceResolution{}, fmt.Errorf("primary notespace id %q is stamped for subject %q, not %q", id, records[0].Stamp.Subject, value)
	}
	return NotespaceResolution{Subject: value, NotespaceID: id, Root: records[0].Root}, nil
}

func configuredNotespaceIndex(cfg *config.Config) (*notespace.Index, []notespace.Record, error) {
	rootSet := map[string]bool{}
	if cfg != nil {
		for _, grove := range cfg.Groves {
			if grove.NotebookRoot != "" {
				rootSet[grove.NotebookRoot] = true
			}
		}
		if cfg.Notebooks != nil {
			for _, definition := range cfg.Notebooks.Definitions {
				if definition != nil && definition.RootDir != "" {
					rootSet[definition.RootDir] = true
				}
			}
		}
	}
	roots := make([]string, 0, len(rootSet))
	for notebookRoot := range rootSet {
		expanded, err := expandCentralizedRoot(notebookRoot)
		if err != nil {
			return nil, nil, err
		}
		entries, err := os.ReadDir(filepath.Join(expanded, NotespaceDirectory))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, nil, fmt.Errorf("read notespaces in %s: %w", expanded, err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				roots = append(roots, filepath.Join(expanded, NotespaceDirectory, entry.Name()))
			}
		}
	}
	sort.Strings(roots)
	idx, err := notespace.BuildIndex(roots)
	if err != nil {
		return nil, nil, err
	}
	var records []notespace.Record
	for _, root := range roots {
		stamp, err := notespace.LoadNotespace(root)
		if err != nil {
			return nil, nil, err
		}
		if stamp != nil {
			records = append(records, notespace.Record{Root: root, Stamp: *stamp})
		}
	}
	return idx, records, nil
}

func subjectForRepository(root string, machine *config.MachineConfig) (string, error) {
	root = filepath.Clean(root)
	if machine != nil && machine.Subjects != nil {
		if value := machine.Subjects[root]; value != "" {
			return value, nil
		}
	}
	remotes, err := gitRemotesIn(root)
	if err != nil {
		return "", err
	}
	value, selection, err := subject.FromRemotes(remotes)
	if err != nil {
		return "", fmt.Errorf("select subject for %s: %w", root, err)
	}
	if selection == subject.SelectionNone {
		return "", fmt.Errorf("repository %s has no remote and no recorded local subject", root)
	}
	return value.String(), nil
}
