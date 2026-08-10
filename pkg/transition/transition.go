// Package transition defines the evidence contract for successful state-changing
// commands.
//
// Grove P2-P5 transition verbs MUST construct Evidence and call FinishSuccess
// (directly or through RenderHuman or RenderJSON) before reporting success. This
// prevents an empty success from being presented as a completed transition.
// Server-backed verbs MUST set ServerBacked and include the accepted response
// returned by the server in ServerEcho; the request sent to the server is not
// evidence of acceptance.
package transition

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Reason explains why a successful transition intentionally changed zero
// items, for example because the requested state was already current.
type Reason string

// Count records the number of items affected in a named category. Value must
// not be negative. Only positive values constitute transition evidence.
type Count struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
}

// ResolvedRoot records both the root as supplied or configured and the path to
// which it resolved. Keeping both values makes expansion and symlink decisions
// visible to operators.
type ResolvedRoot struct {
	Name     string `json:"name"`
	Declared string `json:"declared"`
	Resolved string `json:"resolved"`
}

// ServerEcho is acceptance evidence returned by a server. Response must be a
// non-empty server response value (an acknowledgement token, status, or
// message), not a copy of the client's request.
type ServerEcho struct {
	Accepted  bool   `json:"accepted"`
	Response  string `json:"response"`
	RequestID string `json:"request_id,omitempty"`
}

// Evidence is the final, user-visible proof of a successful transition.
// Action names the transition. Slice order supplied by callers is ignored by
// renderers; counts and roots are rendered in canonical order.
type Evidence struct {
	Action        string         `json:"action"`
	Counts        []Count        `json:"counts,omitempty"`
	ResolvedRoots []ResolvedRoot `json:"resolved_roots,omitempty"`
	ServerBacked  bool           `json:"server_backed,omitempty"`
	ServerEcho    *ServerEcho    `json:"server_echo,omitempty"`
	Reason        Reason         `json:"reason,omitempty"`
}

// Validate checks the successful-transition evidence contract.
func (e Evidence) Validate() error {
	if strings.TrimSpace(e.Action) == "" {
		return errors.New("transition action is required")
	}

	positive := false
	countNames := make(map[string]struct{}, len(e.Counts))
	for i, count := range e.Counts {
		name := strings.TrimSpace(count.Name)
		if name == "" {
			return fmt.Errorf("count %d: name is required", i)
		}
		if _, exists := countNames[name]; exists {
			return fmt.Errorf("count %q is duplicated", name)
		}
		countNames[name] = struct{}{}
		if count.Value < 0 {
			return fmt.Errorf("count %q must not be negative", name)
		}
		positive = positive || count.Value > 0
	}

	rootNames := make(map[string]struct{}, len(e.ResolvedRoots))
	for i, root := range e.ResolvedRoots {
		name := strings.TrimSpace(root.Name)
		if name == "" {
			return fmt.Errorf("resolved root %d: name is required", i)
		}
		if _, exists := rootNames[name]; exists {
			return fmt.Errorf("resolved root %q is duplicated", name)
		}
		rootNames[name] = struct{}{}
		if strings.TrimSpace(root.Declared) == "" {
			return fmt.Errorf("resolved root %q: declared path is required", name)
		}
		if strings.TrimSpace(root.Resolved) == "" {
			return fmt.Errorf("resolved root %q: resolved path is required", name)
		}
	}

	acceptedEcho := false
	if e.ServerEcho != nil {
		if !e.ServerEcho.Accepted {
			return errors.New("server echo does not show acceptance")
		}
		if strings.TrimSpace(e.ServerEcho.Response) == "" {
			return errors.New("accepted server echo response is required")
		}
		acceptedEcho = true
	}
	if e.ServerBacked && !acceptedEcho {
		return errors.New("server-backed transition requires an accepted server echo")
	}

	if !positive && len(e.ResolvedRoots) == 0 && !acceptedEcho && strings.TrimSpace(string(e.Reason)) == "" {
		return errors.New("successful transition has zero evidence; a non-empty reason is required")
	}
	return nil
}

// FinishSuccess validates evidence before a caller reports successful
// completion. P2-P5 transition verbs must call this method or a renderer, both
// of which enforce the same validation.
func (e Evidence) FinishSuccess() error {
	return e.Validate()
}

// RenderHuman validates and writes a deterministic human-readable summary.
func RenderHuman(w io.Writer, evidence Evidence) error {
	e, err := normalized(evidence)
	if err != nil {
		return err
	}

	var b bytes.Buffer
	fmt.Fprintf(&b, "transition: %s\n", e.Action)
	if len(e.Counts) > 0 {
		b.WriteString("counts:\n")
		for _, count := range e.Counts {
			fmt.Fprintf(&b, "  %s: %d\n", count.Name, count.Value)
		}
	}
	if len(e.ResolvedRoots) > 0 {
		b.WriteString("resolved roots:\n")
		for _, root := range e.ResolvedRoots {
			fmt.Fprintf(&b, "  %s: %s -> %s\n", root.Name, root.Declared, root.Resolved)
		}
	}
	if e.ServerEcho != nil {
		fmt.Fprintf(&b, "server accepted: %s", e.ServerEcho.Response)
		if e.ServerEcho.RequestID != "" {
			fmt.Fprintf(&b, " (request %s)", e.ServerEcho.RequestID)
		}
		b.WriteByte('\n')
	}
	if e.Reason != "" {
		fmt.Fprintf(&b, "reason: %s\n", e.Reason)
	}
	_, err = w.Write(b.Bytes())
	return err
}

// RenderJSON validates and writes deterministic indented JSON followed by a
// newline. Object fields and evidence arrays have stable ordering.
func RenderJSON(w io.Writer, evidence Evidence) error {
	e, err := normalized(evidence)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(e)
}

func normalized(e Evidence) (Evidence, error) {
	if err := e.FinishSuccess(); err != nil {
		return Evidence{}, err
	}
	e.Action = strings.TrimSpace(e.Action)
	e.Reason = Reason(strings.TrimSpace(string(e.Reason)))
	e.Counts = append([]Count(nil), e.Counts...)
	e.ResolvedRoots = append([]ResolvedRoot(nil), e.ResolvedRoots...)
	sort.Slice(e.Counts, func(i, j int) bool {
		return e.Counts[i].Name < e.Counts[j].Name
	})
	sort.Slice(e.ResolvedRoots, func(i, j int) bool {
		if e.ResolvedRoots[i].Name != e.ResolvedRoots[j].Name {
			return e.ResolvedRoots[i].Name < e.ResolvedRoots[j].Name
		}
		if e.ResolvedRoots[i].Declared != e.ResolvedRoots[j].Declared {
			return e.ResolvedRoots[i].Declared < e.ResolvedRoots[j].Declared
		}
		return e.ResolvedRoots[i].Resolved < e.ResolvedRoots[j].Resolved
	})
	return e, nil
}
