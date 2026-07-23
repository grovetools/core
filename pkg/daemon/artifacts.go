package daemon

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/grovetools/core/pkg/models"
)

// ValidateArtifactBundle enforces the v1 count/size/path/hash envelope. When
// expectedOrigin is non-empty it also verifies the laptop-stamped origin.
func ValidateArtifactBundle(bundle *models.ArtifactBundle, expectedOrigin, expectedJob string) error {
	if bundle == nil {
		return fmt.Errorf("artifact bundle is nil")
	}
	m := bundle.Manifest
	if m.SchemaVersion != models.ArtifactBundleSchemaVersion {
		return fmt.Errorf("unsupported artifact schema_version %d", m.SchemaVersion)
	}
	if m.JobID == "" || m.JobID != expectedJob {
		return fmt.Errorf("artifact job identity mismatch: got %q, want %q", m.JobID, expectedJob)
	}
	if expectedOrigin != "" && m.Origin != expectedOrigin {
		return fmt.Errorf("artifact origin identity mismatch: got %q, want %q", m.Origin, expectedOrigin)
	}
	if len(m.Files) > models.ArtifactBundleMaxFiles || len(bundle.Files) != len(m.Files) {
		return fmt.Errorf("artifact file count outside limit or manifest mismatch")
	}
	byPath := make(map[string][]byte, len(bundle.Files))
	for _, f := range bundle.Files {
		if _, exists := byPath[f.Path]; exists {
			return fmt.Errorf("duplicate artifact payload path %q", f.Path)
		}
		byPath[f.Path] = f.Data
	}
	var total int64
	seen := make(map[string]struct{}, len(m.Files))
	for _, entry := range m.Files {
		if err := validateArtifactRelativePath(entry.Path); err != nil {
			return err
		}
		if _, exists := seen[entry.Path]; exists {
			return fmt.Errorf("duplicate artifact manifest path %q", entry.Path)
		}
		seen[entry.Path] = struct{}{}
		data, ok := byPath[entry.Path]
		if !ok {
			return fmt.Errorf("artifact payload missing %q", entry.Path)
		}
		if entry.Size < 0 || int64(len(data)) != entry.Size {
			return fmt.Errorf("artifact size mismatch for %q", entry.Path)
		}
		total += entry.Size
		if total > models.ArtifactBundleMaxBytes {
			return fmt.Errorf("artifact bundle exceeds %d bytes", models.ArtifactBundleMaxBytes)
		}
		sum := sha256.Sum256(data)
		if !strings.EqualFold(entry.SHA256, hex.EncodeToString(sum[:])) {
			return fmt.Errorf("artifact sha256 mismatch for %q", entry.Path)
		}
	}
	if total != m.TotalBytes {
		return fmt.Errorf("artifact total size mismatch: got %d, want %d", m.TotalBytes, total)
	}
	return nil
}

func validateArtifactRelativePath(name string) error {
	if name == "" || filepath.IsAbs(name) || strings.Contains(name, "\\") {
		return fmt.Errorf("invalid artifact path %q", name)
	}
	clean := filepath.Clean(name)
	if clean != name || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("artifact path escapes job root: %q", name)
	}
	return nil
}

// GetJobArtifacts fetches one guest-local job bundle.
func (c *RemoteClient) GetJobArtifacts(ctx context.Context, jobID string) (*models.ArtifactBundle, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/jobs/"+url.PathEscape(jobID)+"/artifacts", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.envHttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch job artifacts: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("artifact endpoint returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	var bundle models.ArtifactBundle
	dec := json.NewDecoder(io.LimitReader(resp.Body, models.ArtifactBundleMaxBytes*2+(1<<20)))
	if err := dec.Decode(&bundle); err != nil {
		return nil, fmt.Errorf("decode artifact bundle: %w", err)
	}
	if err := ValidateArtifactBundle(&bundle, "", jobID); err != nil {
		return nil, err
	}
	return &bundle, nil
}

// FetchSatelliteArtifacts asks the laptop daemon to fetch from a satellite over
// its existing pinned ConnManager transport.
func (c *RemoteClient) FetchSatelliteArtifacts(ctx context.Context, origin, jobID string) (*models.ArtifactBundle, error) {
	body, err := json.Marshal(models.SatelliteArtifactFetchRequest{Origin: origin, JobID: jobID})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/satellite-artifacts/fetch", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.envHttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch satellite artifacts: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("satellite artifact endpoint returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	var bundle models.ArtifactBundle
	if err := json.NewDecoder(io.LimitReader(resp.Body, models.ArtifactBundleMaxBytes*2+(1<<20))).Decode(&bundle); err != nil {
		return nil, fmt.Errorf("decode satellite artifact bundle: %w", err)
	}
	if err := ValidateArtifactBundle(&bundle, origin, jobID); err != nil {
		return nil, err
	}
	return &bundle, nil
}
