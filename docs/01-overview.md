# Grove Core

`grove-core` is the foundational Go library for the Grove ecosystem. It provides a shared set of packages, patterns, and utilities that ensure consistency and reduce boilerplate code across all Grove command-line tools.

## Provided Packages and Features

### `cli`
A framework for building standardized command-line interfaces using the Cobra library. It includes helpers for:
*   **Command Creation**: `NewStandardCommand` scaffolds a new command with common flags like `--verbose`, `--json`, and `--config`.
*   **Configuration & Logging**: Automatically initializes configuration and sets up the logger based on command-line flags.
*   **Error Handling**: Provides a structured error handler to present user-friendly messages for common issues.
*   **Version Commands**: A standard constructor `NewVersionCommand` for creating version commands that display build information.
*   **Documentation Endpoint**: A helper `NewDocsCommand` to expose structured documentation from a tool as a JSON object, useful for MCP servers.

### `config`
A system for managing `grove.yml` configuration files. Its features include:
*   **Hierarchical Loading**: Merges configuration from global (`~/.config/grove`), project (`grove.yml`), and local override (`grove.override.yml`) files.
*   **Schema and Validation**: Defines the `grove.yml` structure and validates configurations against a JSON schema and semantic rules (e.g., checking for port conflicts).
*   **Extensibility**: Allows other Grove tools to define and access their own top-level sections within `grove.yml` via an `Extensions` map.
*   **Environment Variable Expansion**: Supports `${VAR}` syntax in configuration files.

### `logging`
A centralized logging infrastructure built on Logrus. It provides:
*   **Component-Based Logging**: `NewLogger("component-name")` creates a logger that automatically tags messages with the component's name.
*   **Configuration-Driven**: Log level, format (text or JSON), and file output are configured globally in `grove.yml`.
*   **Custom Formatting**: Includes a custom text formatter designed for readability in development environments.
*   **Structured Logging**: Encourages structured logging with fields for machine-readable output.

### `command`
A utility for securely building and executing external commands.
*   **Safe Command Building**: The `SafeBuilder` validates command arguments against common patterns (e.g., filenames, Git references) to mitigate command injection risks.
*   **Context and Timeouts**: Integrates with Go's `context` package to manage command timeouts and cancellation.

### `git`
Provides a set of tools for interacting with Git repositories.
*   **Worktree Management**: A `WorktreeManager` to create, list, and remove Git worktrees for isolated development environments.
*   **Repository Information**: Functions to get the current branch, repository root, remote URL, and detailed status (`ahead`/`behind` counts, dirty state).
*   **Hook Management**: An interface for installing and uninstalling Git hooks (`post-checkout`, `pre-commit`) to integrate with development workflows.
*   **Environment Variables**: Gathers Git context (repo name, branch, commit) and makes it available as environment variables.

### `pkg/tmux`
A Go client for programmatically managing tmux sessions.
*   **Session Management**: Functions to create, check for existence, kill, and list tmux sessions.
*   **Pane and Window Control**: APIs to launch complex layouts with multiple panes, send commands, and capture pane content.
*   **Monitoring**: A function to block until a specified tmux session is closed.

### `pkg/models`
A collection of shared data structures used across multiple Grove tools, particularly `grove-flow` and other orchestration services. This ensures data consistency for concepts like:
*   Sessions
*   Events
*   Tool Executions
*   Transcripts and Messages
*   Notifications

### Other Utilities
*   **`conventional`**: Tools for parsing Conventional Commit messages and generating changelogs.
*   **`util/sanitize`**: A suite of functions for sanitizing strings for use as Docker labels, domain names, filenames, and environment variable keys.
*   **`testutil`**: Helper functions for writing integration tests, including utilities for initializing Git repositories and managing Docker resources.
*   **`version`**: A standard mechanism for embedding build information (version, commit, build date) into binaries at compile time via linker flags.

## Role in the Grove Ecosystem

`grove-core` is the bedrock of the Grove ecosystem. Every executable tool, including `grove-meta`, `grove-context`, `grove-flow`, and `grove-docgen`, is built using the packages it provides. This dependency ensures that all tools share a consistent approach to:
*   **Command-line interaction** and flag parsing.
*   **Configuration file loading** and validation.
*   **Logging output** and structure.
*   **Error reporting** and handling.

By providing this common foundation, `grove-core` significantly accelerates the development of new tools while maintaining a cohesive user experience across the entire suite.

## Installation

Grove-core is a Go library. Add it to your project:
```bash
go get github.com/mattsolo1/grove-core
```

Import in your Go code:
```go
import "github.com/mattsolo1/grove-core/cli"
```

See the [Grove Development Guide](https://github.com/mattsolo1/grove-meta/blob/main/docs/02-installation.md#building-from-source) for development setup.
