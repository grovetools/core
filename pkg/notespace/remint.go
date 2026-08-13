package notespace

import "fmt"

// Re-minting: the one sanctioned way a notespace's immutable id ever changes
// (Phase 3, W3.6 / decision D8).
//
// A stamp id is immutable BY THE VERB: UpdateNotespace refuses an id mismatch,
// MintNotespace and InstallNotespace refuse to clobber an existing stamp, and
// every routing surface treats the id as durable. That is the whole point —
// history, cursors, [primaries] and the server's claim all hang off it.
//
// The single state that immutability cannot describe is a stamp COPIED rather
// than minted: `cp -R` a notespace and two physical roots now claim one id.
// D8's runtime rule parks the later copy and writes evidence naming both paths;
// the repair is to give ONE of them a new identity. That is not an update
// (which preserves the id by definition) and not a mint (which refuses a root
// that already carries a stamp), so it gets its own verb with its own
// preconditions:
//
//   - the caller names the id it expects to find, so a re-mint cannot land on a
//     root that has changed underneath the operator's decision;
//   - the new id is freshly minted here, never supplied — a caller-chosen id is
//     how one duplicate becomes another;
//   - name, subject and kind are carried over verbatim: the copy is still notes
//     ABOUT the same subject, and losing the subject would strand it outside
//     every subject-keyed surface (primaries, siblings, the resolver).
//
// What re-minting does NOT do is rewrite anything outside the stamp. The local
// bindings that referenced the old id ([primaries], [sync.registry]) are
// rewritten by the caller in the same operator sitting — see grove's
// `doctor --fix --remint`, which does both halves and prints the evidence.
// Splitting it that way keeps this package free of config-writing, and keeps
// the binding rewrite where the machine config writer already lives.

// RemintResult reports what a re-mint changed, for the caller's evidence line.
type RemintResult struct {
	// Root is the physical directory whose stamp was rewritten.
	Root string
	// OldID is the duplicated id this root gave up; NewID is what it carries
	// now. The pair is the whole receipt: every other field of the stamp is
	// unchanged by construction.
	OldID string
	NewID string
	// Stamp is the settled stamp as re-read from disk.
	Stamp NotespaceStamp
}

// RemintNotespace gives the notespace at root a new immutable id, preserving
// its mutable metadata. expectedID is the id the caller decided to re-mint
// away from; a root carrying anything else is refused rather than re-keyed.
func RemintNotespace(root, expectedID string) (*RemintResult, error) {
	if root == "" {
		return nil, fmt.Errorf("re-mint requires a notespace root")
	}
	if expectedID == "" {
		return nil, fmt.Errorf("re-mint requires the id being replaced; it is the operator's designation, not a guess")
	}
	current, err := LoadNotespace(root)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, fmt.Errorf("notespace stamp %s is missing; re-mint repairs a duplicated id, it does not create one", NotespaceStampPath(root))
	}
	if current.ID != expectedID {
		return nil, fmt.Errorf("refusing to re-mint %s: it carries id %q, not the designated %q", root, current.ID, expectedID)
	}
	next := NotespaceStamp{ID: newID(), Name: current.Name, Subject: current.Subject, Kind: current.Kind}
	if next.ID == current.ID {
		return nil, fmt.Errorf("re-mint produced the same id %q", next.ID)
	}
	if err := next.validate(); err != nil {
		return nil, err
	}
	if err := replace(NotespaceStampPath(root), next); err != nil {
		return nil, err
	}
	settled, err := LoadNotespace(root)
	if err != nil {
		return nil, err
	}
	if settled == nil || settled.ID != next.ID {
		return nil, fmt.Errorf("notespace stamp %s did not settle on the re-minted id", NotespaceStampPath(root))
	}
	return &RemintResult{Root: root, OldID: current.ID, NewID: settled.ID, Stamp: *settled}, nil
}
