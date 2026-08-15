package workspace

import (
	"fmt"
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
	matches := primariesByName(records, machine)[name]
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

// NotespaceNameRoutes answers ResolveNotespaceName for every display name at
// once: the map holds an entry exactly for the names that identify one recorded
// primary, and omits — rather than reports — the names ResolveNotespaceName
// would refuse (unstamped, not a recorded primary, ambiguous across roots, or
// carrying a duplicated id).
//
// It exists for callers that resolve MANY names against one machine state, like
// the daemon's watch-set pass over every discovered workspace. Calling
// ResolveNotespaceName in a loop is the same work per name; this is one
// validated read of the index and a map lookup per name afterwards.
func NotespaceNameRoutes(cfg *config.Config, machine *config.MachineConfig) (map[string]NotespaceResolution, error) {
	idx, records, err := configuredNotespaceIndex(cfg)
	if err != nil {
		return nil, err
	}
	byName := primariesByName(records, machine)
	routes := make(map[string]NotespaceResolution, len(byName))
	for name, matches := range byName {
		if name == "" || len(matches) != 1 {
			continue
		}
		// Same duplicate-id validation ResolveNotespaceName forces: two physical
		// roots claiming one identity is not a routable answer.
		if _, err := idx.ByID(matches[0].Stamp.ID); err != nil {
			continue
		}
		routes[name] = NotespaceResolution{
			Subject:     matches[0].Stamp.Subject,
			NotespaceID: matches[0].Stamp.ID,
			Root:        matches[0].Root,
		}
	}
	return routes, nil
}

// primariesByName groups the records this machine records as primary for their
// subject by display name, preserving the index's root order. A name with more
// than one record is ambiguous; whether that is an error or an omission is the
// caller's decision, so this returns the grouping rather than a choice.
func primariesByName(records []notespace.Record, machine *config.MachineConfig) map[string][]notespace.Record {
	var primaries map[string]string
	if machine != nil {
		primaries = machine.Primaries
	}
	byName := make(map[string][]notespace.Record, len(records))
	for _, record := range records {
		if primaries[record.Stamp.Subject] != record.Stamp.ID {
			continue
		}
		byName[record.Stamp.Name] = append(byName[record.Stamp.Name], record)
	}
	return byName
}

// resolvePrimary is the routing half of the resolution chain. The rule it
// applies — recorded entry, live stamp, single root, matching subject — lives
// in core/pkg/notespace as Index.PrimaryFor, so the resolver, the sibling verbs
// and doctor all answer "which notespace is primary" the same way.
func resolvePrimary(value string, cfg *config.Config, machine *config.MachineConfig) (NotespaceResolution, error) {
	var primaries map[string]string
	if machine != nil {
		primaries = machine.Primaries
	}
	idx, _, err := configuredNotespaceIndex(cfg)
	if err != nil {
		return NotespaceResolution{}, err
	}
	record, err := idx.PrimaryFor(value, primaries)
	if err != nil {
		return NotespaceResolution{}, err
	}
	return NotespaceResolution{Subject: value, NotespaceID: record.Stamp.ID, Root: record.Root}, nil
}

// configuredNotespaceIndex indexes every stamped notespace under the notebook
// roots recorded config points at. The result is memoized per root set and
// re-validated against the filesystem on every call — see
// notespace_index_cache.go for what a hit costs and what invalidates one.
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
	notebookRoots := make([]string, 0, len(rootSet))
	for notebookRoot := range rootSet {
		notebookRoots = append(notebookRoots, notebookRoot)
	}
	sort.Strings(notebookRoots)

	cached, _ := notespaceIndexCache.LoadOrStore(notespaceIndexKey(notebookRoots), &notespaceIndexEntry{})
	entry := cached.(*notespaceIndexEntry)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.fresh() {
		return entry.idx, entry.records, nil
	}
	idx, records, inputs, err := buildNotespaceIndex(notebookRoots)
	if err != nil {
		// Failures are never cached, and a failed rebuild invalidates whatever
		// the entry held: the disk has moved under it either way.
		entry.valid, entry.idx, entry.records, entry.inputs = false, nil, nil, nil
		return nil, nil, err
	}
	entry.idx, entry.records, entry.inputs, entry.valid = idx, records, inputs, true
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
