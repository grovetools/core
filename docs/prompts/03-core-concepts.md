# Documentation Task: Core Concepts

## Task
Document the fundamental concepts and packages of the `grove-core` library. For each package, explain its purpose and highlight its most important functions and types.

## Packages to Cover
- **`cli`**: The foundation for building CLI tools. Explain `NewStandardCommand`, `GetLogger`, `GetOptions`, and the role of the `ErrorHandler`.
- **`config`**: The hierarchical configuration system. Explain the loading order (global, project, override), the structure of `grove.yml`, and the concept of extensibility via the `Extensions` field. Mention `LoadDefault()` as the main entrypoint. Reference `config/types.go` for the core structs.
- **`logging`**: The centralized logging package. Explain how `NewLogger` works, that it's based on `logrus`, and how it's configured by the `logging` section in `grove.yml`.
- **`errors`**: The structured error handling system. Explain `GroveError`, `ErrorCode`, and the benefits of using constructors like `errors.ServiceNotFound`.
- **`command`**: The secure command execution builder. Explain the purpose of `SafeBuilder`.
- **`git`**: The Git utility package. Describe its main features: worktree management (`WorktreeManager`), hook installation (`HookManager`), and status checking (`GetStatus`).
- **`pkg/tmux`**: The programmatic tmux client. Explain its purpose for managing tmux sessions.
- **`pkg/models`**: The shared data models. Briefly explain that these models (`Session`, `Tool`, `Event`) are used across the Grove ecosystem.

## Output Format
Use a main heading for each package (e.g., `## The cli Package`). Provide short code snippets to illustrate key functions.