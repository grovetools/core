package daemon

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/grovetools/core/pkg/models"
)

func testArtifactBundle(path string, data []byte) *models.ArtifactBundle {
	sum := sha256.Sum256(data)
	return &models.ArtifactBundle{
		Manifest: models.ArtifactManifest{SchemaVersion: 1, Origin: "sat", JobID: "job", TotalBytes: int64(len(data)), Files: []models.ArtifactManifestEntry{{Path: path, Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:])}}},
		Files:    []models.ArtifactFile{{Path: path, Data: data}},
	}
}

func TestValidateArtifactBundleConfinement(t *testing.T) {
	if err := ValidateArtifactBundle(testArtifactBundle("nested/report.md", []byte("ok")), "sat", "job"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../other/secret", "a/../../secret", "/etc/passwd", "a\\..\\secret", "."} {
		t.Run(strings.ReplaceAll(path, "/", "_"), func(t *testing.T) {
			if err := ValidateArtifactBundle(testArtifactBundle(path, []byte("x")), "sat", "job"); err == nil {
				t.Fatalf("accepted hostile path %q", path)
			}
		})
	}
}

func TestValidateArtifactBundleIdentityHashAndSize(t *testing.T) {
	b := testArtifactBundle("report.md", []byte("ok"))
	if err := ValidateArtifactBundle(b, "other", "job"); err == nil {
		t.Fatal("accepted cross-origin bundle")
	}
	if err := ValidateArtifactBundle(b, "sat", "other-job"); err == nil {
		t.Fatal("accepted cross-job bundle")
	}
	b = testArtifactBundle("report.md", []byte("ok"))
	b.Files[0].Data[0] = 'X'
	if err := ValidateArtifactBundle(b, "sat", "job"); err == nil {
		t.Fatal("accepted hash mismatch")
	}
	b = testArtifactBundle("report.md", []byte("ok"))
	b.Manifest.TotalBytes++
	if err := ValidateArtifactBundle(b, "sat", "job"); err == nil {
		t.Fatal("accepted total-size mismatch")
	}
}
