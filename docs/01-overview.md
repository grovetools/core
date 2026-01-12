`grove-core` is a Go library that provides shared packages and utilities for tools in the Grove ecosystem.

## Role in the Ecosystem

All executable tools in the Grove ecosystem import `grove-core` as a Go module. This provides a consistent implementation for command-line interaction, configuration file loading, logging, and error handling, which reduces duplicated code.

## How It Works

A Grove tool uses the `cli` package to create a `cobra` command with standard flags (`--config`, `--json`, `--verbose`). The `config` package then loads and merges `grove.yml` files from a series of locations: a global directory, an optional parent ecosystem directory, the current project directory, and local override files. The `logging` package uses this configuration to set up a logger. Functions like `logging.NewLogger("component")` create log entries tagged with a component name, which are then written to standard output, standard error, or a file as configured.

## Packages

### cli
Provides functions for building command-line interfaces with `cobra`.
*   `NewStandardCommand` creates a command with a standard set of persistent flags (`--verbose`, `--json`, `--config`).
*   `GetLogger` and `GetOptions` initialize a logger and retrieve common options from command-line flags.
*   An error handler presents messages for predefined error types.
*   `NewVersionCommand` creates a command that displays build information.

### command
Executes external commands with validation.
*   A `SafeBuilder` creates `exec.Cmd` instances and can validate arguments against predefined patterns for filenames or Git references.
*   Integrates with Go's `context` package for command timeouts and cancellation.

### config
Manages `grove.yml` configuration files.
*   Loads and merges configuration from multiple sources in order: global, ecosystem, project, and override files.
*   Captures non-standard top-level keys in an `Extensions` map for use by other tools.
*   Replaces `${VAR}` or `${VAR:-default}` syntax with environment variable values.
*   Validates `grove.yml` against an embedded JSON schema.

### conventional
Parses Conventional Commit messages.
*   Parses commit messages into a structured format.
*   Generates changelogs from a list of commits.

### errors
Defines a structured error type for the ecosystem.
*   `GroveError` contains a machine-readable `ErrorCode` (e.g., `CONFIG_NOT_FOUND`), a message, and a map of contextual details.
*   Provides constructor functions for common error conditions such as `ServiceNotFound` or `ConfigInvalid`.

### fs
Provides filesystem utilities.
*   Contains functions to copy single files and recursively copy directories.

### git
Interacts with Git repositories by executing `git` commands.
*   Functions to get repository information such as current branch, repository root, remote URL, and status.
*   A `WorktreeManager` to create, list, and remove Git worktrees.
*   An interface for installing and uninstalling Git hooks.

### logging
Provides a centralized logging system built on Logrus.
*   `NewLogger("component-name")` creates a logger that tags messages with a component name.
*   Log level, format (text or JSON), and file output are configured in `grove.yml`.
*   The `UnifiedLogger` writes a log event to both a structured format for machine processing and a styled format for terminal output.
*   Provides functions to show or hide log output from specific components based on configuration rules.

### pkg/alias
Resolves workspace aliases to absolute file paths.
*   Translates project aliases like `"ecosystem:repo:worktree"` into an absolute path.
*   Translates notebook resource aliases that are prefixed with `nb:`.

### pkg/models
Contains shared data structures for Grove services.
*   Includes types for sessions, events, tool executions, transcripts, and notifications.

### pkg/process
Provides process utilities.
*   Includes a function `IsProcessAlive` to check if a process with a given PID is running.

### pkg/profiling
Provides performance profiling utilities for command-line applications.
*   Integrates with Cobra commands to add flags for enabling CPU and memory profiling.
*   Includes a hierarchical timer for measuring the duration of nested function calls.

### pkg/repo
Manages local clones of remote git repositories.
*   Clones repositories as bare repos and checks out specific versions into worktrees.
*   Maintains a manifest of managed repositories and their worktrees.

### pkg/sessions
Manages metadata for live, stateful sessions.
*   A filesystem-based registry tracks running processes and their associated session metadata.

### pkg/tmux
A Go client for managing tmux sessions, windows, and panes.
*   Functions to create, check for existence, kill, and list tmux sessions.
*   Functions to create windows, split panes, send keys, and capture pane content.
*   A function to block execution until a specified tmux session closes.

### pkg/workspace
Discovers and classifies Grove workspaces on the filesystem.
*   `DiscoveryService` scans configured directories to find ecosystems, projects, and worktrees.
*   Classifies entities based on `grove.yml` content and filesystem markers, assigning a `WorkspaceKind`.
*   `GetProjectByPath` finds the containing workspace for a given directory path without performing a full scan.
*   The `Provider` holds a snapshot of discovered workspaces for in-memory lookups.

### schema
Manages JSON schema for `grove.yml` configuration files.
*   Contains an embedded base schema for core `grove.yml` properties.
*   Provides a validator to check configuration data against the schema.
*   Includes a manifest of extension schemas used for composing a unified schema.

### starship
Provides commands for integration with the Starship shell prompt.
*   An `install` command modifies `starship.toml` to add a custom module.
*   A `status` command, used by the prompt, queries registered providers for status strings to display.

### state
Manages local workspace state.
*   Loads from and saves key-value data to a `.grove/state.yml` file in the current workspace.

### tui
Provides reusable components for building terminal user interfaces with Bubble Tea.
*   `navigator`: A component for browsing projects and files.
*   `logviewer`: A component for tailing and viewing log files.
*   `jsontree`: A component for interactively exploring JSON data.
*   `help`: A component for displaying keybindings.
*   `theme`: A theming system with color palettes and icons.
*   `keymap`: A standardized set of keybindings.

### util/pathutil
Provides path expansion and normalization utilities.
*   Expands home directory (`~`), environment variables, and git variables in path strings.
*   Normalizes paths for case-insensitive filesystem comparisons by resolving symlinks and converting to lowercase on macOS and Windows.

### util/sanitize
Sanitizes strings for various technical contexts.
*   Functions to format strings for use as Docker labels, domain names, filenames, and environment variables.

### version
Embeds build information into binaries.
*   Contains variables that are populated by Go linker flags for version, commit, branch, and build date.

### Installation

Grove-core is a Go library. Add it to your project:
```bash
go get github.com/mattsolo1/grove-core
```

Import in your Go code:
```go
import "github.com/mattsolo1/grove-core/cli"
```

See the [Grove Development Guide](https://github.com/mattsolo1/grove-meta/blob/main/docs/02-installation.md#building-from-source) for development setup.