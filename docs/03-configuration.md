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
| `theme` | (string, optional) <br> Sets the color theme for the terminal interfaces. Accepts a theme family ('ayu', 'catppuccin', 'floraverse', 'github', 'gruvbox', 'kanagawa', 'nord', 'onedark', 'oxocarbon', 'terminal', 'tokyonight') or a specific variant such as 'catppuccin-mocha', 'tokyonight-storm', or 'github-light-high-contrast'. Family names resolve to the family's default variant and adapt to light/dark terminal backgrounds when the family ships both. The complete list of valid names is generated into the JSON schema from the embedded theme registry. |
| `icons` | (string, optional) <br> Controls the icon set used in the UI. Options are 'nerd' (requires a Nerd Font) or 'ascii' (text-based fallbacks). |
| `nvim_embed` | (object, optional) <br> Configuration for the embedded Neovim component. Contains a `user_config` (boolean, required) property to toggle loading user's personal nvim config. |
| `plugins` | (object, optional) <br> Process-based plugin panels: a map of plugin name to command definition. Each entry gets its own PTY pane on the treemux icon rail. See **TUI Plugins** below. |

```toml
[tui]
  theme = "kanagawa"
  icons = "nerd"
  [tui.nvim_embed]
    user_config = true
```

#### TUI Plugins

Settings nested under `tui.plugins.<name>`. The map key is the plugin name: it
labels the rail item and identifies the pane in the persisted layout
(`plugin-<name>`). Consumed by treemux.

| Property | Description |
| :--- | :--- |
| `command` | (string, required) <br> The executable to run in the pane's PTY. |
| `args` | (array of strings, optional) <br> Arguments passed to the command. |
| `icon` | (string, optional) <br> Nerd Font glyph shown in the icon rail. Defaults to a sparkle. |
| `cwd` | (string, optional) <br> Working directory for the command; `~` is expanded. Defaults to the user's home directory. |
| `env` | (array of strings, optional) <br> Extra environment entries in `KEY=VALUE` form, layered on top of the inherited environment. |
| `restart` | (boolean, optional, default: false) <br> Respawn the command automatically when it exits. Without it the pane shows an exited banner and waits for `r`. |
| `position` | (string, optional, default: rail) <br> Where the pane lives. `rail` (a persistent icon-rail pane) is the only supported value; for a panel spawned on a key chord use `[tui.panels.bindings]`, which carries the chord. |

The plugin set is read from the whole config cascade — global, the
`~/.config/grove/plugins/*.toml` fragments, and workspace files — and is
reconciled into the saved layout on every start and on every config reload, so
adding or removing an entry takes effect without resetting the layout.

```toml
[tui.plugins.btop]
  command = "btop"
  icon = ""
  restart = true

[tui.plugins.lazygit]
  command = "lazygit"
  args = ["--use-config-file", "~/.config/lazygit/config.yml"]
  cwd = "~/src/grove"
```

### Daemon event hooks

`[[daemon.hooks.on_event]]` runs a shell command when the daemon broadcasts a
matching lifecycle event — a job finishing, a note changing, a build
completing. It is the exec-side subscription to the same event bus
`/api/stream` exposes; for the full event vocabulary and the streaming
alternative, see the daemon's
[Reacting to grove events](../../daemon/docs/reacting-to-grove-events.md)
guide.

| Property | Description |
| :--- | :--- |
| `events` | (array of strings, required) <br> Event types that trigger the hook. Glob patterns are allowed, so `job_*` catches the whole job lifecycle. An empty list never fires. |
| `filter` | (string, optional) <br> Narrows matches by event field. Terms are `field=glob` pairs, ANDed: `workspace=grove*`, `plan=extensib* status=failed`. Known fields are `workspace`, `plan`, `job_id`, `status`, `source`, `origin`. A bare term with no `=` is a substring match against workspace, plan and job id. |
| `name` | (string, optional) <br> Label used in logs and as the dedupe/cancel key. Defaults to the command. |
| `command` | (string, required) <br> Shell command, run via `sh -c`. |
| `timeout` | (integer, optional, default: 30) <br> Seconds before the hook is killed. |
| `cancel_previous` | (boolean, optional, default: false) <br> SIGTERM an in-flight run of the same hook when a new event fires, instead of running both. |
| `disable_env` | (string, optional) <br> Skip the hook while the named environment variable is non-empty. |
| `enable_env` | (string, optional) <br> Skip the hook unless the named environment variable is non-empty. |

`run_if` is a skill-sync concept and is ignored here: an event already asserts
that something changed.

The event arrives two ways — as JSON on the hook's stdin, and as `GROVE_*`
environment variables (`GROVE_EVENT_TYPE`, `GROVE_EVENT_SEQ`,
`GROVE_EVENT_SOURCE`, `GROVE_JOB_ID`, `GROVE_WORKSPACE`, `GROVE_PLAN`,
`GROVE_JOB_STATUS`, `GROVE_EVENT_ORIGIN`).

```toml
[[daemon.hooks.on_event]]
  name    = "desktop-notify"
  events  = ["job_completed", "job_failed"]
  filter  = "workspace=grove*"
  command = 'notify-send "grove" "$GROVE_JOB_ID $GROVE_EVENT_TYPE"'
  timeout = 30

[[daemon.hooks.on_event]]
  name       = "reindex"
  events     = ["note_event"]
  command    = "my-indexer --stdin"
  enable_env = "GROVE_INDEXER"
```

Terminal job events are deduplicated per hook by job id: the daemon
synthesizes a terminal event from every federated snapshot that shows the
transition, so without dedupe a satellite job would notify repeatedly.

## Security: the exec-config trust gate

Grove's config cascade merges `grove.toml` files that come out of cloned
repositories — the ecosystem, project-notebook, project, and project-local
override layers. Several config keys carry shell commands grove or one of its
satellites executes, so honoring those unconditionally would mean that cloning
a repository and starting an agent session inside it is enough to give the
repo's author code execution on your machine.

Grove gates them by **provenance**. Values from layers you control are always
honored: `~/.config/grove/grove.toml`, its `*.toml` fragments,
`~/.config/grove/plugins/*.toml`, the global override, and
`GROVE_CONFIG_OVERLAY`. Values from repo-controlled layers are quarantined —
stripped before the merge, so no consumer ever sees them — until you trust
that config file.

Exec-bearing keys are classified by how their command comes to run:

| Risk | Keys | Default policy |
| :--- | :--- | :--- |
| **implicit** — runs without you asking | `[[hooks.on_stop]]`, `[[daemon.hooks.on_skill_sync]]`, `[[daemon.hooks.on_event]]`, `[tui.plugins.*]`, `[tui.panels]` `command`, `[tui.panels.bindings.*]`, `[keys.tmux.popups.*]`, `[keys.shell] bindings`, `[keys.nvim.bindings.*] command`, `<provider>.api_key_command`, `[notifications.home_assistant]` `token_command`/`webhook_secret_command`, `notebooks.definitions.*.sync.token_command` | quarantined |
| **explicit** — runs because you invoked the verb | `build_cmd`, `commands`, `[environment]`/`[environments.*]` `provider`/`command`/`commands`, `flow.recipes.get_recipe_cmd`, `satellites.*.provision.gh_token_cmd`/`claude_token_cmd` | reported, honored |

Review and trust a workspace with:

```bash
grove config trust           # show exactly what would be enabled
grove config trust --yes     # trust the config files in scope
grove config trust --revoke  # withdraw trust
grove config trust --list    # list every trusted config file
```

Trust is recorded per config **file** together with a digest of the exec
values you reviewed (`~/.local/state/grove/exec-trust.json`). If the repo
later adds or edits a command, the digest no longer matches and the gate
re-closes — you are asked about the new content rather than inheriting a
decision you made about different content.

| Property | Description |
| :--- | :--- |
| `security.exec_trust` | (string, optional, default: `default`) <br> Enforcement policy. `default` quarantines implicit-risk values from untrusted layers and reports explicit-risk ones. `strict` quarantines every exec-bearing value from untrusted layers. `warn` never strips and only reports. `off` disables the gate entirely. Overridden by the `GROVE_EXEC_TRUST` environment variable. |

```toml
[security]
exec_trust = "strict"
```

`security.exec_trust` is only ever read from the layers you control — a
workspace `grove.toml` setting `exec_trust = "off"` is inert, because
otherwise the hostile file could disable the gate that contains it.

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