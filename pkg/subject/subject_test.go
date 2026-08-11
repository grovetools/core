package subject

import (
	"strings"
	"testing"
)

func TestCanonicalRemote(t *testing.T) {
	tests := []struct{ in, want string }{
		{"git@GitHub.COM:Me/Core.git", "github.com/Me/Core"},
		{"ssh://git@GitHub.COM/Me/Core.git", "github.com/Me/Core"},
		{"https://github.com/Me/Core.git/", "github.com/Me/Core"},
		{"https://User@GitLab.COM/Group/Sub/Repo", "gitlab.com/Group/Sub/Repo"},
		{"ssh://git@example.com:2222/Team/Repo.git", "example.com/2222/Team/Repo"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := CanonicalRemote(tt.in)
			if err != nil || got.String() != tt.want {
				t.Fatalf("CanonicalRemote(%q) = %q, %v; want %q", tt.in, got, err, tt.want)
			}
		})
	}
}

func TestRemoteSelectionIsExplicit(t *testing.T) {
	origin := []Remote{{Name: "backup", URL: "git@example.com:backup/repo.git"}, {Name: "origin", URL: "git@github.com:Me/Core.git"}}
	got, selected, err := FromRemotes(origin)
	if err != nil || selected != SelectionOrigin || got != "github.com/Me/Core" {
		t.Fatalf("origin selection = %q %q %v", got, selected, err)
	}
	got, selected, err = FromRemotes([]Remote{{Name: "upstream", URL: "git@example.com:Team/Repo"}})
	if err != nil || selected != SelectionSole || got != "example.com/Team/Repo" {
		t.Fatalf("sole selection = %q %q %v", got, selected, err)
	}
	got, selected, err = FromRemotes(nil)
	if err != nil || selected != SelectionNone || got != "" {
		t.Fatalf("no-remote selection = %q %q %v", got, selected, err)
	}
	if _, _, err := FromRemotes([]Remote{{Name: "a", URL: "git@a:x/y"}, {Name: "b", URL: "git@b:x/y"}}); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("multiple non-origin remotes were not rejected: %v", err)
	}
}

func TestRecordedSubjectFamilies(t *testing.T) {
	const id = "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	for _, fn := range []func(string) (Value, error){Ecosystem, Local} {
		got, err := fn(id)
		if err != nil || Validate(got.String()) != nil {
			t.Fatalf("recorded subject = %q, %v", got, err)
		}
	}
	local := MintLocal()
	if !strings.HasPrefix(local.String(), LocalPrefix) || Validate(local.String()) != nil {
		t.Fatalf("MintLocal = %q", local)
	}
}
