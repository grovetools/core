# Overview

`grove-core` is the foundational Go library for the Grove ecosystem. It provides a collection of shared packages for building consistent and interoperable command-line developer tools. Its purpose is to enforce consistency and reduce boilerplate across the suite of modular, binary-based tools that Grove promotes.

The philosophy of the Grove ecosystem is that a collection of small, focused, and swappable CLI tools for orchestrating AI-coding. These tools benefit a common module for configuration, logging, git, worktree, and file operations. 

## Key Features

The library is organized into several key packages, each addressing a specific aspect of application development:

*   **`cli`**: A framework for building command-line applications using the `cobra` library. It provides helpers like `cli.NewStandardCommand` to create commands with a consistent set of flags (e.g., `--verbose`, `--json`, `--config`), ensuring a uniform interface for all Grove tools.

*   **`config`**: A hierarchical configuration management system designed to load and merge `grove.yml` files. It supports a three-tiered loading order (global, project-level, and local override), environment variable expansion, and is extensible, allowing individual tools to define their own custom configuration sections within a shared `grove.yml` file.

*   **`logging`**: A centralized package for structured logging based on `logrus`. It enables consistent log formatting, component-based tagging, and multiple output sinks (console and file), all configurable from the `grove.yml` file.

*   **`errors`**: A system for structured error handling. It defines a set of standard error codes and a custom error type, `GroveError`, which allows tools to return detailed, machine-readable errors that can be translated into user-friendly messages by the `cli` package's error handler.

*   **`git`**: A collection of utilities for interacting with Git repositories. It includes functionality for managing Git worktrees, installing and managing Git hooks, and querying repository status.

*   **`pkg/tmux`**: A client for programmatically creating and managing `tmux` sessions. This is used by Grove tools to orchestrate complex development environments with multiple panes and pre-configured commands.

