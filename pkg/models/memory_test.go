package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEcosystemAnalysisWireUsesRootCounters(t *testing.T) {
	analysis := EcosystemAnalysis{
		Name:            "grovetools",
		Path:            "/code",
		ConfiguredRoots: 3,
		IndexedRoots:    2,
	}

	encoded, err := json.Marshal(analysis)
	if err != nil {
		t.Fatalf("marshal EcosystemAnalysis: %v", err)
	}
	wire := string(encoded)
	for _, want := range []string{`"configured_roots":3`, `"indexed_roots":2`} {
		if !strings.Contains(wire, want) {
			t.Errorf("wire %s missing %s", wire, want)
		}
	}
	for _, legacy := range []string{"configured_groves", "indexed_groves"} {
		if strings.Contains(wire, legacy) {
			t.Errorf("wire %s contains legacy key %q", wire, legacy)
		}
	}
}
