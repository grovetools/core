package notespace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemintKeepsMutableMetadataAndChangesTheID(t *testing.T) {
	root := t.TempDir()
	original, err := MintNotespace(root, NotespaceMutable{Name: "core", Subject: testSubject, Kind: "repo"})
	if err != nil {
		t.Fatal(err)
	}

	result, err := RemintNotespace(root, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.OldID != original.ID {
		t.Fatalf("OldID = %q, want %q", result.OldID, original.ID)
	}
	if result.NewID == original.ID {
		t.Fatal("re-mint kept the duplicated id")
	}
	if result.Stamp.Name != original.Name || result.Stamp.Subject != original.Subject || result.Stamp.Kind != original.Kind {
		t.Fatalf("re-mint altered mutable metadata: %+v, want name/subject/kind of %+v", result.Stamp, original)
	}
	// The copy is still notes about the same subject: losing that would strand
	// it outside every subject-keyed surface.
	settled, err := LoadNotespace(root)
	if err != nil || settled == nil || settled.ID != result.NewID {
		t.Fatalf("LoadNotespace after re-mint = %+v, %v", settled, err)
	}
}

func TestRemintRefusesAnUndesignatedRoot(t *testing.T) {
	root := t.TempDir()
	stamp, err := MintNotespace(root, NotespaceMutable{Name: "core", Subject: testSubject, Kind: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	// A decision made against one id must not land on a root that has changed
	// underneath it.
	if _, err := RemintNotespace(root, "01ARZ3NDEKTSV4RRFFQ69G5FAV"); err == nil {
		t.Fatal("re-mint accepted a root carrying a different id")
	}
	after, err := LoadNotespace(root)
	if err != nil || after.ID != stamp.ID {
		t.Fatalf("refused re-mint still rewrote the stamp: %+v, %v", after, err)
	}
	if _, err := RemintNotespace(root, ""); err == nil {
		t.Fatal("re-mint accepted an empty designation")
	}
}

func TestRemintRefusesAnUnstampedRoot(t *testing.T) {
	root := t.TempDir()
	if _, err := RemintNotespace(root, "01ARZ3NDEKTSV4RRFFQ69G5FAV"); err == nil {
		t.Fatal("re-mint minted an identity for a root that had none")
	}
	if _, err := os.Stat(filepath.Join(root, NotespaceStampName)); !os.IsNotExist(err) {
		t.Fatalf("re-mint created %s on a root with no stamp", NotespaceStampName)
	}
}

func TestRemintSeparatesTwoCopiesOfOneID(t *testing.T) {
	keeper, loser := t.TempDir(), t.TempDir()
	stamp, err := MintNotespace(keeper, NotespaceMutable{Name: "core", Subject: testSubject, Kind: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	// The D8 state: one id, two physical roots.
	copyStamp(t, keeper, loser)

	result, err := RemintNotespace(loser, stamp.ID)
	if err != nil {
		t.Fatal(err)
	}
	keeperStamp, err := LoadNotespace(keeper)
	if err != nil || keeperStamp.ID != stamp.ID {
		t.Fatalf("re-minting the loser disturbed the keeper: %+v, %v", keeperStamp, err)
	}
	if result.NewID == keeperStamp.ID {
		t.Fatal("the two roots still claim one id")
	}
}

func copyStamp(t *testing.T, from, to string) {
	t.Helper()
	data, err := os.ReadFile(NotespaceStampPath(from))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(NotespaceStampPath(to), data, 0o644); err != nil {
		t.Fatal(err)
	}
}
