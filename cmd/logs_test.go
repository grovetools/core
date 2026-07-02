package cmd

import "testing"

func TestResolveMinLevelRank(t *testing.T) {
	tests := []struct {
		name    string
		level   string
		want    int
		wantErr bool
	}{
		{name: "empty defaults to info", level: "", want: validLevels["info"]},
		{name: "explicit debug", level: "debug", want: 0},
		{name: "explicit info", level: "info", want: 1},
		{name: "warn", level: "warn", want: 2},
		{name: "warning alias", level: "warning", want: 2},
		{name: "error", level: "error", want: 3},
		{name: "case-insensitive", level: "DEBUG", want: 0},
		{name: "invalid", level: "verbose", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveMinLevelRank(tt.level)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for level %q", tt.level)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("resolveMinLevelRank(%q) = %d, want %d", tt.level, got, tt.want)
			}
		})
	}
}

func TestPassesEventsFilter(t *testing.T) {
	tests := []struct {
		name   string
		logMap map[string]interface{}
		want   bool
	}{
		{
			name:   "event-tagged info passes",
			logMap: map[string]interface{}{"level": "info", "event": "job.created"},
			want:   true,
		},
		{
			name:   "plain info filtered",
			logMap: map[string]interface{}{"level": "info", "msg": "chatter"},
			want:   false,
		},
		{
			name:   "plain debug filtered",
			logMap: map[string]interface{}{"level": "debug"},
			want:   false,
		},
		{
			name:   "warn always passes",
			logMap: map[string]interface{}{"level": "warning"},
			want:   true,
		},
		{
			name:   "error always passes",
			logMap: map[string]interface{}{"level": "error"},
			want:   true,
		},
		{
			name:   "empty event field does not pass",
			logMap: map[string]interface{}{"level": "info", "event": ""},
			want:   false,
		},
		{
			name:   "non-string event field does not pass",
			logMap: map[string]interface{}{"level": "info", "event": 42},
			want:   false,
		},
		{
			name:   "event-tagged debug passes the events predicate",
			logMap: map[string]interface{}{"level": "debug", "event": "job.finished"},
			want:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := passesEventsFilter(tt.logMap); got != tt.want {
				t.Errorf("passesEventsFilter(%v) = %v, want %v", tt.logMap, got, tt.want)
			}
		})
	}
}
