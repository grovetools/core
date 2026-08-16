package models

import "strings"

// NormalizePlanRef returns the canonical note-frontmatter representation of a
// plan reference. Human-facing callers may supply either "foo" or
// "plans/foo"; notes store the latter.
func NormalizePlanRef(ref string) string {
	ref = strings.Trim(strings.TrimSpace(ref), "/")
	if ref == "" {
		return ""
	}
	if strings.HasPrefix(ref, "plans/") {
		return ref
	}
	return "plans/" + ref
}

// NoteLinkedToPlan is the shared association predicate for note UIs. PlanRef is
// the only association axis: tags are topical metadata and are deliberately
// ignored. The match is exact because plan_job carries per-job association.
func NoteLinkedToPlan(planRef, plan string) bool {
	return planRef != "" && planRef == NormalizePlanRef(plan)
}
