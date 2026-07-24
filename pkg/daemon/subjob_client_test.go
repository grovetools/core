package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/grovetools/core/pkg/models"
)

func TestRemoteSubjobTransport(t *testing.T) {
	digest := strings.Repeat("b", 64)
	ev := models.SubjobEvent{SchemaVersion: 1, Kind: models.SubjobReportReady, PlanKey: strings.Repeat("a", 64), ParentJobID: "parent / one", ChildJobID: "child", ReportSHA256: digest}
	var published models.SubjobEvent
	socket := startUnixServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/subjobs/event":
			if r.Method != http.MethodPost {
				t.Errorf("method = %s", r.Method)
			}
			if err := json.NewDecoder(r.Body).Decode(&published); err != nil {
				t.Errorf("decode: %v", err)
			}
			w.WriteHeader(http.StatusAccepted)
		case "/api/subjobs":
			if r.URL.Query().Get("parent_job_id") != ev.ParentJobID || r.URL.Query().Get("plan_key") != ev.PlanKey {
				t.Errorf("query = %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(models.SubjobSnapshot{Reports: map[string]*models.SubjobState{"child": {ChildJobID: "child", State: models.SubjobReportReady}}})
		default:
			http.NotFound(w, r)
		}
	}))
	client, _ := NewRemoteClient(socket)
	if err := client.PublishSubjobEvent(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	if published.ReportSHA256 != digest {
		t.Fatalf("published = %+v", published)
	}
	snap, err := client.GetSubjobSnapshot(context.Background(), ev.PlanKey, ev.ParentJobID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Reports["child"] == nil {
		t.Fatal("missing child snapshot")
	}
}

func TestSubjobTypedAvailabilityErrors(t *testing.T) {
	local := NewLocalClient()
	if _, err := local.GetSubjobSnapshot(context.Background(), "x", "y"); !errors.Is(err, ErrSubjobDaemonUnavailable) {
		t.Fatalf("local error = %v", err)
	}
	socket := startUnixServer(t, http.NotFoundHandler())
	client, _ := NewRemoteClient(socket)
	if _, err := client.GetSubjobSnapshot(context.Background(), "x", "y"); !errors.Is(err, ErrSubjobDaemonUpgradeRequired) {
		t.Fatalf("404 error = %v", err)
	}
}
