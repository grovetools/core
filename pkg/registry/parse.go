package registry

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// noteFrontmatter mirrors what Render emits. It exists so parsing can go
// through a real YAML decoder while emission stays hand-rolled: the writer
// owns byte stability, the reader owns tolerance. TestNoteRoundTrip is the
// contract between them — add a field to one and the test fails until it is
// added to the other.
type noteFrontmatter struct {
	MachineID     string `yaml:"machine_id"`
	Name          string `yaml:"name"`
	Rev           int64  `yaml:"rev"`
	LastSeen      string `yaml:"last_seen"`
	OriginID      string `yaml:"origin_id"`
	GrovedVersion string `yaml:"groved_version"`

	Ecosystems []struct {
		Name     string `yaml:"name"`
		Path     string `yaml:"path"`
		Notebook string `yaml:"notebook"`
		State    string `yaml:"state"`
		Enabled  bool   `yaml:"enabled"`
		Card     *struct {
			ID      string `yaml:"id"`
			Layout  string `yaml:"layout"`
			Remotes []struct {
				Name string `yaml:"name"`
				URL  string `yaml:"url"`
			} `yaml:"remotes"`
			Notebooks []struct {
				Name     string `yaml:"name"`
				Default  bool   `yaml:"default"`
				Audience string `yaml:"audience"`
			} `yaml:"notebooks"`
		} `yaml:"card"`
	} `yaml:"ecosystems"`

	Roots []struct {
		Name     string `yaml:"name"`
		Path     string `yaml:"path"`
		Notebook string `yaml:"notebook"`
		Enabled  bool   `yaml:"enabled"`
		Exists   bool   `yaml:"exists"`
	} `yaml:"roots"`

	Subscriptions []struct {
		Name string `yaml:"name"`
		Role string `yaml:"role"`
		Mode string `yaml:"mode"`
		Pull bool   `yaml:"pull"`
	} `yaml:"subscriptions"`

	Repos []struct {
		Root   string `yaml:"root"`
		Path   string `yaml:"path"`
		Branch string `yaml:"branch"`
		SHA    string `yaml:"sha"`
	} `yaml:"repos"`
}

var frontmatterFence = []byte("---\n")

// splitFrontmatter returns the YAML block between the leading and closing
// fences. A document with no frontmatter is an error rather than an empty
// note: an unparseable machine note must be reported as suspect, never
// silently rendered as a machine with no ecosystems.
func splitFrontmatter(data []byte) ([]byte, error) {
	// Tolerate a UTF-8 BOM and CRLF line endings; an editor round-trip on
	// another machine should not make a note unreadable here.
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(data, frontmatterFence) {
		return nil, fmt.Errorf("note has no frontmatter")
	}
	rest := data[len(frontmatterFence):]
	end := bytes.Index(rest, []byte("\n---"))
	if end < 0 {
		return nil, fmt.Errorf("note frontmatter is not terminated")
	}
	return rest[:end+1], nil
}

// ParseNote decodes a machine note's bytes.
func ParseNote(data []byte) (*Note, error) {
	block, err := splitFrontmatter(data)
	if err != nil {
		return nil, err
	}
	var fm noteFrontmatter
	if err := yaml.Unmarshal(block, &fm); err != nil {
		return nil, fmt.Errorf("failed to parse note frontmatter: %w", err)
	}
	if fm.MachineID == "" {
		return nil, fmt.Errorf("note frontmatter has no machine_id")
	}

	n := &Note{
		MachineID:     fm.MachineID,
		Name:          fm.Name,
		Rev:           fm.Rev,
		LastSeen:      fm.LastSeen,
		OriginID:      fm.OriginID,
		GrovedVersion: fm.GrovedVersion,
	}
	for _, e := range fm.Ecosystems {
		eco := NoteEcosystem{
			Name:     e.Name,
			Path:     e.Path,
			Notebook: e.Notebook,
			State:    e.State,
			Enabled:  e.Enabled,
		}
		if e.Card != nil {
			card := &NoteCard{ID: e.Card.ID, Layout: e.Card.Layout}
			for _, r := range e.Card.Remotes {
				card.Remotes = append(card.Remotes, NoteRemote{Name: r.Name, URL: r.URL})
			}
			for _, nb := range e.Card.Notebooks {
				card.Notebooks = append(card.Notebooks, NoteCardNotebook{
					Name: nb.Name, Default: nb.Default, Audience: nb.Audience,
				})
			}
			eco.Card = card
		}
		n.Ecosystems = append(n.Ecosystems, eco)
	}
	for _, r := range fm.Roots {
		n.Roots = append(n.Roots, NoteRoot{
			Name: r.Name, Path: r.Path, Notebook: r.Notebook, Enabled: r.Enabled, Exists: r.Exists,
		})
	}
	for _, s := range fm.Subscriptions {
		n.Subscriptions = append(n.Subscriptions, NoteSubscription{
			Name: s.Name, Role: s.Role, Mode: s.Mode, Pull: s.Pull,
		})
	}
	for _, r := range fm.Repos {
		n.Repos = append(n.Repos, NoteRepo{Root: r.Root, Path: r.Path, Branch: r.Branch, SHA: r.SHA})
	}
	return n, nil
}
