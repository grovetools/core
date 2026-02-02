# CLI Reference

Complete command reference for `core`.

## core

<div class="terminal">
<span class="term-bold term-fg-9">CORE</span>
 <span class="term-italic">Core libraries and debugging tools for the Grove ecosystem</span>

 <span class="term-italic term-fg-9">USAGE</span>
 core [command]

 <span class="term-italic term-fg-9">COMMANDS</span>
 <span class="term-bold term-fg-6">completion</span>      Generate the autocompletion script for the specified shell
 <span class="term-bold term-fg-6">config-layers</span>   Display the layered configuration for the current context
 <span class="term-bold term-fg-6">editor</span>          Open a file or directory in the dedicated editor window
 <span class="term-bold term-fg-6">logs</span>            Aggregate and display logs from Grove workspaces
 <span class="term-bold term-fg-6">nvim-demo</span>       Demo of embedded Neovim component
 <span class="term-bold term-fg-6">open-in-window</span>  Runs a command in a dedicated, focused tmux window
 <span class="term-bold term-fg-6">paths</span>           Print the XDG-compliant paths used by Grove
 <span class="term-bold term-fg-6">tmux</span>            Tmux window management commands
 <span class="term-bold term-fg-6">version</span>         Print the version information for this binary
 <span class="term-bold term-fg-6">worktrees</span>       Manage git worktrees across workspaces
 <span class="term-bold term-fg-6">ws</span>              Navigate and explore Grove workspaces

 <span class="term-dim">Flags: -c/--config, -h/--help, --json, -v/--verbose</span>

 Use "core [command] --help" for more information.
</div>

### core ws

<div class="terminal">
<span class="term-bold term-fg-9">CORE WS</span>
 <span class="term-italic">Navigate and explore Grove workspaces</span>

 This command launches an interactive TUI to navigate and
 explore all workspaces
 discovered by Grove based on your configuration. It
 provides a hierarchical view
 of ecosystems, projects, and worktrees.

 <span class="term-italic term-fg-9">USAGE</span>
 core ws [flags]
 core ws [command]

 <span class="term-italic term-fg-9">COMMANDS</span>
 <span class="term-bold term-fg-6">cwd</span>  Get workspace information for current working directory

 <span class="term-dim">Flags: -c/--config, -h/--help, --json, -v/--verbose</span>

 Use "core ws [command] --help" for more information.
</div>

#### core ws cwd

<div class="terminal">
<span class="term-bold term-fg-9">CORE WS CWD</span>
 <span class="term-italic">Get workspace information for current working directory</span>

 Get the workspace information for the current working
 directory.
 This command uses GetProjectByPath to find the workspace
 containing the current directory.

 <span class="term-italic term-fg-9">USAGE</span>
 core ws cwd [flags]

 <span class="term-italic term-fg-9">FLAGS</span>
 <span class="term-fg-13">-c, --config</span>   Path to grove.yml config file
 <span class="term-fg-13">-h, --help</span>     help for cwd
 <span class="term-fg-13">    --json</span>     Output workspace in JSON format
 <span class="term-fg-13">-v, --verbose</span>  Enable verbose logging
</div>

### core worktrees

<div class="terminal">
<span class="term-bold term-fg-9">CORE WORKTREES</span>
 <span class="term-italic">Manage git worktrees across workspaces</span>

 Manage and view git worktrees across all workspaces in the
 ecosystem.

 <span class="term-italic term-fg-9">USAGE</span>
 core worktrees [command]

 <span class="term-italic term-fg-9">COMMANDS</span>
 <span class="term-bold term-fg-6">list</span>  Show git worktrees for all workspaces

 <span class="term-dim">Flags: -c/--config, -h/--help, --json, -v/--verbose</span>

 Use "core worktrees [command] --help" for more information.
</div>

#### core worktrees list

<div class="terminal">
<span class="term-bold term-fg-9">CORE WORKTREES LIST</span>
 <span class="term-italic">Show git worktrees for all workspaces</span>

 Display git worktrees for each workspace in the ecosystem
 with their status.
 Only shows workspaces that have additional worktrees
 beyond the main one.

 <span class="term-italic term-fg-9">USAGE</span>
 core worktrees list [flags]

 <span class="term-italic term-fg-9">FLAGS</span>
 <span class="term-fg-13">-c, --config</span>   Path to grove.yml config file
 <span class="term-fg-13">-h, --help</span>     help for list
 <span class="term-fg-13">    --json</span>     Output in JSON format
 <span class="term-fg-13">-v, --verbose</span>  Enable verbose logging
</div>

### core paths

<div class="terminal">
<span class="term-bold term-fg-9">CORE PATHS</span>
 <span class="term-italic">Print the XDG-compliant paths used by Grove</span>

 Print the XDG-compliant paths used by Grove.
 
 This command outputs the paths in JSON format by default,
 making it easy
 to parse from scripts and other tools.
 
 The paths follow the XDG Base Directory Specification:
 - config_dir: Configuration files (grove.yml)
 - data_dir: Persistent data (binaries, plugins, notebooks)
 - state_dir: Runtime state (databases, logs, sessions)
 - cache_dir: Temporary/regenerable data
 - bin_dir: Grove binaries (subdirectory of data_dir)

 <span class="term-italic term-fg-9">USAGE</span>
 core paths [flags]

 <span class="term-italic term-fg-9">FLAGS</span>
 <span class="term-fg-13">-h, --help</span>  help for paths
</div>

### core logs

<div class="terminal">
<span class="term-bold term-fg-9">CORE LOGS</span>
 <span class="term-italic">Aggregate and display logs from Grove workspaces</span>

 Streams logs from one or more workspaces. By default,
 shows logs from the
 current workspace only. Use --ecosystem to show logs from
 all workspaces.

 <span class="term-italic term-fg-9">USAGE</span>
 core logs [flags]

 <span class="term-italic term-fg-9">FLAGS</span>
 <span class="term-fg-13">    --also-show</span>    Temporarily show components/groups, overriding hide rules
 <span class="term-fg-13">    --compact</span>      Disable spacing between log entries (for pretty/full/rich formats)
 <span class="term-fg-13">    --component</span>    Show logs only from these components (acts as a strict whitelist)
 <span class="term-fg-13">    --ecosystem</span>    Show logs from all workspaces in the ecosystem
 <span class="term-fg-13">-f, --follow</span>       Follow log output
 <span class="term-fg-13">    --format</span>       Output format:<span class="term-dim"> (default: text)</span>
                       <span class="term-dim">• text</span>
                       <span class="term-dim">• json</span>
                       <span class="term-dim">• full</span>
                       <span class="term-dim">• rich</span>
                       <span class="term-dim">• pretty</span>
                       <span class="term-dim">• pretty-text</span>
 <span class="term-fg-13">-h, --help</span>         help for logs
 <span class="term-fg-13">    --ignore-hide</span>  Temporarily show components/groups that would be hidden by config
 <span class="term-fg-13">    --json</span>         Output logs in JSON Lines format (shorthand for --format=json)
 <span class="term-fg-13">    --show-all</span>     Show all logs, ignoring any configured show/hide rules
 <span class="term-fg-13">    --tail</span>         Number of lines to show from the end of the logs (default: all)<span class="term-dim"> (default: -1)</span>
 <span class="term-fg-13">-i, --tui</span>          Launch the interactive TUI
 <span class="term-fg-13">-w, --workspaces</span>   Filter by specific workspace names (comma-separated)

 <span class="term-italic term-fg-9">EXAMPLES</span>
 <span class="term-dim"># Follow logs from current workspace</span>
   <span class="term-fg-10">core</span> <span class="term-fg-6">logs</span> <span class="term-fg-13">-f</span>

 <span class="term-dim"># Show logs from all workspaces in ecosystem</span>
   <span class="term-fg-10">core</span> <span class="term-fg-6">logs</span> <span class="term-fg-13">--ecosystem</span> <span class="term-fg-13">-f</span>

 <span class="term-dim"># Get the last 100 log lines in JSON format</span>
   <span class="term-fg-10">core</span> <span class="term-fg-6">logs</span> <span class="term-fg-13">--tail</span> 100 <span class="term-fg-13">--json</span>

 <span class="term-dim"># Follow logs from specific workspaces</span>
   <span class="term-fg-10">core</span> <span class="term-fg-6">logs</span> <span class="term-fg-13">-f</span> <span class="term-fg-13">-w</span> my-project,another-project

 <span class="term-dim"># Show only the pretty CLI output (styled)</span>
   <span class="term-fg-10">core</span> <span class="term-fg-6">logs</span> <span class="term-fg-13">--format=pretty</span>

 <span class="term-dim"># Show only the pretty CLI output (plain text, no ANSI)</span>
   <span class="term-fg-10">core</span> <span class="term-fg-6">logs</span> <span class="term-fg-13">--format=pretty-text</span>

 <span class="term-dim"># Show full details with pretty output indented below each line</span>
   <span class="term-fg-10">core</span> <span class="term-fg-6">logs</span> <span class="term-fg-13">--format=full</span>
</div>

### core editor

<div class="terminal">
<span class="term-bold term-fg-9">CORE EDITOR</span>
 <span class="term-italic">Open a file or directory in the dedicated editor window</span>

 Finds or creates a tmux window (default name "editor",
 index 1) and opens the specified file or current
 directory. By default, if the window exists, it is
 focused. New flags allow customizing the editor command,
 window name/index, and forcing a reset of the window.

 <span class="term-italic term-fg-9">USAGE</span>
 core editor [file] [flags]

 <span class="term-italic term-fg-9">FLAGS</span>
 <span class="term-fg-13">    --cmd</span>           Custom editor command to execute. The file path will be appended if provided. Defaults to $EDITOR or 'nvim'.
 <span class="term-fg-13">-h, --help</span>          help for editor
 <span class="term-fg-13">    --reset</span>         If the editor window exists, kill it and start a fresh session.
 <span class="term-fg-13">    --window-index</span>  Index (position) for the editor window.<span class="term-dim"> (default: 1)</span>
 <span class="term-fg-13">    --window-name</span>   Name of the target tmux window.<span class="term-dim"> (default: editor)</span>
</div>

### core tmux

<div class="terminal">
<span class="term-bold term-fg-9">CORE TMUX</span>
 <span class="term-italic">Tmux window management commands</span>

 Commands for managing core tools in dedicated tmux
 windows.

 <span class="term-italic term-fg-9">USAGE</span>
 core tmux [command]

 <span class="term-italic term-fg-9">COMMANDS</span>
 <span class="term-bold term-fg-6">editor</span>  Open a file or directory in the dedicated editor window

 <span class="term-dim">Flags: -h/--help</span>

 Use "core tmux [command] --help" for more information.
</div>

#### core tmux editor

<div class="terminal">
<span class="term-bold term-fg-9">CORE TMUX EDITOR</span>
 <span class="term-italic">Open a file or directory in the dedicated editor window</span>

 Finds or creates a tmux window (default name "editor",
 index 1) and opens the specified file or current
 directory. By default, if the window exists, it is
 focused. New flags allow customizing the editor command,
 window name/index, and forcing a reset of the window.

 <span class="term-italic term-fg-9">USAGE</span>
 core tmux editor [file] [flags]

 <span class="term-italic term-fg-9">FLAGS</span>
 <span class="term-fg-13">    --cmd</span>           Custom editor command to execute. The file path will be appended if provided. Defaults to $EDITOR or 'nvim'.
 <span class="term-fg-13">-h, --help</span>          help for editor
 <span class="term-fg-13">    --reset</span>         If the editor window exists, kill it and start a fresh session.
 <span class="term-fg-13">    --vim-cmd</span>       Vim command to execute. If editor is already running, sends as :command. Otherwise starts with -c flag.
 <span class="term-fg-13">    --window-index</span>  Index (position) for the editor window. -1 means no positioning.<span class="term-dim"> (default: -1)</span>
 <span class="term-fg-13">    --window-name</span>   Name of the target tmux window.<span class="term-dim"> (default: editor)</span>
</div>

### core version

<div class="terminal">
<span class="term-bold term-fg-9">CORE VERSION</span>
 <span class="term-italic">Print the version information for this binary</span>

 <span class="term-italic term-fg-9">USAGE</span>
 core version [flags]

 <span class="term-italic term-fg-9">FLAGS</span>
 <span class="term-fg-13">-h, --help</span>  help for version
 <span class="term-fg-13">    --json</span>  Output version information in JSON format
</div>

