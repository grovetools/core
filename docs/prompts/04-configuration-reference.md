# Documentation Task: Configuration Reference

## Task
Create a comprehensive reference guide for the `grove.yml` configuration file. Document every top-level key and all their nested fields.

## Source of Truth
The primary source for this documentation is `config/types.go` (defining `Config`, `ServiceConfig`, `AgentConfig`, `Settings`, etc.) and `logging/config.go` (defining `logging.Config`).

## Structure
Create a section for each top-level key:
- `version`
- `services` (document all fields of `ServiceConfig`)
- `networks`
- `volumes`
- `profiles`
- `settings` (document all fields of `Settings`)
- `agent` (document all fields of `AgentConfig`)
- `logging` (document all fields of `logging.Config`)
- Custom Extensions (explain the `Extensions` field and `UnmarshalExtension`).

## Output Format
Use nested headings and tables or definition lists to clearly describe each configuration key, its type, purpose, and default value if applicable.