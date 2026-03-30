package env

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestHelperProcess is a helper function invoked by exec tests to simulate
// grove-env-<name> binaries. It is not a real test and exits immediately
// unless GO_WANT_HELPER_PROCESS=1.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)

	// Read the request from stdin
	var req EnvRequest
	if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
		fmt.Fprintf(os.Stderr, "failed to decode request: %v", err)
		os.Exit(1)
	}

	mode := os.Getenv("HELPER_MODE")
	switch mode {
	case "success_up":
		resp := EnvResponse{
			Status:    "running",
			EnvVars:   map[string]string{"SERVICE_URL": "http://localhost:9090", "API_KEY": "test-key"},
			Endpoints: []string{"http://localhost:9090"},
			State:     map[string]string{"pid": "12345"},
		}
		json.NewEncoder(os.Stdout).Encode(resp)

	case "success_down":
		resp := EnvResponse{
			Status: "stopped",
		}
		json.NewEncoder(os.Stdout).Encode(resp)

	case "response_error":
		resp := EnvResponse{
			Status: "failed",
			Error:  "auth credentials expired",
		}
		json.NewEncoder(os.Stdout).Encode(resp)

	case "malformed_json":
		fmt.Fprint(os.Stdout, "{invalid json")

	case "binary_error":
		fmt.Fprint(os.Stderr, "binary crashed: segfault")
		os.Exit(2)

	case "verify_action":
		// Echo back the action so the test can verify it was set correctly
		resp := EnvResponse{
			Status: "ok",
			State:  map[string]string{"received_action": req.Action, "received_provider": req.Provider},
		}
		json.NewEncoder(os.Stdout).Encode(resp)

	default:
		fmt.Fprintf(os.Stderr, "unknown helper mode: %s", mode)
		os.Exit(1)
	}
}

// newTestExecProviderWithArgs creates an ExecProvider that re-invokes the test
// binary as a mock helper process via a wrapper script.
func newTestExecProviderWithArgs(t *testing.T, mode string) *ExecProvider {
	t.Helper()
	// We need the exec to actually re-invoke the test binary with the right flags.
	// The trick is: exec.CommandContext(ctx, binaryName) is called, so we need
	// binaryName to point to a script/binary that behaves correctly.
	// Since the Go test binary needs -test.run=TestHelperProcess, we'll
	// create a small shell script wrapper.

	tmpDir := t.TempDir()
	scriptPath := tmpDir + "/grove-env-test"

	script := fmt.Sprintf("#!/bin/sh\nexec %s -test.run=TestHelperProcess -- \"$@\"\n", os.Args[0])
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("failed to create helper script: %v", err)
	}

	// Verify the helper script is executable
	if _, err := exec.LookPath(scriptPath); err != nil {
		// LookPath may fail for absolute paths on some systems, that's ok
	}

	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
	t.Setenv("HELPER_MODE", mode)

	p := &ExecProvider{binaryName: scriptPath}
	return p
}

func TestExecProvider_Up_Success(t *testing.T) {
	p := newTestExecProviderWithArgs(t, "success_up")

	resp, err := p.Up(context.Background(), EnvRequest{Provider: "cloud"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.Status != "running" {
		t.Errorf("expected status 'running', got %q", resp.Status)
	}
	if resp.EnvVars["SERVICE_URL"] != "http://localhost:9090" {
		t.Errorf("expected SERVICE_URL, got %v", resp.EnvVars)
	}
	if resp.EnvVars["API_KEY"] != "test-key" {
		t.Errorf("expected API_KEY, got %v", resp.EnvVars)
	}
	if len(resp.Endpoints) != 1 || resp.Endpoints[0] != "http://localhost:9090" {
		t.Errorf("unexpected endpoints: %v", resp.Endpoints)
	}
	if resp.State["pid"] != "12345" {
		t.Errorf("expected state with pid, got %v", resp.State)
	}
}

func TestExecProvider_Up_ResponseError(t *testing.T) {
	p := newTestExecProviderWithArgs(t, "response_error")

	resp, err := p.Up(context.Background(), EnvRequest{Provider: "cloud"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "auth credentials expired") {
		t.Errorf("expected error about auth, got %q", err.Error())
	}
	// Response should still be available
	if resp == nil {
		t.Error("expected response even on error")
	}
}

func TestExecProvider_Up_MalformedJSON(t *testing.T) {
	p := newTestExecProviderWithArgs(t, "malformed_json")

	_, err := p.Up(context.Background(), EnvRequest{Provider: "cloud"})
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "failed to parse provider response") {
		t.Errorf("expected parse error, got %q", err.Error())
	}
}

func TestExecProvider_Up_BinaryError(t *testing.T) {
	p := newTestExecProviderWithArgs(t, "binary_error")

	_, err := p.Up(context.Background(), EnvRequest{Provider: "cloud"})
	if err == nil {
		t.Fatal("expected error for binary failure, got nil")
	}
	if !strings.Contains(err.Error(), "exec provider") {
		t.Errorf("expected exec provider error, got %q", err.Error())
	}
}

func TestExecProvider_Down_Success(t *testing.T) {
	p := newTestExecProviderWithArgs(t, "success_down")

	err := p.Down(context.Background(), EnvRequest{Provider: "cloud"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestExecProvider_Down_Error(t *testing.T) {
	p := newTestExecProviderWithArgs(t, "binary_error")

	err := p.Down(context.Background(), EnvRequest{Provider: "cloud"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestExecProvider_VerifyAction(t *testing.T) {
	// Verify that Up sets action to "up"
	p := newTestExecProviderWithArgs(t, "verify_action")
	resp, err := p.Up(context.Background(), EnvRequest{Provider: "cloud"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.State["received_action"] != "up" {
		t.Errorf("expected action 'up', got %q", resp.State["received_action"])
	}
}

func TestExecProvider_BinaryName(t *testing.T) {
	p := NewExecProvider("my-custom-provider", "")
	if p.binaryName != "grove-env-my-custom-provider" {
		t.Errorf("expected binary name 'grove-env-my-custom-provider', got %q", p.binaryName)
	}
}

func TestExecProvider_CustomCommand(t *testing.T) {
	p := NewExecProvider("my-provider", "./tools/my-provider")
	if p.binaryName != "./tools/my-provider" {
		t.Errorf("expected binary name './tools/my-provider', got %q", p.binaryName)
	}
}
