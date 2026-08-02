package forge

import (
	"fmt"
	"net/url"
	"strings"
)

// maxSegmentLen bounds owner/name length. GitHub caps both at 39/100; this is
// a loose bound whose job is to reject absurd input, not to mirror a forge.
const maxSegmentLen = 128

// ParseRemoteURL resolves a git remote URL into a Repo identity.
//
// Accepted forms:
//
//	https://host/owner/repo(.git)      http://…, with or without userinfo/port
//	ssh://git@host(:port)/owner/repo(.git)
//	git://host/owner/repo(.git)
//	git@host:owner/repo(.git)          scp-like syntax
//	host:owner/repo(.git)              scp-like without user
//
// It is deliberately strict, because the result is fed to `gh --repo <slug>`
// and interpolated into REST paths:
//
//   - Host is the lowercased hostname only — no userinfo, no port. Nothing
//     here treats "github.com.evil.example" or "evil.example/github.com" as
//     github.com; host comparison is the caller's, on an exact string.
//   - The path must be exactly two segments. Deeper paths are rejected rather
//     than truncated to the last two, so ".../legit/repo" appended to an
//     attacker-chosen prefix cannot be laundered into an identity.
//   - "." and ".." segments, empty segments, and any segment containing a
//     path separator are rejected — no traversal reaches a REST path.
//   - Segments must match [A-Za-z0-9._-]+ and must not begin with "-", which
//     would otherwise be parsed as a flag by the `gh` CLI.
//
// The returned error is always a *Error with ClassPermanent: a URL that does
// not parse will not parse later either.
func ParseRemoteURL(remoteURL string) (Repo, error) {
	raw := strings.TrimSpace(remoteURL)
	if raw == "" {
		return Repo{}, parseErr(remoteURL, "empty remote URL")
	}
	if strings.ContainsAny(raw, "\x00\n\r\t \\") {
		return Repo{}, parseErr(remoteURL, "remote URL contains illegal characters")
	}

	host, path, err := splitRemote(raw)
	if err != nil {
		return Repo{}, err
	}

	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" {
		return Repo{}, parseErr(remoteURL, "remote URL has no host")
	}
	if strings.ContainsAny(host, "/@:") {
		return Repo{}, parseErr(remoteURL, "malformed host %q", host)
	}

	path = strings.Trim(path, "/")
	path = strings.TrimSuffix(path, ".git")
	path = strings.Trim(path, "/")

	segs := strings.Split(path, "/")
	if len(segs) != 2 {
		return Repo{}, parseErr(remoteURL, "expected exactly owner/repo, got %d path segment(s)", len(segs))
	}
	owner, name := segs[0], segs[1]
	if err := validateSegment(remoteURL, "owner", owner); err != nil {
		return Repo{}, err
	}
	if err := validateSegment(remoteURL, "repository", name); err != nil {
		return Repo{}, err
	}

	return Repo{Host: host, Owner: owner, Name: name}, nil
}

// splitRemote separates a remote URL into hostname and path, handling both
// URL and scp-like syntax.
func splitRemote(raw string) (host, path string, err error) {
	if hasScheme(raw) {
		u, perr := url.Parse(raw)
		if perr != nil {
			return "", "", parseErr(raw, "unparseable remote URL: %v", perr)
		}
		switch strings.ToLower(u.Scheme) {
		case "https", "http", "ssh", "git", "git+ssh":
		default:
			return "", "", parseErr(raw, "unsupported remote URL scheme %q", u.Scheme)
		}
		if u.Opaque != "" {
			return "", "", parseErr(raw, "opaque remote URL")
		}
		// u.Hostname() drops userinfo and port for us.
		return u.Hostname(), u.EscapedPath(), nil
	}

	// scp-like: [user@]host:path — the first colon separates host from path,
	// and the path side must not look like a port-only URL remnant.
	at := strings.Index(raw, "@")
	rest := raw
	if at >= 0 {
		rest = raw[at+1:]
	}
	colon := strings.Index(rest, ":")
	if colon < 0 {
		return "", "", parseErr(raw, "not a recognizable git remote URL")
	}
	host = rest[:colon]
	path = rest[colon+1:]
	if path == "" {
		return "", "", parseErr(raw, "remote URL has no path")
	}
	// "host:1234/owner/repo" is an ssh URL missing its scheme; refusing it is
	// safer than guessing whether 1234 is a port or an owner.
	if first, _, ok := strings.Cut(path, "/"); ok && isAllDigits(first) {
		return "", "", parseErr(raw, "ambiguous scp-like remote URL with numeric first segment %q", first)
	}
	return host, path, nil
}

// hasScheme reports whether raw begins with "<scheme>://".
func hasScheme(raw string) bool {
	i := strings.Index(raw, "://")
	if i <= 0 {
		return false
	}
	for _, r := range raw[:i] {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '+', r == '-', r == '.':
		default:
			return false
		}
	}
	return true
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// validateSegment enforces the owner/repo charset. It is the last line of
// defense before the value reaches an argv or a REST path.
func validateSegment(raw, kind, seg string) error {
	if seg == "" {
		return parseErr(raw, "%s is empty", kind)
	}
	if seg == "." || seg == ".." {
		return parseErr(raw, "%s %q is a path traversal segment", kind, seg)
	}
	if len(seg) > maxSegmentLen {
		return parseErr(raw, "%s is longer than %d characters", kind, maxSegmentLen)
	}
	if strings.HasPrefix(seg, "-") {
		// `gh pr list --repo -x/y` would parse -x/y as a flag.
		return parseErr(raw, "%s %q may not start with '-'", kind, seg)
	}
	for _, r := range seg {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.', r == '_', r == '-':
		default:
			return parseErr(raw, "%s %q contains illegal character %q", kind, seg, string(r))
		}
	}
	return nil
}

func parseErr(raw, format string, args ...any) *Error {
	return &Error{
		Class:   ClassPermanent,
		Op:      "ResolveRepo",
		Message: fmt.Sprintf(format, args...) + fmt.Sprintf(" [%q]", raw),
	}
}

// ValidateRef checks that a git ref or SHA is safe to interpolate into an
// argv or a REST path. It is not a full git ref-format check — it rejects the
// shapes that would change the meaning of the command carrying it.
func ValidateRef(ref string) error {
	if strings.TrimSpace(ref) == "" {
		return &Error{Class: ClassPermanent, Op: "Checks", Message: "empty ref"}
	}
	if strings.HasPrefix(ref, "-") {
		return &Error{Class: ClassPermanent, Op: "Checks", Message: fmt.Sprintf("ref %q may not start with '-'", ref)}
	}
	if len(ref) > 512 {
		return &Error{Class: ClassPermanent, Op: "Checks", Message: "ref is too long"}
	}
	if strings.ContainsAny(ref, "\x00\n\r\t \\?*:[]^~") {
		return &Error{Class: ClassPermanent, Op: "Checks", Message: fmt.Sprintf("ref %q contains illegal characters", ref)}
	}
	for _, seg := range strings.Split(ref, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return &Error{Class: ClassPermanent, Op: "Checks", Message: fmt.Sprintf("ref %q contains an empty or traversal segment", ref)}
		}
	}
	return nil
}
