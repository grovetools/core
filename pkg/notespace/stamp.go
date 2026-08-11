// Package notespace owns durable notebook and notespace identity stamps.
// Missing stamps may be minted only by explicit materialization/migration
// callers. A present malformed stamp is always an error and is never replaced.
package notespace

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/pelletier/go-toml/v2"

	"github.com/grovetools/core/pkg/subject"
)

const (
	NotespaceStampName = ".notespace.toml"
	NotebookStampName  = ".notebook.toml"
)

type NotespaceStamp struct {
	ID      string `toml:"id" json:"id"`
	Name    string `toml:"name" json:"name"`
	Subject string `toml:"subject" json:"subject"`
	Kind    string `toml:"kind" json:"kind"`
}

type NotebookStamp struct {
	ID   string `toml:"id" json:"id"`
	Name string `toml:"name" json:"name"`
}

// NotespaceMutable is the explicitly mutable half of a notespace stamp. ID is
// deliberately absent so update callers cannot accidentally re-key history.
type NotespaceMutable struct {
	Name    string
	Subject string
	Kind    string
}

type Record struct {
	Root  string
	Stamp NotespaceStamp
}

type Index struct {
	byID      map[string][]Record
	bySubject map[string][]Record
}

func NotespaceStampPath(root string) string { return filepath.Join(root, NotespaceStampName) }
func NotebookStampPath(root string) string  { return filepath.Join(root, NotebookStampName) }

func LoadNotespace(root string) (*NotespaceStamp, error) {
	var stamp NotespaceStamp
	found, err := load(NotespaceStampPath(root), &stamp)
	if err != nil || !found {
		return nil, err
	}
	if err := stamp.validate(); err != nil {
		return nil, fmt.Errorf("invalid notespace stamp %s: %w", NotespaceStampPath(root), err)
	}
	return &stamp, nil
}

func LoadNotebook(root string) (*NotebookStamp, error) {
	var stamp NotebookStamp
	found, err := load(NotebookStampPath(root), &stamp)
	if err != nil || !found {
		return nil, err
	}
	if err := stamp.validate(); err != nil {
		return nil, fmt.Errorf("invalid notebook stamp %s: %w", NotebookStampPath(root), err)
	}
	return &stamp, nil
}

func load(path string, dst any) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read identity stamp %s: %w", path, err)
	}
	dec := toml.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return false, fmt.Errorf("parse identity stamp %s: %w", path, err)
	}
	return true, nil
}

func MintNotespace(root string, mutable NotespaceMutable) (*NotespaceStamp, error) {
	if existing, err := LoadNotespace(root); err != nil || existing != nil {
		return existing, err
	}
	stamp := NotespaceStamp{ID: newID(), Name: mutable.Name, Subject: mutable.Subject, Kind: mutable.Kind}
	if err := stamp.validate(); err != nil {
		return nil, err
	}
	if err := installNew(NotespaceStampPath(root), stamp); err != nil {
		return nil, err
	}
	return LoadNotespace(root)
}

func MintNotebook(root, name string) (*NotebookStamp, error) {
	if existing, err := LoadNotebook(root); err != nil || existing != nil {
		return existing, err
	}
	stamp := NotebookStamp{ID: newID(), Name: name}
	if err := stamp.validate(); err != nil {
		return nil, err
	}
	if err := installNew(NotebookStampPath(root), stamp); err != nil {
		return nil, err
	}
	return LoadNotebook(root)
}

// InstallNotespace installs a migration/reconciliation-provided immutable id.
// It has the same load-first and no-clobber behavior as MintNotespace.
func InstallNotespace(root string, stamp NotespaceStamp) (*NotespaceStamp, error) {
	if existing, err := LoadNotespace(root); err != nil || existing != nil {
		return existing, err
	}
	if err := stamp.validate(); err != nil {
		return nil, err
	}
	if err := installNew(NotespaceStampPath(root), stamp); err != nil {
		return nil, err
	}
	return LoadNotespace(root)
}

func InstallNotebook(root string, stamp NotebookStamp) (*NotebookStamp, error) {
	if existing, err := LoadNotebook(root); err != nil || existing != nil {
		return existing, err
	}
	if err := stamp.validate(); err != nil {
		return nil, err
	}
	if err := installNew(NotebookStampPath(root), stamp); err != nil {
		return nil, err
	}
	return LoadNotebook(root)
}

// UpdateNotebook rewrites only the display name and refuses an id mismatch.
func UpdateNotebook(root, expectedID, name string) (*NotebookStamp, error) {
	current, err := LoadNotebook(root)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, fmt.Errorf("notebook stamp %s is missing", NotebookStampPath(root))
	}
	if current.ID != expectedID {
		return nil, fmt.Errorf("notebook stamp id changed: expected %q, found %q", expectedID, current.ID)
	}
	next := NotebookStamp{ID: current.ID, Name: name}
	if err := next.validate(); err != nil {
		return nil, err
	}
	if err := replace(NotebookStampPath(root), next); err != nil {
		return nil, err
	}
	settled, err := LoadNotebook(root)
	if err == nil && settled.ID != expectedID {
		return nil, fmt.Errorf("notebook stamp id changed during update: expected %q, found %q", expectedID, settled.ID)
	}
	return settled, err
}

// UpdateNotespace rewrites only mutable metadata and refuses an id mismatch.
func UpdateNotespace(root, expectedID string, mutable NotespaceMutable) (*NotespaceStamp, error) {
	current, err := LoadNotespace(root)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, fmt.Errorf("notespace stamp %s is missing", NotespaceStampPath(root))
	}
	if current.ID != expectedID {
		return nil, fmt.Errorf("notespace stamp id changed: expected %q, found %q", expectedID, current.ID)
	}
	next := NotespaceStamp{ID: current.ID, Name: mutable.Name, Subject: mutable.Subject, Kind: mutable.Kind}
	if err := next.validate(); err != nil {
		return nil, err
	}
	if err := replace(NotespaceStampPath(root), next); err != nil {
		return nil, err
	}
	settled, err := LoadNotespace(root)
	if err == nil && settled.ID != expectedID {
		return nil, fmt.Errorf("notespace stamp id changed during update: expected %q, found %q", expectedID, settled.ID)
	}
	return settled, err
}

func installNew(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create stamp directory: %w", err)
	}
	data, err := toml.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode identity stamp: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".stamp-*.tmp")
	if err != nil {
		return fmt.Errorf("create stamp temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write stamp temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync stamp temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close stamp temp file: %w", err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return fmt.Errorf("chmod stamp temp file: %w", err)
	}
	// Hard-link is an atomic create-if-absent. A concurrent winner is adopted
	// by the mandatory re-read rather than overwritten by rename.
	if err := os.Link(tmpName, path); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("install identity stamp %s: %w", path, err)
	}
	return nil
}

func replace(path string, value any) error {
	data, err := toml.Marshal(value)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".stamp-update-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace identity stamp %s: %w", path, err)
	}
	return nil
}

func (s NotespaceStamp) validate() error {
	if _, err := ulid.ParseStrict(s.ID); err != nil {
		return fmt.Errorf("id is not a ULID: %w", err)
	}
	if err := cleanText("name", s.Name); err != nil {
		return err
	}
	if err := subject.Validate(s.Subject); err != nil {
		return err
	}
	return cleanText("kind", s.Kind)
}

func (s NotebookStamp) validate() error {
	if _, err := ulid.ParseStrict(s.ID); err != nil {
		return fmt.Errorf("id is not a ULID: %w", err)
	}
	return cleanText("name", s.Name)
}

func cleanText(field, value string) error {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\r\n\t") {
		return fmt.Errorf("%s is empty or contains surrounding/control whitespace", field)
	}
	return nil
}

func newID() string { return ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader).String() }

// BuildIndex loads exactly the supplied physical roots. It does not discover,
// sort, or select roots; callers obtain this list from recorded config/layout.
func BuildIndex(roots []string) (*Index, error) {
	idx := &Index{byID: make(map[string][]Record), bySubject: make(map[string][]Record)}
	for _, root := range roots {
		stamp, err := LoadNotespace(root)
		if err != nil {
			return nil, err
		}
		if stamp == nil {
			continue
		}
		record := Record{Root: root, Stamp: *stamp}
		idx.byID[stamp.ID] = append(idx.byID[stamp.ID], record)
		idx.bySubject[stamp.Subject] = append(idx.bySubject[stamp.Subject], record)
	}
	return idx, nil
}

func (i *Index) ByID(id string) ([]Record, error) {
	if i == nil {
		return nil, nil
	}
	out := append([]Record(nil), i.byID[id]...)
	sort.Slice(out, func(a, b int) bool { return out[a].Root < out[b].Root })
	if len(out) > 1 {
		return out, fmt.Errorf("duplicate notespace id %q at %s and %s", id, out[0].Root, out[1].Root)
	}
	return out, nil
}

func (i *Index) BySubject(value string) []Record {
	if i == nil {
		return nil
	}
	out := append([]Record(nil), i.bySubject[value]...)
	sort.Slice(out, func(a, b int) bool { return out[a].Root < out[b].Root })
	return out
}

// IsIdentityStamp reports identity files that sync as ordinary dot-documents
// but must never enter document three-way merge.
func IsIdentityStamp(path string) bool {
	base := filepath.Base(filepath.Clean(path))
	return base == NotespaceStampName || base == NotebookStampName
}
