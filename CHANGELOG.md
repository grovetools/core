## v0.4.1-nightly.516efdf (2025-10-03)

## v0.4.0 (2025-10-01)

This release introduces a centralized TUI toolkit to create a consistent look and feel across all Grove command-line tools. The new `tui` package includes a foundational theme system based on the Kanagawa Dragon color palette (cccf82e, 45a4811), a standardized keymap with vim-style navigation (aec0078), and reusable components for rendering help menus (d71c14f), tables, and other UI elements (a8dc6e1). As part of this effort, existing help menus have been refactored to use the new centralized component, reducing code duplication and improving consistency.

Workspace management has been consolidated into a new `pkg/workspace` core library (4f846c0). This package provides primitives for preparing Git worktrees, managing submodules, and generating Go workspace files, creating a reusable foundation for tools like `grove-flow`.

Documentation generation has been improved with support for automatic Table of Contents creation and other configuration updates (920fff8, 3e7ce4e). The documentation content itself has also been made more succinct and focused (50e1c94, 9126d18, b5ae13a).

### Features

- Add centralized TUI foundation for consistent ecosystem styling (aec0078)
- Implement centralized TUI toolkit for ecosystem consistency (a8dc6e1)
- Enhance theme package with Colors struct for easier access (45a4811)
- Update theme to use Kanagawa Dragon color palette (cccf82e)
- Implement consistent help menus across Grove TUI ecosystem (d71c14f)
- Consolidate workspace management primitives into a new `pkg/workspace` (4f846c0)
- Add Table of Contents generation and other docgen configuration updates (920fff8)
- Update and improve documentation content for brevity and clarity (50e1c94, 9126d18, 4dcf49e)

### Bug Fixes

- Add changelog parsing to the release workflow (d20e6e3)

### Refactoring

- Standardize docgen.config.yml key order and settings (0d0ceb9)

### Documentation

- Add initial documentation structure and templates (d0e1e3b, 3e7ce4e)
- Rename Introduction sections to Overview for consistency (5855b14)
- Simplify grove-core documentation to a single overview page (b5ae13a)
- Update docgen configuration and overview prompt (fb31a10)

### File Changes

```
 .github/workflows/release.yml      |  20 +-
 Makefile                           |   8 +-
 README.md                          |  90 ++++++--
 docs/01-overview.md                |  76 +++++++
 docs/README.md.tpl                 |   7 +
 docs/docgen.config.yml             |  23 ++
 docs/docs.rules                    |   1 +
 docs/prompts/01-overview.md        |  30 +++
 git/worktree.go                    |   9 +-
 go.mod                             |  17 +-
 go.sum                             |  27 ++-
 logging/pretty.go                  |  98 +++++++-
 pkg/docs/docs.json                 |  26 +++
 pkg/workspace/go_workspace.go      | 179 +++++++++++++++
 pkg/workspace/go_workspace_test.go | 202 +++++++++++++++++
 pkg/workspace/prepare.go           |  38 ++++
 pkg/workspace/prepare_test.go      | 214 ++++++++++++++++++
 pkg/workspace/submodules.go        | 223 ++++++++++++++++++
 pkg/workspace/submodules_test.go   | 447 +++++++++++++++++++++++++++++++++++++
 pkg/workspace/types.go             |  24 ++
 tui/components/components.go       | 264 ++++++++++++++++++++++
 tui/components/help/help.go        | 280 +++++++++++++++++++++++
 tui/components/table/table.go      | 258 +++++++++++++++++++++
 tui/keymap/keymap.go               | 285 +++++++++++++++++++++++
 tui/theme/theme.go                 | 270 ++++++++++++++++++++++
 25 files changed, 3075 insertions(+), 41 deletions(-)
```

## v0.3.0 (2025-09-26)

This release introduces a major overhaul of the logging system across the Grove ecosystem. A new centralized logging package provides configurable, structured logging with support for both console and file outputs (5d3178d). This is complemented by a pretty logging wrapper that enables simultaneous user-friendly terminal output and machine-readable file logging (721fcca, fc3a772). Logging has been enhanced to support commands like `grove logs` with improved caller information, version metadata, and JSON file output capabilities (fe09ca4). As part of this effort, CLI logging was updated to use this new core system (4d23dcd), and terminal output now features magenta highlighting for component names for better readability (064a845).

Another significant addition is a new, centralized tmux client package designed for programmatic session management (09372bf). This client includes utilities for consistent session naming, improved error handling (0150b36), and the ability to query the cursor position to support advanced TUI testing (e711670).

To improve developer experience and standardization, a reusable `docs` command has been added, allowing any Grove tool to easily serve its documentation in a standard JSON format (a5ba628). Finally, a new GitHub Action workflow has been implemented to automate the release process (297490b).

### Features

* Add GitHub Action workflow for automated releases (297490b)
* Add magenta highlighting for component names in terminal output (064a845)
* Enhance logging with structured file output and improved caller info (fe09ca4)
* Add a pretty logging wrapper for simultaneous structured and user-friendly console output (721fcca)
* Add a centralized, configurable logging package for the Grove ecosystem (5d3178d)
* Add a reusable 'docs' command for standardized JSON documentation output (a5ba628)
* Add GetCursorPosition method to the tmux client for TUI testing (e711670)
* Add centralized tmux utilities and improve error handling (0150b36)
* Add a comprehensive, centralized client package for managing tmux sessions (09372bf)

### Bug Fixes

* Improve CLI logging to use the new centralized structured logging system (4d23dcd)

### Code Refactoring

* Decouple pretty and structured logging with intelligent terminal output control (fc3a772)

### Chores

* Update .gitignore to track CLAUDE.md and ignore go.work files (6964d36)

### File Changes

```
 .github/workflows/release.yml |  39 ++++++
 .gitignore                    |   7 ++
 cli/command.go                |   7 +-
 cli/docs.go                   |  22 ++++
 examples/logging-demo/main.go |  62 ++++++++++
 go.mod                        |  14 +++
 go.sum                        |  34 ++++++
 logging/README.md             | 156 ++++++++++++++++++++++++
 logging/config.go             |  39 ++++++
 logging/example_test.go       |  69 +++++++++++
 logging/formatter.go          |  60 ++++++++++
 logging/logger.go             | 273 ++++++++++++++++++++++++++++++++++++++++++
 logging/logger_test.go        | 210 ++++++++++++++++++++++++++++++++
 logging/pretty.go             | 130 ++++++++++++++++++++
 pkg/tmux/client.go            |  41 +++++++
 pkg/tmux/doc.go               |  30 +++++
 pkg/tmux/launch.go            |  79 ++++++++++++
 pkg/tmux/launch_test.go       | 159 ++++++++++++++++++++++++
 pkg/tmux/monitor.go           |  26 ++++
 pkg/tmux/monitor_test.go      | 137 +++++++++++++++++++++
 pkg/tmux/session.go           | 118 ++++++++++++++++++
 pkg/tmux/session_test.go      | 127 ++++++++++++++++++++
 pkg/tmux/types.go             |  14 +++
 pkg/tmux/util.go              |  40 +++++++
 24 files changed, 1892 insertions(+), 1 deletion(-)
```

## v0.2.13 (2025-09-12)

### Code Refactoring

* remove Docker dependencies from grove-core

## v0.2.12 (2025-08-25)

### Bug Fixes

* checks if branchPrefix is empty before constructing the branch name, avoiding the creation of branch names with leading slashes

## v0.2.11 (2025-08-15)

### Features

* support flow jobs in Session model

### Bug Fixes

* use claude as home dir in agent docker image

## v0.2.10 (2025-08-12)

### Features

* add canopy models to pkg

## v0.2.9 (2025-08-08)

### Chores

* update Docker image references from grovepm to mattsolo1

## v0.2.8 (2025-08-08)

### Features

* **conventional:** add conventional commit parsing and changelog generation

