package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/grovetools/core/pkg/paths"
)

// Machine is one machine note as a READER sees it: the parsed document, where
// it came from, and the verdict of the read-side integrity checks.
//
// Validation lives here, at read time, and nowhere else. The apply path stays
// dumb on purpose — the pull pipeline writes whatever the server hands it,
// exactly as it does for every other document — because a validating apply
// path would be a second, subtly different trust boundary that only the
// registry exercised. Every surface that RENDERS a machine validates it.
type Machine struct {
	// Note is nil when the document could not be parsed; Err then says why
	// and Suspect carries the reader-facing reason.
	Note *Note
	// Path is the note's workspace-relative path.
	Path string
	// PathID is the machine id taken from the PATH, which is the id the
	// registry's single-writer rule is keyed on. When it disagrees with the
	// document's own machine_id, the path wins for identity and the note is
	// marked suspect.
	PathID string
	// Self is true for this machine's own note.
	Self bool
	// Suspect lists every integrity finding, empty when the note is clean.
	// Advisory under the interim trust model: any token can write any note
	// until device principals land, so these are evidence, not enforcement.
	Suspect []string
	// Err is the parse failure, if any.
	Err error
}

// Suspicious reports whether any integrity check fired.
func (m Machine) Suspicious() bool { return len(m.Suspect) > 0 }

// Label renders "name (short id)" using the id from the PATH — never the
// document's self-reported name alone, and never the document's self-reported
// id, which is precisely what a forged note would lie about.
func (m Machine) Label() string {
	name := ""
	if m.Note != nil {
		name = m.Note.Name
	}
	return describe(name, m.PathID)
}

// StaleFor returns how long ago this machine last wrote its note, measured
// against the READER's clock, and whether that is computable at all.
//
// Advisory by construction. The sync protocol never compares client
// timestamps for ordering (migration 0003_file_mtime); this is application
// content, read by a human, and a machine with a skewed clock produces a
// misleading age rather than a broken replica.
func (m Machine) StaleFor(now time.Time) (time.Duration, bool) {
	if m.Note == nil || m.Note.LastSeen == "" {
		return 0, false
	}
	seen, err := time.Parse(DateFormat, m.Note.LastSeen)
	if err != nil {
		return 0, false
	}
	d := now.UTC().Sub(seen)
	if d < 0 {
		d = 0 // a note stamped "today" from a machine east of here is not stale
	}
	return d, true
}

// DeclaredMissing returns the ecosystems this machine declared but does not
// have on disk — the materialization verb's input, and the one thing a
// machines listing exists to make visible across hosts.
func (m Machine) DeclaredMissing() []NoteEcosystem {
	if m.Note == nil {
		return nil
	}
	var out []NoteEcosystem
	for _, e := range m.Note.Ecosystems {
		if !e.Enabled {
			continue
		}
		if e.State == StateDeclaredMissing || e.State == StateUnmanifested {
			out = append(out, e)
		}
	}
	return out
}

// Ecosystem reconciliation states, mirrored from config.MachineEcosystem* so
// readers of a note do not have to depend on the config package's constants
// (the values travel in the document, not in a Go type).
const (
	StatePresent         = "present"
	StateDeclaredMissing = "declared-missing"
	StateUnmanifested    = "unmanifested"
)

// ReadMachines reads and validates every machine note under a registry
// workspace root. selfID is this machine's own id (may be empty), used only to
// mark the caller's own row.
//
// It is the shared read surface behind `grove machines` and the treemux
// machines panel: v1 reads the replicated files directly rather than adding a
// daemon endpoint, because the notes ARE the API — a machine with no daemon
// running can still answer "what do I know about my other machines".
//
// A note that fails to parse is returned as a suspect entry rather than
// dropped: silently omitting an unreadable machine would hide exactly the
// case worth seeing.
func ReadMachines(root, selfID string) ([]Machine, error) {
	if root == "" {
		return nil, fmt.Errorf("registry workspace root is not resolvable")
	}
	dir := filepath.Join(root, MachinesDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			// Not an error: a machine that has joined but never pulled has no
			// machines/ directory yet.
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read %s: %w", dir, err)
	}

	cache, _ := LoadRevCache() // a missing/corrupt cache degrades to "no history"
	out := make([]Machine, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), NoteExt) {
			continue
		}
		if strings.HasPrefix(entry.Name(), ".") {
			continue // never replicated in the first place; ignore local strays
		}
		rel := MachinesDir + "/" + entry.Name()
		id := MachineIDFromPath(rel)
		m := Machine{Path: rel, PathID: id, Self: id != "" && id == selfID}

		data, rerr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if rerr != nil {
			m.Err = rerr
			m.Suspect = append(m.Suspect, "unreadable: "+rerr.Error())
			out = append(out, m)
			continue
		}
		note, perr := ParseNote(data)
		if perr != nil {
			m.Err = perr
			m.Suspect = append(m.Suspect, "unparseable: "+perr.Error())
			out = append(out, m)
			continue
		}
		m.Note = note
		m.Suspect = append(m.Suspect, cache.check(id, note)...)
		out = append(out, m)
	}

	sort.Slice(out, func(i, j int) bool {
		li, lj := strings.ToLower(out[i].Label()), strings.ToLower(out[j].Label())
		if li != lj {
			return li < lj
		}
		return out[i].PathID < out[j].PathID
	})

	// Best-effort: a read-only surface must still work when the state dir is
	// unwritable, it just loses rev-regression detection across runs.
	_ = cache.Save()
	return out, nil
}

// --- rev cache ------------------------------------------------------------

// RevCacheFileName is the local, per-reader record of the highest rev seen for
// each machine. It lives in state, not in the registry workspace: a cache
// stored in a replicated document would be forgeable by whoever forged the
// note it is supposed to catch.
const RevCacheFileName = "revs.json"

// RevCache remembers the highest rev this reader has seen per machine.
type RevCache struct {
	// Revs maps machine id -> highest observed rev. Only ever raised: a note
	// whose rev went backwards is the finding, so lowering the watermark to
	// match it would erase the evidence on the next read.
	Revs map[string]int64 `json:"revs"`

	path  string
	dirty bool
}

// RevCachePath returns the cache's location, or "" when no state directory
// resolves.
func RevCachePath() string {
	stateDir := paths.StateDir()
	if stateDir == "" {
		return ""
	}
	return filepath.Join(stateDir, "registry", RevCacheFileName)
}

// LoadRevCache reads the rev cache. A missing file is not an error.
func LoadRevCache() (*RevCache, error) {
	c := &RevCache{Revs: map[string]int64{}, path: RevCachePath()}
	if c.path == "" {
		return c, fmt.Errorf("cannot resolve grove state directory")
	}
	data, err := os.ReadFile(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return c, fmt.Errorf("failed to read %s: %w", c.path, err)
	}
	if err := json.Unmarshal(data, c); err != nil {
		// A corrupt cache is not worth failing a read over — it only costs
		// rev-regression detection until the next successful write.
		c.Revs = map[string]int64{}
		return c, fmt.Errorf("failed to parse %s: %w", c.path, err)
	}
	if c.Revs == nil {
		c.Revs = map[string]int64{}
	}
	return c, nil
}

// check runs the read-side integrity rules for one note and folds its rev into
// the watermark. Two rules, both from the interim trust model:
//
//  1. machine_id must equal the id in the path. The path is what the
//     single-writer rule is keyed on, so a document claiming a different
//     identity than its own location is either a botched copy or a forgery.
//  2. rev must not go backwards. Every write bumps it, so a lower rev than
//     this reader has already seen means the note was replaced by an older
//     one — a restore, or someone else writing this machine's note.
func (c *RevCache) check(pathID string, note *Note) []string {
	var suspect []string
	if note.MachineID != "" && pathID != "" && note.MachineID != pathID {
		suspect = append(suspect, fmt.Sprintf(
			"tampered: document machine_id %q does not match its path id %q", note.MachineID, pathID))
	}
	if pathID == "" {
		return suspect
	}
	if seen, ok := c.Revs[pathID]; ok && note.Rev < seen {
		suspect = append(suspect, fmt.Sprintf(
			"suspect: rev regressed from %d to %d", seen, note.Rev))
	}
	if note.Rev > c.Revs[pathID] {
		c.Revs[pathID] = note.Rev
		c.dirty = true
	}
	return suspect
}

// Save persists the cache when anything changed.
func (c *RevCache) Save() error {
	if c == nil || !c.dirty || c.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return fmt.Errorf("failed to create %s: %w", filepath.Dir(c.path), err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(c.path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("failed to write %s: %w", c.path, err)
	}
	c.dirty = false
	return nil
}

// describe renders "name (short id)" without importing core/pkg/machine,
// which would make this leaf package depend on the identity minter just to
// format a label. The short form is the TRAILING 8 characters for the reason
// machine.ShortID documents: a ULID's leading characters are its millisecond
// timestamp, identical for two hosts minted back to back from one dotfiles
// repo — exactly the collision the short form must survive.
func describe(name, id string) string {
	short := id
	if len(short) > shortIDLen {
		short = short[len(short)-shortIDLen:]
	}
	switch {
	case name == "" && short == "":
		return "unknown"
	case short == "":
		return name
	case name == "":
		return short
	default:
		return name + " (" + short + ")"
	}
}

const shortIDLen = 8
