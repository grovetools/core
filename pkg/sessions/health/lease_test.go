package health

import (
	"testing"
	"time"

	"github.com/grovetools/core/pkg/models"
)

func TestSessionLeasePolicy(t *testing.T) {
	now := time.Unix(10_000, 0)
	policy := LeasePolicy{Interactive: 2 * time.Hour, Headless: 30 * time.Minute, TurnBased: time.Hour}
	tests := []struct {
		name    string
		session *models.Session
		lease   time.Duration
		has     bool
		expired bool
	}{
		{"interactive", &models.Session{Type: "interactive_agent", Status: "running", LastActivity: now.Add(-2*time.Hour - time.Second)}, 2 * time.Hour, true, true},
		{"headless exact boundary", &models.Session{Type: "headless_agent", Status: "running", LastActivity: now.Add(-30 * time.Minute)}, 30 * time.Minute, true, false},
		{"turn based", &models.Session{Type: "chat", Status: "running", LastActivity: now.Add(-time.Hour - time.Second)}, time.Hour, true, true},
		{"turn based pending user exempt", &models.Session{Type: "chat", Status: "pending_user", LastActivity: now.Add(-24 * time.Hour)}, 0, false, false},
		{"interactive pending user expires", &models.Session{Type: "interactive_agent", Status: "pending_user", LastActivity: now.Add(-3 * time.Hour)}, 2 * time.Hour, true, true},
		{"terminal exempt", &models.Session{Type: "headless_agent", Status: "completed", LastActivity: now.Add(-24 * time.Hour)}, 0, false, false},
		{"remote exempt", &models.Session{Type: "headless_agent", Status: "running", Origin: "sat", LastActivity: now.Add(-24 * time.Hour)}, 0, false, false},
		{"future activity", &models.Session{Type: "headless_agent", Status: "running", LastActivity: now.Add(time.Minute)}, 30 * time.Minute, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lease, has := LeaseFor(tt.session, policy)
			if lease != tt.lease || has != tt.has {
				t.Fatalf("LeaseFor = (%v,%v), want (%v,%v)", lease, has, tt.lease, tt.has)
			}
			if got := LeaseExpired(tt.session, now, policy); got != tt.expired {
				t.Fatalf("LeaseExpired = %v, want %v", got, tt.expired)
			}
		})
	}
}
