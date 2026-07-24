package models

import "time"

// SubjobEventKind identifies a Pi Flow child report lifecycle transition.
type SubjobEventKind string

const (
	SubjobReportReady SubjobEventKind = "report_ready"
	SubjobJoined      SubjobEventKind = "joined"
)

// SubjobEvent is the daemon wire event. Report contents never ride this bus.
type SubjobEvent struct {
	SchemaVersion int             `json:"schema_version"`
	Kind          SubjobEventKind `json:"kind"`
	PlanKey       string          `json:"plan_key"`
	ParentJobID   string          `json:"parent_job_id"`
	ChildJobID    string          `json:"child_job_id"`
	ReportSHA256  string          `json:"report_sha256"`
	Timestamp     time.Time       `json:"timestamp"`
}

// SubjobState is the latest materialized state for one child report digest.
type SubjobState struct {
	PlanKey      string          `json:"plan_key"`
	ParentJobID  string          `json:"parent_job_id"`
	ChildJobID   string          `json:"child_job_id"`
	State        SubjobEventKind `json:"state"`
	ReportSHA256 string          `json:"report_sha256"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// SubjobSnapshot is filtered by exact plan and parent identity.
type SubjobSnapshot struct {
	Reports map[string]*SubjobState `json:"reports"`
}
