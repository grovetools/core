package workspace

import (
	"bufio"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/subject"
	"github.com/grovetools/core/util/pathutil"
)

// CodeRootSubjectSource names the rule that produced a code root's subject. It
// travels with the value so a caller can put the reason in a receipt instead of
// re-deriving it.
type CodeRootSubjectSource string

const (
	// CodeRootSubjectEcosystem: the root carries an ecosystem card whose minted
	// id is the identity (eco:<ULID>).
	CodeRootSubjectEcosystem CodeRootSubjectSource = "ecosystem"
	// CodeRootSubjectRemote: the root is a repository and its canonical remote
	// selection is the identity.
	CodeRootSubjectRemote CodeRootSubjectSource = "remote"
	// CodeRootSubjectRecorded: the machine already records a local subject for
	// this exact path.
	CodeRootSubjectRecorded CodeRootSubjectSource = "recorded"
	// CodeRootSubjectNone: nothing recorded answers for this root. The value is
	// empty and the caller decides whether to mint.
	CodeRootSubjectNone CodeRootSubjectSource = "none"
)

// CodeRootSubject is a derived subject plus the evidence that produced it.
type CodeRootSubject struct {
	Value  subject.Value
	Source CodeRootSubjectSource
	// Manifest is the ecosystem manifest that carried the card, set only for
	// CodeRootSubjectEcosystem.
	Manifest string
	// Selection is the remote rule that applied, set only for
	// CodeRootSubjectRemote.
	Selection subject.Selection
}

// SubjectForCodeRoot derives the authoritative subject of one explicit code
// root. Every answer is a fact recorded in or about that exact tree — never a
// directory name, a sibling, or a sort order — in the settled precedence:
//
//	an ecosystem card's minted id       -> eco:<ULID>
//	canonical git remote selection      -> <host>/<path>
//	a machine-recorded local subject    -> local:<ULID>
//
// A root that matches none of them yields CodeRootSubjectNone with an empty
// value: minting the local subject that would close that case is an explicit
// materialization the caller performs and records, not something derivation
// does behind its back.
//
// The ecosystem card comes first because it is the only identity that survives
// a clone onto another machine. That also makes it the one claim that may not
// silently degrade: a card present but un-minted or malformed is an error here,
// because falling through to a remote — or worse, to a freshly minted local
// subject — would make the same tree answer to a different subject on the next
// run, and [primaries] is keyed by subject.
func SubjectForCodeRoot(root string, recorded map[string]string) (CodeRootSubject, error) {
	if strings.TrimSpace(root) == "" {
		return CodeRootSubject{}, fmt.Errorf("cannot derive a subject for an empty code root")
	}
	root = filepath.Clean(root)

	if manifest := config.FindEcosystemManifest(root); manifest != "" {
		card, err := config.LoadEcosystemCard(manifest)
		if err != nil {
			return CodeRootSubject{}, fmt.Errorf("read ecosystem identity from %s: %w", manifest, err)
		}
		if card != nil {
			if strings.TrimSpace(card.ID) == "" {
				return CodeRootSubject{}, fmt.Errorf("%s declares an [ecosystem] card with no id; mint one with 'grove ecosystem adopt' before deriving a subject for %s", manifest, root)
			}
			value, err := subject.Ecosystem(card.ID)
			if err != nil {
				return CodeRootSubject{}, fmt.Errorf("%s declares ecosystem id %q: %w", manifest, card.ID, err)
			}
			return CodeRootSubject{Value: value, Source: CodeRootSubjectEcosystem, Manifest: manifest}, nil
		}
	}

	remotes, err := gitRemotesAtRoot(root)
	if err != nil {
		return CodeRootSubject{}, err
	}
	value, selection, err := subject.FromRemotes(remotes)
	if err != nil {
		return CodeRootSubject{}, fmt.Errorf("select subject for %s: %w", root, err)
	}
	if selection != subject.SelectionNone {
		return CodeRootSubject{Value: value, Source: CodeRootSubjectRemote, Selection: selection}, nil
	}

	if value := recordedSubjectFor(root, recorded); value != "" {
		if err := subject.Validate(value); err != nil {
			return CodeRootSubject{}, fmt.Errorf("recorded subject for %s: %w", root, err)
		}
		return CodeRootSubject{Value: subject.Value(value), Source: CodeRootSubjectRecorded}, nil
	}
	return CodeRootSubject{Source: CodeRootSubjectNone}, nil
}

// recordedSubjectFor looks up a machine-recorded subject for root, retrying
// once through the canonical path so a recorded entry written for the resolved
// spelling still answers for a symlinked or relative one.
func recordedSubjectFor(root string, recorded map[string]string) string {
	if len(recorded) == 0 {
		return ""
	}
	if value := recorded[root]; value != "" {
		return value
	}
	canonical, err := pathutil.CanonicalPath(root)
	if err != nil || canonical == root {
		return ""
	}
	return recorded[canonical]
}

// gitRemotesAtRoot lists the remotes of the repository whose top level is
// exactly root. A directory that is not a repository root — notably one that
// merely sits inside one — owns no remotes, and saying so beats inheriting the
// enclosing repository's identity.
func gitRemotesAtRoot(root string) ([]subject.Remote, error) {
	out, err := exec.Command("git", "-C", root, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return nil, nil
	}
	if same, cmpErr := pathutil.ComparePaths(strings.TrimSpace(string(out)), root); cmpErr != nil || !same {
		return nil, nil
	}
	return gitRemotesIn(root)
}

// gitRemotesIn reads root's configured remotes. `git config --get-regexp`
// exits 1 when nothing matches, which is "this repository has no remote" — an
// explicit empty result, not a failure.
func gitRemotesIn(root string) ([]subject.Remote, error) {
	out, err := exec.Command("git", "-C", root, "config", "--get-regexp", `^remote\..*\.url$`).Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("read git remotes for %s: %w", root, err)
	}
	return parseGitRemoteURLs(out)
}

func parseGitRemoteURLs(out []byte) ([]subject.Remote, error) {
	var remotes []subject.Remote
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		key, url, ok := strings.Cut(scanner.Text(), " ")
		if !ok {
			key, url, ok = strings.Cut(scanner.Text(), "\t")
		}
		parts := strings.Split(key, ".")
		if !ok || len(parts) < 3 || strings.TrimSpace(url) == "" {
			continue
		}
		remotes = append(remotes, subject.Remote{Name: parts[1], URL: strings.TrimSpace(url)})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return remotes, nil
}
