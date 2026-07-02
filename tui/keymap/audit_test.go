package keymap

import (
	"testing"

	"github.com/charmbracelet/bubbles/key"
)

// cleanFixture embeds Base and exposes all seven Base section getters plus a
// custom section for its own field. It should produce zero gaps.
type cleanFixture struct {
	Base
	ViewGit key.Binding
}

func newCleanFixture() cleanFixture {
	return cleanFixture{
		Base: DefaultVim(),
		ViewGit: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "view git"),
		),
	}
}

func (f cleanFixture) Sections() []Section {
	return []Section{
		f.NavigationSection(),
		f.ActionsSection(),
		f.SearchSection(),
		f.SelectionSection(),
		f.ViewSection(),
		f.FoldSection(),
		f.SystemSection(),
		NewSection("Git", f.ViewGit),
	}
}

// gapFixture contains one field per gap kind, plus fields that must be skipped.
type gapFixture struct {
	Base
	Orphan   key.Binding // enabled, has keys, in no section -> missing-from-sections
	BadLabel key.Binding // help label "gg" but key is "g" -> help-key-mismatch
	NoDesc   key.Binding // keys but empty help desc -> empty-help
	Disabled key.Binding // disabled orphan -> skipped
	NoKeys   key.Binding // zero-value binding, no keys -> skipped
}

func newGapFixture() gapFixture {
	disabled := key.NewBinding(
		key.WithKeys("D"),
		key.WithHelp("D", "disabled action"),
	)
	disabled.SetEnabled(false)
	return gapFixture{
		Base: DefaultVim(),
		Orphan: key.NewBinding(
			key.WithKeys("O"),
			key.WithHelp("O", "orphan action"),
		),
		BadLabel: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("gg", "go to top"),
		),
		NoDesc: key.NewBinding(
			key.WithKeys("ctrl+q"),
			key.WithHelp("ctrl+q", ""),
		),
		Disabled: disabled,
	}
}

func (f gapFixture) Sections() []Section {
	return []Section{
		f.NavigationSection(),
		f.ActionsSection(),
		f.SearchSection(),
		f.SelectionSection(),
		f.ViewSection(),
		f.FoldSection(),
		f.SystemSection(),
		// BadLabel and NoDesc ARE in a section so only their specific gap fires.
		NewSection("Custom", f.BadLabel, f.NoDesc),
	}
}

func gapsFor(gaps []Gap, field string) []Gap {
	var out []Gap
	for _, g := range gaps {
		if g.Field == field {
			out = append(out, g)
		}
	}
	return out
}

func hasGap(gaps []Gap, field, kind string) bool {
	for _, g := range gaps {
		if g.Field == field && g.Kind == kind {
			return true
		}
	}
	return false
}

func TestAuditCoverage_CleanFixtureHasNoGaps(t *testing.T) {
	gaps := AuditCoverage(newCleanFixture())
	if len(gaps) != 0 {
		t.Errorf("expected zero gaps for clean fixture, got %d: %+v", len(gaps), gaps)
	}
}

// TestAuditCoverage_BaseGettersCoverBase asserts the fixed Base section
// getters produce zero "missing-from-sections" gaps for a keymap whose
// Sections() returns all seven Base getters.
func TestAuditCoverage_BaseGettersCoverBase(t *testing.T) {
	gaps := AuditCoverage(newCleanFixture())
	for _, g := range gaps {
		if g.Kind == GapMissingFromSections {
			t.Errorf("Base field in no section: %+v", g)
		}
	}
}

func TestAuditCoverage_MissingFromSections(t *testing.T) {
	gaps := AuditCoverage(newGapFixture())
	if !hasGap(gaps, "Orphan", GapMissingFromSections) {
		t.Errorf("expected missing-from-sections gap for Orphan, got %+v", gaps)
	}
}

func TestAuditCoverage_HelpKeyMismatch(t *testing.T) {
	gaps := AuditCoverage(newGapFixture())
	if !hasGap(gaps, "BadLabel", GapHelpKeyMismatch) {
		t.Errorf("expected help-key-mismatch gap for BadLabel, got %+v", gaps)
	}
}

func TestAuditCoverage_EmptyHelp(t *testing.T) {
	gaps := AuditCoverage(newGapFixture())
	if !hasGap(gaps, "NoDesc", GapEmptyHelp) {
		t.Errorf("expected empty-help gap for NoDesc, got %+v", gaps)
	}
}

func TestAuditCoverage_SkipsDisabledAndKeyless(t *testing.T) {
	gaps := AuditCoverage(newGapFixture())
	if g := gapsFor(gaps, "Disabled"); len(g) != 0 {
		t.Errorf("disabled binding must be skipped, got %+v", g)
	}
	if g := gapsFor(gaps, "NoKeys"); len(g) != 0 {
		t.Errorf("keyless binding must be skipped, got %+v", g)
	}
}

func TestAuditCoverage_ExpectedGapCount(t *testing.T) {
	// The dirty fixture should report exactly the three intended gaps —
	// nothing from Base (all getters included) and nothing from the
	// skipped fields.
	gaps := AuditCoverage(newGapFixture())
	if len(gaps) != 3 {
		t.Errorf("expected exactly 3 gaps, got %d: %+v", len(gaps), gaps)
	}
}

// miniKM exercises pointer receivers and embedded-field paths: its Sections()
// only exposes navigation, so every non-navigation Base binding must be
// reported with a "Base."-prefixed field path.
type miniKM struct {
	Base
}

func (m *miniKM) Sections() []Section {
	return []Section{m.NavigationSection()}
}

func TestAuditCoverage_PointerReceiverAndEmbeddedPath(t *testing.T) {
	gaps := AuditCoverage(&miniKM{Base: DefaultVim()})

	if !hasGap(gaps, "Base.Search", GapMissingFromSections) {
		t.Errorf("expected missing-from-sections gap for Base.Search, got %+v", gaps)
	}
	// Navigation bindings are covered and must not be reported.
	for _, f := range []string{"Base.Up", "Base.Down", "Base.Home", "Base.End", "Base.Top", "Base.Bottom"} {
		if hasGap(gaps, f, GapMissingFromSections) {
			t.Errorf("%s is in NavigationSection and must not be reported", f)
		}
	}
}

// TestAuditCoverage_NoFalsePositiveLabels asserts the label heuristic accepts
// all default preset labels (abbreviations like "C-u", "space", "PgUp", "Del",
// arrows, and alternate lists like "k/up").
func TestAuditCoverage_NoFalsePositiveLabels(t *testing.T) {
	for name, base := range map[string]Base{
		"vim":    DefaultVim(),
		"emacs":  DefaultEmacs(),
		"arrows": DefaultArrows(),
	} {
		gaps := AuditCoverage(cleanFixture{Base: base, ViewGit: key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "view git"))})
		for _, g := range gaps {
			if g.Kind == GapHelpKeyMismatch {
				t.Errorf("%s preset: false-positive help-key-mismatch: %+v", name, g)
			}
		}
	}
}
