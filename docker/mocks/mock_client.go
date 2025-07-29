package mocks

import (
	"context"
	"io"
	
	"github.com/mattsolo1/grove-core/docker"
)

// MockClient is a mock implementation of docker.Client for testing
type MockClient struct {
	IsContainerRunningFunc      func(ctx context.Context, containerName string) bool
	StopAndRemoveContainerFunc  func(ctx context.Context, containerName string, force bool) error
	EnsureNetworkExistsFunc     func(ctx context.Context, networkName string) error
	StartTraefikContainerFunc   func(ctx context.Context, args docker.TraefikContainerArgs) error
	GetNetworkInfoFunc          func(ctx context.Context, networkName string) (*docker.NetworkInfo, error)
	ListContainersOnNetworkFunc func(ctx context.Context, networkName string) ([]string, error)
	RemoveNetworkFunc           func(ctx context.Context, networkName string) error
	
	// Image operations
	BuildImageFunc     func(ctx context.Context, buildContextPath string, dockerfilePath string, imageName string) error
	ImageExistsFunc    func(ctx context.Context, imageName string) (bool, error)
	
	// Container execution
	ExecInContainerFunc func(ctx context.Context, containerName string, cmd []string, stdin io.Reader) (stdout string, stderr string, err error)
}

// IsContainerRunning calls the mock function
func (m *MockClient) IsContainerRunning(ctx context.Context, containerName string) bool {
	if m.IsContainerRunningFunc != nil {
		return m.IsContainerRunningFunc(ctx, containerName)
	}
	return false
}

// StopAndRemoveContainer calls the mock function
func (m *MockClient) StopAndRemoveContainer(ctx context.Context, containerName string, force bool) error {
	if m.StopAndRemoveContainerFunc != nil {
		return m.StopAndRemoveContainerFunc(ctx, containerName, force)
	}
	return nil
}

// EnsureNetworkExists calls the mock function
func (m *MockClient) EnsureNetworkExists(ctx context.Context, networkName string) error {
	if m.EnsureNetworkExistsFunc != nil {
		return m.EnsureNetworkExistsFunc(ctx, networkName)
	}
	return nil
}

// StartTraefikContainer calls the mock function
func (m *MockClient) StartTraefikContainer(ctx context.Context, args docker.TraefikContainerArgs) error {
	if m.StartTraefikContainerFunc != nil {
		return m.StartTraefikContainerFunc(ctx, args)
	}
	return nil
}

// GetNetworkInfo calls the mock function
func (m *MockClient) GetNetworkInfo(ctx context.Context, networkName string) (*docker.NetworkInfo, error) {
	if m.GetNetworkInfoFunc != nil {
		return m.GetNetworkInfoFunc(ctx, networkName)
	}
	return &docker.NetworkInfo{}, nil
}

// ListContainersOnNetwork calls the mock function
func (m *MockClient) ListContainersOnNetwork(ctx context.Context, networkName string) ([]string, error) {
	if m.ListContainersOnNetworkFunc != nil {
		return m.ListContainersOnNetworkFunc(ctx, networkName)
	}
	return []string{}, nil
}

// RemoveNetwork calls the mock function
func (m *MockClient) RemoveNetwork(ctx context.Context, networkName string) error {
	if m.RemoveNetworkFunc != nil {
		return m.RemoveNetworkFunc(ctx, networkName)
	}
	return nil
}

// BuildImage calls the mock function
func (m *MockClient) BuildImage(ctx context.Context, buildContextPath string, dockerfilePath string, imageName string) error {
	if m.BuildImageFunc != nil {
		return m.BuildImageFunc(ctx, buildContextPath, dockerfilePath, imageName)
	}
	return nil
}

// ImageExists calls the mock function
func (m *MockClient) ImageExists(ctx context.Context, imageName string) (bool, error) {
	if m.ImageExistsFunc != nil {
		return m.ImageExistsFunc(ctx, imageName)
	}
	return false, nil
}

// ExecInContainer calls the mock function
func (m *MockClient) ExecInContainer(ctx context.Context, containerName string, cmd []string, stdin io.Reader) (stdout string, stderr string, err error) {
	if m.ExecInContainerFunc != nil {
		return m.ExecInContainerFunc(ctx, containerName, cmd, stdin)
	}
	return "", "", nil
}

// Ensure MockClient implements the interface
var _ docker.Client = (*MockClient)(nil)