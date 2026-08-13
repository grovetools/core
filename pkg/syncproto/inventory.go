package syncproto

import (
	"fmt"
	"sort"
	"strings"
)

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
// id, its display name, what notebooks.toml records about sharing it, and the
// notespaces contained by its root.
type LocalNotebook struct {
	ID   NotebookID
	Name string
	// Shared is `[notebooks.<n>.sync] share = true`.
	Shared bool
	// Recorded is whether the sync table exists at all — coderoot.Notebook's
	// SyncRecorded(). It is carried separately because notebooks.toml is
	// tri-state and so is the server: "recorded as unshared per D9" and "never
	// considered" are different facts, and a delta that folded them together
	// would drop the very state unshare exists to write down.
	Recorded   bool
	Notespaces []NotespaceID
}

// LocalShareState renders the recorded tri-state the way the server renders
// its own: "" when nothing was ever recorded, otherwise shared/unshared.
func (n LocalNotebook) LocalShareState() string {
	switch {
	case n.Shared:
		return NotebookShareStateShared
	case n.Recorded:
		return NotebookShareStateUnshared
	default:
		return ""
	}
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
	// LocalShared / LocalShareState / ServerShareState are reported rather
	// than reconciled: a notebook this machine still records as shared while
	// the server has it unshared is exactly the state D9 produces on another
	// machine, and the operator — not this function — decides what it means.
	//
	// LocalShareState is the tri-state ("" = never recorded), symmetric with
	// ServerShareState; LocalShared is the same fact collapsed to the one
	// question most callers ask.
	LocalShared      bool
	LocalShareState  string
	ServerShareState string
	// LocalDuplicate marks a notebook id recorded more than once on this
	// machine (D8). The entry then describes the UNION of what those records
	// hold — nothing is dropped — and the names are listed in the delta's
	// DuplicateLocalNotebooks. A verb must refuse to act on this notebook
	// until the operator re-mints one of the copies.
	LocalDuplicate bool
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
	// DuplicateLocalNotebooks are notebook ids this machine records more than
	// once — the copied-stamp state D8 calls an expected runtime condition
	// that must be surfaced with evidence. The same argument that keeps
	// unparented notespaces in the delta applies: a map keyed by id would let
	// the last record silently win, and the state most likely to be lost that
	// way is the shared one.
	DuplicateLocalNotebooks []DuplicateLocalNotebook
}

// DuplicateLocalNotebook is one id recorded more than once locally, with every
// name that claims it — the evidence an operator needs to decide which copy to
// re-mint.
type DuplicateLocalNotebook struct {
	ID    NotebookID
	Names []string
}

// Conflicts returns a non-nil error when the local side of the delta cannot be
// read as a fact — today, when a notebook id is recorded twice. It is the one
// spelling of "refuse loudly" every verb built on this delta shares: the delta
// itself stays renderable (an operator meets the duplicate here first, and
// hiding it would be the original bug), and the verb that would ACT on it
// stops.
func (d InventoryDelta) Conflicts() error {
	if len(d.DuplicateLocalNotebooks) == 0 {
		return nil
	}
	parts := make([]string, 0, len(d.DuplicateLocalNotebooks))
	for _, dup := range d.DuplicateLocalNotebooks {
		parts = append(parts, dup.ID.String()+" recorded as "+strings.Join(dup.Names, ", "))
	}
	return fmt.Errorf("this machine records %d duplicate notebook id(s): %s; re-mint one copy (grove doctor --fix) before sharing or pulling",
		len(d.DuplicateLocalNotebooks), strings.Join(parts, "; "))
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
		entry, seen := locals[nb.ID]
		if !seen {
			entry = &deltaSide{members: map[NotespaceID]struct{}{}}
			locals[nb.ID] = entry
		}
		// A repeated id merges rather than overwrites. Overwriting would drop
		// one record's whole contribution, and which record loses would depend
		// on input order — so the copy carrying `share = true` is exactly the
		// one that can disappear.
		entry.duplicate = seen
		entry.names = append(entry.names, nb.Name)
		if entry.name == "" {
			entry.name = nb.Name
		}
		if state := nb.LocalShareState(); shareStateRank(state) > shareStateRank(entry.shareState) {
			entry.shareState = state
		}
		for _, ns := range nb.Notespaces {
			if ns != "" {
				entry.members[ns] = struct{}{}
			}
		}
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
			d.LocalShareState = l.shareState
			d.LocalShared = l.shareState == NotebookShareStateShared
			d.LocalDuplicate = l.duplicate
			if l.duplicate {
				names := append([]string(nil), l.names...)
				sort.Strings(names)
				out.DuplicateLocalNotebooks = append(out.DuplicateLocalNotebooks,
					DuplicateLocalNotebook{ID: id, Names: names})
			}
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
	names      []string
	shareState string
	duplicate  bool
	members    map[NotespaceID]struct{}
}

// shareStateRank orders the tri-state so merging two records of one id keeps
// the strongest fact: a recorded state beats silence, and shared beats
// unshared. Nothing about a duplicate is silently resolved — the merge only
// decides what the delta REPORTS, and it reports the duplicate too.
func shareStateRank(state string) int {
	switch state {
	case NotebookShareStateShared:
		return 2
	case NotebookShareStateUnshared:
		return 1
	default:
		return 0
	}
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
