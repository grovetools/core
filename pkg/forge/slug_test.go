package forge_test

import (
	"strings"
	"testing"

	"github.com/grovetools/core/pkg/forge"
)

// TestValidateRef pins what may be interpolated into an argv or a URL path.
func TestValidateRef(t *testing.T) {
	ok := []string{
		"main",
		"refs/heads/main",
		"feature/forge-provider",
		"deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		"v1.2.3",
		"release-2026.08",
	}
	for _, ref := range ok {
		if err := forge.ValidateRef(ref); err != nil {
			t.Errorf("ValidateRef(%q) = %v, want nil", ref, err)
		}
	}

	bad := []string{
		"",
		"   ",
		"-oProxyCommand=curl evil",
		"--upload-pack=evil",
		"../../../etc/passwd",
		"refs/heads/../../secret",
		"refs//heads",
		"refs/heads/.",
		"main branch",
		"main\nrm -rf /",
		"main\x00",
		"main?",
		"main*",
		"main[1]",
		"main^",
		"main~1",
		`back\slash`,
		strings.Repeat("a", 513),
	}
	for _, ref := range bad {
		err := forge.ValidateRef(ref)
		if err == nil {
			t.Errorf("ValidateRef(%q) = nil, want an error", ref)
			continue
		}
		if cls := forge.ClassOf(err); cls != forge.ClassPermanent {
			t.Errorf("ValidateRef(%q) class = %q, want %q", ref, cls, forge.ClassPermanent)
		}
	}
}

// TestParseRemoteURLLongSegments covers the length bound separately from the
// shared conformance table.
func TestParseRemoteURLLongSegments(t *testing.T) {
	long := strings.Repeat("a", 129)
	if _, err := forge.ParseRemoteURL("https://github.com/" + long + "/flow"); err == nil {
		t.Error("an over-long owner was accepted")
	}
	if _, err := forge.ParseRemoteURL("https://github.com/grove/" + long); err == nil {
		t.Error("an over-long repository name was accepted")
	}
	ok := strings.Repeat("a", 128)
	if _, err := forge.ParseRemoteURL("https://github.com/grove/" + ok); err != nil {
		t.Errorf("a 128-character name was rejected: %v", err)
	}
}

// TestParseRemoteURLErrorsQuoteTheInput keeps diagnostics actionable: the
// operator needs to see which remote failed.
func TestParseRemoteURLErrorsQuoteTheInput(t *testing.T) {
	_, err := forge.ParseRemoteURL("https://github.com/-x/flow")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "-x") {
		t.Errorf("error does not mention the offending input: %v", err)
	}
}

// TestParseRemoteURLIsPure: the same input always yields the same identity,
// with no network and no filesystem involved.
func TestParseRemoteURLIsPure(t *testing.T) {
	const in = "git@github.com:grove/flow.git"
	first, err := forge.ParseRemoteURL(in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := forge.ParseRemoteURL(in)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Errorf("ParseRemoteURL is not deterministic: %+v vs %+v", first, second)
	}
}

// TestScpLikeAmbiguityRejected: "host:2222/owner/repo" is an ssh URL missing
// its scheme, not a repo owned by "2222". Guessing either way is wrong, so it
// is refused.
func TestScpLikeAmbiguityRejected(t *testing.T) {
	if _, err := forge.ParseRemoteURL("git@forge.example.com:2222/grove/flow.git"); err == nil {
		t.Error("an ambiguous scp-like URL with a numeric first segment was accepted")
	}
	// The unambiguous ssh:// form of the same thing is fine.
	got, err := forge.ParseRemoteURL("ssh://git@forge.example.com:2222/grove/flow.git")
	if err != nil {
		t.Fatalf("ssh:// with a port failed: %v", err)
	}
	if got.Host != "forge.example.com" || got.Slug() != "grove/flow" {
		t.Errorf("got %+v", got)
	}
}
