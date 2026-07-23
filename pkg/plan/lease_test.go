package plan

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLeaseRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := Lease{
		HolderOrigin: "grove-satellite",
		JobID:        "job-abc123",
		AcquiredAt:   time.Now().Truncate(time.Second),
		TTL:          DefaultLeaseTTL,
	}
	if err := WriteLease(dir, want); err != nil {
		t.Fatalf("WriteLease: %v", err)
	}
	got, err := ReadLease(dir)
	if err != nil {
		t.Fatalf("ReadLease: %v", err)
	}
	if got == nil {
		t.Fatal("ReadLease returned nil for a written lease")
	}
	if got.HolderOrigin != want.HolderOrigin || got.JobID != want.JobID || got.TTL != want.TTL {
		t.Errorf("round-trip mismatch: got %+v want %+v", got, want)
	}
	if !got.AcquiredAt.Equal(want.AcquiredAt) {
		t.Errorf("AcquiredAt mismatch: got %v want %v", got.AcquiredAt, want.AcquiredAt)
	}
}

func TestLeaseRejectsIncompleteOrUnknownClaims(t *testing.T) {
	dir := t.TempDir()
	if err := WriteLease(dir, Lease{}); err == nil {
		t.Fatal("WriteLease accepted an incomplete claim")
	}
	for name, body := range map[string]string{
		"missing holder": "job_id: job-1\nacquired_at: 2026-01-01T00:00:00Z\nttl: 1h\n",
		"unknown field":  "holder_origin: sat\njob_id: job-1\nacquired_at: 2026-01-01T00:00:00Z\nttl: 1h\ntoken: secret\n",
		"zero ttl":       "holder_origin: sat\njob_id: job-1\nacquired_at: 2026-01-01T00:00:00Z\nttl: 0s\n",
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(LeasePath(dir), []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if got, err := ReadLease(dir); err == nil || got != nil {
				t.Fatalf("ReadLease = (%+v, %v), want fail-closed error", got, err)
			}
		})
	}
}

func TestLeaseAbsentIsNilNotError(t *testing.T) {
	got, err := ReadLease(t.TempDir())
	if err != nil {
		t.Fatalf("ReadLease on empty dir: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil lease for absent file, got %+v", got)
	}
}

func TestLeaseExpiry(t *testing.T) {
	live := &Lease{AcquiredAt: time.Now().Add(-time.Minute), TTL: time.Hour}
	if live.Expired() {
		t.Error("fresh lease reported expired")
	}
	stale := &Lease{AcquiredAt: time.Now().Add(-2 * time.Hour), TTL: time.Hour}
	if !stale.Expired() {
		t.Error("old lease not reported expired")
	}
	// Zero-value lease carries no live claim.
	if !(&Lease{}).Expired() {
		t.Error("zero lease should be treated as expired/absent")
	}
}

func TestRemoveLease(t *testing.T) {
	dir := t.TempDir()
	if err := WriteLease(dir, Lease{HolderOrigin: "x", JobID: "y", AcquiredAt: time.Now(), TTL: time.Hour}); err != nil {
		t.Fatalf("WriteLease: %v", err)
	}
	if err := RemoveLease(dir); err != nil {
		t.Fatalf("RemoveLease: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, LeaseFileName)); !os.IsNotExist(err) {
		t.Errorf("lease file still present after RemoveLease: %v", err)
	}
	// Removing an absent lease is not an error.
	if err := RemoveLease(dir); err != nil {
		t.Errorf("RemoveLease on absent file: %v", err)
	}
}
