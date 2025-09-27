# Core Concepts

The `grove-core` library is composed of several packages, each designed to provide a specific piece of functionality for building standardized command-line tools. This document outlines the purpose and key components of each major package.

## The `cli` Package

The `cli` package provides the foundation for building command-line interfaces that are consistent across the Grove ecosystem. It uses the `cobra` library internally and adds a layer of standardization for flags, logging, and error handling.

Key functions and types include:

*   **`NewStandardCommand`**: Creates a `cobra.Command` with a set of standard persistent flags: `--verbose`, `--json`, and `--config`. This ensures that all tools built with `grove-core` have a consistent set of base options.

    ```go
    import "github.com/mattsolo1/grove-core/cli"

    var rootCmd = cli.NewStandardCommand("my-tool", "A brief description of my tool")
    ```

*   **`GetLogger`**: Retrieves a `logrus` logger instance that is pre-configured based on the standard command-line flags. For example, it respects the `--verbose` and `--json` flags. Note that the main `logging` package is the primary way to get loggers; this is a helper for CLI integration.

*   **`GetOptions`**: A utility function to parse the standard flags from a `cobra.Command` into a `CommandOptions` struct for easy access.

*   **`ErrorHandler`**: A structured error handler that provides user-friendly output for specific error types defined in the `errors` package. It can display more detailed information when the `--verbose` flag is used.

## The `config` Package

The `config` package implements a hierarchical configuration system that loads settings from `grove.yml` files. It allows for global, per-project, and local override configurations, providing a flexible way to manage settings.

Core concepts include:

*   **Hierarchical Loading**: Configuration is loaded and merged from three locations in the following order of precedence (later files override earlier ones):
    1.  **Global**: `~/.config/grove/grove.yml`
    2.  **Project**: `grove.yml` found in the current or a parent directory.
    3.  **Override**: `grove.override.yml` in the same directory as the project file.

*   **`LoadDefault()`**: This is the primary entry point for the configuration system. It automatically finds and loads the configuration according to the hierarchical rules.

    ```go
    import "github.com/mattsolo1/grove-core/config"

    cfg, err := config.LoadDefault()
    if err != nil {
        // handle error
    }
    ```

*   **Extensibility**: The system is designed to be extensible. Tools in the Grove ecosystem can define their own top-level keys in `grove.yml`. These custom sections are captured in the `Extensions` field of the `Config` struct and can be unmarshaled into a tool-specific struct using the `UnmarshalExtension` method.

*   **Core Structs**: The `config/types.go` file defines the primary configuration structs, such as `Config`, `ServiceConfig`, and `Settings`, which represent the standard sections of a `grove.yml` file.

## The `logging` Package

This package provides a centralized, structured logging facility for the entire Grove ecosystem. It is built on top of `logrus` and integrates with the `config` package.

*   **`NewLogger`**: The main function used to create a logger instance. It takes a `component` string, which is used to tag all log messages originating from that logger. All loggers share a single underlying configuration.

    ```go
    import "github.com/mattsolo1/grove-core/logging"

    log := logging.NewLogger("my-component")
    log.Info("This is an informational message.")
    ```

*   **Configuration**: The logger's behavior (level, output file, format) is configured via the `logging` section in `grove.yml`. This allows for unified log management across all tools that use `grove-core`.

## The `errors` Package

The `errors` package provides a structured error handling system. It allows for the creation of errors with specific codes and detailed, machine-readable context.

*   **`GroveError`**: The custom error type that includes an `ErrorCode`, a human-readable `Message`, and a map of `Details`. This allows errors to be inspected programmatically.

*   **`ErrorCode`**: A set of predefined constants (e.g., `ErrCodeConfigNotFound`, `ErrCodeServiceNotFound`) that represent specific, known error conditions.

*   **Constructors**: The package provides constructor functions like `errors.ServiceNotFound()` to create `GroveError` instances consistently. This is beneficial because the `cli.ErrorHandler` can recognize these specific error codes and provide tailored, helpful messages to the user.

    ```go
    import "github.com/mattsolo1/grove-core/errors"

    func findService(name string) error {
        if !exists(name) {
            return errors.ServiceNotFound(name)
        }
        return nil
    }
    ```

## The `command` Package

The `command` package offers a way to build and execute external commands with an emphasis on security and reliability.

*   **`SafeBuilder`**: The central component of the package. It is used to construct commands (`exec.Cmd`) while providing validation for arguments. It helps prevent common security issues like command injection by validating inputs against predefined patterns (e.g., for Git references, service names, and file names). It also applies a default execution timeout to prevent processes from hanging indefinitely.

## The `git` Package

The `git` package contains a collection of utilities for interacting with Git repositories from Go code.

Its main features are:

*   **Worktree Management (`WorktreeManager`)**: Provides functions to programmatically list, create, and remove Git worktrees. This is useful for orchestrating tasks that require isolated environments on different branches.
*   **Hook Installation (`HookManager`)**: Manages the installation and uninstallation of Git hooks (e.g., `post-checkout`, `pre-commit`) that can trigger actions within Grove tools, enabling tighter integration with the developer's Git workflow.
*   **Status Checking (`GetStatus`)**: A function to retrieve a detailed `StatusInfo` struct for a repository, including the current branch, ahead/behind counts relative to upstream, and the number of modified, staged, and untracked files.

## The `pkg/tmux` Package

This package provides a programmatic Go client for managing `tmux` sessions. Its primary purpose is to allow tools to create, control, and interact with terminal sessions, panes, and windows automatically. This is particularly useful for setting up complex development environments or running processes in managed, observable terminal sessions.

## The `pkg/models` Package

The `pkg/models` package defines a set of shared data structures that are used across different tools in the Grove ecosystem. These models provide a consistent representation for core concepts. Key models include:

*   **`Session`**: Represents a development or automation session.
*   **`ToolExecution`**: Represents a single execution of a tool.
*   **`Event`**: Represents a generic event that occurs within the system.