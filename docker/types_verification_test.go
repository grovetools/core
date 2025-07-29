package docker

import (
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
)

func TestDockerV26Types(t *testing.T) {
	// Verify NetworkListOptions is in types package
	t.Run("NetworkListOptions", func(t *testing.T) {
		opts := types.NetworkListOptions{
			Filters: filters.NewArgs(filters.KeyValuePair{
				Key:   "name",
				Value: "test",
			}),
		}
		t.Logf("NetworkListOptions created successfully: %+v", opts)
	})

	// Verify NetworkCreate is in types package
	t.Run("NetworkCreate", func(t *testing.T) {
		createOpts := types.NetworkCreate{
			Driver: "bridge",
			IPAM: &network.IPAM{
				Driver: "default",
			},
			Labels: map[string]string{
				"test": "label",
			},
		}
		t.Logf("NetworkCreate created successfully: %+v", createOpts)
	})

	// Verify ExecConfig is in types package
	t.Run("ExecConfig", func(t *testing.T) {
		execConfig := types.ExecConfig{
			AttachStdout: true,
			AttachStderr: true,
			AttachStdin:  true,
			Cmd:          []string{"echo", "hello"},
			Tty:          false,
		}
		t.Logf("ExecConfig created successfully: %+v", execConfig)
	})

	// Verify ExecStartCheck is in types package
	t.Run("ExecStartCheck", func(t *testing.T) {
		execStart := types.ExecStartCheck{
			Detach: false,
			Tty:    false,
		}
		t.Logf("ExecStartCheck created successfully: %+v", execStart)
	})

	// Verify container types are in the right place
	t.Run("ContainerTypes", func(t *testing.T) {
		// ListOptions
		listOpts := container.ListOptions{
			All:    true,
			Latest: false,
			Size:   true,
			Filters: filters.NewArgs(filters.KeyValuePair{
				Key:   "status",
				Value: "running",
			}),
		}
		t.Logf("container.ListOptions created successfully: %+v", listOpts)

		// Config
		config := &container.Config{
			Image: "test:latest",
			Cmd:   []string{"echo", "hello"},
		}
		t.Logf("container.Config created successfully: %+v", config)

		// HostConfig
		hostConfig := &container.HostConfig{
			RestartPolicy: container.RestartPolicy{
				Name: container.RestartPolicyUnlessStopped,
			},
			Mounts: []mount.Mount{
				{
					Type:   mount.TypeBind,
					Source: "/src",
					Target: "/dst",
				},
			},
		}
		t.Logf("container.HostConfig created successfully: %+v", hostConfig)
	})

	// Verify image types
	t.Run("ImageTypes", func(t *testing.T) {
		listOpts := image.ListOptions{
			All: true,
			Filters: filters.NewArgs(filters.KeyValuePair{
				Key:   "reference",
				Value: "test",
			}),
		}
		t.Logf("image.ListOptions created successfully: %+v", listOpts)

		pullOpts := image.PullOptions{
			RegistryAuth: "",
		}
		t.Logf("image.PullOptions created successfully: %+v", pullOpts)
	})

	// Verify network types
	t.Run("NetworkingTypes", func(t *testing.T) {
		netConfig := &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				"test-network": {
					NetworkID: "test-id",
				},
			},
		}
		t.Logf("network.NetworkingConfig created successfully: %+v", netConfig)
	})
}