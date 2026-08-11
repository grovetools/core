package syncproto

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCapabilitiesHandshakeRoundTrip(t *testing.T) {
	req := CapabilitiesRequest{
		ClientName:       "groved",
		ClientVersion:    "0.1.0",
		ProtocolVersions: []int{ProtocolVersion},
		OriginID:         "origin-abc",
		DeviceID:         "laptop",
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded CapabilitiesRequest
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, req, decoded)

	resp := CapabilitiesResponse{
		ServerName:      "grove-syncd",
		ProtocolVersion: ProtocolVersion,
		Capabilities: Capabilities{
			ProtocolVersions: []int{1},
			Blobs:            true,
			Notify:           true,
			MaxInlineSize:    256 * 1024,
			BlobChunkSize:    4 * 1024 * 1024,
			Compression:      []string{"zstd"},
			ContentEncodings: []string{ContentEncodingPlaintext},
		},
	}

	data, err = json.Marshal(resp)
	require.NoError(t, err)

	var decodedResp CapabilitiesResponse
	require.NoError(t, json.Unmarshal(data, &decodedResp))
	assert.Equal(t, resp, decodedResp)
}

func TestCanonicalCapabilitiesBindsEpochNonceAndOrderedOffer(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	req := CapabilitiesRequest{
		DeviceID: "01ARZ3NDEKTSV4RRFFQ69G5FAV", ServerEpoch: "epoch-1",
		Timestamp: "2026-08-08T12:34:56.123456789Z", Nonce: base64.StdEncoding.EncodeToString(make([]byte, 32)),
		ProtocolVersions: []int{ProtocolVersionDeviceSession, ProtocolVersionLegacy},
	}
	payload, err := CanonicalCapabilities(req)
	require.NoError(t, err)
	assert.Contains(t, string(payload), "protocol_versions=2,1\n")
	require.NoError(t, SetCapabilitiesSignature(&req, ed25519.Sign(private, payload)))
	require.NoError(t, VerifyCapabilities(req, public))
	for name, mutate := range map[string]func(*CapabilitiesRequest){
		"epoch":       func(r *CapabilitiesRequest) { r.ServerEpoch = "epoch-2" },
		"nonce":       func(r *CapabilitiesRequest) { r.Nonce = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32)) },
		"offer order": func(r *CapabilitiesRequest) { r.ProtocolVersions = []int{1, 2} },
	} {
		t.Run(name, func(t *testing.T) {
			changed := req
			mutate(&changed)
			assert.Error(t, VerifyCapabilities(changed, public))
		})
	}
}

func TestEnrollmentCanonicalSigningAndFingerprint(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	for i := range seed {
		seed[i] = byte(i)
	}
	private := ed25519.NewKeyFromSeed(seed)
	req := EnrollRequest{
		DeviceID:      "01K1ABCDEFGHJKMNPQRSTVWXYZ",
		Name:          "solair-air",
		PublicKey:     base64.StdEncoding.EncodeToString(private.Public().(ed25519.PublicKey)),
		RequestedUser: "owner",
		Timestamp:     "2026-08-08T12:34:56.123456789Z",
	}
	payload, err := EnrollmentSigningBytes(req)
	require.NoError(t, err)
	assert.Equal(t, "grove.sync.enrollment.v1\n"+
		"device_id=01K1ABCDEFGHJKMNPQRSTVWXYZ\n"+
		"name=solair-air\n"+
		"public_key="+req.PublicKey+"\n"+
		"requested_user=owner\n"+
		"timestamp=2026-08-08T12:34:56.123456789Z\n", string(payload))

	require.NoError(t, SignEnrollment(&req, private))
	require.NoError(t, VerifyEnrollment(req))

	tampered := req
	tampered.Name = "other"
	assert.Error(t, VerifyEnrollment(tampered))
}

func TestEnrollmentRejectsNonCanonicalInputs(t *testing.T) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	base := EnrollRequest{
		DeviceID:  "device",
		Name:      "name",
		PublicKey: base64.StdEncoding.EncodeToString(private.Public().(ed25519.PublicKey)),
		Timestamp: "2026-08-08T12:34:56Z",
	}
	for name, mutate := range map[string]func(*EnrollRequest){
		"separator":                func(r *EnrollRequest) { r.Name = "bad\nname" },
		"offset timestamp":         func(r *EnrollRequest) { r.Timestamp = "2026-08-08T12:34:56+00:00" },
		"trailing fractional zero": func(r *EnrollRequest) { r.Timestamp = "2026-08-08T12:34:56.100Z" },
		"noncanonical public key": func(r *EnrollRequest) {
			r.PublicKey = base64.RawStdEncoding.EncodeToString(private.Public().(ed25519.PublicKey))
		},
	} {
		t.Run(name, func(t *testing.T) {
			req := base
			mutate(&req)
			_, err := CanonicalEnrollment(req)
			assert.Error(t, err)
		})
	}
}

func TestDeviceFingerprintVector(t *testing.T) {
	// RFC 8032 test vector 1 public key, then SHA-256 over its raw 32 bytes.
	const publicKey = "11qYAYKxCrfVS/7TyWQHOg7hcvPapiMlrwIaaPcHURo="
	fingerprint, err := DeviceFingerprint(publicKey)
	require.NoError(t, err)
	assert.Equal(t, "21fe31dfa154a261626bf854046fd2271b7bed4b6abe45aa58877ef47f9721b9", fingerprint)
}

func TestEnrollmentWireCompatibility(t *testing.T) {
	req := EnrollRequest{DeviceID: "device", PublicKey: "key", Timestamp: "time", Signature: "sig"}
	data, err := json.Marshal(req)
	require.NoError(t, err)
	assert.JSONEq(t, `{"device_id":"device","public_key":"key","timestamp":"time","signature":"sig"}`, string(data))

	caps, err := json.Marshal(Capabilities{})
	require.NoError(t, err)
	assert.NotContains(t, string(caps), "device_enrollment", "old wire shape must remain unchanged when unsupported")
	caps, err = json.Marshal(Capabilities{DeviceEnrollment: true})
	require.NoError(t, err)
	assert.Contains(t, string(caps), `"device_enrollment":true`)
}

func TestCapabilitiesSupportsVersion(t *testing.T) {
	c := &Capabilities{ProtocolVersions: []int{1, 2, 3}}
	assert.True(t, c.SupportsVersion(1))
	assert.True(t, c.SupportsVersion(2))
	assert.True(t, c.SupportsVersion(3))
	assert.False(t, c.SupportsVersion(4))

	empty := &Capabilities{}
	assert.False(t, empty.SupportsVersion(1))
}

func TestPushRequestRoundTrip(t *testing.T) {
	req := PushRequest{
		ProtocolVersion: ProtocolVersionNotespaceID,
		NotespaceID:     "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		NotespaceName:   "grovetools",
		OriginID:        "origin-abc",
		DeviceID:        "laptop",
		Events: []SyncEvent{
			{
				Type:            EventDocumentCreated,
				NotespaceID:     "01ARZ3NDEKTSV4RRFFQ69G5FAV",
				Path:            "plans/sync/notes.md",
				ContentHash:     "deadbeef",
				Content:         []byte("# hello"),
				ContentEncoding: ContentEncodingPlaintext,
				Size:            7,
				OriginID:        "origin-abc",
			},
			{
				Type:        EventDocumentMoved,
				NotespaceID: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
				DocumentID:  "doc-uuid-1",
				Path:        "completed/notes.md",
				PrevPath:    "in_progress/notes.md",
				ContentHash: "deadbeef",
				BaseVersion: 3,
			},
		},
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded PushRequest
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, req, decoded)
}

func TestPushResponseRoundTrip(t *testing.T) {
	resp := PushResponse{
		Results: []PushResult{
			{Status: PushStatusAccepted, DocumentID: "doc-uuid-1", Version: 4, Seq: 100},
			{Status: PushStatusConflict, DocumentID: "doc-uuid-2"},
			{Status: PushStatusRejected, Error: "path escapes notespace"},
		},
		Cursor: 100,
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded PushResponse
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, resp, decoded)
}

func TestPullRoundTrip(t *testing.T) {
	req := PullRequest{
		ProtocolVersion: ProtocolVersionNotespaceID,
		NotespaceID:     "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Cursor:          42,
		Limit:           500,
		Wait:            "30s",
		ExcludeOrigin:   "origin-abc",
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)

	var decodedReq PullRequest
	require.NoError(t, json.Unmarshal(data, &decodedReq))
	assert.Equal(t, req, decodedReq)

	resp := PullResponse{
		Events: []SyncEvent{
			{
				Seq:         43,
				Type:        EventDocumentUpdated,
				NotespaceID: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
				DocumentID:  "doc-uuid-1",
				Path:        "plans/sync/notes.md",
				ContentHash: "cafef00d",
				Version:     5,
				OriginID:    "origin-other",
				ReceivedAt:  time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC),
			},
		},
		Cursor: 43,
		More:   true,
	}

	data, err = json.Marshal(resp)
	require.NoError(t, err)

	var decodedResp PullResponse
	require.NoError(t, json.Unmarshal(data, &decodedResp))
	assert.Equal(t, resp, decodedResp)
}

func TestPullResponseSnapshotRequired(t *testing.T) {
	resp := PullResponse{SnapshotRequired: true}
	data, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(data), "snapshot_required")
}

func TestSnapshotManifestRoundTrip(t *testing.T) {
	m := SnapshotManifest{
		NotespaceID:   "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		NotespaceName: "grovetools",
		Cursor:        99,
		Documents: []DocumentSnapshot{
			{ID: "doc-uuid-1", Path: "plans/sync/notes.md", Version: 5, Hash: "cafef00d", Size: 1234},
		},
	}

	data, err := json.Marshal(m)
	require.NoError(t, err)

	var decoded SnapshotManifest
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, m, decoded)
}

func TestWireFieldNames(t *testing.T) {
	// The JSON wire names are the protocol contract: third-party servers
	// implement against them. Lock them down.
	data, err := json.Marshal(SyncEvent{
		Seq:         1,
		Type:        EventDocumentMoved,
		NotespaceID: "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		DocumentID:  "id",
		Path:        "a/b.md",
		PrevPath:    "a/a.md",
		ContentHash: "h",
		BaseVersion: 1,
		Version:     2,
		Size:        3,
		OriginID:    "o",
		Actor:       "u",
	})
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))
	for _, key := range []string{
		"seq", "type", "notespace", "document_id", "path", "prev_path",
		"content_hash", "base_version", "version", "size", "origin_id", "actor",
	} {
		assert.Contains(t, raw, key)
	}
	assert.NotContains(t, raw, "content", "empty content must be omitted")
	assert.NotContains(t, raw, "received_at", "zero timestamps must be omitted")
}

func TestPathNormalization(t *testing.T) {
	assert.Equal(t, "plans/x/y.md", NormalizePath("plans/x/y.md"))
	// On non-Windows platforms ToSlash/FromSlash are identity; this asserts
	// the helpers exist and round-trip.
	assert.Equal(t, NormalizePath(LocalizePath("plans/x/y.md")), "plans/x/y.md")
}

// TestSyncEventMtimeWireFormat: the fidelity mtime round-trips through JSON,
// and a zero mtime is omitted entirely (omitzero) so pre-mtime peers see an
// unchanged wire shape — the backward-compatibility contract.
func TestSyncEventMtimeWireFormat(t *testing.T) {
	mtime := time.Date(2026, 7, 11, 9, 30, 0, 0, time.UTC)
	ev := SyncEvent{Type: EventDocumentCreated, NotespaceID: "01ARZ3NDEKTSV4RRFFQ69G5FAV", Path: "a.md", Mtime: mtime}

	data, err := json.Marshal(ev)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"mtime"`)

	var decoded SyncEvent
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.True(t, decoded.Mtime.Equal(mtime))

	// Zero mtime: omitted on events and snapshots alike.
	data, err = json.Marshal(SyncEvent{Type: EventDocumentDeleted, NotespaceID: "01ARZ3NDEKTSV4RRFFQ69G5FAV", Path: "a.md"})
	require.NoError(t, err)
	assert.NotContains(t, string(data), `"mtime"`)

	snap := DocumentSnapshot{ID: "id", Path: "a.md", Version: 1, Hash: "h", Size: 1, Mtime: mtime}
	data, err = json.Marshal(snap)
	require.NoError(t, err)
	var decodedSnap DocumentSnapshot
	require.NoError(t, json.Unmarshal(data, &decodedSnap))
	assert.True(t, decodedSnap.Mtime.Equal(mtime))

	data, err = json.Marshal(DocumentSnapshot{ID: "id", Path: "a.md"})
	require.NoError(t, err)
	assert.NotContains(t, string(data), `"mtime"`)
}
