package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

// Client abstracts Docker operations for testing
type Client interface {
	IsContainerRunning(ctx context.Context, containerName string) bool
	StopAndRemoveContainer(ctx context.Context, containerName string, force bool) error
	EnsureNetworkExists(ctx context.Context, networkName string) error
	StartTraefikContainer(ctx context.Context, args TraefikContainerArgs) error
	GetNetworkInfo(ctx context.Context, networkName string) (*NetworkInfo, error)
	ListContainersOnNetwork(ctx context.Context, networkName string) ([]string, error)
	RemoveNetwork(ctx context.Context, networkName string) error
	
	// Image operations
	BuildImage(ctx context.Context, buildContextPath string, dockerfilePath string, imageName string) error
	ImageExists(ctx context.Context, imageName string) (bool, error)
	
	// Container execution
	ExecInContainer(ctx context.Context, containerName string, cmd []string, stdin io.Reader) (stdout string, stderr string, err error)
}

// TraefikContainerArgs holds the arguments for starting a Traefik container
type TraefikContainerArgs struct {
	Name          string
	Image         string
	HTTPPort      string
	HTTPSPort     string
	DashboardPort string
	NetworkName   string
	ConfigDir     string
}

// NetworkInfo holds information about a Docker network
type NetworkInfo struct {
	Name   string
	ID     string
	Driver string
}

// SDKClient implements Client using the Docker SDK
type SDKClient struct {
	cli *client.Client
}

// NewSDKClient creates a new Docker SDK client
func NewSDKClient() (*SDKClient, error) {
	// First check if DOCKER_HOST is already set
	if os.Getenv("DOCKER_HOST") != "" {
		cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			return nil, fmt.Errorf("failed to create docker client with DOCKER_HOST: %w", err)
		}
		return &SDKClient{cli: cli}, nil
	}
	
	// Try common Docker socket locations
	homeDir, _ := os.UserHomeDir()
	socketPaths := []string{
		fmt.Sprintf("unix://%s/.config/colima/default/docker.sock", homeDir), // Colima default
		"unix:///var/run/docker.sock",                                         // Standard Docker
		fmt.Sprintf("unix://%s/.docker/run/docker.sock", homeDir),            // Docker Desktop
		fmt.Sprintf("unix://%s/.colima/default/docker.sock", homeDir),        // Colima alternate
	}
	
	var lastErr error
	for _, socketPath := range socketPaths {
		cli, err := client.NewClientWithOpts(
			client.WithHost(socketPath),
			client.WithAPIVersionNegotiation(),
		)
		if err != nil {
			lastErr = err
			continue
		}
		
		// Test the connection
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		
		_, err = cli.Ping(ctx)
		if err == nil {
			return &SDKClient{cli: cli}, nil
		}
		
		cli.Close()
		lastErr = err
	}
	
	// If we get here, no socket worked
	return nil, fmt.Errorf("failed to connect to Docker. Make sure Docker is running. Last error: %w", lastErr)
}

// IsContainerRunning checks if a container is running
func (c *SDKClient) IsContainerRunning(ctx context.Context, containerName string) bool {
	containers, err := c.cli.ContainerList(ctx, container.ListOptions{
		Filters: filters.NewArgs(filters.KeyValuePair{
			Key:   "name",
			Value: containerName,
		}),
	})
	if err != nil {
		return false
	}
	
	for _, cont := range containers {
		// Check exact name match (Docker adds "/" prefix)
		for _, name := range cont.Names {
			if name == "/"+containerName {
				return cont.State == "running"
			}
		}
	}
	return false
}

// StopAndRemoveContainer stops and removes a container
func (c *SDKClient) StopAndRemoveContainer(ctx context.Context, containerName string, force bool) error {
	// First try to stop the container
	if err := c.cli.ContainerStop(ctx, containerName, container.StopOptions{}); err != nil {
		// If container doesn't exist, that's fine
		if !client.IsErrNotFound(err) && !force {
			return fmt.Errorf("failed to stop container: %w", err)
		}
	}
	
	// Then remove it
	if err := c.cli.ContainerRemove(ctx, containerName, container.RemoveOptions{
		Force: force,
	}); err != nil {
		// If container doesn't exist, that's fine
		if !client.IsErrNotFound(err) {
			return fmt.Errorf("failed to remove container: %w", err)
		}
	}
	
	return nil
}

// EnsureNetworkExists creates a network if it doesn't exist
func (c *SDKClient) EnsureNetworkExists(ctx context.Context, networkName string) error {
	// Check if network exists
	networks, err := c.cli.NetworkList(ctx, network.ListOptions{
		Filters: filters.NewArgs(filters.KeyValuePair{
			Key:   "name",
			Value: networkName,
		}),
	})
	if err != nil {
		return fmt.Errorf("failed to list networks: %w", err)
	}
	
	// Check for exact name match
	for _, net := range networks {
		if net.Name == networkName {
			return nil // Network already exists
		}
	}
	
	// Create the network
	_, err = c.cli.NetworkCreate(ctx, networkName, network.CreateOptions{
		Driver: "bridge",
	})
	if err != nil {
		return fmt.Errorf("failed to create network: %w", err)
	}
	
	return nil
}

// StartTraefikContainer starts a new Traefik container with the given configuration
func (c *SDKClient) StartTraefikContainer(ctx context.Context, args TraefikContainerArgs) error {
	// Pull the image if needed
	reader, err := c.cli.ImagePull(ctx, args.Image, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}
	defer reader.Close()
	// Consume the output to ensure pull completes
	io.Copy(io.Discard, reader)
	
	// Configure port bindings
	exposedPorts := nat.PortSet{
		"80/tcp":   struct{}{},
		"443/tcp":  struct{}{},
		"8080/tcp": struct{}{},
	}
	
	portBindings := nat.PortMap{
		"80/tcp": []nat.PortBinding{
			{HostPort: args.HTTPPort},
		},
		"443/tcp": []nat.PortBinding{
			{HostPort: args.HTTPSPort},
		},
		"8080/tcp": []nat.PortBinding{
			{HostPort: args.DashboardPort},
		},
	}
	
	// Create the container
	resp, err := c.cli.ContainerCreate(ctx, &container.Config{
		Image:        args.Image,
		ExposedPorts: exposedPorts,
		Labels: map[string]string{
			"grove.managed": "true",
		},
	}, &container.HostConfig{
		PortBindings: portBindings,
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: "/var/run/docker.sock",
				Target: "/var/run/docker.sock",
			},
			{
				Type:   mount.TypeBind,
				Source: args.ConfigDir,
				Target: "/etc/traefik",
			},
		},
		RestartPolicy: container.RestartPolicy{
			Name: container.RestartPolicyUnlessStopped,
		},
	}, &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			args.NetworkName: {},
		},
	}, nil, args.Name)
	
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}
	
	// Start the container
	if err := c.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		// Clean up the created container
		c.cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		return fmt.Errorf("failed to start container: %w", err)
	}
	
	return nil
}

// GetNetworkInfo retrieves information about a Docker network
func (c *SDKClient) GetNetworkInfo(ctx context.Context, networkName string) (*NetworkInfo, error) {
	networks, err := c.cli.NetworkList(ctx, network.ListOptions{
		Filters: filters.NewArgs(filters.KeyValuePair{
			Key:   "name",
			Value: networkName,
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list networks: %w", err)
	}
	
	// Find exact match
	for _, net := range networks {
		if net.Name == networkName {
			return &NetworkInfo{
				Name:   net.Name,
				ID:     net.ID,
				Driver: net.Driver,
			}, nil
		}
	}
	
	return nil, fmt.Errorf("network %s not found", networkName)
}

// ListContainersOnNetwork returns a list of container names connected to the specified network
func (c *SDKClient) ListContainersOnNetwork(ctx context.Context, networkName string) ([]string, error) {
	containers, err := c.cli.ContainerList(ctx, container.ListOptions{
		Filters: filters.NewArgs(filters.KeyValuePair{
			Key:   "network",
			Value: networkName,
		}),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}
	
	var names []string
	for _, cont := range containers {
		// Remove leading "/" from names
		for _, name := range cont.Names {
			cleanName := strings.TrimPrefix(name, "/")
			names = append(names, cleanName)
		}
	}
	
	return names, nil
}

// RemoveNetwork removes a Docker network
func (c *SDKClient) RemoveNetwork(ctx context.Context, networkName string) error {
	return c.cli.NetworkRemove(ctx, networkName)
}

// ImageExists checks if a Docker image exists locally
func (c *SDKClient) ImageExists(ctx context.Context, imageName string) (bool, error) {
	images, err := c.cli.ImageList(ctx, image.ListOptions{
		Filters: filters.NewArgs(filters.KeyValuePair{
			Key:   "reference",
			Value: imageName,
		}),
	})
	if err != nil {
		return false, fmt.Errorf("failed to list images: %w", err)
	}
	
	return len(images) > 0, nil
}

// BuildImage builds a Docker image from a build context
func (c *SDKClient) BuildImage(ctx context.Context, buildContextPath string, dockerfilePath string, imageName string) error {
	// Create a tar archive of the build context
	buildContext, err := c.createBuildContext(buildContextPath)
	if err != nil {
		return fmt.Errorf("failed to create build context: %w", err)
	}
	defer buildContext.Close()
	
	// Determine the relative dockerfile path
	relativeDockerfilePath := dockerfilePath
	if filepath.IsAbs(dockerfilePath) {
		var err error
		relativeDockerfilePath, err = filepath.Rel(buildContextPath, dockerfilePath)
		if err != nil {
			return fmt.Errorf("failed to get relative dockerfile path: %w", err)
		}
	}
	
	// Build the image
	buildResponse, err := c.cli.ImageBuild(ctx, buildContext, types.ImageBuildOptions{
		Dockerfile: relativeDockerfilePath,
		Tags:       []string{imageName},
		Remove:     true, // Remove intermediate containers
	})
	if err != nil {
		return fmt.Errorf("failed to build image: %w", err)
	}
	defer buildResponse.Body.Close()
	
	// Consume the build output
	_, err = io.Copy(io.Discard, buildResponse.Body)
	if err != nil {
		return fmt.Errorf("failed to read build response: %w", err)
	}
	
	return nil
}

// createBuildContext creates a tar archive of the build context directory
func (c *SDKClient) createBuildContext(contextPath string) (io.ReadCloser, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	defer tw.Close()
	
	err := filepath.Walk(contextPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// Get relative path
		relPath, err := filepath.Rel(contextPath, path)
		if err != nil {
			return err
		}
		
		// Skip the root directory itself
		if relPath == "." {
			return nil
		}
		
		// Create tar header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath
		
		// Write header
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		
		// Write file content if it's a regular file
		if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			
			_, err = io.Copy(tw, file)
			if err != nil {
				return err
			}
		}
		
		return nil
	})
	
	if err != nil {
		return nil, err
	}
	
	return io.NopCloser(&buf), nil
}

// ExecInContainer executes a command inside a running container
func (c *SDKClient) ExecInContainer(ctx context.Context, containerName string, cmd []string, stdin io.Reader) (stdout string, stderr string, err error) {
	// Create the exec instance
	execConfig := container.ExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		AttachStdin:  stdin != nil,
		Cmd:          cmd,
	}
	
	execIDResp, err := c.cli.ContainerExecCreate(ctx, containerName, execConfig)
	if err != nil {
		return "", "", fmt.Errorf("failed to create exec instance: %w", err)
	}
	
	// Attach to the exec instance
	attachResp, err := c.cli.ContainerExecAttach(ctx, execIDResp.ID, container.ExecStartOptions{})
	if err != nil {
		return "", "", fmt.Errorf("failed to attach to exec instance: %w", err)
	}
	defer attachResp.Close()
	
	// Handle stdin if provided
	if stdin != nil {
		go func() {
			defer attachResp.CloseWrite()
			io.Copy(attachResp.Conn, stdin)
		}()
	}
	
	// Read stdout and stderr
	var stdoutBuf, stderrBuf bytes.Buffer
	
	// Start the exec instance
	if err := c.cli.ContainerExecStart(ctx, execIDResp.ID, container.ExecStartOptions{}); err != nil {
		return "", "", fmt.Errorf("failed to start exec instance: %w", err)
	}
	
	// Copy output
	_, err = io.Copy(&stdoutBuf, attachResp.Reader)
	if err != nil {
		return "", "", fmt.Errorf("failed to read exec output: %w", err)
	}
	
	// Inspect exec to get exit code
	execInspect, err := c.cli.ContainerExecInspect(ctx, execIDResp.ID)
	if err != nil {
		return "", "", fmt.Errorf("failed to inspect exec instance: %w", err)
	}
	
	if execInspect.ExitCode != 0 {
		return stdoutBuf.String(), stderrBuf.String(), fmt.Errorf("command exited with code %d", execInspect.ExitCode)
	}
	
	return stdoutBuf.String(), stderrBuf.String(), nil
}

// Close closes the Docker client connection
func (c *SDKClient) Close() error {
	return c.cli.Close()
}