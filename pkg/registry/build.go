package registry

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/grovetools/core/config"
)

// BuildInput is everything the note writer observes about this machine.
// Assembled by the caller (the daemon) so this package stays free of global
// state and the whole render is testable from fixtures.
type BuildInput struct {
	// MachineID is the state-held ULID. Required — a note keyed on nothing is
	// not a note.
	MachineID string
	// Name is the config-held display name.
	Name string
	// OriginID is sync.db's install id at write time.
	OriginID string
	// GrovedVersion is the daemon build writing the note.
	GrovedVersion string
	// Machine is the declared intent (machine.toml). Nil means "nothing
	// declared", which renders as empty ecosystems/roots rather than an error:
	// a machine with no subscriptions still has a presence.
	Machine *config.MachineConfig
	// Subscriptions is this machine's sync subscription list.
	Subscriptions []config.SyncWorkspace
}

// Build assembles a note from observed state. It sets neither Rev nor
// LastSeen: those are the writer's business, because their whole purpose is to
// change only when the REST of the note changed (or a day rolled over), and
// that comparison needs the rest of the note first.
//
// Everything here is derived — intent from config, state from the filesystem —
// so two calls with an unchanged machine produce an identical Note and
// therefore identical bytes. That is the property the write-suppression rests
// on, and the reason nothing time-varying (a scan timestamp, a duration, a
// process id) may ever enter this function.
func Build(in BuildInput) *Note {
	n := &Note{
		MachineID:     in.MachineID,
		Name:          in.Name,
		OriginID:      in.OriginID,
		GrovedVersion: in.GrovedVersion,
	}

	// Ecosystems: declared intent reconciled against the disk, each carrying a
	// copy of the ecosystem's own card. The card is embedded per machine
	// rather than shared as one ecosystems/<name>.md note precisely because a
	// shared note would have several writers, which the single-writer rule —
	// the thing that makes registry conflicts impossible — forbids.
	roots := map[string]string{}
	for _, state := range config.ReconcileMachineEcosystems(in.Machine) {
		eco := NoteEcosystem{
			Name:     state.Name,
			Path:     state.Path,
			Notebook: state.Notebook,
			State:    state.State,
			Enabled:  state.Enabled,
			Repos:    append([]string(nil), state.Repos...),
			Exclude:  append([]string(nil), state.Exclude...),
		}
		if state.Manifest != "" {
			if card, err := config.LoadEcosystemCard(state.Manifest); err == nil && card != nil {
				eco.Card = convertCard(card)
			}
		}
		n.Ecosystems = append(n.Ecosystems, eco)
		if state.State == StatePresent || state.State == StateUnmanifested {
			roots[state.Name] = state.Path
		}
	}

	// Bare roots: first-class, and deliberately never reconciled as
	// "declared-missing" — nothing can materialize ~/code/chickens, so an
	// absent one is reported (Exists) but is not an action item.
	if in.Machine != nil {
		for _, name := range sortedRootNames(in.Machine.Machine.Roots) {
			r := in.Machine.Machine.Roots[name]
			path := expandRootPath(r.Path)
			exists := false
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				exists = true
				roots[name] = path
			}
			n.Roots = append(n.Roots, NoteRoot{
				Name:     name,
				Path:     path,
				Notebook: r.Notebook,
				Enabled:  r.Enabled == nil || *r.Enabled,
				Exists:   exists,
			})
		}
	}

	// Subscriptions, minus everything secret. The sync config also holds a
	// static token and a token_command; neither may enter a document that
	// replicates to every device — and the push path would quarantine the note
	// on the secret heuristics if one did, which would take the machine's
	// whole presence dark rather than leak.
	for _, sub := range in.Subscriptions {
		n.Subscriptions = append(n.Subscriptions, NoteSubscription{
			Name: sub.Name,
			Role: sub.Role,
			Mode: sub.Mode,
			Pull: sub.Pull,
		})
	}

	n.Repos = CollectRepoTips(roots)
	return n
}

// convertCard flattens config.EcosystemCard's notebook MAP into a name-sorted
// slice. Map iteration order is random in Go, and a note whose bytes changed
// on every write would defeat the write suppression entirely.
func convertCard(card *config.EcosystemCard) *NoteCard {
	out := &NoteCard{ID: card.ID, Layout: card.Layout}
	for _, r := range card.Remotes {
		out.Remotes = append(out.Remotes, NoteRemote{Name: r.Name, URL: r.URL})
	}
	for _, name := range sortedNotebookNames(card.Notebooks) {
		nb := card.Notebooks[name]
		out.Notebooks = append(out.Notebooks, NoteCardNotebook{
			Name: name, Default: nb.Default, Audience: nb.Audience,
		})
	}
	return out
}

// expandRootPath resolves $VARs and a leading ~/ and makes the path absolute,
// mirroring config's own expandPath (which ReconcileMachineEcosystems applies
// to ecosystems) so both kinds of root render the same way.
func expandRootPath(path string) string {
	expanded := os.ExpandEnv(path)
	if strings.HasPrefix(expanded, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			expanded = filepath.Join(home, expanded[2:])
		}
	}
	if abs, err := filepath.Abs(expanded); err == nil {
		return abs
	}
	return expanded
}

func sortedRootNames(m map[string]config.MachineRoot) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedNotebookNames(m map[string]config.EcosystemNotebook) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
