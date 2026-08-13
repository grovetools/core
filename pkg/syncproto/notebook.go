package syncproto

import (
	"strings"

	"github.com/oklog/ulid/v2"
)

// Notebook-scope wire types (P3). They extend protocol v3 rather than
// replacing it: every request here embeds the same RequestIdentity, carries
// the same idempotency discipline, and keys on the same ULIDs as the
// notespace-identity surfaces in contract.go.
//
// The model in one paragraph: the notebook is the only sync knob. A notespace
// is shared because the notebook containing it is shared — containment is
// consent — so the server records membership (notespace → notebook) and share
// state (per notebook), and nothing else. Moving a notespace between notebooks
// is a re-parent that preserves the notespace id and therefore its entire
// stream, history and cursors. Unsharing is forward-only: the server retains
// everything, copies pulled elsewhere are never retracted, and re-sharing the
// same notebook id resumes the same history (D9).

// NotebookID is a notebook's durable ULID, minted client-side into
// .notebook.toml. Like NotespaceID it is a distinct wire type because it
// routes; a notebook NAME is display-only payload and never routes.
type NotebookID string

func (id NotebookID) String() string { return string(id) }

// Notebook share states. There is deliberately no third state: a notebook the
// server has never heard of is absent, not "unknown".
const (
	NotebookShareStateShared   = "shared"
	NotebookShareStateUnshared = "unshared"
)

// Member dispositions reported per notespace by a share, so the verb can print
// evidence for every notespace it touched rather than a bare success.
const (
	MemberDispositionAttached      = "attached"       // was unparented, now belongs to this notebook
	MemberDispositionAlreadyMember = "already-member" // no change
	MemberDispositionRejected      = "rejected"       // named in the request, not applied
)

// Structured error codes added by the notebook scope model.
const (
	ErrorUnregisteredNotebook = "unregistered_notebook"
	ErrorNotebookUnshared     = "notebook_unshared"
	ErrorMembershipConflict   = "membership_conflict"
)

// NotebookShareRequest registers a notebook identity and declares its
// membership. It is the wire half of `grove notebook share`, and it is
// idempotent: repeating it with the same members is a no-op that still returns
// full per-member evidence.
//
// Members must already be registered notespaces. Sharing never re-parents: a
// member currently belonging to a DIFFERENT notebook is rejected and the whole
// request fails, because moving a notespace between notebooks is an explicit
// act with its own verb (NotespaceReparentRequest). A share that silently
// stole members would make the one mechanism two.
type NotebookShareRequest struct {
	RequestIdentity
	NotebookID NotebookID    `json:"notebook_id"`
	Name       string        `json:"name"`
	Members    []NotespaceID `json:"members,omitempty"`
}

func (r NotebookShareRequest) Validate() *ProtocolError {
	if err := r.RequestIdentity.Validate(); err != nil {
		return err
	}
	if _, err := ulid.ParseStrict(r.NotebookID.String()); err != nil {
		return &ProtocolError{Code: ErrorRegistrationConflict, Message: "notebook_id must be a ULID"}
	}
	if strings.TrimSpace(r.Name) == "" || strings.ContainsAny(r.Name, "\r\n\t") {
		return &ProtocolError{Code: ErrorRegistrationConflict, Message: "notebook name is required"}
	}
	seen := make(map[NotespaceID]struct{}, len(r.Members))
	for _, member := range r.Members {
		if _, err := ulid.ParseStrict(member.String()); err != nil {
			return &ProtocolError{Code: ErrorRegistrationConflict, Message: "every member must be a notespace ULID"}
		}
		if _, dup := seen[member]; dup {
			return &ProtocolError{Code: ErrorRegistrationConflict, Message: "member " + member.String() + " is listed twice"}
		}
		seen[member] = struct{}{}
	}
	return nil
}

// NotebookMemberResult is one notespace's outcome inside a share. Evidence is
// per-notespace by design: "shared 12 notespaces" is not evidence, a list is.
type NotebookMemberResult struct {
	NotespaceID    NotespaceID    `json:"notespace_id"`
	Disposition    string         `json:"disposition"`
	FromNotebookID NotebookID     `json:"from_notebook_id,omitempty"`
	Error          *ProtocolError `json:"error,omitempty"`
}

type NotebookShareResponse struct {
	NotebookID NotebookID `json:"notebook_id"`
	Name       string     `json:"name"`
	ShareState string     `json:"share_state"`
	Version    int64      `json:"version"`
	// Resumed marks a re-share of a notebook id the server had unshared: the
	// same history resumes, which is the visible half of D9's promise.
	Resumed bool                   `json:"resumed,omitempty"`
	Members []NotebookMemberResult `json:"members"`
	Error   *ProtocolError         `json:"error,omitempty"`
}

// NotebookUnshareRequest stops sharing a notebook, forward-only (D9).
// ExpectedVersion makes a stale operator decision fail rather than clobber a
// newer one.
type NotebookUnshareRequest struct {
	RequestIdentity
	NotebookID      NotebookID `json:"notebook_id"`
	ExpectedVersion int64      `json:"expected_version"`
}

func (r NotebookUnshareRequest) Validate() *ProtocolError {
	if err := r.RequestIdentity.Validate(); err != nil {
		return err
	}
	if _, err := ulid.ParseStrict(r.NotebookID.String()); err != nil {
		return &ProtocolError{Code: ErrorRegistrationConflict, Message: "notebook_id must be a ULID"}
	}
	if r.ExpectedVersion <= 0 {
		return &ProtocolError{Code: ErrorStaleResolution, Message: "positive expected_version is required"}
	}
	return nil
}

// UnshareRetentionStatement is the server's own words for what unsharing does
// and does not do. It ships in the response so the client's user copy and the
// server's behavior cannot drift apart: the sentence a user reads is the
// sentence the server sent.
const UnshareRetentionStatement = "unshare is forward-only: this server retains the notebook's notespaces and their full history, copies already pulled elsewhere are not retracted, and re-sharing this notebook id resumes the same history"

type NotebookUnshareResponse struct {
	NotebookID NotebookID `json:"notebook_id"`
	ShareState string     `json:"share_state"`
	Version    int64      `json:"version"`
	// RetainedNotespaces counts what the server kept — the evidence that
	// forward-only means retained, not deleted.
	RetainedNotespaces int            `json:"retained_notespaces"`
	RetainedHistory    bool           `json:"retained_history"`
	Retention          string         `json:"retention,omitempty"`
	Error              *ProtocolError `json:"error,omitempty"`
}

// NotespaceReparentRequest moves one notespace between notebooks atomically.
// The notespace id — and therefore its event stream, documents, and every
// client cursor keyed on it — is untouched; only membership moves. This is the
// server half of `grove notespace move`.
type NotespaceReparentRequest struct {
	RequestIdentity
	NotespaceID NotespaceID `json:"notespace_id"`
	// FromNotebookID is the membership the client believes it is moving out
	// of; empty means "currently unparented". It is checked, not trusted: a
	// client working from a stale view must fail rather than re-parent a
	// notespace out of a notebook it did not know about.
	FromNotebookID  NotebookID `json:"from_notebook_id,omitempty"`
	ToNotebookID    NotebookID `json:"to_notebook_id"`
	ExpectedVersion int64      `json:"expected_version"`
}

func (r NotespaceReparentRequest) Validate() *ProtocolError {
	if err := r.RequestIdentity.Validate(); err != nil {
		return err
	}
	if _, err := ulid.ParseStrict(r.NotespaceID.String()); err != nil {
		return &ProtocolError{Code: ErrorRegistrationConflict, Message: "notespace_id must be a ULID"}
	}
	if _, err := ulid.ParseStrict(r.ToNotebookID.String()); err != nil {
		return &ProtocolError{Code: ErrorRegistrationConflict, Message: "to_notebook_id must be a ULID"}
	}
	if r.FromNotebookID != "" {
		if _, err := ulid.ParseStrict(r.FromNotebookID.String()); err != nil {
			return &ProtocolError{Code: ErrorRegistrationConflict, Message: "from_notebook_id must be a ULID when present"}
		}
	}
	if r.FromNotebookID == r.ToNotebookID {
		return &ProtocolError{Code: ErrorMembershipConflict, Message: "from_notebook_id and to_notebook_id must differ"}
	}
	if r.ExpectedVersion < 0 {
		return &ProtocolError{Code: ErrorStaleResolution, Message: "expected_version must not be negative"}
	}
	return nil
}

type NotespaceReparentResponse struct {
	NotespaceID    NotespaceID `json:"notespace_id"`
	FromNotebookID NotebookID  `json:"from_notebook_id,omitempty"`
	ToNotebookID   NotebookID  `json:"to_notebook_id"`
	Version        int64       `json:"version"`
	// Cursor is the notespace's event-log head after the move. A re-parent
	// must not advance it; the caller asserting cursor-in == cursor-out is how
	// "same id, cursors intact" stops being a claim and becomes a check.
	Cursor           int64          `json:"cursor"`
	HistoryPreserved bool           `json:"history_preserved"`
	Error            *ProtocolError `json:"error,omitempty"`
}
