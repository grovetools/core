package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fleetLabManifest is the exact shape of the real Machine A ecosystem manifest
// this derivation was written for: an ecosystem whose identity lives only in
// its card, with no git remotes at the root at all.
const fleetLabManifest = `name = "fleet-lab"
workspaces = ["*"]

[ecosystem]
id = "01KZME1YNY6Q0XEVTBNWQ72P1Z"
layout = "flat"

[[ecosystem.remotes]]
name = "repo-a"
url = "https://forge.matthew.solar/matt/fleet-lab-repo-a.git"

[[ecosystem.remotes]]
name = "repo-b"
url = "https://forge.matthew.solar/matt/fleet-lab-repo-b.git"

[ecosystem.notebooks.fleet-lab-notes]
default = true
`

func subjectSandbox(t *testing.T) {
	t.Helper()
	t.Setenv("GROVE_HOME", t.TempDir())
}

func writeTree(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitRepo(t *testing.T, dir string, remotes ...[2]string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("init", "-q")
	for _, remote := range remotes {
		run("remote", "add", remote[0], remote[1])
	}
	return dir
}

func TestSubjectForCodeRootDerivesEcosystemCardIdentity(t *testing.T) {
	subjectSandbox(t)
	root := t.TempDir()
	writeTree(t, filepath.Join(root, "grove.toml"), fleetLabManifest)

	got, err := SubjectForCodeRoot(root, nil)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if got.Value.String() != "eco:01KZME1YNY6Q0XEVTBNWQ72P1Z" {
		t.Fatalf("subject = %q, want eco:01KZME1YNY6Q0XEVTBNWQ72P1Z", got.Value)
	}
	if got.Source != CodeRootSubjectEcosystem {
		t.Fatalf("source = %q, want %q", got.Source, CodeRootSubjectEcosystem)
	}
	if got.Manifest != filepath.Join(root, "grove.toml") {
		t.Fatalf("manifest evidence = %q", got.Manifest)
	}

	// Deriving twice is the property the whole fix exists for: undo and reapply
	// must land on the same subject, because [primaries] is keyed by it.
	again, err := SubjectForCodeRoot(root, nil)
	if err != nil || again.Value != got.Value {
		t.Fatalf("second derivation = %+v err=%v, want %q", again, err, got.Value)
	}
}

func TestSubjectForCodeRootEcosystemCardOutranksRemotes(t *testing.T) {
	subjectSandbox(t)
	root := gitRepo(t, t.TempDir(), [2]string{"origin", "git@github.com:grovetools/fleet-lab.git"})
	writeTree(t, filepath.Join(root, "grove.toml"), fleetLabManifest)

	got, err := SubjectForCodeRoot(root, nil)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if got.Source != CodeRootSubjectEcosystem || got.Value.String() != "eco:01KZME1YNY6Q0XEVTBNWQ72P1Z" {
		t.Fatalf("card did not outrank the remote: %+v", got)
	}
}

func TestSubjectForCodeRootRefusesUnusableEcosystemIdentity(t *testing.T) {
	for _, tc := range []struct {
		name     string
		manifest string
		want     string
	}{
		{
			name:     "malformed id",
			manifest: "name = \"broken\"\n\n[ecosystem]\nid = \"not-a-ulid\"\n",
			want:     "invalid ULID",
		},
		{
			name:     "id of the right length but not base32",
			manifest: "name = \"broken\"\n\n[ecosystem]\nid = \"01KZME1YNY6Q0XEVTBNWQ72PIU\"\n",
			want:     "invalid ULID",
		},
		{
			name:     "un-minted card",
			manifest: "name = \"unminted\"\n\n[ecosystem]\nlayout = \"flat\"\n",
			want:     "no id",
		},
		{
			name:     "whitespace-padded id",
			manifest: "name = \"padded\"\n\n[ecosystem]\nid = \" 01KZME1YNY6Q0XEVTBNWQ72P1Z\"\n",
			want:     "invalid ULID",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			subjectSandbox(t)
			root := t.TempDir()
			writeTree(t, filepath.Join(root, "grove.toml"), tc.manifest)

			got, err := SubjectForCodeRoot(root, nil)
			if err == nil {
				t.Fatalf("unusable ecosystem identity silently became %+v", got)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q does not explain %q", err, tc.want)
			}
		})
	}
}

func TestSubjectForCodeRootRefusesUnusableIdentityEvenWithFallbacks(t *testing.T) {
	// A card that cannot be honoured must not quietly degrade to the remote or
	// to a recorded local subject: both would silently be a different identity.
	subjectSandbox(t)
	root := gitRepo(t, t.TempDir(), [2]string{"origin", "https://github.com/grovetools/broken.git"})
	writeTree(t, filepath.Join(root, "grove.toml"), "[ecosystem]\nid = \"not-a-ulid\"\n")

	recorded := map[string]string{root: "local:01ARZ3NDEKTSV4RRFFQ69G5FAV"}
	if got, err := SubjectForCodeRoot(root, recorded); err == nil {
		t.Fatalf("unusable card fell through to %+v", got)
	}
}

func TestSubjectForCodeRootManifestWithoutCardIsNotAnEcosystem(t *testing.T) {
	// Every repo in the ecosystem carries a grove.toml. Only the [ecosystem]
	// table makes one an identity claim; without it the repo rule applies.
	subjectSandbox(t)
	root := gitRepo(t, t.TempDir(), [2]string{"origin", "https://github.com/grovetools/core.git"})
	writeTree(t, filepath.Join(root, "grove.toml"), "name = \"core\"\nversion = \"1.0\"\n")

	got, err := SubjectForCodeRoot(root, nil)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if got.Source != CodeRootSubjectRemote || got.Value.String() != "github.com/grovetools/core" {
		t.Fatalf("got %+v, want the canonical remote subject", got)
	}
}

func TestSubjectForCodeRootRemoteSelectionAndShape(t *testing.T) {
	for _, tc := range []struct {
		name    string
		remotes [][2]string
		want    string
		wantErr string
	}{
		{
			name:    "origin outranks other remotes",
			remotes: [][2]string{{"upstream", "https://github.com/grovetools/upstream.git"}, {"origin", "https://github.com/grovetools/core.git"}},
			want:    "github.com/grovetools/core",
		},
		{
			name:    "sole non-origin remote",
			remotes: [][2]string{{"fork", "https://forge.matthew.solar/matt/fleet-lab-repo-a.git"}},
			want:    "forge.matthew.solar/matt/fleet-lab-repo-a",
		},
		{
			name:    "scp form canonicalizes to host/path",
			remotes: [][2]string{{"origin", "git@GitHub.com:grovetools/Core.git"}},
			want:    "github.com/grovetools/Core",
		},
		{
			name:    "trailing slash and no .git suffix",
			remotes: [][2]string{{"origin", "https://forge.matthew.solar/matt/fleet-lab-repo-b/"}},
			want:    "forge.matthew.solar/matt/fleet-lab-repo-b",
		},
		{
			name:    "two non-origin remotes are ambiguous, never sorted",
			remotes: [][2]string{{"a", "https://github.com/grovetools/a.git"}, {"b", "https://github.com/grovetools/b.git"}},
			wantErr: "ambiguous",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			subjectSandbox(t)
			root := gitRepo(t, t.TempDir(), tc.remotes...)

			got, err := SubjectForCodeRoot(root, nil)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("got %+v err=%v, want an error mentioning %q", got, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("derive: %v", err)
			}
			if got.Source != CodeRootSubjectRemote || got.Value.String() != tc.want {
				t.Fatalf("got %+v, want %q", got, tc.want)
			}
		})
	}
}

func TestSubjectForCodeRootTrueLocalRootRecordsOrDefers(t *testing.T) {
	subjectSandbox(t)
	root := gitRepo(t, t.TempDir())

	got, err := SubjectForCodeRoot(root, nil)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if got.Source != CodeRootSubjectNone || got.Value != "" {
		t.Fatalf("a remoteless root minted %+v instead of deferring to its caller", got)
	}

	const recordedValue = "local:01ARZ3NDEKTSV4RRFFQ69G5FAV"
	got, err = SubjectForCodeRoot(root, map[string]string{root: recordedValue})
	if err != nil {
		t.Fatalf("derive with record: %v", err)
	}
	if got.Source != CodeRootSubjectRecorded || got.Value.String() != recordedValue {
		t.Fatalf("got %+v, want the recorded %q", got, recordedValue)
	}
}

func TestSubjectForCodeRootRejectsUnusableRecord(t *testing.T) {
	subjectSandbox(t)
	root := gitRepo(t, t.TempDir())

	if got, err := SubjectForCodeRoot(root, map[string]string{root: "local:not-a-ulid"}); err == nil {
		t.Fatalf("a corrupt record silently became %+v", got)
	}
}

func TestSubjectForCodeRootDoesNotInheritAnEnclosingRepository(t *testing.T) {
	subjectSandbox(t)
	repo := gitRepo(t, t.TempDir(), [2]string{"origin", "https://github.com/grovetools/core.git"})
	nested := filepath.Join(repo, "sub", "tree")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := SubjectForCodeRoot(nested, nil)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if got.Source != CodeRootSubjectNone {
		t.Fatalf("nested path inherited the enclosing repository's identity: %+v", got)
	}
}

func TestSubjectForCodeRootRejectsEmptyRoot(t *testing.T) {
	subjectSandbox(t)
	if got, err := SubjectForCodeRoot("  ", nil); err == nil {
		t.Fatalf("empty root derived %+v", got)
	}
}
