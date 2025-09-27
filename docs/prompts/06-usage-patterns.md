# Documentation Task: Usage Patterns & Examples

## Task
Document common usage patterns and practical recipes for `grove-core`, showing advanced or specific use cases.

## Patterns to Document
1.  **Defining Custom Extensions**: Show how a tool can define its own configuration struct and use `config.UnmarshalExtension` to parse a custom top-level key from `grove.yml`.
2.  **Hierarchical Configuration**: Provide a concrete example with three files (`~/.config/grove/grove.yml`, `./grove.yml`, `./grove.override.yml`) to demonstrate how settings are merged.
3.  **Managing Git Worktrees**: Show a sequence of calls to `git.WorktreeManager` to list, create, and remove a worktree.
4.  **Pretty Console Output**: Explain the purpose of `logging.PrettyLogger` for user-facing messages vs. `logging.NewLogger` for structured logs.
5.  **Custom Error Handling**: Show how to use the `cli.ErrorHandler` to display user-friendly messages for specific `GroveError` codes.

## Output Format
Use clear headings for each pattern. Provide descriptive text and focused code snippets for each example.