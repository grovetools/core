package notespace

import (
	"fmt"
	"sort"
	"strings"
)

// The recorded relationship queries (Phase 4, W4.1/W4.4).
//
// Primary and sibling are machine-relative RELATIONSHIPS, so they exist here as
// queries over two inputs and nothing else: the stamp index (what is on disk)
// and the recorded [primaries] table (what this machine routes to). They are
// deliberately not properties of a notespace — there is no IsPrimary field and
// there will not be one, because a notespace carries no fact about which
// machine points at it, and a struct field would invite exactly that reading.
//
// The table is passed as a plain map rather than a *config.MachineConfig so
// this package keeps depending only on identity, which is what lets the
// resolver, the grove verbs, doctor and (later) nb share one implementation of
// the rules instead of each re-deriving them.
//
// Every query here is fail-closed on the two conditions that make an answer
// untrustworthy: a duplicated id (two physical roots claiming one identity,
// D8) and a malformed stamp (refused at BuildIndex time, never re-minted). A
// query that cannot be answered returns an error naming what it found; none of
// them repairs anything, and none of them guesses.

// Records returns every stamped record the index holds, in root order.
func (i *Index) Records() []Record {
	if i == nil {
		return nil
	}
	out := make([]Record, 0, len(i.byID))
	for _, records := range i.byID {
		out = append(out, records...)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Root < out[b].Root })
	return out
}

// Subjects returns every subject the index holds, sorted.
func (i *Index) Subjects() []string {
	if i == nil {
		return nil
	}
	out := make([]string, 0, len(i.bySubject))
	for value := range i.bySubject {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

// SiblingsFor returns every recorded notespace stamped for one subject, the
// recorded primary first and the rest in root order.
//
// A subject with no recorded primary, or one whose primary is not in this
// index, still lists its notespaces — in root order, with nothing promoted.
// Ordering is presentation; the fact of primariness is PrimaryFor's answer and
// a caller that needs it asks for it rather than inferring it from position.
func (i *Index) SiblingsFor(value string, primaries map[string]string) []Record {
	records := i.BySubject(value)
	primary := primaries[value]
	if primary == "" || len(records) < 2 {
		return records
	}
	sort.SliceStable(records, func(a, b int) bool {
		return records[a].Stamp.ID == primary && records[b].Stamp.ID != primary
	})
	return records
}

// PrimaryFor resolves the recorded primary notespace for a subject.
//
// Each refusal names a different repair, so they are kept distinct rather than
// collapsed into "not found": nothing recorded is `notespace primary`'s job, a
// recorded id with no stamp is a deleted or unpulled root, a duplicated id is
// D8's copied stamp, and a stamp claiming another subject is a mis-keyed
// binding.
func (i *Index) PrimaryFor(value string, primaries map[string]string) (Record, error) {
	id := primaries[value]
	if id == "" {
		return Record{}, fmt.Errorf("subject %q has no recorded primary notespace", value)
	}
	records, err := i.ByID(id)
	if err != nil {
		return Record{}, err
	}
	if len(records) == 0 {
		return Record{}, fmt.Errorf("primary notespace id %q for subject %q has no stamped root", id, value)
	}
	if records[0].Stamp.Subject != value {
		return Record{}, fmt.Errorf("primary notespace id %q is stamped for subject %q, not %q", id, records[0].Stamp.Subject, value)
	}
	return records[0], nil
}

// PrimaryProblemKind names the five ways a recorded primary can be wrong.
type PrimaryProblemKind string

const (
	// PrimaryMissing: a subject has notes on this machine and no [primaries]
	// entry, so unqualified writes for it resolve to nothing.
	PrimaryMissing PrimaryProblemKind = "missing"
	// PrimaryDangling: an entry names an id no stamped root carries.
	PrimaryDangling PrimaryProblemKind = "dangling"
	// PrimaryDuplicate: one id is recorded as the primary of more than one
	// subject. The binding table is wrong: a notespace is stamped for exactly
	// one subject, so at most one of those entries can be true.
	PrimaryDuplicate PrimaryProblemKind = "duplicate"
	// PrimaryUnresolvable: the entry names an id the index cannot resolve to a
	// single root, because two physical roots claim one identity (D8).
	//
	// It is kept apart from PrimaryDuplicate because it is a fact about the
	// DISK rather than about the binding table, and the two carry different
	// repairs. Every surface that reports it also reports the duplicated roots
	// themselves, in the words and with the repair that condition owns — so a
	// caller that has already named the physical duplicate drops this
	// restatement of it rather than showing one fault twice.
	PrimaryUnresolvable PrimaryProblemKind = "unresolvable"
	// PrimaryMismatched: the named notespace is stamped for another subject.
	PrimaryMismatched PrimaryProblemKind = "mismatched"
)

// PrimaryProblem is one violation of the exactly-one-primary-per-subject rule.
type PrimaryProblem struct {
	Kind        PrimaryProblemKind
	Subject     string
	NotespaceID string
	Detail      string
}

func (p PrimaryProblem) String() string {
	head := fmt.Sprintf("%s primary for subject %s", p.Kind, p.Subject)
	if p.NotespaceID != "" {
		head += " (" + p.NotespaceID + ")"
	}
	if strings.TrimSpace(p.Detail) == "" {
		return head
	}
	return head + ": " + p.Detail
}

// AuditPrimaries reports every way the recorded table and the stamp index
// disagree about primariness, in a deterministic order.
//
// It is the one implementation of the rule Phase 4 enforces in two places: the
// verbs check it before they create a sibling or flip a pointer, and doctor
// reports it standing. Two copies of "exactly one primary" would be two
// definitions of it, and the sibling verbs are precisely what makes the second
// notespace for a subject ordinary rather than a mistake — the invariant has to
// hold at the moment of creation, not only in the next doctor run.
func (i *Index) AuditPrimaries(primaries map[string]string) []PrimaryProblem {
	var problems []PrimaryProblem

	for _, value := range i.Subjects() {
		if primaries[value] != "" {
			continue
		}
		roots := make([]string, 0, len(i.BySubject(value)))
		for _, record := range i.BySubject(value) {
			roots = append(roots, record.Root)
		}
		problems = append(problems, PrimaryProblem{
			Kind:    PrimaryMissing,
			Subject: value,
			Detail:  fmt.Sprintf("%d stamped notespace(s) with no [primaries] entry: %s", len(roots), strings.Join(roots, ", ")),
		})
	}

	// One id recorded as the primary of several subjects is a duplicate in the
	// binding table rather than on disk: the notespace is stamped for exactly
	// one subject, so at most one of those entries can be true.
	subjectsByID := map[string][]string{}
	for value, id := range primaries {
		subjectsByID[id] = append(subjectsByID[id], value)
	}

	values := make([]string, 0, len(primaries))
	for value := range primaries {
		values = append(values, value)
	}
	sort.Strings(values)
	for _, value := range values {
		id := primaries[value]
		if others := subjectsByID[id]; len(others) > 1 {
			sorted := append([]string(nil), others...)
			sort.Strings(sorted)
			problems = append(problems, PrimaryProblem{
				Kind:        PrimaryDuplicate,
				Subject:     value,
				NotespaceID: id,
				Detail:      "recorded as the primary of " + strings.Join(sorted, ", "),
			})
			continue
		}
		records, err := i.ByID(id)
		if err != nil {
			problems = append(problems, PrimaryProblem{
				Kind:        PrimaryUnresolvable,
				Subject:     value,
				NotespaceID: id,
				Detail:      err.Error(),
			})
			continue
		}
		switch {
		case len(records) == 0:
			problems = append(problems, PrimaryProblem{
				Kind:        PrimaryDangling,
				Subject:     value,
				NotespaceID: id,
				Detail:      "no stamped root on this machine carries this id",
			})
		case records[0].Stamp.Subject != value:
			problems = append(problems, PrimaryProblem{
				Kind:        PrimaryMismatched,
				Subject:     value,
				NotespaceID: id,
				Detail:      fmt.Sprintf("%s is stamped for subject %s", records[0].Root, records[0].Stamp.Subject),
			})
		}
	}
	return problems
}
