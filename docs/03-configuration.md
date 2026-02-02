## Grove Configuration

This section details the core configuration properties found in `grove.yml`. This schema defines the structure for the Grove Ecosystem configuration, controlling project discovery, workspace definitions, and global settings.

| Property | Description |
| :--- | :--- |
| `version` | (string, required) <br> Defines the configuration version schema being used (e.g., '1.0'). This ensures compatibility with the installed version of the Grove CLI tools and validates the file structure. |
| `name` | (string, optional) <br> Specifies the name of the project or ecosystem. This is used for display purposes in the terminal UI, logs, and window titles to identify the current context. |
| `workspaces` | (array of strings, optional) <br> A list of directory patterns (glob patterns) identifying where workspace directories are located within this ecosystem. This is the primary mechanism for defining the scope of a Grove Ecosystem. |
| `groves` | (object, optional) <br> Defines root directories that the discovery service should scan to find projects and ecosystems. Unlike `workspaces` which look for projects *inside* the current ecosystem, `groves` defines roots for *other* ecosystems or standalone projects to be included in the context. |
| `explicit_projects` | (array of objects, optional) <br> Allows you to manually define specific projects to include in the Grove context without relying on automatic discovery. See **Explicit Project Item** below for details. |
| `notebooks` | (object, optional) <br> Configuration settings for the notebook integration, allowing you to define multiple notebook definitions and rules for their usage. See **Notebooks Configuration** below for details. |
| `context` | (object, optional) <br> Configuration for the `cx` (context) tool, specifically regarding repository management. See **Context Configuration** below. |
| `tui` | (object, optional) <br> Settings controlling the appearance and behavior of the Terminal User Interface (TUI). See **TUI Configuration** below. |
| `build_cmd` | (string, optional, default: make build) <br> Specifies a custom shell command to run when building projects within this ecosystem. This overrides the default behavior if your project requires a specific build chain. |
| `build_after` | (array of strings, optional) <br> A list of project identifiers that must be built successfully before the current project is built. This establishes a dependency graph for the build process. |

```toml
version = "1.0"
name = "my-ecosystem"
workspaces = ["packages/*", "apps/*"]
build_cmd = "go build ./..."
```

### Explicit Project Item

Structure for items within the `explicit_projects` array.

| Property | Description |
| :--- | :--- |
| `path` | (string, required) <br> The absolute or relative file system path to the project directory. |
| `name` | (string, optional) <br> A display name for the project. If omitted, the directory name will be used. |
| `description` | (string, optional) <br> A human-readable description of the project, used for documentation or UI tooltips. |
| `enabled` | (boolean, required) <br> Toggles whether this explicit project is currently active and visible in the Grove context. |

```toml
[[explicit_projects]]
  path = "~/legacy/old-app"
  name = "Legacy App"
  enabled = true
```

### Notebooks Configuration

Settings nested under the `notebooks` key.

| Property | Description |
| :--- | :--- |
| `definitions` | (object, optional) <br> A map defining specific notebook configurations keyed by name. Each value in this map follows the structure defined in the **Notebook Options** section. |
| `rules` | (object, optional) <br> Defines rules for applying notebook definitions. |

**Rules Object:**

| Property | Description |
| :--- | :--- |
| `default` | (string, optional) <br> Specifies the name of the notebook definition from `definitions` to use as the default active notebook. |
| `global` | (object, optional) <br> Configuration for the system-wide global notebook. Contains a `root_dir` (string, required) property specifying the absolute path to the global notebook root. |

```toml
[notebooks.rules]
  default = "engineering"
  [notebooks.rules.global]
    root_dir = "~/.grove/global-notes"
```

### Context Configuration

Settings nested under the `context` key.

| Property | Description |
| :--- | :--- |
| `repos_dir` | (string, optional, default: ~/.grove/cx) <br> Specifies the directory where the `cx repo` command stores bare repositories. Change this if you need to store cloned contexts in a non-standard location. |

```toml
[context]
  repos_dir = "/mnt/data/grove/repos"
```

### TUI Configuration

Settings nested under the `tui` key.

| Property | Description |
| :--- | :--- |
| `theme` | (string, optional) <br> Sets the color theme for the terminal interface. Valid options typically include 'kanagawa'. |
| `icons` | (string, optional) <br> Controls the icon set used in the UI. Options are 'nerd' (requires a Nerd Font) or 'ascii' (text-based fallbacks). |
| `nvim_embed` | (object, optional) <br> Configuration for the embedded Neovim component. Contains a `user_config` (boolean, required) property to toggle loading user's personal nvim config. |

```toml
[tui]
  theme = "kanagawa"
  icons = "nerd"
  [tui.nvim_embed]
    user_config = true
```

## Notebook Options

These settings configure the `notebook` extension, typically found in `grove.yml` or a dedicated notebook configuration file. They control how and where notes, plans, and other documentation artifacts are stored and generated.

| Property | Description |
| :--- | :--- |
| `root_dir` | (string, optional) <br> The absolute path to the notebook root directory. If specified, this enables "Centralized Mode," creating a unified knowledge base separate from project source code. |
| `types` | (object, optional) <br> Allows definition of custom note types (e.g., 'meeting', 'incident'). This object maps type names to their specific configurations. |
| `sync` | (any, optional) <br> Configuration settings for synchronizing this notebook with external services (e.g., GitHub Issues). |
| `notes_path_template` | (string, optional) <br> Defines the path structure for standard notes. Supports variables to dynamically organize notes by date, type, or project. |
| `plans_path_template` | (string, optional) <br> Defines the path structure for 'plans' (implementation guides). |
| `chats_path_template` | (string, optional) <br> Defines the path structure for saving LLM chat transcripts. |
| `templates_path_template` | (string, optional) <br> Defines the location where reusable note templates are stored. |
| `recipes_path_template` | (string, optional) <br> Defines the location for 'recipes' (pre-defined workflows or scaffoldings). |
| `in_progress_path_template` | (string, optional) <br> Defines the directory for active or in-flight tasks and notes. |
| `completed_path_template` | (string, optional) <br> Defines the archive directory for finished tasks and notes. |
| `prompts_path_template` | (string, optional) <br> Defines the storage location for custom system prompts used by documentation generation tools. |

```toml
root_dir = "~/.grove/notebooks"
notes_path_template = "workspaces/{{ .Workspace.Name }}/{{ .NoteType }}"
plans_path_template = "workspaces/{{ .Workspace.Name }}/plans"
```

## Logging Options

These settings configure the `logging` extension in `grove.yml`. They control the verbosity, formatting, and output destinations for Grove CLI tool logs.

| Property | Description |
| :--- | :--- |
| `level` | (string, optional, default: info) <br> Sets the global logging verbosity. Common values include `debug`, `info`, `warn`, and `error`. |
| `report_caller` | (boolean, optional, default: true) <br> When enabled, log entries will include the filename and line number of the code that generated the log message. |
| `log_startup` | (boolean, optional) <br> Controls whether a standard startup banner or initialization message is written to the logs when the tool begins execution. |
| `show_current_project` | (boolean, optional) <br> If set to true, logs originating from the currently active project context will always be shown, overriding other filtering rules defined in `component_filtering`. |
| `groups` | (object, optional) <br> Allows defining named groups of components. These groups can then be referenced in the `component_filtering` section to manage visibility for multiple components at once. |
| `file` | (object, optional) <br> Configuration for writing logs to disk. See **File Logging** below. |
| `format` | (object, optional) <br> Configuration for the log output format. See **Log Formatting** below. |
| `component_filtering` | (object, optional) <br> Rules for filtering logs based on the source component. See **Component Filtering** below. |

```toml
[logging]
  level = "debug"
  show_current_project = true
  [logging.groups]
    backend = ["api", "db", "auth"]
```

### File Logging

Configuration for the file output sink.

| Property | Description |
| :--- | :--- |
| `enabled` | (boolean, required, default: true) <br> Toggles writing logs to a file. |
| `path` | (string, required) <br> The absolute or relative filesystem path where the log file should be created. |
| `format` | (string, optional, default: json) <br> The format of the log file content. Usually `json` for machine parsing or `text` for human readability. |

```toml
[logging.file]
  enabled = true
  path = "./.grove/logs/session.log"
  format = "json"
```

### Log Formatting

Configuration for how logs appear in the console.

| Property | Description |
| :--- | :--- |
| `preset` | (string, required) <br> A named formatting preset (e.g., `default`, `simple`) that applies a collection of styling rules. |
| `disable_timestamp` | (boolean, required) <br> If true, timestamps are omitted from the console output, producing cleaner output for simple CLI interactions. |
| `disable_component` | (boolean, required) <br> If true, the name of the component generating the log is omitted from the output. |
| `structured_to_stderr` | (string, required) <br> Controls if and how structured logs are emitted to Standard Error. Useful for separating human-readable output (stdout) from machine logs. |

```toml
[logging.format]
  preset = "default"
  disable_timestamp = false
  structured_to_stderr = "auto"
```

### Component Filtering

Detailed control over which system components emit logs.

| Property | Description |
| :--- | :--- |
| `only` | (array of strings, optional) <br> Strict allowlist. If populated, **only** logs from these components will be displayed. All others are suppressed. |
| `show` | (array of strings, optional) <br> Ensures logs from these components are displayed, overriding any general hiding rules (like `hide`). |
| `hide` | (array of strings, optional) <br> Suppresses logs from these specific components. |

```toml
[logging.component_filtering]
  only = ["api", "db"]
  hide = ["cache-layer"]
```