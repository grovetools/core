<!-- DOCGEN:OVERVIEW:START -->

`grove-core` is the foundational Go library for the Grove ecosystem. It provides shared infrastructure for configuration management, workspace discovery, logging, and terminal user interface (TUI) components, ensuring consistency across all Grove CLI tools.

## Core Mechanisms

**Layered Configuration**: Configuration is loaded and merged from multiple sources in a specific precedence order:
1.  **Global**: `~/.config/grove/grove.yml` (System-wide defaults and search paths).
2.  **Ecosystem**: `grove.yml` in a parent directory defining a workspace boundary.
3.  **Project**: `grove.yml` in the current working directory.
4.  **Overrides**: `grove.override.yml` for local, git-ignored developer settings.

**Workspace Discovery**: The `DiscoveryService` scans directories defined in the `groves` configuration. It classifies filesystem locations into three types based on file markers:
*   **Ecosystems**: Directories containing a `grove.yml` with a `workspaces` key.
*   **Projects**: Directories containing a `grove.yml` or `.git`.
*   **Worktrees**: Git worktrees located in `.grove-worktrees/` directories.

**Unified Logging**: The logging system writes two streams simultaneously:
*   **Structured**: JSON-formatted logs written to `.grove/logs/` in the workspace root for machine analysis.
*   **Human-Readable**: Styled, colored text written to `stderr` for interactive use.
*   **Filtering**: Supports component-based filtering rules defined in `grove.yml`.

## Packages & Features

### Application Infrastructure
*   **`cli`**: Wraps `spf13/cobra` to provide standard flags (`--json`, `--verbose`, `--config`) and styled help output across all tools.
*   **`config`**: Handles YAML parsing, environment variable expansion (`${VAR}`), and JSON schema validation.
*   **`logging`**: A wrapper around `logrus` providing the unified logging streams and component registry.

### System Integration
*   **`pkg/tmux`**: A client for controlling `tmux` servers. Manages sessions, windows, and panes via the CLI or socket. Supports socket isolation for testing.
*   **`git`**: Wrappers for git operations, specifically focusing on worktree management and status retrieval.
*   **`command`**: A safe command executor that validates arguments to prevent injection and handles timeouts.

### TUI Components
The `tui` package provides reusable [Bubble Tea](https://github.com/charmbracelet/bubbletea) components:
*   **`navigator`**: A list-based browser for selecting projects or files.
*   **`logviewer`**: A component for tailing files and streaming logs with filtering capabilities.
*   **`jsontree`**: An interactive viewer for exploring structured JSON data.
*   **`theme`**: Centralized color palette and style definitions (Kanagawa, Gruvbox).

## The `core` Debugging Tool

While primarily a library, this repository compiles to a `core` binary used for debugging the ecosystem state.

*   **`core ws list`**: JSON output of the full discovery tree. Used by `nav` to populate the project list.
*   **`core config-layers`**: Prints the merged configuration and the source file for each value.
*   **`core logs`**: Aggregates and streams logs from `.grove/logs/`.
*   **`core nvim-demo`**: Demonstrates the embedded Neovim component integration.

<!-- DOCGEN:OVERVIEW:END -->


<!-- DOCGEN:TOC:START -->

See the [documentation](docs/) for detailed usage instructions:
- [Overview](docs/01-overview.md)

<!-- DOCGEN:TOC:END -->
