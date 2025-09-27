# Usage Patterns and Examples

This document provides practical examples and recipes for using `grove-core` to handle common development scenarios. These patterns demonstrate how to leverage the library's features for configuration, automation, and user interaction.

## 1. Defining Custom Configuration Extensions

The `grove.yml` configuration system is extensible, allowing tools to define their own top-level configuration blocks without modifying `grove-core`. This is achieved through the `Extensions` field in the `config.Config` struct.

This pattern is useful when you are building a tool that needs its own specific settings but should integrate with the existing Grove configuration ecosystem.

**Step 1: Define a custom section in `grove.yml`**

Add a top-level key that is unique to your tool. In this example, we'll add a `flow` block for a hypothetical `grove-flow` tool.

```yaml
# grove.yml
version: "1.0"

services:
  api:
    image: node:18

# Custom section for our tool
flow:
  chat_directory: "/path/to/chats"
  max_messages: 100
  default_model: "claude-3-opus"
```

**Step 2: Create a Go struct to hold the configuration**

In your tool's Go code, define a struct that maps to the YAML structure of your custom section. Use `yaml` tags to specify the field names.

```go
package main

// FlowConfig defines the structure for the 'flow' section in grove.yml
type FlowConfig struct {
    ChatDirectory string `yaml:"chat_directory"`
    MaxMessages   int    `yaml:"max_messages"`
    DefaultModel  string `yaml:"default_model"`
}
```

**Step 3: Load the configuration and unmarshal the extension**

Use `config.LoadDefault()` to load the entire `grove.yml` file, and then use the `UnmarshalExtension` method to parse your custom section into your struct.

```go
import (
    "fmt"
    "log"

    "github.com/mattsolo1/grove-core/config"
)

func main() {
    // Load the complete grove.yml configuration
    coreCfg, err := config.LoadDefault()
    if err != nil {
        log.Fatalf("Failed to load configuration: %v", err)
    }

    // Create an instance of our custom config struct
    var flowCfg FlowConfig

    // Unmarshal the 'flow' extension into our struct
    if err := coreCfg.UnmarshalExtension("flow", &flowCfg); err != nil {
        log.Fatalf("Failed to parse 'flow' configuration: %v", err)
    }

    // Now you can use the type-safe configuration
    fmt.Printf("Chat directory: %s\n", flowCfg.ChatDirectory)
    fmt.Printf("Default model: %s\n", flowCfg.DefaultModel)
}
```

## 2. Hierarchical Configuration Merging

`grove-core` loads configuration from up to three different files, merging them to produce a final, consolidated configuration. This allows for a flexible setup with global defaults, project-specific settings, and local developer overrides.

The merge order is:
1.  **Global**: `~/.config/grove/grove.yml` (lowest precedence)
2.  **Project**: `grove.yml` in the project directory
3.  **Override**: `grove.override.yml` in the project directory (highest precedence)

**Example Scenario:**

Imagine you want a global default setting, a project-specific service definition, and a local override for a service port.

**File 1: Global Configuration (`~/.config/grove/grove.yml`)**

This file sets a global default for all projects, such as the port for the Master Control Program (MCP).

```yaml
# ~/.config/grove/grove.yml
settings:
  mcp_port: 1667 # Default MCP port for all projects
```

**File 2: Project Configuration (`./grove.yml`)**

This is the main configuration file for the project, checked into version control. It defines the project's services and overrides the global network name.

```yaml
# ./grove.yml
version: "1.0"

settings:
  project_name: my-web-app

services:
  api:
    image: my-api:1.2.0
    ports:
      - "8080:80" # Default port for the API
```

**File 3: Local Override (`./grove.override.yml`)**

This file is typically ignored by version control (e.g., in `.gitignore`). It allows a developer to override settings for their local environment without affecting the project configuration. Here, we change the public port for the `api` service to avoid a conflict on the local machine.

```yaml
# ./grove.override.yml
services:
  api:
    ports:
      - "9999:80" # Override the host port for local development
```

When `config.LoadDefault()` is called from within the project directory, the final merged configuration will effectively be:
- `settings.mcp_port`: `1667` (from global)
- `settings.project_name`: `my-web-app` (from project)
- `services.api.image`: `my-api:1.2.0` (from project)
- `services.api.ports`: `["9999:80"]` (from override)

## 3. Managing Git Worktrees

The `git.WorktreeManager` provides a programmatic interface for managing Git worktrees. This is useful for automation that needs to operate on different branches in isolated directory trees without performing a full checkout.

This example shows how to create a worktree for a feature branch, list all worktrees, and then clean it up.

```go
import (
    "context"
    "fmt"
    "log"
    "path/filepath"

    "github.com/mattsolo1/grove-core/git"
)

func main() {
    ctx := context.Background()
    repoPath := "." // Assume current directory is a Git repository
    worktreeDir := "/tmp/my-feature-worktree"
    branchName := "new-feature-branch"

    manager := git.NewWorktreeManager()

    // 1. Create a new worktree and a new branch
    fmt.Printf("Creating worktree at %s for branch %s...\n", worktreeDir, branchName)
    err := manager.CreateWorktree(ctx, repoPath, worktreeDir, branchName, true)
    if err != nil {
        log.Fatalf("Failed to create worktree: %v", err)
    }
    fmt.Println("Worktree created successfully.")

    // 2. List all worktrees for the repository
    fmt.Println("\nListing all worktrees:")
    worktrees, err := manager.ListWorktrees(ctx, repoPath)
    if err != nil {
        log.Fatalf("Failed to list worktrees: %v", err)
    }
    for _, wt := range worktrees {
        fmt.Printf("- Path: %s, Branch: %s, Commit: %s\n",
            filepath.Base(wt.Path), wt.Branch, wt.Commit[:7])
    }

    // 3. Remove the worktree when done
    fmt.Printf("\nRemoving worktree at %s...\n", worktreeDir)
    if err := manager.RemoveWorktree(ctx, repoPath, worktreeDir); err != nil {
        log.Fatalf("Failed to remove worktree: %v", err)
    }
    fmt.Println("Worktree removed successfully.")
}
```

## 4. Distinguishing Pretty and Structured Logging

The `logging` package provides two distinct logging mechanisms for different purposes:

-   **`logging.NewLogger`**: Creates a structured logger (`logrus`) for internal application logging. These logs are intended for diagnostics, debugging, and machine parsing. They write to `stderr` and optionally to a file, and their format is controlled by `grove.yml`.
-   **`logging.PrettyLogger`**: Creates a logger for generating styled, user-facing output directly to the console (`stderr`). This is used for providing clear feedback to the user, such as success or failure messages. Its output is not structured and is not sent to the log file.

```go
import (
    "errors"
    "github.com/mattsolo1/grove-core/logging"
)

func main() {
    // Structured logger for internal diagnostics
    structuredLog := logging.NewLogger("file-processor")

    // Pretty logger for user-facing status messages
    prettyLog := logging.NewPrettyLogger()

    filePath := "/path/to/important.doc"

    // Use the structured logger to record what the application is doing.
    // This goes to stderr and the configured log file.
    structuredLog.WithField("file", filePath).Info("Starting to process file")

    // Use the pretty logger to inform the user about the result.
    // This only goes to the console (stderr).
    err := processFile(filePath)
    if err != nil {
        prettyLog.ErrorPretty("Failed to process file", err)
        structuredLog.WithError(err).Error("File processing failed")
    } else {
        prettyLog.Success("Successfully processed file!")
        prettyLog.Path("Output saved to", "/path/to/output.txt")
        structuredLog.Info("File processing completed successfully")
    }
}

func processFile(path string) error {
    // Simulate an error
    return errors.New("permission denied")
}
```

## 5. Custom Error Handling

The `errors` package provides a set of structured error types that can be recognized and handled by the `cli.ErrorHandler`. This allows you to return specific, typed errors from your application logic and have them automatically translated into user-friendly messages at the CLI layer.

**Step 1: Return a specific `GroveError` from your function**

Use one of the error constructors from the `errors` package, like `ServiceNotFound`.

```go
import "github.com/mattsolo1/grove-core/errors"

// findService simulates looking for a service in the configuration.
func findService(name string, availableServices map[string]bool) error {
    if !availableServices[name] {
        // Return a specific, structured error.
        return errors.ServiceNotFound(name)
    }
    return nil
}
```

**Step 2: Use the `cli.ErrorHandler` in your command**

In your `main.go`, instantiate an `ErrorHandler` and use it to handle errors returned from your command's execution.

```go
import (
    "os"
    "github.com/mattsolo1/grove-core/cli"
    "github.com/spf13/cobra"
)

func main() {
    rootCmd := cli.NewStandardCommand("start", "Starts a service")

    rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
        if len(args) == 0 {
            // ...
        }
        serviceName := args[0]

        // Get options to check for the --verbose flag
        opts := cli.GetOptions(cmd)
        
        // This is where the error from findService would be returned
        err := findService(serviceName, map[string]bool{"db": true})
        if err != nil {
            // The handler will inspect the error and print a friendly message.
            handler := cli.NewErrorHandler(opts.Verbose)
            return handler.Handle(err)
        }

        return nil
    }

    if err := rootCmd.Execute(); err != nil {
        os.Exit(1)
    }
}
```

If you run this command with a non-existent service name (e.g., `./my-tool start api`), the `ErrorHandler` will catch the `ErrCodeServiceNotFound` error and print a helpful, formatted message to the user:

```sh
$ ./my-tool start api
❌ Service 'api' not found in grove.yml
Run 'grove services' to see available services.
```