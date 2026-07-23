package models

// Artifact return is deliberately bounded at every hop. These limits are part
// of the v1 wire contract and must be enforced by the guest publisher, laptop
// forwarder, and final writer.
const (
	ArtifactBundleSchemaVersion       = 1
	ArtifactBundleMaxFiles            = 128
	ArtifactBundleMaxBytes      int64 = 32 << 20
)

// ArtifactManifestEntry declares one regular file relative to the owning
// job's .artifacts/<job-id> directory.
type ArtifactManifestEntry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// ArtifactManifest binds returned bytes to one origin-qualified job identity.
// Origin is empty on a guest-local response and is forcibly stamped by the
// laptop daemon after transport over the pinned satellite connection.
type ArtifactManifest struct {
	SchemaVersion int                     `json:"schema_version"`
	Origin        string                  `json:"origin"`
	JobID         string                  `json:"job_id"`
	Files         []ArtifactManifestEntry `json:"files"`
	TotalBytes    int64                   `json:"total_bytes"`
}

// ArtifactFile carries the bytes named by the corresponding manifest entry.
// []byte is base64 encoded by encoding/json.
type ArtifactFile struct {
	Path string `json:"path"`
	Data []byte `json:"data"`
}

// ArtifactBundle is the bounded, hash-addressed artifact return envelope.
type ArtifactBundle struct {
	Manifest ArtifactManifest `json:"manifest"`
	Files    []ArtifactFile   `json:"files"`
}

// SatelliteArtifactFetchRequest is accepted only by the laptop daemon. The
// target name selects its pinned ConnManager transport; it is never trusted
// from the guest response.
type SatelliteArtifactFetchRequest struct {
	Origin string `json:"origin"`
	JobID  string `json:"job_id"`
}
