package forge_test

import (
	"errors"
	"testing"

	"github.com/grovetools/core/pkg/forge"
)

// TestRollupPrecedence pins the merge rule. It is the single place where
// "unknown never becomes green" is decided for every surface downstream.
func TestRollupPrecedence(t *testing.T) {
	c := func(s forge.CheckState) forge.Check { return forge.Check{Name: string(s), State: s} }

	cases := []struct {
		name   string
		checks []forge.Check
		want   forge.CheckState
	}{
		{"empty is none", nil, forge.CheckStateNone},
		{"all success", []forge.Check{c(forge.CheckStateSuccess), c(forge.CheckStateSuccess)}, forge.CheckStateSuccess},
		{"failure wins over everything", []forge.Check{
			c(forge.CheckStateSuccess), c(forge.CheckStatePending),
			c(forge.CheckStateUnknown), c(forge.CheckStateFailure),
		}, forge.CheckStateFailure},
		{"unknown beats pending", []forge.Check{c(forge.CheckStatePending), c(forge.CheckStateUnknown)}, forge.CheckStateUnknown},
		{"unknown beats success", []forge.Check{c(forge.CheckStateSuccess), c(forge.CheckStateUnknown)}, forge.CheckStateUnknown},
		{"pending beats success", []forge.Check{c(forge.CheckStateSuccess), c(forge.CheckStatePending)}, forge.CheckStatePending},
		{"neutral beats success", []forge.Check{c(forge.CheckStateSuccess), c(forge.CheckStateNeutral)}, forge.CheckStateNeutral},
		{"pending beats neutral", []forge.Check{c(forge.CheckStateNeutral), c(forge.CheckStatePending)}, forge.CheckStatePending},
		{"zero-valued check is unknown, not green", []forge.Check{c(forge.CheckStateSuccess), {Name: "unset"}}, forge.CheckStateUnknown},
		{"novel state is unknown, not ignored", []forge.Check{c(forge.CheckStateSuccess), {Name: "x", State: "invented_by_a_future_forge"}}, forge.CheckStateUnknown},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := forge.RollupState(tc.checks)
			if got != tc.want {
				t.Errorf("RollupState = %q, want %q", got, tc.want)
			}
			if tc.want != forge.CheckStateSuccess && got.IsGreen() {
				t.Errorf("RollupState = %q reports IsGreen", got)
			}
		})
	}
}

func TestCheckStateIsGreen(t *testing.T) {
	green := []forge.CheckState{forge.CheckStateSuccess}
	notGreen := []forge.CheckState{
		"", forge.CheckStateUnknown, forge.CheckStateNone, forge.CheckStatePending,
		forge.CheckStateFailure, forge.CheckStateNeutral, "anything_else",
	}
	for _, s := range green {
		if !s.IsGreen() {
			t.Errorf("%q.IsGreen() = false", s)
		}
	}
	for _, s := range notGreen {
		if s.IsGreen() {
			t.Errorf("%q.IsGreen() = true; only success is green", s)
		}
	}
	if forge.CheckState("").Normalized() != forge.CheckStateUnknown {
		t.Error("the zero CheckState must normalize to unknown")
	}
}

func TestParsePRState(t *testing.T) {
	cases := map[string]forge.PRState{
		"open":         forge.PRStateOpen,
		"OPEN":         forge.PRStateOpen,
		"  Closed  ":   forge.PRStateClosed,
		"MERGED":       forge.PRStateMerged,
		"":             forge.PRStateUnknown,
		"locked":       forge.PRStateUnknown,
		"draft":        forge.PRStateUnknown,
		"super_merged": forge.PRStateUnknown,
	}
	for in, want := range cases {
		if got := forge.ParsePRState(in); got != want {
			t.Errorf("ParsePRState(%q) = %q, want %q", in, got, want)
		}
	}
	if forge.PRState("").Normalized() != forge.PRStateUnknown {
		t.Error("the zero PRState must normalize to unknown")
	}
}

func TestErrorClassification(t *testing.T) {
	cases := []struct {
		class    forge.ErrorClass
		sentinel error
		is       func(error) bool
	}{
		{forge.ClassRetryable, forge.ErrRetryable, forge.IsRetryable},
		{forge.ClassPermanent, forge.ErrPermanent, forge.IsPermanent},
		{forge.ClassUnavailable, forge.ErrUnavailable, forge.IsUnavailable},
		{forge.ClassUnsupported, forge.ErrUnsupported, forge.IsUnsupported},
	}

	for _, tc := range cases {
		t.Run(string(tc.class), func(t *testing.T) {
			cause := errors.New("underlying")
			err := forge.Errorf(tc.class, "github", "ListPRs", cause, "something went wrong")

			if got := forge.ClassOf(err); got != tc.class {
				t.Errorf("ClassOf = %q, want %q", got, tc.class)
			}
			if !errors.Is(err, tc.sentinel) {
				t.Errorf("errors.Is(err, %v) = false", tc.sentinel)
			}
			if !tc.is(err) {
				t.Errorf("Is%s(err) = false", tc.class)
			}
			if !errors.Is(err, cause) {
				t.Error("the underlying cause is not preserved for errors.Is")
			}
			// Classes are mutually exclusive.
			for _, other := range cases {
				if other.class != tc.class && errors.Is(err, other.sentinel) {
					t.Errorf("a %q error also matches %q", tc.class, other.class)
				}
			}
		})
	}
}

// TestClassOfFailsClosed: an error nobody classified must not be retried
// forever, and a nil error has no class at all.
func TestClassOfFailsClosed(t *testing.T) {
	if got := forge.ClassOf(nil); got != "" {
		t.Errorf("ClassOf(nil) = %q, want empty", got)
	}
	plain := errors.New("who knows")
	if got := forge.ClassOf(plain); got != forge.ClassPermanent {
		t.Errorf("ClassOf(unclassified) = %q, want %q", got, forge.ClassPermanent)
	}
	if forge.IsRetryable(plain) {
		t.Error("an unclassified error is retryable; it must fail closed")
	}
}

// TestClassOfUnwrapsNesting: wrapping must not lose the class, since errors
// cross package boundaries on their way to the poller.
func TestClassOfUnwrapsNesting(t *testing.T) {
	inner := forge.Errorf(forge.ClassRetryable, "forgejo", "Checks", nil, "throttled")
	wrapped := errors.Join(errors.New("context"), inner)
	if got := forge.ClassOf(wrapped); got != forge.ClassRetryable {
		t.Errorf("ClassOf(wrapped) = %q, want %q", got, forge.ClassRetryable)
	}
}

func TestCapabilityAbsenceIsUnknown(t *testing.T) {
	var nilCaps forge.Capabilities
	if got := nilCaps.Get(forge.CapListPRs); got != forge.SupportUnknown {
		t.Errorf("nil Capabilities.Get = %q, want %q", got, forge.SupportUnknown)
	}
	if nilCaps.Supports(forge.CapListPRs) {
		t.Error("nil Capabilities.Supports = true")
	}

	caps := forge.Capabilities{
		forge.CapListPRs:   forge.SupportSupported,
		forge.CapMutations: forge.SupportUnsupported,
		forge.CapIssues:    "", // explicitly stored empty
	}
	if got := caps.Get(forge.CapChecks); got != forge.SupportUnknown {
		t.Errorf("absent capability = %q, want %q", got, forge.SupportUnknown)
	}
	if got := caps.Get(forge.CapIssues); got != forge.SupportUnknown {
		t.Errorf("empty-valued capability = %q, want %q", got, forge.SupportUnknown)
	}
	if got := caps.Get(forge.CapMutations); got != forge.SupportUnsupported {
		t.Errorf("mutations = %q, want %q", got, forge.SupportUnsupported)
	}
	if !caps.Supports(forge.CapListPRs) {
		t.Error("Supports(list_prs) = false")
	}

	// Clone must not alias: a caller mutating its copy cannot corrupt the
	// provider's matrix.
	clone := caps.Clone()
	clone[forge.CapListPRs] = forge.SupportUnsupported
	if caps.Get(forge.CapListPRs) != forge.SupportSupported {
		t.Error("Clone aliased the original map")
	}

	// Keys is deterministic.
	keys := caps.Keys()
	for i := 1; i < len(keys); i++ {
		if keys[i-1] >= keys[i] {
			t.Fatalf("Keys() is not sorted: %v", keys)
		}
	}
}

func TestListPROptionBounds(t *testing.T) {
	if got := (forge.ListPROptions{}).EffectiveLimit(); got != forge.MaxItems {
		t.Errorf("zero Limit = %d, want %d", got, forge.MaxItems)
	}
	if got := (forge.ListPROptions{Limit: -5}).EffectiveLimit(); got != forge.MaxItems {
		t.Errorf("negative Limit = %d, want %d", got, forge.MaxItems)
	}
	if got := (forge.ListPROptions{Limit: forge.MaxItems * 10}).EffectiveLimit(); got != forge.MaxItems {
		t.Errorf("oversized Limit = %d, want it clamped to %d", got, forge.MaxItems)
	}
	if got := (forge.ListPROptions{Limit: 7}).EffectiveLimit(); got != 7 {
		t.Errorf("Limit 7 = %d", got)
	}
	if got := (forge.ListPROptions{}).EffectiveState(); got != forge.PRStateOpen {
		t.Errorf("zero State = %q, want %q", got, forge.PRStateOpen)
	}
}

func TestRepoIdentity(t *testing.T) {
	r := forge.Repo{Host: "github.com", Owner: "grove", Name: "flow"}
	if r.Slug() != "grove/flow" {
		t.Errorf("Slug = %q", r.Slug())
	}
	if r.String() != "github.com/grove/flow" {
		t.Errorf("String = %q", r.String())
	}
	if r.IsZero() {
		t.Error("a populated repo reports IsZero")
	}
	if !(forge.Repo{}).IsZero() {
		t.Error("the zero repo does not report IsZero")
	}

	// Same slug, different host: not the same repo. This is the property the
	// host-confusion cases in the conformance suite protect.
	other := forge.Repo{Host: "github.com.evil.example", Owner: "grove", Name: "flow"}
	if r.String() == other.String() {
		t.Error("repos on different hosts share an identity")
	}
}

func TestUnknownRollup(t *testing.T) {
	r := forge.UnknownRollup("abc123")
	if r.State.IsGreen() {
		t.Error("UnknownRollup is green")
	}
	if r.State != forge.CheckStateUnknown || r.Ref != "abc123" {
		t.Errorf("UnknownRollup = %+v", r)
	}
}
