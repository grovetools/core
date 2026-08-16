// Package slug provides canonical slugs for note and job identities.
package slug

import "strings"

// MaxLength is the maximum length of a canonical slug.
const MaxLength = 50

// Canonical converts text to the shared Grove kebab slug form.
//
// The result contains only lowercase ASCII letters, digits, and hyphens.
// Whitespace becomes a hyphen; other unsupported characters are discarded.
// Hyphens are collapsed and trimmed, and the result is at most MaxLength
// bytes. Because the output is ASCII, truncation cannot split a UTF-8 sequence.
func Canonical(text string) string {
	var b strings.Builder
	b.Grow(min(len(text), MaxLength))

	pendingHyphen := false
	for _, r := range text {
		var c byte
		switch {
		case r >= 'A' && r <= 'Z':
			c = byte(r + ('a' - 'A'))
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			c = byte(r)
		case r == '-' || isASCIIWhitespace(r):
			if b.Len() > 0 {
				pendingHyphen = true
			}
			continue
		default:
			continue
		}

		if pendingHyphen {
			if b.Len() == MaxLength {
				break
			}
			b.WriteByte('-')
			pendingHyphen = false
		}
		if b.Len() == MaxLength {
			break
		}
		b.WriteByte(c)
	}

	return strings.TrimRight(b.String(), "-")
}

// StripNotePrefix removes a note identity prefix from stem. It recognizes
// YYYYMMDD- and YYYYMMDD-HHMMSS-. The argument is a filename stem: callers
// should remove any directory and extension first. Unrecognized stems are
// returned unchanged.
func StripNotePrefix(stem string) string {
	if len(stem) >= 16 && digits(stem[:8]) && stem[8] == '-' &&
		digits(stem[9:15]) && stem[15] == '-' {
		return stem[16:]
	}
	if len(stem) >= 9 && digits(stem[:8]) && stem[8] == '-' {
		return stem[9:]
	}
	return stem
}

// StripJobPrefix removes a job's decimal sequence prefix (for example, "18-")
// from stem. The argument is a filename stem. Unrecognized stems are returned
// unchanged.
func StripJobPrefix(stem string) string {
	for i := 0; i < len(stem); i++ {
		if stem[i] == '-' {
			if i > 0 && digits(stem[:i]) {
				return stem[i+1:]
			}
			return stem
		}
		if stem[i] < '0' || stem[i] > '9' {
			return stem
		}
	}
	return stem
}

func digits(s string) bool {
	if s == "" {
		return false
	}
	for i := range s {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func isASCIIWhitespace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	default:
		return false
	}
}
