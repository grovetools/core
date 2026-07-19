package frontmatter

import (
	"strings"
	"testing"
)

// TestParse_PlanLinkFields pins that the metadata parser carries BOTH halves of
// the note<->plan linkage. plan_job is a NoteIndexEntry column, and the daemon's
// note enrichment fills that column from this parser — if plan_job stops being
// parsed here, every index-sourced read silently sees it empty.
func TestParse_PlanLinkFields(t *testing.T) {
	doc := `---
title: My Note
plan_ref: plans/my-feature
plan_job: 01-do-the-thing.md
priority: p1
---

body
`

	meta, err := Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if meta.PlanRef != "plans/my-feature" {
		t.Errorf("PlanRef = %q, want plans/my-feature", meta.PlanRef)
	}
	if meta.PlanJob != "01-do-the-thing.md" {
		t.Errorf("PlanJob = %q, want 01-do-the-thing.md", meta.PlanJob)
	}
	if meta.Priority != "p1" {
		t.Errorf("Priority = %q, want p1", meta.Priority)
	}
}

// TestParse_PlanJobAbsent keeps the field zero rather than inventing a value
// when a note is linked to a plan but has no promoted job.
func TestParse_PlanJobAbsent(t *testing.T) {
	doc := `---
title: Unpromoted
plan_ref: plans/my-feature
---
`

	meta, err := Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if meta.PlanRef != "plans/my-feature" {
		t.Errorf("PlanRef = %q, want plans/my-feature", meta.PlanRef)
	}
	if meta.PlanJob != "" {
		t.Errorf("PlanJob = %q, want empty", meta.PlanJob)
	}
}
