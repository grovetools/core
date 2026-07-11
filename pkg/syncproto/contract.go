// Package syncproto defines the wire types for the notebook sync protocol:
// the contract between sync clients (the daemon's SyncHandler) and any
// grove-syncd-compatible server. It mirrors core/pkg/env/contract.go — plain
// structs with JSON tags and no transport logic — so MIT clients and
// third-party servers share one vocabulary.
//
// Protocol shape (see the sync protocol plan):
//   - Push: batched POST of SyncEvents; per-document Version is the
//     concurrency token.
//   - Pull: cursor-based replay of the per-workspace durable event log.
//   - Capabilities: version/feature negotiation performed once per
//     connection before push/pull.
//   - Snapshot: a manifest of {id, path, version, hash, size} + cursor,
//     not a tarball — resumable and free for hash-equal adoption.
//
// Events carry metadata (id/path/hash/version/actor), never content, except
// on push where inline content for small documents rides along. Ordering is
// server-arrival (event log sequence); client wall-clocks are never compared.
package syncproto

import (
	"path/filepath"
	"time"
)

// ProtocolVersion is the sync protocol version this package describes.
const ProtocolVersion = 1

// Sync event types. Renames are first-class moved events (emitted from nb's
// typed move notifications), and prefix events cover directory-level
// operations such as plan archival.
const (
	EventDocumentCreated = "document_created"
	EventDocumentUpdated = "document_updated"
	EventDocumentMoved   = "document_moved"
	EventDocumentDeleted = "document_deleted"
	EventPrefixMoved     = "prefix_moved"
	EventPrefixDeleted   = "prefix_deleted"
)

// Content encodings. Only plaintext is implemented; aes256gcm is reserved so
// an opt-in E2EE workspace mode can arrive without a protocol break.
const (
	ContentEncodingPlaintext = "plaintext"
	ContentEncodingAES256GCM = "aes256gcm"
)

// NormalizePath converts a client-local path to the protocol's wire form.
// The protocol mandates forward-slash, workspace-relative paths: a Windows
// client pushing `plans\x\y.md` must not create a distinct document.
func NormalizePath(path string) string {
	return filepath.ToSlash(path)
}

// LocalizePath converts a wire path back to the client's native separator
// for local filesystem I/O.
func LocalizePath(path string) string {
	return filepath.FromSlash(path)
}

// Capabilities describes the optional features a server supports, negotiated
// via the capabilities handshake before any push/pull traffic.
type Capabilities struct {
	ProtocolVersions []int    `json:"protocol_versions"`           // Protocol versions the server accepts
	Blobs            bool     `json:"blobs,omitempty"`             // Content-addressed blob tier for large documents
	Notify           bool     `json:"notify,omitempty"`            // SSE notify-poke channel ("workspace advanced to seq N")
	Search           bool     `json:"search,omitempty"`            // Server-side search (Phase 3)
	MaxInlineSize    int64    `json:"max_inline_size,omitempty"`   // Largest document stored inline, in bytes (default 256KB)
	BlobChunkSize    int64    `json:"blob_chunk_size,omitempty"`   // Fixed chunk size for the blob tier, in bytes (default 4MB)
	MaxBlobSize      int64    `json:"max_blob_size,omitempty"`     // Largest single blob the server accepts, in bytes (0 = unadvertised)
	Compression      []string `json:"compression,omitempty"`       // Supported blob compressions (e.g. "zstd")
	ContentEncodings []string `json:"content_encodings,omitempty"` // Supported document content encodings
}

// SupportsVersion reports whether the server accepts the given protocol version.
func (c *Capabilities) SupportsVersion(v int) bool {
	for _, pv := range c.ProtocolVersions {
		if pv == v {
			return true
		}
	}
	return false
}

// CapabilitiesRequest is sent by a client to negotiate protocol version and
// features. OriginID identifies the installation for echo suppression and is
// distinct from the user/actor identity carried by the auth token.
type CapabilitiesRequest struct {
	ClientName       string `json:"client_name"`              // e.g. "groved"
	ClientVersion    string `json:"client_version,omitempty"` // Client build version
	ProtocolVersions []int  `json:"protocol_versions"`        // Versions the client speaks
	OriginID         string `json:"origin_id,omitempty"`      // Persistent per-install origin id
	DeviceID         string `json:"device_id,omitempty"`      // Machine identity (from ~/.config/grove/machines)
}

// CapabilitiesResponse is the server's half of the handshake.
type CapabilitiesResponse struct {
	ServerName      string       `json:"server_name"`              // e.g. "grove-syncd"
	ServerVersion   string       `json:"server_version,omitempty"` // Server build version
	ProtocolVersion int          `json:"protocol_version"`         // Negotiated version for this connection
	Capabilities    Capabilities `json:"capabilities"`             // Feature set
	Error           string       `json:"error,omitempty"`          // Set when negotiation fails (e.g. no common version)
}

// SyncEvent is one entry in a workspace's append-only event log. On push the
// server assigns Seq and Version; on pull both are populated. Events carry
// metadata only — content rides inline on push for small documents and is
// otherwise fetched separately — which keeps logs and SSE payloads free of
// document bodies.
type SyncEvent struct {
	Seq        int64  `json:"seq,omitempty"`         // Server-assigned log sequence (0 on push)
	Type       string `json:"type"`                  // One of the Event* constants
	Workspace  string `json:"workspace"`             // Sync workspace name
	DocumentID string `json:"document_id,omitempty"` // Server-allocated UUID (empty on first push of a new document)
	Path       string `json:"path"`                  // Slash-normalized workspace-relative path (or prefix for prefix events)
	PrevPath   string `json:"prev_path,omitempty"`   // Previous path for moved/prefix_moved events
	// ContentHash is the SHA-256 of the document content (hex). Hash-gating
	// on both push and pull is the echo-suppression backstop.
	ContentHash string `json:"content_hash,omitempty"`
	// BaseVersion is the document version this change was made against; the
	// server rejects pushes whose base is stale (concurrency token).
	BaseVersion int64 `json:"base_version,omitempty"`
	// Version is the per-document monotonic version assigned by the server
	// when the event is accepted.
	Version int64 `json:"version,omitempty"`
	// Content is the inline document body, present only on push for
	// documents at or below the server's MaxInlineSize. Larger documents go
	// through the blob tier.
	Content         []byte `json:"content,omitempty"`
	ContentEncoding string `json:"content_encoding,omitempty"` // Defaults to plaintext
	Size            int64  `json:"size,omitempty"`             // Content size in bytes
	OriginID        string `json:"origin_id,omitempty"`        // Originating installation (echo suppression dedup key)
	Actor           string `json:"actor,omitempty"`            // Display-only author identity
	// ReceivedAt is the server-arrival timestamp; it defines ordering.
	// Client timestamps are never compared.
	ReceivedAt time.Time `json:"received_at,omitzero"`
}

// PushRequest is a batched client→server upload of local changes for one
// workspace.
type PushRequest struct {
	Workspace string      `json:"workspace"`
	OriginID  string      `json:"origin_id"` // Persistent per-install origin id
	DeviceID  string      `json:"device_id,omitempty"`
	Events    []SyncEvent `json:"events"`
}

// Push result statuses.
const (
	PushStatusAccepted = "accepted" // Event applied; DocumentID/Version assigned
	PushStatusConflict = "conflict" // BaseVersion was stale; client must merge and re-push
	PushStatusRejected = "rejected" // Event invalid (see Error)
)

// PushResult reports the outcome of a single pushed event, in request order.
type PushResult struct {
	Status     string `json:"status"`                // One of the PushStatus* constants
	DocumentID string `json:"document_id,omitempty"` // Allocated/confirmed document UUID
	Version    int64  `json:"version,omitempty"`     // New document version when accepted
	Seq        int64  `json:"seq,omitempty"`         // Event log sequence when accepted
	Error      string `json:"error,omitempty"`       // Populated for rejected events
}

// PushResponse answers a PushRequest with one result per event.
type PushResponse struct {
	Results []PushResult `json:"results"`
	Cursor  int64        `json:"cursor,omitempty"` // Workspace log head after this push
	Error   string       `json:"error,omitempty"`  // Request-level failure
}

// PullRequest replays a workspace's event log from a cursor. Wait enables
// long-polling: the server holds the request open up to the given duration
// when no events are available.
type PullRequest struct {
	Workspace     string `json:"workspace"`
	Cursor        int64  `json:"cursor"`                   // Replay events with Seq > Cursor
	Limit         int    `json:"limit,omitempty"`          // Max events to return (server may cap)
	Wait          string `json:"wait,omitempty"`           // Long-poll duration (e.g. "30s")
	ExcludeOrigin string `json:"exclude_origin,omitempty"` // Skip events pushed by this origin (echo suppression)
}

// PullResponse carries replayed events and the new cursor. A client whose
// cursor has been garbage-collected past the tombstone retention window gets
// SnapshotRequired and must resync from the manifest.
type PullResponse struct {
	Events           []SyncEvent `json:"events"`
	Cursor           int64       `json:"cursor"`                      // Resume point for the next pull
	More             bool        `json:"more,omitempty"`              // Further events available beyond Limit
	SnapshotRequired bool        `json:"snapshot_required,omitempty"` // Cursor too old; resync via snapshot manifest
	Error            string      `json:"error,omitempty"`
}

// DocumentSnapshot is one entry in a workspace's snapshot manifest.
type DocumentSnapshot struct {
	ID      string `json:"id"`      // Document UUID
	Path    string `json:"path"`    // Slash-normalized workspace-relative path
	Version int64  `json:"version"` // Current document version
	Hash    string `json:"hash"`    // SHA-256 of current content (hex)
	Size    int64  `json:"size"`    // Content size in bytes
}

// SnapshotManifest is the resumable snapshot form: the full document listing
// of a workspace plus the log cursor it corresponds to. Hash-equal local
// files are adopted in place with no write; only divergent documents need
// fetching. The manifest doubles as the periodic anti-entropy pass.
type SnapshotManifest struct {
	Workspace string             `json:"workspace"`
	Cursor    int64              `json:"cursor"` // Log position this manifest reflects
	Documents []DocumentSnapshot `json:"documents"`
}
