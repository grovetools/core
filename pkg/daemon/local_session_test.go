package daemon

import (
	"context"
	"os"
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

func TestLocalClientRegisterSessionIntentPersistsScope(t *testing.T) {
	t.Setenv("GROVE_HOME", t.TempDir())
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Setenv("GROVE_SCOPE", cwd)
	wantScope := ResolveClientScope()
	if wantScope == "" {
		t.Fatal("test directory did not resolve to a daemon scope")
	}

	client := NewLocalClient()
	if err := client.RegisterSessionIntent(context.Background(), SessionIntent{JobID: "job-scoped", Provider: "pi"}); err != nil {
		t.Fatalf("RegisterSessionIntent: %v", err)
	}
	registry, err := sessions.NewFileSystemRegistry()
	if err != nil {
		t.Fatalf("NewFileSystemRegistry: %v", err)
	}
	got, err := registry.Find("job-scoped")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got.Scope != wantScope {
		t.Errorf("Scope = %q, want %q", got.Scope, wantScope)
	}
}
