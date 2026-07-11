package plan

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	// LeaseFileName is the advisory dispatch-lease dotfile written into a plan
	// directory. It rides the Record plane (the landed DocSpace syncs
	// dot-prefixed files, per M2 contract C14) to the hub/satellite.
	LeaseFileName = ".grove-lease.yml"

	// DefaultLeaseTTL is how long a dispatch lease is treated as live absent an
	// explicit release. It must comfortably outlast a laptop-daemon restart
	// (which loses the in-memory jobID→lease map, C14) so the lease still
	// eventually expires and the plan dir frees up.
	DefaultLeaseTTL = 6 * time.Hour
)

// Lease is an advisory marker (M2 contract C14) written by the laptop into a
// plan directory when it dispatches that plan to a satellite. It is
// informational only: the sole M2 enforcement is `flow plan run` refusing a
// leased plan dir without --force. There is no hub involvement and no
// server-side enforcement. "Expired" is treated identically to "absent".
type Lease struct {
	// HolderOrigin is the satellite registry name the plan was dispatched to.
	HolderOrigin string `yaml:"holder_origin"`
	// JobID is the satellite-returned job identifier, used to release the lease
	// when its federated terminal event is observed.
	JobID string `yaml:"job_id"`
	// AcquiredAt is when the lease was written (laptop clock).
	AcquiredAt time.Time `yaml:"acquired_at"`
	// TTL bounds the lease's life so a laptop that never observes the terminal
	// event (e.g. it was offline) still releases by expiry.
	TTL time.Duration `yaml:"ttl"`
}

// leaseOnDisk is the serialized form. TTL is stored as a human-readable Go
// duration string ("6h0m0s") rather than raw nanoseconds.
type leaseOnDisk struct {
	HolderOrigin string    `yaml:"holder_origin"`
	JobID        string    `yaml:"job_id"`
	AcquiredAt   time.Time `yaml:"acquired_at"`
	TTL          string    `yaml:"ttl"`
}

// LeasePath returns the absolute lease-file path inside a plan directory.
func LeasePath(planDir string) string {
	return filepath.Join(planDir, LeaseFileName)
}

// Expired reports whether the lease is past its TTL as of now. A lease with a
// zero AcquiredAt or zero TTL is considered expired (it carries no live claim).
func (l *Lease) Expired() bool {
	if l == nil || l.AcquiredAt.IsZero() || l.TTL <= 0 {
		return true
	}
	return time.Now().After(l.AcquiredAt.Add(l.TTL))
}

// WriteLease writes the lease into planDir/.grove-lease.yml, overwriting any
// existing lease. The 0o600 mode matches how flow's loader persists plan files.
func WriteLease(planDir string, l Lease) error {
	out := leaseOnDisk{
		HolderOrigin: l.HolderOrigin,
		JobID:        l.JobID,
		AcquiredAt:   l.AcquiredAt,
		TTL:          l.TTL.String(),
	}
	data, err := yaml.Marshal(out)
	if err != nil {
		return fmt.Errorf("marshaling lease: %w", err)
	}
	if err := os.WriteFile(LeasePath(planDir), data, 0o600); err != nil {
		return fmt.Errorf("writing lease: %w", err)
	}
	return nil
}

// ReadLease reads planDir/.grove-lease.yml. It returns (nil, nil) when no lease
// file is present — callers treat absent and expired identically.
func ReadLease(planDir string) (*Lease, error) {
	data, err := os.ReadFile(LeasePath(planDir))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading lease: %w", err)
	}
	var on leaseOnDisk
	if err := yaml.Unmarshal(data, &on); err != nil {
		return nil, fmt.Errorf("parsing lease: %w", err)
	}
	l := &Lease{
		HolderOrigin: on.HolderOrigin,
		JobID:        on.JobID,
		AcquiredAt:   on.AcquiredAt,
	}
	if on.TTL != "" {
		ttl, perr := time.ParseDuration(on.TTL)
		if perr != nil {
			return nil, fmt.Errorf("parsing lease ttl %q: %w", on.TTL, perr)
		}
		l.TTL = ttl
	}
	return l, nil
}

// RemoveLease deletes planDir/.grove-lease.yml. A missing file is not an error.
func RemoveLease(planDir string) error {
	if err := os.Remove(LeasePath(planDir)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing lease: %w", err)
	}
	return nil
}
