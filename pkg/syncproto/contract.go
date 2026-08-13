// Package syncproto defines the wire types for the notebook sync protocol:
// the contract between sync clients (the daemon's SyncHandler) and any
// grove-syncd-compatible server. It mirrors core/pkg/env/contract.go — plain
// structs with JSON tags and no transport logic — so MIT clients and
// third-party servers share one vocabulary.
//
// Protocol shape (see the sync protocol plan):
//   - Push: batched POST of SyncEvents; per-document Version is the
//     concurrency token.
//   - Pull: cursor-based replay of the per-notespace durable event log.
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

	"github.com/oklog/ulid/v2"

	"github.com/grovetools/core/pkg/subject"
)

// Protocol versions are negotiated explicitly. ProtocolVersion remains the
// legacy alias so existing v1 clients continue to compile unchanged.
const (
	ProtocolVersionLegacy        = 1
	ProtocolVersionDeviceSession = 2
	ProtocolVersionNotespaceID   = 3
	ProtocolVersion              = ProtocolVersionNotespaceID
)

// SupportedProtocolVersions returns the ordered offer used by new clients.
// The order is preference order; servers still select the highest common
// numeric version so reordered offers cannot downgrade a handshake.
func SupportedProtocolVersions() []int {
	return []int{ProtocolVersionNotespaceID, ProtocolVersionDeviceSession, ProtocolVersionLegacy}
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
// an opt-in E2EE notespace mode can arrive without a protocol break.
const (
	ContentEncodingPlaintext = "plaintext"
	ContentEncodingAES256GCM = "aes256gcm"
)

// NormalizePath converts a client-local path to the protocol's wire form.
// The protocol mandates forward-slash, notespace-relative paths: a Windows
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
	Notify           bool     `json:"notify,omitempty"`            // SSE notify-poke channel ("notespace advanced to seq N")
	Search           bool     `json:"search,omitempty"`            // Server-side search (Phase 3)
	DeviceEnrollment bool     `json:"device_enrollment,omitempty"` // Server accepts signed device enrollment requests
	NotebookScope    bool     `json:"notebook_scope,omitempty"`    // Notebook-grained share/membership surfaces (share, unshare, reparent, inventory membership) are served
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

// Structured protocol error codes. They are stable machine values; Message is
// display-only and clients must branch on Code.
const (
	ErrorProtocolVersion       = "protocol_version"
	ErrorUnregisteredNotespace = "unregistered_notespace"
	ErrorRegistrationConflict  = "registration_conflict"
	ErrorStaleResolution       = "stale_resolution"
)

type ProtocolError struct {
	Code              string `json:"code"`
	Message           string `json:"message,omitempty"`
	ConflictID        string `json:"conflict_id,omitempty"`
	CurrentVersion    int64  `json:"current_version,omitempty"`
	SupportedVersions []int  `json:"supported_versions,omitempty"`
}

func (e *ProtocolError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Code + ": " + e.Message
	}
	return e.Code
}

type ErrorResponse struct {
	Error *ProtocolError `json:"error"`
}

// NotespaceID and NotespaceName are deliberately distinct wire types: routing
// APIs accept the former, while the latter is display-only payload.
type (
	NotespaceID   string
	NotespaceName string
)

func (id NotespaceID) String() string     { return string(id) }
func (name NotespaceName) String() string { return string(name) }

// RequestIdentity is embedded by authenticated, replay-safe v3 mutations.
// DeviceID is attribution claimed by the client and must match the verified
// v2/v3 device session at the server.
type RequestIdentity struct {
	ProtocolVersion int    `json:"protocol_version"`
	IdempotencyKey  string `json:"idempotency_key"`
	DeviceID        string `json:"device_id"`
}

func (r RequestIdentity) Validate() *ProtocolError {
	if r.ProtocolVersion != ProtocolVersionNotespaceID {
		return &ProtocolError{Code: ErrorProtocolVersion, Message: "notespace identity protocol v3 is required", SupportedVersions: []int{ProtocolVersionNotespaceID}}
	}
	if strings.TrimSpace(r.IdempotencyKey) == "" || strings.ContainsAny(r.IdempotencyKey, "\r\n\t") {
		return &ProtocolError{Code: ErrorRegistrationConflict, Message: "idempotency_key is required"}
	}
	if strings.TrimSpace(r.DeviceID) == "" || strings.ContainsAny(r.DeviceID, "\r\n\t") {
		return &ProtocolError{Code: ErrorRegistrationConflict, Message: "authenticated device_id is required"}
	}
	return nil
}

const (
	RegistrationIntentInherit        = "inherit"
	RegistrationIntentProposePrimary = "propose-primary"
	RegistrationIntentCreateSibling  = "create-sibling"
	RegistrationIntentReconcile      = "reconcile"
)

type RegisterRequest struct {
	RequestIdentity
	Intent              string        `json:"intent"`
	Subject             string        `json:"subject"`
	NotespaceName       NotespaceName `json:"notespace_name"`
	Kind                string        `json:"kind"`
	ProposedNotespaceID NotespaceID   `json:"proposed_notespace_id,omitempty"`
}

func (r RegisterRequest) Validate() *ProtocolError {
	if err := r.RequestIdentity.Validate(); err != nil {
		return err
	}
	switch r.Intent {
	case RegistrationIntentInherit, RegistrationIntentProposePrimary, RegistrationIntentCreateSibling, RegistrationIntentReconcile:
	default:
		return &ProtocolError{Code: ErrorRegistrationConflict, Message: "explicit registration intent is required"}
	}
	if err := subject.Validate(r.Subject); err != nil {
		return &ProtocolError{Code: ErrorRegistrationConflict, Message: err.Error()}
	}
	if strings.TrimSpace(r.NotespaceName.String()) == "" || strings.TrimSpace(r.Kind) == "" {
		return &ProtocolError{Code: ErrorRegistrationConflict, Message: "notespace_name and kind are required"}
	}
	if r.Intent == RegistrationIntentInherit && r.ProposedNotespaceID != "" {
		return &ProtocolError{Code: ErrorRegistrationConflict, Message: "inherit must not propose an id"}
	}
	if r.Intent != RegistrationIntentInherit {
		if _, err := ulid.ParseStrict(r.ProposedNotespaceID.String()); err != nil {
			return &ProtocolError{Code: ErrorRegistrationConflict, Message: "proposed_notespace_id must be a ULID for this intent"}
		}
	}
	return nil
}

type RegisterResponse struct {
	NotespaceID  NotespaceID    `json:"notespace_id"`
	Inherited    bool           `json:"inherited,omitempty"`
	ClaimVersion int64          `json:"claim_version"`
	ServerEcho   string         `json:"server_echo,omitempty"`
	Error        *ProtocolError `json:"error,omitempty"`
}

type InventoryRequest struct {
	RequestIdentity
}

type InventoryNotebook struct {
	ID   NotebookID `json:"id"`
	Name string     `json:"name"`
	// ShareState and Version make the inventory answerable on its own: a join
	// delta must be able to tell a notebook that is shared from one this
	// server retains after an unshare (D9), without a second round trip.
	ShareState string `json:"share_state,omitempty"`
	Version    int64  `json:"version,omitempty"`
	// NotespaceIDs is the membership roll, ordered. It is the same fact as
	// InventoryNotespace.NotebookID read from the other side, carried twice
	// on purpose so neither a notebook-first nor a notespace-first client has
	// to reconstruct it.
	NotespaceIDs []NotespaceID `json:"notespace_ids,omitempty"`
}

type InventoryNotespace struct {
	ID         NotespaceID   `json:"id"`
	Name       NotespaceName `json:"name"`
	Subject    string        `json:"subject"`
	Kind       string        `json:"kind"`
	NotebookID NotebookID    `json:"notebook_id,omitempty"`
	Aliases    []string      `json:"aliases,omitempty"`
}

type SubjectClaim struct {
	Subject              string            `json:"subject"`
	InheritedNotespaceID NotespaceID       `json:"inherited_notespace_id"`
	Version              int64             `json:"version"`
	Metadata             map[string]string `json:"metadata,omitempty"`
}

type InventoryResponse struct {
	Notebooks  []InventoryNotebook  `json:"notebooks"`
	Notespaces []InventoryNotespace `json:"notespaces"`
	Claims     []SubjectClaim       `json:"subject_claims,omitempty"`
	Error      *ProtocolError       `json:"error,omitempty"`
}

// RegistrationResolutionRequest resolves one conflict atomically. ExpectedVersion
// prevents a stale operator decision from overwriting a newer resolution.
type RegistrationResolutionRequest struct {
	RequestIdentity
	ConflictID          string      `json:"conflict_id"`
	SurvivorNotespaceID NotespaceID `json:"survivor_notespace_id"`
	LoserNotespaceID    NotespaceID `json:"loser_notespace_id"`
	LoserDisposition    string      `json:"loser_disposition"` // create-sibling or reconcile
	ExpectedVersion     int64       `json:"expected_version"`
}

func (r RegistrationResolutionRequest) Validate() *ProtocolError {
	if err := r.RequestIdentity.Validate(); err != nil {
		return err
	}
	if r.ConflictID == "" || r.SurvivorNotespaceID == "" || r.LoserNotespaceID == "" || r.ExpectedVersion <= 0 {
		return &ProtocolError{Code: ErrorStaleResolution, Message: "conflict, survivor, loser, and positive expected_version are required"}
	}
	if r.SurvivorNotespaceID == r.LoserNotespaceID {
		return &ProtocolError{Code: ErrorRegistrationConflict, Message: "survivor and loser must differ"}
	}
	if r.LoserDisposition != RegistrationIntentCreateSibling && r.LoserDisposition != RegistrationIntentReconcile {
		return &ProtocolError{Code: ErrorRegistrationConflict, Message: "loser_disposition must be create-sibling or reconcile"}
	}
	return nil
}

type RegistrationResolutionResponse struct {
	Subject              string         `json:"subject"`
	InheritedNotespaceID NotespaceID    `json:"inherited_notespace_id"`
	Version              int64          `json:"version"`
	Error                *ProtocolError `json:"error,omitempty"`
}

// SubjectRerecordRequest atomically moves a subject claim and retains OldSubject
// as an alias for NotespaceID.
type SubjectRerecordRequest struct {
	RequestIdentity
	NotespaceID     NotespaceID `json:"notespace_id"`
	OldSubject      string      `json:"old_subject"`
	NewSubject      string      `json:"new_subject"`
	ExpectedVersion int64       `json:"expected_version"`
}

func (r SubjectRerecordRequest) Validate() *ProtocolError {
	if err := r.RequestIdentity.Validate(); err != nil {
		return err
	}
	if r.NotespaceID == "" || r.ExpectedVersion <= 0 {
		return &ProtocolError{Code: ErrorStaleResolution, Message: "notespace_id and positive expected_version are required"}
	}
	if r.OldSubject == r.NewSubject {
		return &ProtocolError{Code: ErrorRegistrationConflict, Message: "old_subject and new_subject must differ"}
	}
	if err := subject.Validate(r.OldSubject); err != nil {
		return &ProtocolError{Code: ErrorRegistrationConflict, Message: err.Error()}
	}
	if err := subject.Validate(r.NewSubject); err != nil {
		return &ProtocolError{Code: ErrorRegistrationConflict, Message: err.Error()}
	}
	return nil
}

type SubjectRerecordResponse struct {
	NotespaceID NotespaceID    `json:"notespace_id"`
	Subject     string         `json:"subject"`
	Aliases     []string       `json:"aliases"`
	Version     int64          `json:"version"`
	Error       *ProtocolError `json:"error,omitempty"`
}

// ValidateDataRequest is the shared fleet-cutover guard. Device-session v2 is
// still accepted for authentication negotiation, but all data operations are
// v3 and id-keyed.
func ValidateDataRequest(protocolVersion int, notespaceID NotespaceID) *ProtocolError {
	if protocolVersion != ProtocolVersionNotespaceID {
		return &ProtocolError{Code: ErrorProtocolVersion, Message: "notespace-id data protocol v3 is required", SupportedVersions: []int{ProtocolVersionNotespaceID}}
	}
	if strings.TrimSpace(notespaceID.String()) == "" {
		return &ProtocolError{Code: ErrorUnregisteredNotespace, Message: "notespace id is required and must be registered"}
	}
	return nil
}

// SyncEvent is one entry in a notespace's append-only event log.
type SyncEvent struct {
	Seq              int64         `json:"seq,omitempty"`
	Type             string        `json:"type"`
	NotespaceID      NotespaceID   `json:"notespace"`
	NotespaceName    NotespaceName `json:"notespace_name,omitempty"`
	DocumentID       string        `json:"document_id,omitempty"`
	Path             string        `json:"path"`
	PrevPath         string        `json:"prev_path,omitempty"`
	ContentHash      string        `json:"content_hash,omitempty"`
	BaseVersion      int64         `json:"base_version,omitempty"`
	Version          int64         `json:"version,omitempty"`
	Content          []byte        `json:"content,omitempty"`
	ContentEncoding  string        `json:"content_encoding,omitempty"`
	Size             int64         `json:"size,omitempty"`
	OriginID         string        `json:"origin_id,omitempty"`
	Actor            string        `json:"actor,omitempty"`
	VerifiedDeviceID string        `json:"verified_device_id,omitempty"`
	ReceivedAt       time.Time     `json:"received_at,omitzero"`
	Mtime            time.Time     `json:"mtime,omitzero"`
}

type PushRequest struct {
	ProtocolVersion int           `json:"protocol_version"`
	NotespaceID     NotespaceID   `json:"notespace"`
	NotespaceName   NotespaceName `json:"notespace_name,omitempty"`
	OriginID        string        `json:"origin_id"`
	DeviceID        string        `json:"device_id,omitempty"`
	Events          []SyncEvent   `json:"events"`
}

const (
	PushStatusAccepted = "accepted"
	PushStatusConflict = "conflict"
	PushStatusRejected = "rejected"
)

type PushResult struct {
	Status     string `json:"status"`
	DocumentID string `json:"document_id,omitempty"`
	Version    int64  `json:"version,omitempty"`
	Seq        int64  `json:"seq,omitempty"`
	Error      string `json:"error,omitempty"`
}

type PushResponse struct {
	Results []PushResult   `json:"results"`
	Cursor  int64          `json:"cursor,omitempty"`
	Error   *ProtocolError `json:"error,omitempty"`
}

type PullRequest struct {
	ProtocolVersion int         `json:"protocol_version"`
	NotespaceID     NotespaceID `json:"notespace"`
	Cursor          int64       `json:"cursor"`
	Limit           int         `json:"limit,omitempty"`
	Wait            string      `json:"wait,omitempty"`
	ExcludeOrigin   string      `json:"exclude_origin,omitempty"`
}

type PullResponse struct {
	Events           []SyncEvent    `json:"events"`
	Cursor           int64          `json:"cursor"`
	More             bool           `json:"more,omitempty"`
	SnapshotRequired bool           `json:"snapshot_required,omitempty"`
	Error            *ProtocolError `json:"error,omitempty"`
}

type DocumentSnapshot struct {
	ID      string    `json:"id"`
	Path    string    `json:"path"`
	Version int64     `json:"version"`
	Hash    string    `json:"hash"`
	Size    int64     `json:"size"`
	Mtime   time.Time `json:"mtime,omitzero"`
}

type SnapshotRequest struct {
	ProtocolVersion int         `json:"protocol_version"`
	NotespaceID     NotespaceID `json:"notespace"`
}

type SnapshotManifest struct {
	NotespaceID   NotespaceID        `json:"notespace"`
	NotespaceName NotespaceName      `json:"notespace_name,omitempty"`
	Cursor        int64              `json:"cursor"`
	Documents     []DocumentSnapshot `json:"documents"`
	Error         *ProtocolError     `json:"error,omitempty"`
}

type HistoryRequest struct {
	ProtocolVersion int         `json:"protocol_version"`
	NotespaceID     NotespaceID `json:"notespace"`
	DocumentID      string      `json:"document_id,omitempty"`
	Limit           int         `json:"limit,omitempty"`
}

type CursorRequest struct {
	ProtocolVersion int         `json:"protocol_version"`
	NotespaceID     NotespaceID `json:"notespace"`
	Cursor          int64       `json:"cursor,omitempty"`
}
