# Docker API v26 Type Locations

This document clarifies the correct type locations for Docker API v26.1.4.

## Key Type Changes

### Network-related Types

1. **NetworkListOptions** - Located in `github.com/docker/docker/api/types`
   ```go
   import "github.com/docker/docker/api/types"
   opts := types.NetworkListOptions{
       Filters: filters.NewArgs(...),
   }
   ```

2. **NetworkCreate** - Located in `github.com/docker/docker/api/types`
   ```go
   import "github.com/docker/docker/api/types"
   createOpts := types.NetworkCreate{
       Driver: "bridge",
   }
   ```

### Exec-related Types

1. **ExecConfig** - Located in `github.com/docker/docker/api/types`
   ```go
   import "github.com/docker/docker/api/types"
   execConfig := types.ExecConfig{
       AttachStdout: true,
       AttachStderr: true,
       Cmd:          []string{"echo", "hello"},
   }
   ```

2. **ExecStartCheck** - Located in `github.com/docker/docker/api/types`
   ```go
   import "github.com/docker/docker/api/types"
   execStart := types.ExecStartCheck{
       Detach: false,
       Tty:    false,
   }
   ```

### Container-related Types (unchanged)

These remain in their expected locations:
- `container.Config` - In `github.com/docker/docker/api/types/container`
- `container.HostConfig` - In `github.com/docker/docker/api/types/container`
- `container.ListOptions` - In `github.com/docker/docker/api/types/container`
- `container.StartOptions` - In `github.com/docker/docker/api/types/container`
- `container.StopOptions` - In `github.com/docker/docker/api/types/container`
- `container.RemoveOptions` - In `github.com/docker/docker/api/types/container`

### Network Package Types

The `github.com/docker/docker/api/types/network` package contains:
- `NetworkingConfig`
- `EndpointSettings`
- `IPAM`
- `IPAMConfig`

### Image Package Types

The `github.com/docker/docker/api/types/image` package contains:
- `ListOptions`
- `PullOptions`

## Common Migration Issues

When upgrading to Docker API v26, watch out for these changes:

1. `network.ListOptions` → `types.NetworkListOptions`
2. `network.CreateOptions` → `types.NetworkCreate`
3. `container.ExecOptions` → `types.ExecConfig`
4. `container.ExecStartOptions` → `types.ExecStartCheck`

## Verification Test

The `types_verification_test.go` file contains examples of all these types being used correctly.