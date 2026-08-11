package registry

import (
	"sort"
	"strings"

	"github.com/grovetools/core/config"
)

// PublishedEcosystem is the materialization view derived from the registry
// note's publish-time member origins. Layout and Remotes are compatibility
// projections for existing confirmation UIs; neither comes from EcosystemCard.
type PublishedEcosystem struct {
	ID      string
	Name    string
	Members []NoteMemberOrigin
	Layout  string
	Remotes []config.EcosystemRemote
}

type Offer struct {
	Name        string
	Card        PublishedEcosystem
	Publishers  []string
	Paths       []string
	Conflicting bool
}

func (o Offer) PrimaryRemote() (config.EcosystemRemote, bool) {
	if len(o.Card.Remotes) == 0 {
		return config.EcosystemRemote{}, false
	}
	for _, remote := range o.Card.Remotes {
		if remote.Name == "origin" {
			return remote, true
		}
	}
	return o.Card.Remotes[0], true
}

func Offers(machines []Machine) []Offer {
	byName := make(map[string]*Offer)
	for _, machine := range machines {
		if machine.Note == nil {
			continue
		}
		for _, ecosystem := range machine.Note.Ecosystems {
			if ecosystem.Card == nil || strings.TrimSpace(ecosystem.Name) == "" {
				continue
			}
			published := ecosystem.Card.PublishedEcosystem()
			existing := byName[ecosystem.Name]
			if existing == nil {
				byName[ecosystem.Name] = &Offer{
					Name: ecosystem.Name, Card: published,
					Publishers: []string{machine.Label()}, Paths: nonEmpty(ecosystem.Path),
				}
				continue
			}
			existing.Publishers = appendUnique(existing.Publishers, machine.Label())
			existing.Paths = appendUnique(existing.Paths, ecosystem.Path)
			if !samePublished(existing.Card, published) {
				existing.Conflicting = true
			}
		}
	}
	out := make([]Offer, 0, len(byName))
	for _, offer := range byName {
		out = append(out, *offer)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func FindOffer(machines []Machine, name string) (Offer, bool) {
	for _, offer := range Offers(machines) {
		if offer.Name == name {
			return offer, true
		}
	}
	return Offer{}, false
}

func (c *NoteCard) PublishedEcosystem() PublishedEcosystem {
	if c == nil {
		return PublishedEcosystem{}
	}
	out := PublishedEcosystem{ID: c.ID, Name: c.Name, Members: append([]NoteMemberOrigin(nil), c.Members...)}
	rootMember := false
	for _, member := range out.Members {
		if member.Path == "." {
			rootMember = true
		}
		name := "origin"
		if len(out.Members) > 1 || member.Path != "." {
			name = member.Path
		}
		out.Remotes = append(out.Remotes, config.EcosystemRemote{Name: name, URL: member.Origin})
	}
	if rootMember {
		out.Layout = config.LayoutSuperrepo
	} else if len(out.Members) > 0 {
		out.Layout = config.LayoutFlat
	}
	return out
}

func samePublished(a, b PublishedEcosystem) bool {
	if a.ID != b.ID || a.Name != b.Name || len(a.Members) != len(b.Members) {
		return false
	}
	for i := range a.Members {
		if a.Members[i] != b.Members[i] {
			return false
		}
	}
	return true
}

func appendUnique(list []string, value string) []string {
	if strings.TrimSpace(value) == "" {
		return list
	}
	for _, existing := range list {
		if existing == value {
			return list
		}
	}
	return append(list, value)
}

func nonEmpty(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return []string{value}
}
