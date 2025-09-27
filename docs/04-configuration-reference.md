# Grove Configuration Reference (`grove.yml`)

This document provides a comprehensive reference for all the configuration options available in the `grove.yml` file.

## Top-Level Keys

The `grove.yml` file is structured around several top-level keys that define the workspace, its services, and behavior.

| Key          | Type                           | Description                                                                                              |
| :----------- | :----------------------------- | :------------------------------------------------------------------------------------------------------- |
| `version`    | `string`                       | Specifies the configuration file version. Primarily for compatibility with Docker Compose.               |
| `services`   | `map[string]ServiceConfig]`    | Defines the application services that make up your workspace.                                            |
| `networks`   | `map[string]NetworkConfig]`    | Defines the Docker networks your services will connect to.                                               |
| `volumes`    | `map[string]VolumeConfig]`     | Defines the Docker volumes used by your services for persistent data.                                    |
| `profiles`   | `map[string]ProfileConfig]`    | Groups services together for different environments or use cases (e.g., `development`, `testing`).       |
| `settings`   | `Settings`                     | Contains Grove-specific settings for project management and behavior.                                    |
| `agent`      | `AgentConfig`                  | Configures the Grove agent for enhanced development workflows.                                           |
| `logging`    | `LoggingConfig`                | Configures the centralized logging system for all Grove tools.                                           |
| `workspaces` | `list[string]`                 | (Not yet implemented) Defines paths to other projects to include in the current workspace.               |
| `*`          | `map[string]any`               | Any other top-level key is treated as a custom extension for other tools in the Grove ecosystem.         |

---

## `version`

Specifies the version of the configuration file format. It is recommended to align this with a compatible Docker Compose version string.

- **Type**: `string`
- **Default**: `"1.0"`

```yaml
version: "1.0"
```

---

## `services`

This section defines the individual components of your application. Each key under `services` is a service name, and its value is an object describing the service's configuration.

```yaml
services:
  api:
    image: node:18-alpine
    build: ./api
    ports:
      - "8080:8080"
    volumes:
      - ./api:/app
  db:
    image: postgres:14
    environment:
      - POSTGRES_DB=mydb
```

### Service Configuration Fields

| Key           | Type                  | Description                                                                                                   |
| :------------ | :-------------------- | :------------------------------------------------------------------------------------------------------------ |
| `image`       | `string`              | The Docker image to use for the service container.                                                            |
| `build`       | `string` or `object`  | Path to the build context, or an object with build details (e.g., `context`, `dockerfile`).                   |
| `ports`       | `list[string]`        | A list of port mappings in `"HOST:CONTAINER"` format.                                                         |
| `volumes`     | `list[string]`        | A list of volume mounts in `"HOST/VOLUME:CONTAINER"` format.                                                  |
| `environment` | `list[string]`        | A list of environment variables in `"KEY=VALUE"` format.                                                      |
| `env_file`    | `list[string]`        | A list of paths to files containing environment variables.                                                    |
| `depends_on`  | `list[string]`        | A list of other service names that this service depends on.                                                   |
| `labels`      | `map[string]string`   | A map of Docker labels to apply to the service container. Used for Traefik routing.                           |
| `command`     | `string` or `list`    | Overrides the default command for the container.                                                              |
| `profiles`    | `list[string]`        | A list of profile names this service belongs to.                                                              |

---

## `networks`

Defines custom Docker networks.

- **Type**: `map[string]NetworkConfig`

```yaml
networks:
  my-network:
    driver: bridge
  external-net:
    external: true
```

### Network Configuration Fields

| Key        | Type      | Description                                                    |
| :--------- | :-------- | :------------------------------------------------------------- |
| `driver`   | `string`  | The network driver to use (e.g., `bridge`, `overlay`).         |
| `external` | `boolean` | If `true`, specifies that this network is created externally.  |

---

## `volumes`

Defines named Docker volumes.

- **Type**: `map[string]VolumeConfig`

```yaml
volumes:
  db-data:
    driver: local
  external-data:
    external: true
```

### Volume Configuration Fields

| Key        | Type      | Description                                                    |
| :--------- | :-------- | :------------------------------------------------------------- |
| `driver`   | `string`  | The volume driver to use (e.g., `local`).                      |
| `external` | `boolean` | If `true`, specifies that this volume is created externally.   |

---

## `profiles`

Defines named groups of services. You can start a specific profile using `grove up <profile-name>`.

- **Type**: `map[string]ProfileConfig`

```yaml
profiles:
  default:
    services:
      - api
      - web
  all:
    services:
      - api
      - web
      - db
      - cache
```

### Profile Configuration Fields

| Key        | Type           | Description                                                        |
| :--------- | :------------- | :----------------------------------------------------------------- |
| `services` | `list[string]` | A list of service names to include when this profile is activated. |
| `env_file` | `list[string]` | A list of environment files to apply for this profile.             |

---

## `settings`

This key holds Grove-specific configuration that controls how the workspace is managed.

- **Type**: `Settings`

```yaml
settings:
  project_name: my-app
  network_name: my-app-net
  domain_suffix: my-app.local
  default_profile: all
  traefik_enabled: true
  mcp_port: 1668
```

### Settings Fields

| Key               | Type      | Description                                                                                                     | Default       |
| :---------------- | :-------- | :-------------------------------------------------------------------------------------------------------------- | :------------ |
| `project_name`    | `string`  | Overrides the default Docker Compose project name.                                                              | (inferred)    |
| `network_name`    | `string`  | The name of the default network that all services connect to.                                                   | `"grove"`     |
| `domain_suffix`   | `string`  | The domain suffix used for service routing (e.g., `api.my-app.local`).                                          | `"localhost"` |
| `default_profile` | `string`  | The profile to use when no profile is specified in a command.                                                   | `"default"`   |
| `traefik_enabled` | `boolean` | If `true`, a Traefik reverse proxy service is automatically managed for the workspace.                          | `true`        |
| `auto_inference`  | `boolean` | If `true`, Grove will automatically infer common settings for services (e.g., Node.js volumes).                 | `true`        |
| `concurrency`     | `integer` | The number of concurrent operations for tasks like starting services.                                           | (CPU count)   |
| `mcp_port`        | `integer` | The port for the Grove Master Control Program (MCP) server, used for inter-tool communication.                  | `1667`        |
| `canopy_port`     | `integer` | The port for the Grove Canopy UI server.                                                                        | `8888`        |

---

## `agent`

Configures the Grove Agent, a sidecar container that provides development assistance.

- **Type**: `AgentConfig`

```yaml
agent:
  enabled: true
  image: custom/agent:latest
  extra_volumes:
    - ~/.ssh:/home/claude/.ssh:ro
```

### Agent Configuration Fields

| Key                            | Type           | Description                                                                                             | Default                                     |
| :----------------------------- | :------------- | :------------------------------------------------------------------------------------------------------ | :------------------------------------------ |
| `enabled`                      | `boolean`      | Enables or disables the agent.                                                                          | `false`                                     |
| `image`                        | `string`       | The Docker image for the agent container.                                                               | `"ghcr.io/mattsolo1/grove-agent:v0.1.0"`      |
| `logs_path`                    | `string`       | The host path to mount for agent project logs.                                                          | `"~/.claude/projects"`                      |
| `extra_volumes`                | `list[string]` | A list of additional volumes to mount into the agent container.                                         | (none)                                      |
| `notes_dir`                    | `string`       | A directory on the host to mount for the agent to read/write notes.                                     | (none)                                      |
| `args`                         | `list[string]` | Additional command-line arguments to pass to the agent's entrypoint.                                    | (none)                                      |
| `output_format`                | `string`       | Output format for piped input: `text` (default), `json`, or `stream-json`.                              | `"text"`                                    |
| `mount_workspace_at_host_path` | `boolean`      | Mounts the host's git repository root to the same absolute path inside the container.                     | `false`                                     |
| `use_superproject_root`        | `boolean`      | When in a git submodule, uses the parent repository root as the context.                                | `false`                                     |

---

## `logging`

Configures the behavior of the structured logging system used by Grove tools.

- **Type**: `LoggingConfig`

```yaml
logging:
  level: debug
  report_caller: true
  file:
    enabled: true
    path: /var/log/grove/app.log
    format: json
  format:
    preset: default
```

### Logging Configuration Fields

| Key             | Type           | Description                                                                                                   | Default  |
| :-------------- | :------------- | :------------------------------------------------------------------------------------------------------------ | :------- |
| `level`         | `string`       | Minimum log level to output (`debug`, `info`, `warn`, `error`). Can be overridden by `GROVE_LOG_LEVEL`.         | `"info"` |
| `report_caller` | `boolean`      | If `true`, includes the file, line, and function name in logs. Can be overridden by `GROVE_LOG_CALLER=true`.  | `false`  |
| `file`          | `FileSinkConfig` | Configuration for logging to a file.                                                                          | (none)   |
| `format`        | `FormatConfig` | Configuration for the console log output format.                                                              | (none)   |

### `file` Fields

| Key       | Type      | Description                                                                     | Default                               |
| :-------- | :-------- | :------------------------------------------------------------------------------ | :------------------------------------ |
| `enabled` | `boolean` | If `true`, enables logging to a file.                                           | `false`                               |
| `path`    | `string`  | The full path to the log file.                                                  | `.grove/logs/workspace-<date>.log`    |
| `format`  | `string`  | The format for the file log output (`text` or `json`).                          | `"text"`                              |

### `format` Fields

| Key                  | Type      | Description                                                                                              | Default  |
| :------------------- | :-------- | :------------------------------------------------------------------------------------------------------- | :------- |
| `preset`             | `string`  | Console output format: `default` (rich text), `simple` (minimal), or `json`.                             | `default`|
| `disable_timestamp`  | `boolean` | If `true`, removes the timestamp from console output.                                                    | `false`  |
| `disable_component`  | `boolean` | If `true`, removes the component name (e.g., `[api]`) from console output.                               | `false`  |
| `structured_to_stderr`| `string` | When to send structured logs to stderr: `auto` (default), `always`, or `never`.                           | `auto`   |

---

## Custom Extensions

The `grove.yml` file is designed to be extensible. Any top-level key that is not a standard Grove key (like `services`, `settings`, etc.) is captured as a custom extension. This allows other tools in the Grove ecosystem, such as `grove-flow`, to define their own configuration sections within the same file.

These custom sections can be parsed in Go using the `UnmarshalExtension` method on a loaded `Config` object.

### Example

A `grove.yml` with a custom `flow` section:

```yaml
version: "1.0"
services:
  # ...

settings:
  # ...

# Custom section for grove-flow
flow:
  chat_directory: "/path/to/chats"
  max_messages: 100
```

This `flow` block can be loaded by a tool into a custom struct:

```go
import "github.com/mattsolo1/grove-core/config"

type FlowConfig struct {
    ChatDirectory string `yaml:"chat_directory"`
    MaxMessages   int    `yaml:"max_messages"`
}

func main() {
    groveCfg, err := config.LoadDefault()
    if err != nil {
        // handle error
    }

    var flowCfg FlowConfig
    if err := groveCfg.UnmarshalExtension("flow", &flowCfg); err != nil {
        // handle error
    }

    // Now use flowCfg.ChatDirectory, etc.
}
```