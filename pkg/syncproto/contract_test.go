package syncproto

import (
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

func TestCapabilitiesSupportsVersion(t *testing.T) {
	c := &Capabilities{ProtocolVersions: []int{1, 2}}
	assert.True(t, c.SupportsVersion(1))
	assert.True(t, c.SupportsVersion(2))
	assert.False(t, c.SupportsVersion(3))

	empty := &Capabilities{}
	assert.False(t, empty.SupportsVersion(1))
}

func TestPushRequestRoundTrip(t *testing.T) {
	req := PushRequest{
		Workspace: "grovetools",
		OriginID:  "origin-abc",
		DeviceID:  "laptop",
		Events: []SyncEvent{
			{
				Type:            EventDocumentCreated,
				Workspace:       "grovetools",
				Path:            "plans/sync/notes.md",
				ContentHash:     "deadbeef",
				Content:         []byte("# hello"),
				ContentEncoding: ContentEncodingPlaintext,
				Size:            7,
				OriginID:        "origin-abc",
			},
			{
				Type:        EventDocumentMoved,
				Workspace:   "grovetools",
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
			{Status: PushStatusRejected, Error: "path escapes workspace"},
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
		Workspace:     "grovetools",
		Cursor:        42,
		Limit:         500,
		Wait:          "30s",
		ExcludeOrigin: "origin-abc",
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
				Workspace:   "grovetools",
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
		Workspace: "grovetools",
		Cursor:    99,
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
		Workspace:   "ws",
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
		"seq", "type", "workspace", "document_id", "path", "prev_path",
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
