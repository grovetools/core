package logutil

import "testing"

func TestPassesEventsFilter(t *testing.T) {
	tests := []struct {
		name  string
		event interface{}
		level string
		want  bool
	}{
		{name: "structured event", event: "job.created", level: "debug", want: true},
		{name: "empty event", event: "", level: "info", want: false},
		{name: "non-string event", event: 42, level: "info", want: false},
		{name: "warning", level: "warning", want: true},
		{name: "error", level: "error", want: true},
		{name: "fatal", level: "fatal", want: true},
		{name: "plain info", level: "info", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PassesEventsFilter(tt.event, tt.level); got != tt.want {
				t.Fatalf("PassesEventsFilter(%v, %q) = %v, want %v", tt.event, tt.level, got, tt.want)
			}
		})
	}
}
