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
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Protocol versions are negotiated explicitly. ProtocolVersion remains the
// legacy alias so existing v1 clients continue to compile unchanged.
const (
	ProtocolVersionLegacy        = 1
	ProtocolVersionDeviceSession = 2
	ProtocolVersion              = ProtocolVersionLegacy
)

// SupportedProtocolVersions returns the ordered offer used by new clients.
// The order is preference order; servers still select the highest common
// numeric version so reordered offers cannot downgrade a handshake.
func SupportedProtocolVersions() []int {
	return []int{ProtocolVersionDeviceSession, ProtocolVersionLegacy}
}

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
	DeviceEnrollment bool     `json:"device_enrollment,omitempty"` // Server accepts signed device enrollment requests
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
	DeviceID         string `json:"device_id,omitempty"`      // Durable machine identity (ULID; core/pkg/machine, XDG state)
	ServerEpoch      string `json:"server_epoch,omitempty"`   // exact epoch discovered from GET /sync/identity
	Timestamp        string `json:"timestamp,omitempty"`      // canonical UTC timestamp for a v2 proof
	Nonce            string `json:"nonce,omitempty"`          // canonical base64 32-byte random nonce
	Signature        string `json:"signature,omitempty"`      // canonical base64 Ed25519 signature
}

// IdentityResponse is the intentionally-public server identity needed to
// bootstrap a device handshake without a legacy bearer credential.
type IdentityResponse struct {
	ServerEpoch      string `json:"server_epoch"`
	ProtocolVersions []int  `json:"protocol_versions"`
	ServerName       string `json:"server_name,omitempty"`
}

// CapabilitiesResponse is the server's half of the handshake.
type CapabilitiesResponse struct {
	ServerName       string       `json:"server_name"`              // e.g. "grove-syncd"
	ServerVersion    string       `json:"server_version,omitempty"` // Server build version
	ServerEpoch      string       `json:"server_epoch,omitempty"`   // Server database identity, minted on first open; a changed epoch means the server store was recreated and push-only clients must re-push their full document set (empty = pre-epoch server)
	ProtocolVersion  int          `json:"protocol_version"`         // Negotiated version for this connection
	Capabilities     Capabilities `json:"capabilities"`             // Feature set
	SessionToken     string       `json:"session_token,omitempty"`  // raw v2 session bearer, returned once
	SessionExpiresAt string       `json:"session_expires_at,omitempty"`
	Error            string       `json:"error,omitempty"` // Set when negotiation fails (e.g. no common version)
}

// Device enrollment statuses.
const (
	DeviceStatusPending  = "pending"
	DeviceStatusApproved = "approved"
	DeviceStatusRevoked  = "revoked"
)

const (
	enrollmentSignatureDomain   = "grove.sync.enrollment.v1"
	capabilitiesSignatureDomain = "grove.sync.capabilities.v2"
)

// EnrollRequest proves possession of a device public key and asks the server
// to register it. RequestedUser is only a request hint; an unauthenticated
// enrollment must never select its own authority.
type EnrollRequest struct {
	DeviceID      string `json:"device_id"`
	Name          string `json:"name,omitempty"`
	PublicKey     string `json:"public_key"` // canonical base64 of 32 raw Ed25519 bytes
	RequestedUser string `json:"requested_user,omitempty"`
	Code          string `json:"code,omitempty"` // optional one-time enrollment voucher
	Timestamp     string `json:"timestamp"`      // CanonicalTimestamp form
	Signature     string `json:"signature"`      // canonical base64 Ed25519 signature
}

// EnrollResponse is returned for both first enrollment and idempotent retries.
type EnrollResponse struct {
	DeviceID    string `json:"device_id,omitempty"`
	Status      string `json:"status,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Error       string `json:"error,omitempty"`
}

// DeviceInfo is the public, secret-free representation of a registered device.
type DeviceInfo struct {
	DeviceID       string `json:"device_id"`
	Name           string `json:"name,omitempty"`
	PublicKey      string `json:"public_key"`
	Fingerprint    string `json:"fingerprint"`
	UserID         int64  `json:"user_id"`
	Status         string `json:"status"`
	ParentDeviceID string `json:"parent_device_id,omitempty"`
	EnrolledAt     string `json:"enrolled_at"`
	ApprovedAt     string `json:"approved_at,omitempty"`
	ApprovedBy     string `json:"approved_by,omitempty"`
	LastSeen       string `json:"last_seen,omitempty"`
}

// DeviceListResponse returns devices without exposing any credential.
type DeviceListResponse struct {
	Devices []DeviceInfo `json:"devices"`
	Error   string       `json:"error,omitempty"`
}

// DeviceApprovalRequest supplies the authorization assignment made by an
// administrator. Zero UserID means the server's default owner assignment.
type DeviceApprovalRequest struct {
	UserID int64 `json:"user_id,omitempty"`
}

type EnrollCodeRequest struct {
	TTLSeconds int64 `json:"ttl_seconds,omitempty"`
}

type EnrollCodeResponse struct {
	Code      string `json:"code,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
	Error     string `json:"error,omitempty"`
}

type DeviceRevokeResponse struct {
	Devices  int    `json:"devices"`
	Sessions int    `json:"sessions"`
	Error    string `json:"error,omitempty"`
}

// DeviceApprovalResponse reports the row as committed by approval.
type DeviceApprovalResponse struct {
	Device DeviceInfo `json:"device"`
	Error  string     `json:"error,omitempty"`
}

// CanonicalTimestamp renders signed request times in the one accepted UTC form.
func CanonicalTimestamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// CanonicalCapabilities returns the bytes covered by a v2 device handshake.
// The exact ordered protocol offer is signed to prevent negotiation tampering.
func CanonicalCapabilities(req CapabilitiesRequest) ([]byte, error) {
	if req.DeviceID == "" || req.ServerEpoch == "" || req.Timestamp == "" || req.Nonce == "" || len(req.ProtocolVersions) == 0 {
		return nil, fmt.Errorf("device_id, server_epoch, timestamp, nonce, and protocol_versions are required")
	}
	fields := []struct{ name, value string }{
		{"device_id", req.DeviceID},
		{"server_epoch", req.ServerEpoch},
		{"timestamp", req.Timestamp},
		{"nonce", req.Nonce},
	}
	for _, field := range fields {
		if strings.ContainsAny(field.value, "\x00\r\n") {
			return nil, fmt.Errorf("capabilities %s contains a forbidden separator", field.name)
		}
	}
	parsed, err := time.Parse(time.RFC3339Nano, req.Timestamp)
	if err != nil || req.Timestamp != CanonicalTimestamp(parsed) {
		return nil, fmt.Errorf("capabilities timestamp %q is not canonical RFC3339 UTC", req.Timestamp)
	}
	nonce, err := base64.StdEncoding.Strict().DecodeString(req.Nonce)
	if err != nil || len(nonce) != 32 || base64.StdEncoding.EncodeToString(nonce) != req.Nonce {
		return nil, fmt.Errorf("invalid capabilities nonce encoding")
	}
	versions := make([]string, len(req.ProtocolVersions))
	for i, version := range req.ProtocolVersions {
		if version <= 0 {
			return nil, fmt.Errorf("invalid protocol version %d", version)
		}
		versions[i] = fmt.Sprint(version)
	}
	var b strings.Builder
	b.WriteString(capabilitiesSignatureDomain)
	b.WriteByte('\n')
	for _, field := range fields {
		b.WriteString(field.name)
		b.WriteByte('=')
		b.WriteString(field.value)
		b.WriteByte('\n')
	}
	b.WriteString("protocol_versions=")
	b.WriteString(strings.Join(versions, ","))
	b.WriteByte('\n')
	return []byte(b.String()), nil
}

func SetCapabilitiesSignature(req *CapabilitiesRequest, signature []byte) error {
	if req == nil || len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("invalid capabilities signature")
	}
	req.Signature = base64.StdEncoding.EncodeToString(signature)
	return nil
}

func VerifyCapabilities(req CapabilitiesRequest, public ed25519.PublicKey) error {
	payload, err := CanonicalCapabilities(req)
	if err != nil {
		return err
	}
	signature, err := base64.StdEncoding.Strict().DecodeString(req.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize || !ed25519.Verify(public, payload, signature) {
		return fmt.Errorf("invalid capabilities signature")
	}
	return nil
}

// CanonicalEnrollment returns the versioned, domain-separated bytes covered
// by an enrollment signature. It intentionally does not marshal JSON.
func CanonicalEnrollment(req EnrollRequest) ([]byte, error) {
	fields := []struct {
		name, value string
	}{
		{"device_id", req.DeviceID},
		{"name", req.Name},
		{"public_key", req.PublicKey},
		{"requested_user", req.RequestedUser},
	}
	// Preserve the exact Phase-1 canonical bytes when no voucher is present;
	// code-bearing requests sign the code before the timestamp.
	if req.Code != "" {
		fields = append(fields, struct{ name, value string }{"code", req.Code})
	}
	fields = append(fields, struct{ name, value string }{"timestamp", req.Timestamp})
	for _, field := range fields {
		if strings.ContainsAny(field.value, "\x00\r\n") {
			return nil, fmt.Errorf("enrollment %s contains a forbidden separator", field.name)
		}
	}
	if req.DeviceID == "" || req.PublicKey == "" || req.Timestamp == "" {
		return nil, fmt.Errorf("enrollment device_id, public_key, and timestamp are required")
	}
	parsed, err := time.Parse(time.RFC3339Nano, req.Timestamp)
	if err != nil || req.Timestamp != CanonicalTimestamp(parsed) {
		return nil, fmt.Errorf("enrollment timestamp %q is not canonical RFC3339 UTC", req.Timestamp)
	}
	if _, err := DecodeDevicePublicKey(req.PublicKey); err != nil {
		return nil, err
	}

	var b strings.Builder
	b.WriteString(enrollmentSignatureDomain)
	b.WriteByte('\n')
	for _, field := range fields {
		b.WriteString(field.name)
		b.WriteByte('=')
		b.WriteString(field.value)
		b.WriteByte('\n')
	}
	return []byte(b.String()), nil
}

// EnrollmentSigningBytes is the explicit signing-oriented name for
// CanonicalEnrollment.
func EnrollmentSigningBytes(req EnrollRequest) ([]byte, error) {
	return CanonicalEnrollment(req)
}

// SetEnrollmentSignature installs the canonical base64 representation of a
// raw Ed25519 signature produced by a custody package.
func SetEnrollmentSignature(req *EnrollRequest, signature []byte) error {
	if req == nil {
		return fmt.Errorf("enrollment request is nil")
	}
	if len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("invalid Ed25519 signature length %d", len(signature))
	}
	req.Signature = base64.StdEncoding.EncodeToString(signature)
	return nil
}

// SignEnrollment canonicalizes req and fills its Signature.
func SignEnrollment(req *EnrollRequest, private ed25519.PrivateKey) error {
	if req == nil {
		return fmt.Errorf("enrollment request is nil")
	}
	if len(private) != ed25519.PrivateKeySize {
		return fmt.Errorf("invalid Ed25519 private key length %d", len(private))
	}
	payload, err := CanonicalEnrollment(*req)
	if err != nil {
		return err
	}
	req.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(private, payload))
	return nil
}

// VerifyEnrollment verifies an enrollment's canonical proof of possession.
func VerifyEnrollment(req EnrollRequest) error {
	public, err := DecodeDevicePublicKey(req.PublicKey)
	if err != nil {
		return err
	}
	payload, err := CanonicalEnrollment(req)
	if err != nil {
		return err
	}
	signature, err := base64.StdEncoding.Strict().DecodeString(req.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("invalid enrollment signature encoding")
	}
	if !ed25519.Verify(public, payload, signature) {
		return fmt.Errorf("invalid enrollment signature")
	}
	return nil
}

// DecodeDevicePublicKey parses the canonical wire encoding of a public key.
func DecodeDevicePublicKey(encoded string) (ed25519.PublicKey, error) {
	decoded, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(decoded) != ed25519.PublicKeySize || base64.StdEncoding.EncodeToString(decoded) != encoded {
		return nil, fmt.Errorf("invalid Ed25519 public key encoding")
	}
	return ed25519.PublicKey(decoded), nil
}

// DeviceFingerprint returns the full lowercase hexadecimal SHA-256 digest of
// the decoded raw public key.
func DeviceFingerprint(encodedPublicKey string) (string, error) {
	public, err := DecodeDevicePublicKey(encodedPublicKey)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(public)
	return hex.EncodeToString(sum[:]), nil
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
	Content          []byte `json:"content,omitempty"`
	ContentEncoding  string `json:"content_encoding,omitempty"`   // Defaults to plaintext
	Size             int64  `json:"size,omitempty"`               // Content size in bytes
	OriginID         string `json:"origin_id,omitempty"`          // Originating installation (echo suppression dedup key)
	Actor            string `json:"actor,omitempty"`              // Display-only client-asserted author identity
	VerifiedDeviceID string `json:"verified_device_id,omitempty"` // Server-stamped authenticated device; empty for service credentials
	// ReceivedAt is the server-arrival timestamp; it defines ordering.
	// Client timestamps are never compared.
	ReceivedAt time.Time `json:"received_at,omitzero"`
	// Mtime is the source file's modification time, captured client-side at
	// enqueue. It is fidelity metadata ONLY — replicas restore it via
	// os.Chtimes after writing — and is NEVER an ordering, OCC, or conflict
	// input (the "client timestamps are never compared" invariant stands).
	// Zero means unknown (old client/server): replicas keep the write time.
	Mtime time.Time `json:"mtime,omitzero"`
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
	// Mtime is the source file's modification time as last pushed (fidelity
	// metadata only, never compared; zero = unknown). Snapshot hydration uses
	// it to restore mtimes on freshly materialized replicas.
	Mtime time.Time `json:"mtime,omitzero"`
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
