package logs

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

const (
	foldedFieldBytes   = 2 * 1024
	expandedFieldBytes = 256 * 1024
	listErrorRunes     = 120
)

// sanitizeDisplayText strips terminal escape sequences and all control bytes
// except newlines and tabs. Log fields are untrusted input: rendering them must
// never be able to move the cursor, clear the screen, or inject key sequences.
func sanitizeDisplayText(s string) string {
	s = ansi.Strip(s)
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r >= 0x20 && r != 0x7f {
			return r
		}
		return -1
	}, s)
}

func truncateUTF8Bytes(s string, limit int) (string, bool) {
	if len(s) <= limit {
		return s, false
	}
	cut := limit
	for cut > 0 && !utf8.ValidString(s[:cut]) {
		cut--
	}
	return s[:cut], true
}

func displayFieldValue(value interface{}, expanded bool) string {
	var text string
	switch v := value.(type) {
	case map[string]interface{}, []interface{}:
		if b, err := json.MarshalIndent(v, "", "  "); err == nil {
			text = string(b)
		} else {
			text = fmt.Sprintf("%v", v)
		}
	case string:
		text = v
	case float64:
		if v == float64(int64(v)) {
			text = fmt.Sprintf("%.0f", v)
		} else {
			text = fmt.Sprintf("%.2f", v)
		}
	case bool:
		text = fmt.Sprintf("%t", v)
	default:
		text = fmt.Sprintf("%v", v)
	}
	text = sanitizeDisplayText(text)
	original := len(text)
	limit := foldedFieldBytes
	if expanded {
		limit = expandedFieldBytes
	}
	if clipped, truncated := truncateUTF8Bytes(text, limit); truncated {
		if expanded {
			return fmt.Sprintf("%s\n… [truncated: showing %d of %d bytes]", clipped, len(clipped), original)
		}
		return fmt.Sprintf("%s\n… [folded: %d bytes; enter to expand fields]", clipped, original)
	}
	if !expanded && original > foldedFieldBytes/2 {
		return fmt.Sprintf("%s\n[large field: %d bytes; enter to expand fields]", text, original)
	}
	return text
}

func firstErrorLine(raw map[string]interface{}) string {
	if raw == nil {
		return ""
	}
	value, ok := raw["error"]
	if !ok || value == nil {
		return ""
	}
	line := displayFieldValue(value, false)
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	runes := []rune(strings.TrimSpace(line))
	if len(runes) > listErrorRunes {
		line = string(runes[:listErrorRunes]) + "…"
	} else {
		line = string(runes)
	}
	return line
}

func wrapDisplay(s string, width int) string {
	if width < 8 {
		width = 8
	}
	return ansi.Wrap(s, width, "")
}
