# Best Practices

This document outlines recommended best practices for developing tools with the `grove-core` library. Adhering to these guidelines will help you build applications that are maintainable, consistent with the Grove ecosystem, and provide a better experience for users.

## Logging

The `logging` package is designed for structured, machine-readable logs. To get the most out of it, you should favor structured fields over simple string formatting.

### Use Structured Fields

Always prefer using `WithField` or `WithFields` to add context to your log messages. This makes logs easier to parse, filter, and analyze.

**Recommended:**
```go
import "github.comcom/mattsolo1/grove-core/logging"

log := logging.NewLogger("file-processor")

// Use structured fields to add context
log.WithField("file_path", "/data/file.txt").Info("Processing file")
```

**Avoid:**
```go
// Avoid string formatting, as it loses the structured context
log.Infof("Processing file %s", "/data/file.txt")
```

### Use `WithError` for Errors

When logging an error, use the `WithError` method. This attaches the full error to the log entry under a dedicated `error` key, preserving its context and stack trace if available.

**Recommended:**
```go
if err != nil {
    log.WithError(err).Error("Failed to process file")
}
```

**Avoid:**
```go
if err != nil {
    // This loses the structured error information
    log.Errorf("Failed to process file: %v", err)
}
```

## Configuration

The `config` package is designed to handle declarative configuration. Your `grove.yml` file should describe the desired state of your application, not the procedural steps to get there.

### Keep `grove.yml` Declarative

The configuration file should define *what* your services and settings are. Logic for how to use these settings belongs in your Go code. This separation of concerns makes the configuration easier to understand and manage.

### Use Environment Variables for Secrets

Sensitive information such as API keys, passwords, or tokens should not be stored directly in `grove.yml`. Instead, use environment variable expansion. The `config` loader will automatically substitute `${VAR_NAME}` or `${VAR_NAME:-default}` with the corresponding environment variable at load time.

**Example `grove.yml`:**
```yaml
services:
  api:
    image: my-api:latest
    environment:
      - DATABASE_URL=postgres://${DB_USER}:${DB_PASSWORD}@db:5432/mydb
      - API_KEY=${SERVICE_API_KEY}
```

You can then run your tool with the secrets provided in the environment:
```sh
DB_PASSWORD=mysecretpassword SERVICE_API_KEY=key123 my-tool up
```

## Error Handling

The `errors` package provides a system for structured errors that integrate with the `cli.ErrorHandler`. This pattern separates error creation from error presentation.

### Return `GroveError` Types

From your application logic, return specific `GroveError` types using the provided constructors (e.g., `errors.ServiceNotFound`, `errors.ConfigInvalid`). This allows the CLI layer to catch these specific error codes and display helpful, user-friendly messages.

**Example:**
```go
package myapp

import "github.com/mattsolo1/grove-core/errors"

// This function's logic is only concerned with its domain, not
// with how errors are presented to the user.
func FindService(name string, cfg *MyConfig) error {
    if _, ok := cfg.Services[name]; !ok {
        // Return a structured, typed error.
        return errors.ServiceNotFound(name)
    }
    return nil
}
```

The `cli.ErrorHandler` in your command's entry point will then translate this error into a clear message for the user, as it recognizes the `ErrCodeServiceNotFound` code.

## Extensibility

When building a new tool that integrates with the Grove ecosystem, it is best to create a custom configuration extension within `grove.yml` rather than using a separate configuration file.

### Design a Custom Configuration Extension

Define a Go struct for your tool's configuration and a corresponding top-level key in `grove.yml`. Use `config.UnmarshalExtension` to load your settings in a type-safe manner. This keeps all project configuration in a single, predictable location (`grove.yml`).

**Example:**

**`grove.yml`:**
```yaml
version: "1.0"
services:
  # ...

# Custom section for a new tool called "grove-flow"
flow:
  default_model: "claude-3-opus"
  max_retries: 3
```

**Go Code:**
```go
import "github.com/mattsolo1/grove-core/config"

type FlowConfig struct {
    DefaultModel string `yaml:"default_model"`
    MaxRetries   int    `yaml:"max_retries"`
}

func loadFlowConfig() (*FlowConfig, error) {
    coreCfg, err := config.LoadDefault()
    if err != nil {
        return nil, err
    }

    var flowCfg FlowConfig
    if err := coreCfg.UnmarshalExtension("flow", &flowCfg); err != nil {
        return nil, err
    }

    return &flowCfg, nil
}
```

## CLI Design

To ensure a consistent user experience across all tools in the Grove ecosystem, always use the helpers provided by the `cli` package.

### Use `NewStandardCommand`

Always initialize your root command and subcommands with `cli.NewStandardCommand`. This function automatically adds a standard set of persistent flags (`--verbose`, `--json`, `--config`) to your command, ensuring that all Grove tools share a common and predictable interface.

**Example:**
```go
package main

import (
	"fmt"
	"os"

	"github.com/mattsolo1/grove-core/cli"
)

func main() {
	rootCmd := cli.NewStandardCommand(
		"my-tool",
		"A brief description of my tool.",
	)

	// Add subcommands and logic here...

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```