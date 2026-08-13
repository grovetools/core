package syncproto

import "testing"

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
