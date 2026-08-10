// Package transition defines the evidence contract for successful state-changing
// commands.
//
// Grove P2-P5 transition verbs MUST construct Evidence and call FinishSuccess
// (directly or through RenderHuman or RenderJSON) before reporting success. This
// prevents an empty success from being presented as a completed transition.
// Server-backed verbs MUST create a ServerReceipt from the submitted request and
// the distinct response returned by the server. The request sent to the server
// is not evidence of acceptance.
package transition

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
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

// ServerReceipt is opaque evidence derived from a server's accepted response.
// It can only be created with NewServerReceipt, which binds it to the submitted
// request and rejects a request passed back as its own response.
type ServerReceipt struct {
	response       string
	requestID      string
	requestDigest  [sha256.Size]byte
	responseDigest [sha256.Size]byte
	sealed         bool
}

// NewServerReceipt captures acceptance evidence from response data returned by
// a server. request and response must both be non-empty and response must be
// distinct from request. Callers must invoke this only after the server's
// protocol has identified response as accepted; there is deliberately no
// caller-settable Accepted flag.
func NewServerReceipt(request, response, requestID string) (*ServerReceipt, error) {
	if strings.TrimSpace(request) == "" {
		return nil, errors.New("server request is required")
	}
	if strings.TrimSpace(response) == "" {
		return nil, errors.New("accepted server response is required")
	}

	requestDigest := sha256.Sum256([]byte(request))
	responseDigest := sha256.Sum256([]byte(response))
	if requestDigest == responseDigest {
		return nil, errors.New("server response must be distinct from the submitted request")
	}

	return &ServerReceipt{
		response:       response,
		requestID:      requestID,
		requestDigest:  requestDigest,
		responseDigest: responseDigest,
		sealed:         true,
	}, nil
}

// MarshalJSON exposes the accepted response while keeping the receipt's
// construction seal and request binding private.
func (r ServerReceipt) MarshalJSON() ([]byte, error) {
	if err := r.validate(); err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Accepted  bool   `json:"accepted"`
		Response  string `json:"response"`
		RequestID string `json:"request_id,omitempty"`
	}{
		Accepted:  true,
		Response:  r.response,
		RequestID: r.requestID,
	})
}

func (r ServerReceipt) validate() error {
	if !r.sealed {
		return errors.New("server receipt was not created from an accepted response")
	}
	if strings.TrimSpace(r.response) == "" {
		return errors.New("accepted server response is required")
	}
	if sha256.Sum256([]byte(r.response)) != r.responseDigest {
		return errors.New("server receipt response does not match its acceptance binding")
	}
	if r.requestDigest == r.responseDigest {
		return errors.New("server response must be distinct from the submitted request")
	}
	return nil
}

// Evidence is the final, user-visible proof of a successful transition.
// Action names the transition. Slice order supplied by callers is ignored by
// renderers; counts and roots are rendered in canonical order. The presence of
// ServerReceipt is the sole, unambiguous server-backed state.
type Evidence struct {
	Action        string         `json:"action"`
	Counts        []Count        `json:"counts,omitempty"`
	ResolvedRoots []ResolvedRoot `json:"resolved_roots,omitempty"`
	ServerReceipt *ServerReceipt `json:"server_receipt,omitempty"`
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

	acceptedResponse := false
	if e.ServerReceipt != nil {
		if err := e.ServerReceipt.validate(); err != nil {
			return err
		}
		acceptedResponse = true
	}

	if !positive && len(e.ResolvedRoots) == 0 && !acceptedResponse && strings.TrimSpace(string(e.Reason)) == "" {
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
// Every caller-controlled string is quoted, so control characters cannot forge
// additional lines in the evidence grammar.
func RenderHuman(w io.Writer, evidence Evidence) error {
	e, err := normalized(evidence)
	if err != nil {
		return err
	}

	var b bytes.Buffer
	fmt.Fprintf(&b, "transition: %s\n", humanString(e.Action))
	if len(e.Counts) > 0 {
		b.WriteString("counts:\n")
		for _, count := range e.Counts {
			fmt.Fprintf(&b, "  %s: %d\n", humanString(count.Name), count.Value)
		}
	}
	if len(e.ResolvedRoots) > 0 {
		b.WriteString("resolved roots:\n")
		for _, root := range e.ResolvedRoots {
			fmt.Fprintf(&b, "  %s: %s -> %s\n", humanString(root.Name), humanString(root.Declared), humanString(root.Resolved))
		}
	}
	if e.ServerReceipt != nil {
		fmt.Fprintf(&b, "server accepted: %s", humanString(e.ServerReceipt.response))
		if e.ServerReceipt.requestID != "" {
			fmt.Fprintf(&b, " (request %s)", humanString(e.ServerReceipt.requestID))
		}
		b.WriteByte('\n')
	}
	if e.Reason != "" {
		fmt.Fprintf(&b, "reason: %s\n", humanString(string(e.Reason)))
	}
	_, err = w.Write(b.Bytes())
	return err
}

func humanString(value string) string {
	return strconv.QuoteToGraphic(value)
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
