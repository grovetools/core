package daemon

import (
	"context"
	"testing"

	"github.com/grovetools/core/pkg/sessions"
)

func TestLocalClientRegisterSessionIntentPersistsParentJobID(t *testing.T) {
	t.Setenv("GROVE_HOME", t.TempDir())

	client := NewLocalClient()
	if err := client.RegisterSessionIntent(context.Background(), SessionIntent{
		JobID:       "child-job",
		ParentJobID: "parent-job",
		Provider:    "pi",
	}); err != nil {
		t.Fatalf("RegisterSessionIntent: %v", err)
	}

	registry, err := sessions.NewFileSystemRegistry()
	if err != nil {
		t.Fatalf("NewFileSystemRegistry: %v", err)
	}
	got, err := registry.Find("child-job")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got.ParentJobID != "parent-job" {
		t.Errorf("ParentJobID = %q, want parent-job", got.ParentJobID)
	}
}
