package config

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// MachineEditOptions supplies compare-and-swap and optional stamp-index
// validation. ExpectedRevision="" means the caller accepts the latest file;
// KnownNotespaceIDs=nil means the caller does not yet have a stamp index.
type MachineEditOptions struct {
	ExpectedRevision  string
	KnownNotespaceIDs map[string]struct{}
}

// MachineRevision returns the SHA-256 revision of machine.toml. A missing file
// has the stable revision of empty content.
func MachineRevision(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return machineRevision(data), nil
}

func machineRevision(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }

// EditMachineConfig serializes a surgical, verified machine.toml transaction.
// The callback may alter sync.registry, primaries, and subjects together, which
// is the local atomic half of subject re-record. Every candidate is parsed and
// optionally cross-checked against the caller's stamp index before rename.
func EditMachineConfig(path string, opts MachineEditOptions, mutate func(*MachineConfig) error) (revision string, changed bool, err error) {
	if path == "" {
		return "", false, fmt.Errorf("machine config path is not resolvable")
	}
	unlock, err := lockMachineFile(path)
	if err != nil {
		return "", false, err
	}
	defer unlock()
	if err := reviewConfigWritePath(path); err != nil {
		return "", false, err
	}

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", false, fmt.Errorf("failed to read machine config %s: %w", path, err)
	}
	currentRevision := machineRevision(existing)
	if opts.ExpectedRevision != "" && opts.ExpectedRevision != currentRevision {
		return currentRevision, false, fmt.Errorf("machine config changed: expected revision %s, found %s", opts.ExpectedRevision, currentRevision)
	}
	cfg := &MachineConfig{}
	if len(existing) != 0 {
		cfg, err = ParseMachineConfigContent(path, string(existing))
		if err != nil {
			return currentRevision, false, err
		}
	}
	if mutate == nil {
		return currentRevision, false, fmt.Errorf("machine config edit callback is nil")
	}
	if err := mutate(cfg); err != nil {
		return currentRevision, false, err
	}
	if err := cfg.Validate(); err != nil {
		return currentRevision, false, err
	}
	if opts.KnownNotespaceIDs != nil {
		if err := ValidateMachineBindings(cfg, opts.KnownNotespaceIDs); err != nil {
			return currentRevision, false, err
		}
	}

	updated := renderMachineIdentityTables(string(existing), cfg)
	if updated == string(existing) {
		return currentRevision, false, nil
	}
	verify := func(candidate string) error {
		parsed, err := ParseMachineConfigContent(path, candidate)
		if err != nil {
			return err
		}
		if opts.KnownNotespaceIDs != nil {
			return ValidateMachineBindings(parsed, opts.KnownNotespaceIDs)
		}
		return nil
	}
	if err := atomicWriteVerified(path, updated, verify); err != nil {
		return currentRevision, false, err
	}
	return machineRevision([]byte(updated)), true, nil
}

// ValidateMachineBindings is the whole-file binding rule EditMachineConfig
// enforces: every [primaries] entry and the [sync.registry] binding must name
// an id the supplied stamp index can reach.
//
// It is exported because the check is whole-file rather than per-key, so a
// caller that is about to make an IRREVERSIBLE change before the edit (the D8
// re-mint rewrites a stamp on disk, then repairs the binding that followed it)
// has to be able to run the same rule as a preflight. Discovering an unrelated
// broken binding only at the write leaves a torn transaction the writer cannot
// undo.
func ValidateMachineBindings(cfg *MachineConfig, known map[string]struct{}) error {
	if cfg == nil {
		return nil
	}
	for value, id := range cfg.Primaries {
		if _, ok := known[id]; !ok {
			return fmt.Errorf("[primaries] %q references notespace id %q absent from supplied stamp index", value, id)
		}
	}
	if cfg.Sync.Registry != nil {
		if _, ok := known[cfg.Sync.Registry.NotespaceID]; !ok {
			return fmt.Errorf("[sync.registry] references notespace id %q absent from supplied stamp index", cfg.Sync.Registry.NotespaceID)
		}
	}
	return nil
}

// RerecordSubject atomically moves the primary key and any matching local path
// records. Stamp and server alias updates are separate receipt-backed steps;
// callers must not invoke this helper unless they are executing that D5
// transaction.
func RerecordSubject(path, expectedRevision, oldSubject, newSubject, notespaceID string, knownIDs map[string]struct{}) (string, bool, error) {
	return EditMachineConfig(path, MachineEditOptions{ExpectedRevision: expectedRevision, KnownNotespaceIDs: knownIDs}, func(cfg *MachineConfig) error {
		if got := cfg.Primaries[oldSubject]; got != notespaceID {
			return fmt.Errorf("cannot re-record subject: [primaries] %q is %q, want %q", oldSubject, got, notespaceID)
		}
		if got, exists := cfg.Primaries[newSubject]; exists && got != notespaceID {
			return fmt.Errorf("cannot re-record subject: [primaries] %q already points to %q", newSubject, got)
		}
		if cfg.Primaries == nil {
			cfg.Primaries = make(map[string]string)
		}
		delete(cfg.Primaries, oldSubject)
		cfg.Primaries[newSubject] = notespaceID
		for canonicalPath, value := range cfg.Subjects {
			if value == oldSubject {
				cfg.Subjects[canonicalPath] = newSubject
			}
		}
		return nil
	})
}

func renderMachineIdentityTables(content string, cfg *MachineConfig) string {
	if cfg.Sync.Registry == nil {
		content = deleteTOMLTable(content, "sync.registry")
	} else {
		block := "[sync.registry]\n" +
			"notebook = " + strconv.Quote(cfg.Sync.Registry.Notebook) + "\n" +
			"notespace_id = " + strconv.Quote(cfg.Sync.Registry.NotespaceID) + "\n"
		content = setTOMLTable(content, "sync.registry", block)
	}
	content = replaceStringMapTable(content, "primaries", cfg.Primaries)
	content = replaceStringMapTable(content, "subjects", cfg.Subjects)
	return content
}

func replaceStringMapTable(content, table string, values map[string]string) string {
	if len(values) == 0 {
		return deleteTOMLTable(content, table)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[%s]\n", table)
	for _, key := range sortedKeys(values) {
		fmt.Fprintf(&b, "%s = %s\n", strconv.Quote(key), strconv.Quote(values[key]))
	}
	return setTOMLTable(content, table, b.String())
}

func lockMachineFile(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	lockPath := path + ".lock"
	deadline := time.Now().Add(5 * time.Second)
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
			_ = f.Close()
			return func() { _ = os.Remove(lockPath) }, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("lock machine config %s: %w", path, err)
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for machine config lock %s", lockPath)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
