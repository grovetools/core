package notespace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	siblingID1 = "01J0000000000000000000000A"
	siblingID2 = "01J0000000000000000000000B"
	siblingID3 = "01J0000000000000000000000C"
	otherID    = "01J0000000000000000000000D"
)

// stampedRoots materializes notespace roots under one temp dir and returns the
// index over exactly those roots, which is the only way any caller builds one.
func stampedRoots(t *testing.T, stamps ...NotespaceStamp) (*Index, []string) {
	t.Helper()
	base := t.TempDir()
	roots := make([]string, 0, len(stamps))
	for _, stamp := range stamps {
		root := filepath.Join(base, stamp.Name)
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := InstallNotespace(root, stamp); err != nil {
			t.Fatal(err)
		}
		roots = append(roots, root)
	}
	idx, err := BuildIndex(roots)
	if err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	return idx, roots
}

func TestSiblingsForOrdersThePrimaryFirst(t *testing.T) {
	subject := "example.com/org/core"
	idx, roots := stampedRoots(t,
		NotespaceStamp{ID: siblingID1, Name: "a-core", Subject: subject, Kind: "repo"},
		NotespaceStamp{ID: siblingID2, Name: "b-core", Subject: subject, Kind: "repo"},
		NotespaceStamp{ID: siblingID3, Name: "c-other", Subject: "example.com/org/other", Kind: "repo"},
	)

	// The primary sorts last by root, so position cannot be an accident.
	siblings := idx.SiblingsFor(subject, map[string]string{subject: siblingID2})
	if len(siblings) != 2 || siblings[0].Stamp.ID != siblingID2 || siblings[1].Stamp.ID != siblingID1 {
		t.Fatalf("siblings = %+v", siblings)
	}

	// With nothing recorded, nothing is promoted: root order stands.
	unrecorded := idx.SiblingsFor(subject, nil)
	if len(unrecorded) != 2 || unrecorded[0].Root != roots[0] {
		t.Fatalf("unrecorded siblings = %+v", unrecorded)
	}
	if got := idx.Subjects(); len(got) != 2 || got[0] != subject {
		t.Fatalf("subjects = %v", got)
	}
	if got := idx.Records(); len(got) != 3 {
		t.Fatalf("records = %+v", got)
	}
}

func TestPrimaryForIsFailClosed(t *testing.T) {
	subject := "example.com/org/core"
	idx, roots := stampedRoots(t,
		NotespaceStamp{ID: siblingID1, Name: "core", Subject: subject, Kind: "repo"},
		NotespaceStamp{ID: siblingID2, Name: "core-sibling", Subject: subject, Kind: "repo"},
	)

	record, err := idx.PrimaryFor(subject, map[string]string{subject: siblingID1})
	if err != nil || record.Root != roots[0] {
		t.Fatalf("PrimaryFor() = %+v, %v", record, err)
	}

	cases := []struct {
		name      string
		subject   string
		primaries map[string]string
		want      string
	}{
		{"unrecorded", subject, nil, "no recorded primary notespace"},
		{"dangling", subject, map[string]string{subject: otherID}, "has no stamped root"},
		{"mismatched", "example.com/org/other", map[string]string{"example.com/org/other": siblingID1}, "is stamped for subject"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := idx.PrimaryFor(tc.subject, tc.primaries); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestPrimaryForRefusesADuplicatedID(t *testing.T) {
	subject := "example.com/org/core"
	idx, _ := stampedRoots(t,
		NotespaceStamp{ID: siblingID1, Name: "original", Subject: subject, Kind: "repo"},
		NotespaceStamp{ID: siblingID1, Name: "copy", Subject: subject, Kind: "repo"},
	)
	if _, err := idx.PrimaryFor(subject, map[string]string{subject: siblingID1}); err == nil || !strings.Contains(err.Error(), "duplicate notespace id") {
		t.Fatalf("err = %v, want a duplicate-id refusal", err)
	}
}

func TestBuildIndexRefusesAMalformedStamp(t *testing.T) {
	root := filepath.Join(t.TempDir(), "broken")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(NotespaceStampPath(root), []byte("id = ["), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildIndex([]string{root}); err == nil {
		t.Fatal("BuildIndex accepted a malformed stamp")
	}
}

func TestAuditPrimariesNamesEveryViolation(t *testing.T) {
	core := "example.com/org/core"
	other := "example.com/org/other"
	loose := "example.com/org/loose"
	idx, _ := stampedRoots(t,
		NotespaceStamp{ID: siblingID1, Name: "core", Subject: core, Kind: "repo"},
		NotespaceStamp{ID: siblingID2, Name: "core-sibling", Subject: core, Kind: "repo"},
		NotespaceStamp{ID: siblingID3, Name: "other", Subject: other, Kind: "repo"},
	)

	clean := idx.AuditPrimaries(map[string]string{core: siblingID1, other: siblingID3})
	if len(clean) != 0 {
		t.Fatalf("a legal sibling set reported %v", clean)
	}

	problems := idx.AuditPrimaries(map[string]string{
		core:  siblingID1,
		other: siblingID1,
		loose: otherID,
	})
	got := map[PrimaryProblemKind]int{}
	for _, problem := range problems {
		got[problem.Kind]++
	}
	// core/other share one id (two duplicates), loose points at nothing
	// (dangling); the stamped `other` notespace loses its own entry to the
	// duplicate, so it is not additionally reported as missing.
	if got[PrimaryDuplicate] != 2 || got[PrimaryDangling] != 1 || len(problems) != 3 {
		t.Fatalf("problems = %v", problems)
	}

	missing := idx.AuditPrimaries(map[string]string{core: siblingID1})
	if len(missing) != 1 || missing[0].Kind != PrimaryMissing || missing[0].Subject != other {
		t.Fatalf("missing = %v", missing)
	}
	if !strings.Contains(missing[0].String(), "no [primaries] entry") {
		t.Fatalf("problem text = %q", missing[0].String())
	}

	mismatched := idx.AuditPrimaries(map[string]string{core: siblingID1, other: siblingID2})
	if len(mismatched) != 1 || mismatched[0].Kind != PrimaryMismatched {
		t.Fatalf("mismatched = %v", mismatched)
	}
}
