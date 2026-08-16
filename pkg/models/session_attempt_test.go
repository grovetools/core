package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestJobInfoJSONCarriesAttemptID(t *testing.T) {
	data, err := json.Marshal(JobInfo{ID: "job-1", AttemptID: "attempt-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"attempt_id":"attempt-1"`) {
		t.Fatalf("JobInfo JSON missing attempt_id: %s", data)
	}
}

func TestSessionJSONKeepsAttemptAndProvenanceSeparateFromOrigin(t *testing.T) {
	data, err := json.Marshal(Session{
		ID: "job-1", AttemptID: "attempt-1", Synthetic: true,
		Provenance: "flow_job_projection",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, field := range []string{`"attempt_id":"attempt-1"`, `"synthetic":true`, `"provenance":"flow_job_projection"`} {
		if !strings.Contains(got, field) {
			t.Errorf("JSON %s missing %s", got, field)
		}
	}
	if strings.Contains(got, `"origin"`) {
		t.Errorf("synthetic provenance overloaded origin: %s", got)
	}

	var roundTrip Session
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if roundTrip.AttemptID != "attempt-1" || !roundTrip.Synthetic || roundTrip.Provenance != "flow_job_projection" || roundTrip.Origin != "" {
		t.Fatalf("round trip = %+v", roundTrip)
	}
}
