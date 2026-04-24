package env

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestDaemonProvider_Up_Success(t *testing.T) {
	mock := &MockDaemonEnvClient{
		UpResponse: &EnvResponse{
			Status:    "running",
			EnvVars:   map[string]string{"DB_URL": "postgres://localhost"},
			Endpoints: []string{"http://localhost:8080"},
			State:     map[string]string{"container_id": "abc123"},
		},
	}
	provider := NewDaemonProvider(mock)

	resp, err := provider.Up(context.Background(), EnvRequest{Provider: "native"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !mock.UpCalled {
		t.Error("expected EnvUp to be called")
	}
	if resp.Status != "running" {
		t.Errorf("expected status 'running', got %q", resp.Status)
	}
	if resp.EnvVars["DB_URL"] != "postgres://localhost" {
		t.Errorf("expected DB_URL env var, got %v", resp.EnvVars)
	}
	if mock.LastUpReq.Action != "up" {
		t.Errorf("expected action 'up', got %q", mock.LastUpReq.Action)
	}
}

func TestDaemonProvider_Up_ClientError(t *testing.T) {
	mock := &MockDaemonEnvClient{
		UpError: errors.New("connection refused"),
	}
	provider := NewDaemonProvider(mock)

	_, err := provider.Up(context.Background(), EnvRequest{Provider: "docker"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("expected error to contain 'connection refused', got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "daemon provider failed") {
		t.Errorf("expected error to contain context wrapper, got %q", err.Error())
	}
}

func TestDaemonProvider_Up_ResponseError(t *testing.T) {
	mock := &MockDaemonEnvClient{
		UpResponse: &EnvResponse{
			Status: "failed",
			Error:  "port 5432 already in use",
		},
	}
	provider := NewDaemonProvider(mock)

	resp, err := provider.Up(context.Background(), EnvRequest{Provider: "native"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "port 5432 already in use") {
		t.Errorf("expected error to contain response error message, got %q", err.Error())
	}
	// Response should still be returned even on error
	if resp == nil {
		t.Error("expected response to be returned alongside error")
	}
}

func TestDaemonProvider_Down_Success(t *testing.T) {
	mock := &MockDaemonEnvClient{
		DownResponse: &EnvResponse{
			Status: "stopped",
		},
	}
	provider := NewDaemonProvider(mock)

	err := provider.Down(context.Background(), EnvRequest{Provider: "native"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !mock.DownCalled {
		t.Error("expected EnvDown to be called")
	}
	if mock.LastDownReq.Action != "down" {
		t.Errorf("expected action 'down', got %q", mock.LastDownReq.Action)
	}
}

func TestDaemonProvider_Down_ClientError(t *testing.T) {
	mock := &MockDaemonEnvClient{
		DownError: errors.New("daemon unreachable"),
	}
	provider := NewDaemonProvider(mock)

	err := provider.Down(context.Background(), EnvRequest{Provider: "docker"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "daemon unreachable") {
		t.Errorf("expected error to contain 'daemon unreachable', got %q", err.Error())
	}
}

func TestDaemonProvider_Down_ResponseError(t *testing.T) {
	mock := &MockDaemonEnvClient{
		DownResponse: &EnvResponse{
			Status: "failed",
			Error:  "container not found",
		},
	}
	provider := NewDaemonProvider(mock)

	err := provider.Down(context.Background(), EnvRequest{Provider: "native"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "container not found") {
		t.Errorf("expected error to contain 'container not found', got %q", err.Error())
	}
}

func TestDaemonProvider_Down_NilResponse(t *testing.T) {
	mock := &MockDaemonEnvClient{
		DownResponse: nil,
	}
	provider := NewDaemonProvider(mock)

	err := provider.Down(context.Background(), EnvRequest{Provider: "native"})
	if err != nil {
		t.Fatalf("expected no error for nil response, got %v", err)
	}
}
