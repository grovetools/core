# Grove Core

Grove Core is the shared library for the Grove ecosystem, providing standardized CLI patterns and utilities for all Grove tools.

## Installation

This is a library package and should be imported by other Grove tools:

```go
import "github.com/yourorg/grove-core/cli"
```

## Key Features

- **Standardized CLI Commands**: Use `cli.NewStandardCommand()` to create commands with consistent flags
- **Unified Logging**: Use `cli.GetLogger()` for consistent logging across all tools
- **Configuration Loading**: Use `config.LoadGroveConfig()` to load grove.yml files

## Standard Flags

All Grove tools using this library will have these flags:
- `--verbose, -v`: Enable verbose logging
- `--json`: Output in JSON format
- `--config, -c`: Path to grove.yml config file

## Version

Current version: v0.1.0