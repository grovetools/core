package syncproto

import "sort"

// The join delta, as a pure function.
//
// `grove sync join` and `grove notebook share|pull` all need the same answer:
// given what this machine has recorded and what the server reports, what
// exists on one side only? Computing that in the verb would put the rule in
// three places and make it untestable without a server, so it lives here
// beside the wire types it reads, takes plain values, and touches nothing.
//
// The delta is a description, never an instruction. Nothing in it moves until
// a verb acts on a keypress — which is why it names directions ("this could be
// shared") rather than actions ("share this").

// LocalNotebook is one recorded notebook as this machine knows it: its stamp
// id, its display name, whether notebooks.toml records it as shared, and the
// notespaces contained by its root.
type LocalNotebook struct {
	ID         NotebookID
	Name       string
	Shared     bool
	Notespaces []NotespaceID
}

// Delta directions. A notebook present on both sides is Direction "" — it is
// still reported, because its per-notespace membership may differ.
const (
	DeltaDirectionShare = "share" // recorded here, absent from the server
	DeltaDirectionPull  = "pull"  // on the server, not recorded here
)

// NotebookDelta is one notebook's difference across the two sides.
type NotebookDelta struct {
	ID   NotebookID
	Name string
	// Direction is DeltaDirectionShare, DeltaDirectionPull, or "" when the
	// notebook exists on both sides.
	Direction string
	// LocalShared / ServerShareState are reported rather than reconciled: a
	// notebook this machine still records as shared while the server has it
	// unshared is exactly the state D9 produces on another machine, and the
	// operator — not this function — decides what it means.
	LocalShared      bool
	ServerShareState string
	// PullEligible is false for a server notebook the server has unshared:
	// its history is retained, but it is not on offer.
	PullEligible bool
	// LocalOnlyNotespaces / ServerOnlyNotespaces are the membership legs of
	// the delta, each ordered.
	LocalOnlyNotespaces  []NotespaceID
	ServerOnlyNotespaces []NotespaceID
}

// InventoryDelta is the whole comparison, notebooks ordered by id so two runs
// over the same inputs render identically.
type InventoryDelta struct {
	Notebooks []NotebookDelta
	// UnparentedServerNotespaces are registered notespaces the server holds
	// in no notebook. They are surfaced rather than hidden: a notespace
	// belonging to nothing is a real state (registered before its notebook
	// was shared) and silently dropping it from the delta is how a machine
	// ends up unable to explain a missing notespace.
	UnparentedServerNotespaces []NotespaceID
}

// BuildInventoryDelta compares recorded local notebooks against a server
// inventory. It is total: unknown ids on either side become one-sided entries
// rather than errors.
func BuildInventoryDelta(local []LocalNotebook, server InventoryResponse) InventoryDelta {
	locals := map[NotebookID]*deltaSide{}
	for _, nb := range local {
		if nb.ID == "" {
			continue
		}
		entry := &deltaSide{name: nb.Name, members: map[NotespaceID]struct{}{}}
		if nb.Shared {
			entry.shareState = NotebookShareStateShared
		}
		for _, ns := range nb.Notespaces {
			if ns != "" {
				entry.members[ns] = struct{}{}
			}
		}
		locals[nb.ID] = entry
	}

	// Membership is read from the notebook roll and from the notespace rows
	// both, so a server that populated only one of them still yields a
	// complete delta.
	servers := map[NotebookID]*deltaSide{}
	for _, nb := range server.Notebooks {
		if nb.ID == "" {
			continue
		}
		entry := &deltaSide{name: nb.Name, shareState: nb.ShareState, members: map[NotespaceID]struct{}{}}
		for _, ns := range nb.NotespaceIDs {
			if ns != "" {
				entry.members[ns] = struct{}{}
			}
		}
		servers[nb.ID] = entry
	}
	var unparented []NotespaceID
	for _, ns := range server.Notespaces {
		if ns.ID == "" {
			continue
		}
		if ns.NotebookID == "" {
			unparented = append(unparented, ns.ID)
			continue
		}
		entry, ok := servers[ns.NotebookID]
		if !ok {
			entry = &deltaSide{members: map[NotespaceID]struct{}{}}
			servers[ns.NotebookID] = entry
		}
		entry.members[ns.ID] = struct{}{}
	}

	ids := map[NotebookID]struct{}{}
	for id := range locals {
		ids[id] = struct{}{}
	}
	for id := range servers {
		ids[id] = struct{}{}
	}

	out := InventoryDelta{Notebooks: []NotebookDelta{}, UnparentedServerNotespaces: sortIDs(unparented)}
	for _, id := range sortNotebookIDs(ids) {
		l, hasLocal := locals[id]
		s, hasServer := servers[id]
		d := NotebookDelta{ID: id}
		switch {
		case hasLocal && !hasServer:
			d.Direction = DeltaDirectionShare
		case !hasLocal && hasServer:
			d.Direction = DeltaDirectionPull
		}
		if hasLocal {
			d.Name = l.name
			d.LocalShared = l.shareState == NotebookShareStateShared
		}
		if hasServer {
			if d.Name == "" {
				d.Name = s.name
			}
			d.ServerShareState = s.shareState
			d.PullEligible = s.shareState != NotebookShareStateUnshared
		}
		d.LocalOnlyNotespaces = missing(l, s)
		d.ServerOnlyNotespaces = missing(s, l)
		out.Notebooks = append(out.Notebooks, d)
	}
	return out
}

// deltaSide is one side's view of a notebook while the delta is being built.
type deltaSide struct {
	name       string
	shareState string
	members    map[NotespaceID]struct{}
}

// missing returns the members have holds that want does not, ordered. A nil
// side contributes nothing, which is what makes a one-sided notebook fall out
// of the same code path as a two-sided one.
func missing(have, want *deltaSide) []NotespaceID {
	if have == nil {
		return nil
	}
	var out []NotespaceID
	for id := range have.members {
		if want != nil {
			if _, ok := want.members[id]; ok {
				continue
			}
		}
		out = append(out, id)
	}
	return sortIDs(out)
}

func sortIDs(ids []NotespaceID) []NotespaceID {
	if len(ids) == 0 {
		return nil
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func sortNotebookIDs(set map[NotebookID]struct{}) []NotebookID {
	out := make([]NotebookID, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
