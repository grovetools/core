package daemon

import (
	"context"
	"strings"
	"testing"

	"github.com/grovetools/core/pkg/env"
)

func TestLocalClient_EnvUp(t *testing.T) {
	client := NewLocalClient()
	resp, err := client.EnvUp(context.Background(), env.EnvRequest{
		Provider: "native",
		PlanDir:  "/tmp/test-plan",
	})
	if err == nil {
		t.Fatal("expected error from LocalClient.EnvUp, got nil")
	}
	if resp != nil {
		t.Errorf("expected nil response, got %v", resp)
	}
	if !strings.Contains(err.Error(), "start groved first") {
		t.Errorf("expected error about starting groved, got %q", err.Error())
	}
}

func TestLocalClient_EnvDown(t *testing.T) {
	client := NewLocalClient()
	resp, err := client.EnvDown(context.Background(), env.EnvRequest{
		Provider: "docker",
		PlanDir:  "/tmp/test-plan",
	})
	if err == nil {
		t.Fatal("expected error from LocalClient.EnvDown, got nil")
	}
	if resp != nil {
		t.Errorf("expected nil response, got %v", resp)
	}
	if !strings.Contains(err.Error(), "start groved first") {
		t.Errorf("expected error about starting groved, got %q", err.Error())
	}
}
