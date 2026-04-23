package services

import (
	"os"
	"testing"
)

func TestExpandEnvVars(t *testing.T) {
	envMap := map[string]string{"DB_HOST": "db.local", "PORT": "8080"}

	cases := []struct{ in, want string }{
		{"$DB_HOST:$PORT", "db.local:8080"},
		{"${DB_HOST}/api", "db.local/api"},
		{"no-vars-here", "no-vars-here"},
		{"$MISSING_VAR", ""},
	}
	for _, c := range cases {
		if got := ExpandEnvVars(c.in, envMap); got != c.want {
			t.Errorf("ExpandEnvVars(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestExpandEnvVars_FallsBackToProcessEnv(t *testing.T) {
	t.Setenv("SERVICES_TEST_FALLBACK", "from-proc")
	got := ExpandEnvVars("$SERVICES_TEST_FALLBACK", nil)
	if got != "from-proc" {
		t.Errorf("got %q, want from-proc", got)
	}
}

func TestExpandEnvVars_MapWinsOverProcessEnv(t *testing.T) {
	t.Setenv("SERVICES_TEST_OVERRIDE", "from-proc")
	got := ExpandEnvVars("$SERVICES_TEST_OVERRIDE", map[string]string{"SERVICES_TEST_OVERRIDE": "from-map"})
	if got != "from-map" {
		t.Errorf("got %q, want from-map", got)
	}
	// Ensure we can still read the env var directly (sanity check).
	_ = os.Getenv("SERVICES_TEST_OVERRIDE")
}
