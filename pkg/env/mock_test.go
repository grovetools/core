package env

import (
	"context"
)

// MockDaemonEnvClient implements DaemonEnvClient for testing.
type MockDaemonEnvClient struct {
	UpResponse   *EnvResponse
	UpError      error
	DownResponse *EnvResponse
	DownError    error

	// Recorded calls
	UpCalled    bool
	DownCalled  bool
	LastUpReq   EnvRequest
	LastDownReq EnvRequest
}

func (m *MockDaemonEnvClient) EnvUp(ctx context.Context, req EnvRequest) (*EnvResponse, error) {
	m.UpCalled = true
	m.LastUpReq = req
	return m.UpResponse, m.UpError
}

func (m *MockDaemonEnvClient) EnvDown(ctx context.Context, req EnvRequest) (*EnvResponse, error) {
	m.DownCalled = true
	m.LastDownReq = req
	return m.DownResponse, m.DownError
}

func (m *MockDaemonEnvClient) EnvStatus(ctx context.Context, worktree string) (*EnvResponse, error) {
	return &EnvResponse{Status: "stopped"}, nil
}
