// Package subject owns the canonical identity shared by code roots and notespaces.
// Subject values are recorded facts: callers must never derive a fallback by sorting
// remotes or local paths.
package subject

import (
	"crypto/rand"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

const (
	EcosystemPrefix = "eco:"
	LocalPrefix     = "local:"
)

// Value is a canonical subject. Its string representation is the wire/config form.
type Value string

func (v Value) String() string { return string(v) }

// Remote is one named git remote.
type Remote struct {
	Name string
	URL  string
}

// Selection explains which explicit remote rule produced a subject.
type Selection string

const (
	SelectionOrigin Selection = "origin"
	SelectionSole   Selection = "sole"
	SelectionNone   Selection = "none"
)

// FromRemotes selects origin when present, otherwise the sole remote. No remote
// is an explicit empty result. Multiple non-origin remotes are ambiguous and are
// never sorted or guessed.
func FromRemotes(remotes []Remote) (Value, Selection, error) {
	var origin *Remote
	for i := range remotes {
		if remotes[i].Name == "origin" {
			if origin != nil {
				return "", "", fmt.Errorf("multiple origin remotes")
			}
			origin = &remotes[i]
		}
	}
	if origin != nil {
		v, err := CanonicalRemote(origin.URL)
		return v, SelectionOrigin, err
	}
	switch len(remotes) {
	case 0:
		return "", SelectionNone, nil
	case 1:
		v, err := CanonicalRemote(remotes[0].URL)
		return v, SelectionSole, err
	default:
		return "", "", fmt.Errorf("remote selection is ambiguous: no origin among %d remotes", len(remotes))
	}
}

// CanonicalRemote implements the repository subject contract: strip scheme and
// user, lowercase the host, convert the SCP separator to '/', trim trailing '/'
// and '.git', and preserve repository-path case.
func CanonicalRemote(raw string) (Value, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("remote URL is empty")
	}

	var host, path string
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return "", fmt.Errorf("invalid remote URL %q", raw)
		}
		if u.RawQuery != "" || u.Fragment != "" {
			return "", fmt.Errorf("remote URL %q contains query or fragment", raw)
		}
		host = u.Hostname()
		if port := u.Port(); port != "" {
			host += "/" + port
		}
		path = u.EscapedPath()
		if decoded, err := url.PathUnescape(path); err == nil {
			path = decoded
		}
	} else {
		// SCP-like form: [user@]host:path. Canonical host/path input is also
		// accepted so Validate can prove an already-recorded value is stable.
		left, right, ok := strings.Cut(raw, ":")
		if ok && left != "" && right != "" && !strings.Contains(left, "/") {
			if at := strings.LastIndex(left, "@"); at >= 0 {
				left = left[at+1:]
			}
			host, path = left, right
		} else if first, rest, found := strings.Cut(raw, "/"); found && first != "" && rest != "" && !strings.Contains(first, "@") {
			host, path = first, rest
		} else {
			return "", fmt.Errorf("remote %q is neither a URL nor SCP-like", raw)
		}
	}
	host = strings.ToLower(strings.TrimSpace(host))
	path = strings.Trim(path, "/")
	path = strings.TrimSuffix(path, ".git")
	path = strings.TrimSuffix(path, "/")
	if host == "" || path == "" || strings.ContainsAny(host+path, "\r\n\t") {
		return "", fmt.Errorf("remote %q has no canonical host/path", raw)
	}
	return Value(host + "/" + path), nil
}

// Ecosystem returns the subject for a recorded ecosystem ULID.
func Ecosystem(id string) (Value, error) { return prefixed(EcosystemPrefix, id) }

// Local returns a machine-local subject for a recorded ULID.
func Local(id string) (Value, error) { return prefixed(LocalPrefix, id) }

// MintLocal mints a new machine-local subject. Calling it is an explicit
// materialization action; it is intentionally not part of remote selection.
func MintLocal() Value {
	return Value(LocalPrefix + ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader).String())
}

func prefixed(prefix, id string) (Value, error) {
	if _, err := ulid.ParseStrict(id); err != nil {
		return "", fmt.Errorf("invalid ULID %q: %w", id, err)
	}
	return Value(prefix + id), nil
}

// Validate checks the three canonical subject families without deriving one.
func Validate(value string) error {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\r\n\t") {
		return fmt.Errorf("invalid empty or whitespace subject %q", value)
	}
	for _, prefix := range []string{EcosystemPrefix, LocalPrefix} {
		if strings.HasPrefix(value, prefix) {
			_, err := prefixed(prefix, strings.TrimPrefix(value, prefix))
			return err
		}
	}
	canonical, err := CanonicalRemote(value)
	if err != nil {
		return fmt.Errorf("invalid repository subject %q: %w", value, err)
	}
	if canonical.String() != value {
		return fmt.Errorf("repository subject %q is not canonical (want %q)", value, canonical)
	}
	return nil
}
