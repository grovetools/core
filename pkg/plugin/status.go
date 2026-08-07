package plugin

import (
	"os"
	"path/filepath"
)

// Status is one row of `grove plugin list`: a pin, plus whether what it claims
// is actually there.
type Status struct {
	Name string
	Pin  *Pin
	// FragmentPresent and BinaryPresent report whether what the pin claims is
	// actually on disk.
	FragmentPresent bool
	BinaryPresent   bool
	// Approved reports whether the exec-trust store still holds the approval
	// for this exact pin. False means the fragment or the lock entry was
	// edited outside `grove plugin`.
	Approved bool
}

// List reports every installed plugin and whether it is intact.
//
// Everything it touches is local: the lockfile, two stats and the trust store.
// Nothing here reaches the network or the plugin's source, which is what makes
// it cheap enough for a host to call on a redraw.
func List() ([]Status, error) {
	lock, err := LoadLock()
	if err != nil {
		return nil, err
	}
	out := make([]Status, 0, len(lock.Plugins))
	for _, name := range lock.Names() {
		pin := lock.Plugins[name]
		st := Status{Name: name, Pin: pin, Approved: IsApproved(pin.Fragment, pin.ConsentDigest)}
		if _, err := os.Stat(pin.Fragment); err == nil {
			st.FragmentPresent = true
		}
		if _, err := os.Stat(pin.Binary); err == nil {
			st.BinaryPresent = true
		}
		out = append(out, st)
	}
	return out, nil
}

// BuiltCommit reports which version the installed binary was actually built
// from, read off the link the installer leaves in the grove bin dir:
//
//	BinDir()/<binary> -> DataDir()/plugins/versions/<name>/<commit|"dev">/bin/<binary>
//
// It is the one thing the lockfile cannot state. Pin.Commit is what is PINNED;
// this is what is on disk, and the two disagree exactly when it matters — a
// rebuild that never finished, a binary replaced by hand, or a dev entry, whose
// answer is the literal "dev" because it was built from a working tree nothing
// is pinned to.
//
// Empty when the link is missing or points outside that layout. A binary
// somebody put there themselves has no built commit to report, and inventing
// one would be worse than saying nothing.
func (p *Pin) BuiltCommit() string {
	if p == nil || p.Binary == "" {
		return ""
	}
	target, err := os.Readlink(p.Binary)
	if err != nil {
		return ""
	}
	binDir := filepath.Dir(target)      // .../versions/<name>/<version>/bin
	versionDir := filepath.Dir(binDir)  // .../versions/<name>/<version>
	nameDir := filepath.Dir(versionDir) // .../versions/<name>
	if filepath.Base(binDir) != "bin" || filepath.Base(filepath.Dir(nameDir)) != "versions" {
		return ""
	}
	return filepath.Base(versionDir)
}
