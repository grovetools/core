Generate a comprehensive overview for grove-core that enumerates all the capabilities it provides.

## Requirements
Create a single-page overview that thoroughly documents what grove-core provides to the Grove ecosystem:

1. **High-level description**: What grove-core is and its purpose as the foundational Go library
2. **Complete enumeration of packages and features**: List ALL packages and their purposes
3. **Key capabilities provided**:
   - CLI framework (Cobra integration)
   - Configuration management (grove.yml)
   - Logging infrastructure
   - Error handling patterns
   - File system utilities
   - Process management
   - Workspace detection
   - Binary linking/management
   - Testing utilities
4. **Ecosystem role**: How every Grove tool depends on grove-core
5. **Installation**: Include brief installation instructions at the bottom

## Installation Format
Include this condensed installation section at the bottom:

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

## Context
Grove-core provides the foundational patterns and utilities that enable consistency, reduce boilerplate, and provide shared functionality across all Grove ecosystem tools.