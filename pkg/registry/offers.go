package registry

import (
	"sort"
	"strings"

	"github.com/grovetools/core/config"
)

// The registry's read side for MATERIALIZATION: turning the ecosystem cards
// other machines embedded in their presence notes into something a peer can
// act on.
//
// There is deliberately no shared `ecosystems/<name>.md` note — that would be a
// multi-writer document, which the single-writer rule forbids. The card is
// canonical in the ecosystem's own repo and travels, per machine, inside that
// machine's note. Several machines therefore publish copies of the SAME card,
// and readers dedup here, at render time.

// Offer is one materializable ecosystem as assembled from the registry: the
// card itself plus who published it and where they keep it.
//
// It is evidence, not authority. Until device principals land, any token can
// write any machine's note (see the interim trust model), so an Offer is what
// the confirmation gate SHOWS the user before anything is cloned — the point
// of the gate is that a human reads the remotes.
type Offer struct {
	// Name is the ecosystem's subscription name (the key a machine declares
	// under [machine.ecosystems.<name>]).
	Name string
	// Card is the embedded card, converted back into the canonical config
	// type so callers share one shape with the on-disk manifest.
	Card config.EcosystemCard
	// Publishers labels every machine whose note carried this card, in the
	// order the notes sorted. More than one is the normal case and is what
	// dedup collapses.
	Publishers []string
	// Paths are the paths those machines keep the ecosystem at, deduped. They
	// are advisory — a path on another host is a hint for this host's default,
	// never a requirement.
	Paths []string
	// Conflicting is true when two machines published cards for this name that
	// do not agree on identity (different ids) or on how to clone it (layout
	// or remotes). The offer then carries the FIRST card seen, and the caller
	// must surface the disagreement rather than silently picking one.
	Conflicting bool
}

// PrimaryRemote returns the remote a peer clones from: the one named "origin"
// when present, otherwise the first declared. It mirrors the clone engine's
// own preference so the confirmation gate shows what will actually be used.
func (o Offer) PrimaryRemote() (config.EcosystemRemote, bool) {
	if len(o.Card.Remotes) == 0 {
		return config.EcosystemRemote{}, false
	}
	for _, r := range o.Card.Remotes {
		if r.Name == "origin" {
			return r, true
		}
	}
	return o.Card.Remotes[0], true
}

// Offers collects every ecosystem card embedded in the given machine notes,
// deduped by subscription name.
//
// Notes that failed to parse contribute nothing; a suspect note contributes
// normally, because suspicion is already rendered beside it and dropping the
// card would hide the very row worth inspecting. Ordering is by name so a
// listing is stable across runs.
func Offers(machines []Machine) []Offer {
	byName := make(map[string]*Offer)
	for _, m := range machines {
		if m.Note == nil {
			continue
		}
		label := m.Label()
		for _, eco := range m.Note.Ecosystems {
			if eco.Card == nil || strings.TrimSpace(eco.Name) == "" {
				continue
			}
			card := eco.Card.EcosystemCard()
			existing, ok := byName[eco.Name]
			if !ok {
				byName[eco.Name] = &Offer{
					Name:       eco.Name,
					Card:       card,
					Publishers: []string{label},
					Paths:      nonEmpty(eco.Path),
				}
				continue
			}
			existing.Publishers = appendUnique(existing.Publishers, label)
			existing.Paths = appendUnique(existing.Paths, eco.Path)
			if !sameCard(existing.Card, card) {
				existing.Conflicting = true
			}
		}
	}

	out := make([]Offer, 0, len(byName))
	for _, o := range byName {
		out = append(out, *o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// FindOffer returns the offer for one ecosystem name.
func FindOffer(machines []Machine, name string) (Offer, bool) {
	for _, o := range Offers(machines) {
		if o.Name == name {
			return o, true
		}
	}
	return Offer{}, false
}

// EcosystemCard converts an embedded note card back into the canonical config
// type. It is the inverse of build.go's convertCard, and exists so materialize
// hands the clone engine exactly the shape it would have read off a manifest.
func (c *NoteCard) EcosystemCard() config.EcosystemCard {
	if c == nil {
		return config.EcosystemCard{}
	}
	out := config.EcosystemCard{ID: c.ID, Layout: c.Layout}
	for _, r := range c.Remotes {
		out.Remotes = append(out.Remotes, config.EcosystemRemote{Name: r.Name, URL: r.URL})
	}
	for _, nb := range c.Notebooks {
		if out.Notebooks == nil {
			out.Notebooks = make(map[string]config.EcosystemNotebook, len(c.Notebooks))
		}
		out.Notebooks[nb.Name] = config.EcosystemNotebook{Default: nb.Default, Audience: nb.Audience}
	}
	return out
}

// sameCard compares the fields that decide WHAT gets cloned. Notebook bindings
// are deliberately excluded: two machines may legitimately bind an ecosystem to
// differently named notebooks, and that is not a reason to flag the offer.
func sameCard(a, b config.EcosystemCard) bool {
	if a.ID != b.ID || a.Layout != b.Layout || len(a.Remotes) != len(b.Remotes) {
		return false
	}
	for i := range a.Remotes {
		if a.Remotes[i] != b.Remotes[i] {
			return false
		}
	}
	return true
}

func appendUnique(list []string, v string) []string {
	if strings.TrimSpace(v) == "" {
		return list
	}
	for _, existing := range list {
		if existing == v {
			return list
		}
	}
	return append(list, v)
}

func nonEmpty(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return []string{v}
}
