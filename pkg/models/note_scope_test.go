package models

import "testing"

func TestNoteLinkedToPlanUsesExactPlanRefOnly(t *testing.T) {
	for _, tc := range []struct {
		name, ref, plan string
		want            bool
	}{
		{"canonical", "plans/misc-fixes", "misc-fixes", true},
		{"unqualified stored ref rejected", "misc-fixes", "plans/misc-fixes", false},
		{"human plan argument", "plans/misc-fixes", "misc-fixes", true},
		{"other plan", "plans/other", "misc-fixes", false},
		{"job-like suffix is not association", "plans/misc-fixes/03-job.md", "misc-fixes", false},
		{"empty", "", "misc-fixes", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := NoteLinkedToPlan(tc.ref, tc.plan); got != tc.want {
				t.Fatalf("NoteLinkedToPlan(%q, %q) = %v, want %v", tc.ref, tc.plan, got, tc.want)
			}
		})
	}
}
