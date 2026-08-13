package syncproto

import (
	"strings"
	"testing"
)

const (
	nbA  = "01ARZ3NDEKTSV4RRFFQ69G5FA0"
	nbB  = "01ARZ3NDEKTSV4RRFFQ69G5FA1"
	nsA  = "01ARZ3NDEKTSV4RRFFQ69G5FA2"
	nsB  = "01ARZ3NDEKTSV4RRFFQ69G5FA3"
	dev  = "01ARZ3NDEKTSV4RRFFQ69G5FA4"
	nsC  = "01ARZ3NDEKTSV4RRFFQ69G5FA5"
	idem = "key"
)

func v3(key string) RequestIdentity {
	return RequestIdentity{ProtocolVersion: ProtocolVersionNotespaceID, IdempotencyKey: key, DeviceID: dev}
}

func TestNotebookShareValidate(t *testing.T) {
	cases := []struct {
		name string
		req  NotebookShareRequest
		want string
	}{
		{
			name: "valid",
			req:  NotebookShareRequest{RequestIdentity: v3(idem), NotebookID: nbA, Name: "nb", Members: []NotespaceID{nsA, nsB}},
		},
		{
			name: "v2 is refused: the notebook surfaces extend v3, they do not fork it",
			req:  NotebookShareRequest{RequestIdentity: RequestIdentity{ProtocolVersion: ProtocolVersionDeviceSession, IdempotencyKey: idem, DeviceID: dev}, NotebookID: nbA, Name: "nb"},
			want: ErrorProtocolVersion,
		},
		{
			name: "notebook id must be a ULID",
			req:  NotebookShareRequest{RequestIdentity: v3(idem), NotebookID: "nb", Name: "nb"},
			want: ErrorRegistrationConflict,
		},
		{
			name: "name is required",
			req:  NotebookShareRequest{RequestIdentity: v3(idem), NotebookID: nbA, Name: "  "},
			want: ErrorRegistrationConflict,
		},
		{
			name: "members must be ULIDs",
			req:  NotebookShareRequest{RequestIdentity: v3(idem), NotebookID: nbA, Name: "nb", Members: []NotespaceID{"core"}},
			want: ErrorRegistrationConflict,
		},
		{
			name: "duplicate member",
			req:  NotebookShareRequest{RequestIdentity: v3(idem), NotebookID: nbA, Name: "nb", Members: []NotespaceID{nsA, nsA}},
			want: ErrorRegistrationConflict,
		},
		{
			name: "expected_version 0 is the claim that this notebook is not registered yet",
			req:  NotebookShareRequest{RequestIdentity: v3(idem), NotebookID: nbA, Name: "nb", ExpectedVersion: 0},
		},
		{
			name: "negative expected_version",
			req:  NotebookShareRequest{RequestIdentity: v3(idem), NotebookID: nbA, Name: "nb", ExpectedVersion: -1},
			want: ErrorStaleResolution,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			switch {
			case tc.want == "" && err != nil:
				t.Fatalf("unexpected error %v", err)
			case tc.want != "" && (err == nil || err.Code != tc.want):
				t.Fatalf("error = %v, want code %q", err, tc.want)
			}
		})
	}
}

func TestNotebookUnshareAndReparentValidate(t *testing.T) {
	if err := (NotebookUnshareRequest{RequestIdentity: v3(idem), NotebookID: nbA, ExpectedVersion: 1}).Validate(); err != nil {
		t.Fatalf("valid unshare rejected: %v", err)
	}
	if err := (NotebookUnshareRequest{RequestIdentity: v3(idem), NotebookID: nbA}).Validate(); err == nil || err.Code != ErrorStaleResolution {
		t.Fatalf("unshare without a version = %v", err)
	}

	valid := NotespaceReparentRequest{RequestIdentity: v3(idem), NotespaceID: nsA, FromNotebookID: nbA, ToNotebookID: nbB, ExpectedVersion: 1}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid reparent rejected: %v", err)
	}
	// Moving out of "unparented" is legal and carries no from id.
	unparented := valid
	unparented.FromNotebookID = ""
	unparented.ExpectedVersion = 0
	if err := unparented.Validate(); err != nil {
		t.Fatalf("reparent from unparented rejected: %v", err)
	}
	same := valid
	same.FromNotebookID = nbB
	if err := same.Validate(); err == nil || err.Code != ErrorMembershipConflict {
		t.Fatalf("same-notebook move = %v", err)
	}
	badTarget := valid
	badTarget.ToNotebookID = "personal"
	if err := badTarget.Validate(); err == nil || err.Code != ErrorRegistrationConflict {
		t.Fatalf("non-ULID destination = %v", err)
	}

	// The out-of-shared leg (W3.4): a move whose destination is no notebook.
	// Without it membership would be write-only-into and a notespace moved out
	// locally would stay in the server's roll forever.
	detach := valid
	detach.ToNotebookID = ""
	if err := detach.Validate(); err != nil {
		t.Fatalf("detach rejected: %v", err)
	}
	if !detach.Detaching() {
		t.Fatal("a move with no destination is a detach")
	}
	if valid.Detaching() {
		t.Fatal("a move into a notebook is not a detach")
	}
	nowhere := detach
	nowhere.FromNotebookID = ""
	if err := nowhere.Validate(); err == nil || err.Code != ErrorMembershipConflict {
		t.Fatalf("detaching an already-unparented notespace moves nothing: %v", err)
	}
}

func TestBuildInventoryDelta(t *testing.T) {
	local := []LocalNotebook{
		{ID: nbA, Name: "nb", Shared: true, Notespaces: []NotespaceID{nsA, nsC}},
		{ID: nbB, Name: "personal", Notespaces: []NotespaceID{nsB}},
	}
	// nbB exists only locally; nbUnshared only on the server, and retained
	// after an unshare so it must be visible but not on offer.
	const nbUnshared = "01ARZ3NDEKTSV4RRFFQ69G5FA6"
	const nsOrphan = "01ARZ3NDEKTSV4RRFFQ69G5FA7"
	const nsServerOnly = "01ARZ3NDEKTSV4RRFFQ69G5FA8"
	server := InventoryResponse{
		Notebooks: []InventoryNotebook{
			{ID: nbA, Name: "nb", ShareState: NotebookShareStateShared, Version: 3, NotespaceIDs: []NotespaceID{nsA, nsServerOnly}},
			{ID: nbUnshared, Name: "retired", ShareState: NotebookShareStateUnshared, Version: 5},
		},
		Notespaces: []InventoryNotespace{
			{ID: nsA, NotebookID: nbA},
			{ID: nsServerOnly, NotebookID: nbA},
			{ID: nsOrphan},
		},
	}

	delta := BuildInventoryDelta(local, server)
	if len(delta.Notebooks) != 3 {
		t.Fatalf("notebooks = %+v, want one entry per id on either side", delta.Notebooks)
	}
	byID := map[NotebookID]NotebookDelta{}
	for _, d := range delta.Notebooks {
		byID[d.ID] = d
	}

	both := byID[nbA]
	if both.Direction != "" {
		t.Fatalf("a notebook on both sides has no direction, got %q", both.Direction)
	}
	if len(both.LocalOnlyNotespaces) != 1 || both.LocalOnlyNotespaces[0] != nsC {
		t.Fatalf("local-only members = %v, want [%s]", both.LocalOnlyNotespaces, nsC)
	}
	if len(both.ServerOnlyNotespaces) != 1 || both.ServerOnlyNotespaces[0] != nsServerOnly {
		t.Fatalf("server-only members = %v", both.ServerOnlyNotespaces)
	}
	if !both.LocalShared || both.ServerShareState != NotebookShareStateShared || !both.PullEligible {
		t.Fatalf("share state not carried through: %+v", both)
	}

	shareSide := byID[nbB]
	if shareSide.Direction != DeltaDirectionShare || len(shareSide.LocalOnlyNotespaces) != 1 {
		t.Fatalf("local-only notebook = %+v", shareSide)
	}
	if shareSide.ServerShareState != "" || shareSide.PullEligible {
		t.Fatalf("a notebook the server has never seen is not pullable: %+v", shareSide)
	}

	retired := byID[nbUnshared]
	if retired.Direction != DeltaDirectionPull {
		t.Fatalf("server-only notebook direction = %q", retired.Direction)
	}
	if retired.PullEligible {
		t.Fatal("an unshared notebook is retained (D9) but must not be offered for pull")
	}

	if len(delta.UnparentedServerNotespaces) != 1 || delta.UnparentedServerNotespaces[0] != nsOrphan {
		t.Fatalf("unparented server notespaces = %v", delta.UnparentedServerNotespaces)
	}
}

// notebooks.toml is tri-state and so is the server; "recorded as unshared per
// D9" and "never considered" must not arrive at the delta as the same input.
func TestBuildInventoryDeltaCarriesTheLocalTriState(t *testing.T) {
	const nbNever = "01ARZ3NDEKTSV4RRFFQ69G5FB0"
	local := []LocalNotebook{
		{ID: nbA, Name: "shared", Shared: true, Recorded: true},
		{ID: nbB, Name: "stopped", Recorded: true},
		{ID: nbNever, Name: "never"},
	}
	byID := map[NotebookID]NotebookDelta{}
	for _, d := range BuildInventoryDelta(local, InventoryResponse{}).Notebooks {
		byID[d.ID] = d
	}
	if got := byID[nbA].LocalShareState; got != NotebookShareStateShared || !byID[nbA].LocalShared {
		t.Fatalf("shared local state = %q", got)
	}
	if got := byID[nbB].LocalShareState; got != NotebookShareStateUnshared || byID[nbB].LocalShared {
		t.Fatalf("recorded-as-unshared collapsed to %q", got)
	}
	if got := byID[nbNever].LocalShareState; got != "" {
		t.Fatalf("a notebook that never recorded sync state = %q, want \"\"", got)
	}
}

// D8 makes a duplicate stamp id an expected runtime state, and the join delta
// is where an operator meets it first. A map keyed by id used to let the last
// record win — losing, in the measured case, the shared one.
func TestBuildInventoryDeltaSurfacesDuplicateLocalIDs(t *testing.T) {
	local := []LocalNotebook{
		{ID: nbA, Name: "work", Shared: true, Recorded: true, Notespaces: []NotespaceID{nsA}},
		{ID: nbA, Name: "work-copy", Notespaces: []NotespaceID{nsB}},
	}
	delta := BuildInventoryDelta(local, InventoryResponse{})
	if len(delta.Notebooks) != 1 {
		t.Fatalf("notebooks = %+v", delta.Notebooks)
	}
	d := delta.Notebooks[0]
	if !d.LocalDuplicate {
		t.Fatalf("the duplicate was not marked: %+v", d)
	}
	if !d.LocalShared || d.LocalShareState != NotebookShareStateShared {
		t.Fatalf("the shared record's state was dropped by the copy: %+v", d)
	}
	if len(d.LocalOnlyNotespaces) != 2 {
		t.Fatalf("membership must be the union of both records, got %v", d.LocalOnlyNotespaces)
	}
	if len(delta.DuplicateLocalNotebooks) != 1 ||
		len(delta.DuplicateLocalNotebooks[0].Names) != 2 ||
		delta.DuplicateLocalNotebooks[0].Names[0] != "work" {
		t.Fatalf("evidence must name every claimant: %+v", delta.DuplicateLocalNotebooks)
	}

	// Rendering the delta is fine; acting on it is not, and the refusal has one
	// spelling every verb can share.
	err := delta.Conflicts()
	if err == nil || !strings.Contains(err.Error(), "work-copy") {
		t.Fatalf("Conflicts() = %v, want a loud error naming the claimants", err)
	}
	if clean := BuildInventoryDelta(local[:1], InventoryResponse{}).Conflicts(); clean != nil {
		t.Fatalf("a delta with no duplicates conflicts: %v", clean)
	}
}

func TestBuildInventoryDeltaReadsMembershipFromEitherSide(t *testing.T) {
	// A server that fills only the per-notespace notebook_id must still yield
	// a complete membership roll, and vice versa.
	fromNotespaces := BuildInventoryDelta(nil, InventoryResponse{
		Notebooks:  []InventoryNotebook{{ID: nbA, ShareState: NotebookShareStateShared}},
		Notespaces: []InventoryNotespace{{ID: nsA, NotebookID: nbA}},
	})
	fromRoll := BuildInventoryDelta(nil, InventoryResponse{
		Notebooks: []InventoryNotebook{{ID: nbA, ShareState: NotebookShareStateShared, NotespaceIDs: []NotespaceID{nsA}}},
	})
	for _, delta := range []InventoryDelta{fromNotespaces, fromRoll} {
		if len(delta.Notebooks) != 1 || len(delta.Notebooks[0].ServerOnlyNotespaces) != 1 || delta.Notebooks[0].ServerOnlyNotespaces[0] != nsA {
			t.Fatalf("membership not recovered: %+v", delta)
		}
	}
}

func TestBuildInventoryDeltaIsDeterministic(t *testing.T) {
	local := []LocalNotebook{{ID: nbB, Name: "b", Notespaces: []NotespaceID{nsC, nsA, nsB}}}
	server := InventoryResponse{Notebooks: []InventoryNotebook{{ID: nbA, Name: "a"}}}
	first := BuildInventoryDelta(local, server)
	for i := 0; i < 5; i++ {
		again := BuildInventoryDelta(local, server)
		if len(again.Notebooks) != len(first.Notebooks) {
			t.Fatal("unstable notebook count")
		}
		for j := range again.Notebooks {
			if again.Notebooks[j].ID != first.Notebooks[j].ID {
				t.Fatalf("unstable notebook order: %v vs %v", again.Notebooks[j].ID, first.Notebooks[j].ID)
			}
		}
	}
	members := first.Notebooks[1].LocalOnlyNotespaces
	for i := 1; i < len(members); i++ {
		if members[i-1] >= members[i] {
			t.Fatalf("members are not ordered: %v", members)
		}
	}
}
