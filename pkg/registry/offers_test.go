package registry

import "testing"

func machineWithOffer(id, machineName, ecosystemName, ecosystemPath string, card *NoteCard) Machine {
	return Machine{Note: &Note{
		MachineID: id,
		Name:      machineName,
		Ecosystems: []NoteEcosystem{{
			Name: ecosystemName, Path: ecosystemPath, Card: card,
		}},
	}}
}

func TestOffersDedupPublishTimeOrigins(t *testing.T) {
	card := &NoteCard{ID: "eco-1", Name: "grovetools", Members: []NoteMemberOrigin{{Path: ".", Origin: "https://example.com/grovetools.git"}}}
	machines := []Machine{
		machineWithOffer("a", "one", "grovetools", "/a", card),
		machineWithOffer("b", "two", "grovetools", "/b", card),
	}
	offers := Offers(machines)
	if len(offers) != 1 || len(offers[0].Publishers) != 2 || offers[0].Conflicting {
		t.Fatalf("offers = %+v", offers)
	}
	remote, ok := offers[0].PrimaryRemote()
	if !ok || remote.URL != "https://example.com/grovetools.git" || offers[0].Card.Layout != "superrepo" {
		t.Fatalf("offer = %+v", offers[0])
	}
}

func TestOffersConflictOnIDOrMemberOrigins(t *testing.T) {
	one := &NoteCard{ID: "one", Name: "eco", Members: []NoteMemberOrigin{{Path: "repo", Origin: "a"}}}
	two := &NoteCard{ID: "two", Name: "eco", Members: []NoteMemberOrigin{{Path: "repo", Origin: "b"}}}
	offers := Offers([]Machine{
		machineWithOffer("a", "one", "eco", "/a", one),
		machineWithOffer("b", "two", "eco", "/b", two),
	})
	if len(offers) != 1 || !offers[0].Conflicting {
		t.Fatalf("offers = %+v", offers)
	}
}

func TestPublishedFlatMembers(t *testing.T) {
	published := (&NoteCard{ID: "id", Name: "eco", Members: []NoteMemberOrigin{{Path: "a", Origin: "ua"}, {Path: "b", Origin: "ub"}}}).PublishedEcosystem()
	if published.Layout != "flat" || len(published.Remotes) != 2 {
		t.Fatalf("published = %+v", published)
	}
}
