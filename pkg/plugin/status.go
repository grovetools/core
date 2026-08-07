package plugin

import "os"

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
