## v0.5.0 (2026-01-14)

This release introduces a complete overhaul of the logging system, a new interactive logs TUI, a reusable embedded Neovim component, and major improvements to the workspace discovery and path resolution systems. The new unified logging API provides a chainable builder pattern for creating both structured and pretty-printed log output from a single call, with extensive configuration options for component-based filtering (dbc1670, c455796). The new `core logs` command leverages this system to provide a powerful tool for viewing logs across an entire ecosystem, complete with a full-featured TUI for interactive browsing, searching, and filtering (ce02c65, 769ec18).

Workspace management has been significantly enhanced with the introduction of a centralized `NotebookLocator` for consistent, configuration-driven path resolution for notes, plans, and chats across both local and centralized notebook structures (f6ded98, d9a4582). Path handling is now more robust with new utilities that correctly normalize for case-sensitivity on different operating systems (6ea4031, dfed5b8), and a new utility prevents bugs related to deleted worktrees (210604e, ed647c8). The alias resolution system has also been centralized into `grove-core` and expanded to natively support notebook resources (9c81658, 8574644).

For developers, this release adds a `build_after` field to `grove.yml` for defining build dependencies (b77e7bb), a reusable embedded Neovim TUI component (3a07cca), and a comprehensive set of over 100 theme icons with Nerd Font and ASCII fallbacks to create richer and more informative TUIs across the ecosystem (bccd627, 081b068, 667bf5a).

### Features

- Add unified logging API with a chainable builder pattern for structured and pretty output (dbc1670)
- Implement `core logs` command with text, JSON, and interactive TUI modes (ce02c65)
- Add extensive component-based log filtering with CLI overrides and filter statistics (c455796)
- Add reusable embedded Neovim TUI component with a demo command (3a07cca)
- Add centralized `NotebookLocator` for consistent notebook, plan, and chat path resolution (f6ded98)
- Add `IsZombieWorktree` utility to prevent bugs from deleted worktrees (210604e)
- Add `CanonicalPath` function for case-correct path normalization on macOS (6ea4031)
- Move and enhance alias resolver from `grove-context` to `grove-core` for shared use (9c81658)
- Add native support for notebook resource aliases (e.g., `@a:nb:notebook:path`) (8574644)
- Add `build_after` field to `grove.yml` for defining build dependency order (b77e7bb)
- Add comprehensive set of over 100 theme icons with Nerd Font and ASCII fallbacks (bccd627)
- Add support for user-configurable note types with TUI metadata (d9a4582)
- Add global writer for dynamic TUI output redirection, fixing output mangling (0dce554)
- Add workspace-aware path resolution for log files (63767f9)
- Add reusable scrollbar utility package for TUI components (d506f4a)
- Add `StreamWriter` utility for streaming command output to TUIs without splitting lines (abeacbe)
- Add support for global override configuration file (`~/.config/grove/grove.override.yml`) (8b2bb9d)
- Add `core config-layers` command to diagnose configuration merging (0c3e40f)
- Add session registry for tracking live agent sessions from `grove-hooks` (13d2214)

### Bug Fixes

- Fix path normalization to correctly handle filesystem case on macOS (dfed5b8)
- Prevent recreation of "zombie" worktree directories by long-running loggers (ed647c8, de8429f)
- Fix `FindEcosystemConfig` to correctly recognize ecosystem worktrees (7489d3d)
- Prevent logviewer deadlock when using `StreamWriter` for TUI output (b89a451)
- Require `nb:` prefix for notebook aliases to resolve ambiguity with project paths (bf00107)
- Fix TUI log filtering to respect CLI override flags (3cf7532)
- Fix logviewer scrollbar calculation to account for text wrapping (e52a77b)
- Make default log filtering show all logs unless explicitly configured (466153c)
- Fix `tmux` commands to use exact session name matching to prevent ambiguity (8651e9b)
- Fix `core ws list` to correctly handle ecosystem worktree contents in focus mode (4b7b0db)

### Miscellaneous

- Standardize CI release workflows and disable CI runs on branches to reduce cost (bba4be4)
- Migrate `docgen` prompts to the notebook workspace (a61df2d)

### File Changes

```
 .cx/core.rules                              |    3 +
 .cx/dev-no-tests.rules                      |   19 +
 .cx/dev-with-tests.rules                    |   16 +
 .cx/docs-only.rules                         |    8 +
 .cx/pkg-only.rules                          |    1 +
 .cx/workspace-only.rules                    |    1 +
 .github/workflows/ci.yml                    |   52 ++
 .github/workflows/release.yml               |   38 +-
 .gitignore                                  |   56 +-
 CLAUDE.md                                   |   30 +
 Makefile                                    |  102 +-
 README.md                                   |    4 +-
 cmd/config.go                               |   60 ++
 cmd/core/main.go                            |   29 +
 cmd/editor.go                               |   78 ++
 cmd/logs.go                                 |  732 +++++++++++++++
 cmd/logs_tui.go                             | 1334 +++++++++++++++++++++++++++
 cmd/nvim_demo.go                            |  342 +++++++
 cmd/open_in_window.go                       |   51 +
 cmd/tmux.go                                 |   18 +
 cmd/tmux_editor.go                          |  147 +++
 cmd/version.go                              |   36 +
 cmd/ws.go                                   |  122 +++
 command/builder.go                          |   29 +-
 command/executor.go                         |   31 +
 config/config.go                            |  340 ++++---
 config/config_test.go                       |   77 ++
 config/merge.go                             |  108 ++-
 config/schema.go                            |   72 +-
 config/schema/definitions/base.schema.json  |  211 +++++
 config/types.go                             |  271 +++++-
 config/validator.go                         |  207 ----
 docs/01-overview.md                         |  170 +++-
 docs/docgen.config.yml                      |   35 +-
 docs/prompts/01-overview.md                 |   30 -
 fs/utils.go                                 |   63 +-
 git/extended_status.go                      |   91 ++
 git/status.go                               |   54 +-
 git/utils.go                                |   24 +-
 git/worktree.go                             |   27 +-
 go.mod                                      |   25 +-
 go.sum                                      |   53 +-
 grove.yml                                   |   12 +-
 logging.schema.json                         |  109 +++
 logging/config.go                           |   90 +-
 logging/context_writer.go                   |   24 +
 logging/formatter.go                        |    8 +-
 logging/global_writer.go                    |   43 +
 logging/logger.go                           |  268 +++++-
 logging/logger_test.go                      |  635 ++++++++++++-
 logging/pretty.go                           |  188 ++--
 logging/unified.go                          |  318 +++++++
 logging/unified_test.go                     |  414 +++++++++
 logging/zombie_writer.go                    |  110 +++
 notebook.schema.json                        |   71 ++
 pkg/alias/notebook.go                       |   17 +
 pkg/alias/resolver.go                       |  425 +++++++++
 pkg/alias/resolver_test.go                  |  143 +++
 pkg/docs/docs.json                          |   14 +-
 pkg/logging/logutil/finder.go               |   87 ++
 pkg/models/session.go                       |    5 +
 pkg/process/process.go                      |   31 +
 pkg/profiling/cobra.go                      |   79 ++
 pkg/profiling/timer.go                      |  142 +++
 pkg/repo/manager.go                         |  440 +++++++--
 pkg/sessions/metadata.go                    |   22 +
 pkg/sessions/registry.go                    |  132 +++
 pkg/tmux/client.go                          |   47 +
 pkg/tmux/launch.go                          |   42 +
 pkg/tmux/session.go                         |  284 +++++-
 pkg/tmux/tui.go                             |  258 ++++++
 pkg/tmux/types.go                           |   21 +
 pkg/workspace/discover.go                   |  582 ++++++++----
 pkg/workspace/discover_test.go              |    8 +-
 pkg/workspace/enrich.go                     |  222 -----
 pkg/workspace/filter/filter.go              |  115 ++-
 pkg/workspace/history.go                    |  127 +++
 pkg/workspace/identifier.go                 |   73 +-
 pkg/workspace/identifier_test.go            |   72 +-
 pkg/workspace/lookup.go                     |  448 +++++++--
 pkg/workspace/lookup_test.go                |  323 ++++++-
 pkg/workspace/notebook_locator.go           |  777 ++++++++++++++++
 pkg/workspace/notebook_locator_test.go      |  116 +++
 pkg/workspace/notebook_resolver.go          |  316 +++++++
 pkg/workspace/prepare.go                    |   83 +-
 pkg/workspace/prepare_test.go               |   51 +-
 pkg/workspace/provider.go                   |  245 +++++
 pkg/workspace/provider_test.go              |  183 ++++
 pkg/workspace/submodules.go                 |  142 +--
 pkg/workspace/submodules_test.go            |   95 +-
 pkg/workspace/transform.go                  |  484 ++++++++--
 pkg/workspace/transform_test.go             |  291 ++++++
 pkg/workspace/types.go                      |  322 ++++++-
 pkg/workspace/utils.go                      |   19 +
 pkg/workspace/zombie.go                     |   68 ++
 schema/definitions/base.schema.json         |  113 +++
 schema/grove.embedded.schema.json           |  113 +++
 schema/manifest.go                          |   18 +
 schema/validator.go                         |   71 ++
 starship/command.go                         |  158 ++++
 starship/provider.go                        |   30 +
 state/state.go                              |  158 ++++
 state/state_test.go                         |  167 ++++
 tests/e2e/main.go                           |   82 ++
 tests/e2e/scenarios_basic.go                |   42 +
 tests/e2e/scenarios_config.go               |  503 ++++++++++
 tests/e2e/scenarios_log_creation.go         |  526 +++++++++++
 tests/e2e/scenarios_logging.go              | 1031 +++++++++++++++++++++
 tests/e2e/scenarios_logs_filtering.go       |  240 +++++
 tests/e2e/scenarios_logs_tui.go             | 1000 ++++++++++++++++++++
 tests/e2e/scenarios_logs_tui_filtering.go   |  186 ++++
 tests/e2e/scenarios_logs_tui_sorting.go     |  508 ++++++++++
 tests/e2e/scenarios_notebooks_debug.go      |  272 ++++++
 tests/e2e/scenarios_workspace.go            |  226 +++++
 tests/e2e/scenarios_workspace_advanced.go   |  481 ++++++++++
 tests/e2e/scenarios_workspace_edge_cases.go |  513 ++++++++++
 tests/e2e/scenarios_zombie_worktrees.go     |  202 ++++
 tests/e2e/test_utils.go                     |   25 +
 tools/logging-schema-generator/main.go      |   37 +
 tools/notebook-schema-generator/main.go     |   37 +
 tools/schema-composer/main.go               |  163 ++++
 tools/schema-generator/main.go              |   30 +
 tui/components/components.go                |    2 +-
 tui/components/help/help.go                 |  264 ++++--
 tui/components/jsontree/keymap.go           |  113 +++
 tui/components/jsontree/model.go            |  969 +++++++++++++++++++
 tui/components/logviewer/logviewer.go       |  311 +++++++
 tui/components/logviewer/writer.go          |   62 ++
 tui/components/navigator/io.go              |    9 +-
 tui/components/navigator/keymap.go          |    6 +-
 tui/components/navigator/model.go           |  105 ++-
 tui/components/nvim/model.go                |  303 ++++++
 tui/components/nvim/nvim.go                 |  538 +++++++++++
 tui/components/table/table.go               |   87 +-
 tui/keymap/keymap.go                        |    1 -
 tui/theme/icons.go                          |  777 ++++++++++++++++
 tui/theme/theme.go                          |  553 ++++++++---
 tui/utils/scrollbar/scrollbar.go            |  100 ++
 tui/wsnav/io.go                             |   14 +
 tui/wsnav/model.go                          |   68 ++
 tui/wsnav/update.go                         |   42 +
 tui/wsnav/util.go                           |   39 +
 tui/wsnav/view.go                           |  241 +++++
 tui/wsnav/wsnav.go                          |   60 ++
 util/pathutil/normalize.go                  |  119 +++
 145 files changed, 24669 insertions(+), 1884 deletions(-)
```

## v0.4.2 (2026-01-14)

This release introduces a complete overhaul of the logging system, a new interactive logs TUI, a reusable embedded Neovim component, and major improvements to the workspace discovery and path resolution systems. The new unified logging API provides a chainable builder pattern for creating both structured and pretty-printed log output from a single call, with extensive configuration options for component-based filtering (dbc1670, c455796). The new `core logs` command leverages this system to provide a powerful tool for viewing logs across an entire ecosystem, complete with a full-featured TUI for interactive browsing, searching, and filtering (ce02c65, 769ec18).

Workspace management has been significantly enhanced with the introduction of a centralized `NotebookLocator` for consistent, configuration-driven path resolution for notes, plans, and chats across both local and centralized notebook structures (f6ded98, d9a4582). Path handling is now more robust with new utilities that correctly normalize for case-sensitivity on different operating systems (6ea4031, dfed5b8), and a new `IsZombieWorktree` utility prevents bugs related to deleted worktrees (210604e, ed647c8). The alias resolution system has also been centralized into `grove-core` and expanded to natively support notebook resources (9c81658, 8574644).

For developers, this release adds a `build_after` field to `grove.yml` for defining build dependencies (b77e7bb), a reusable embedded Neovim TUI component (3a07cca), and a comprehensive set of over 100 theme icons with Nerd Font and ASCII fallbacks to create richer and more informative TUIs across the ecosystem (bccd627, 081b068, 667bf5a).

### Features

- Add unified logging API with a chainable builder pattern for structured and pretty output (dbc1670)
- Implement `core logs` command with text, JSON, and interactive TUI modes (ce02c65)
- Add extensive component-based log filtering with CLI overrides and filter statistics (c455796)
- Add reusable embedded Neovim TUI component with a demo command (3a07cca)
- Add centralized `NotebookLocator` for consistent notebook, plan, and chat path resolution (f6ded98)
- Add `IsZombieWorktree` utility to prevent bugs from deleted worktrees (210604e)
- Add `CanonicalPath` function for case-correct path normalization on macOS (6ea4031)
- Move and enhance alias resolver from `grove-context` to `grove-core` for shared use (9c81658)
- Add native support for notebook resource aliases (e.g., `@a:nb:notebook:path`) (8574644)
- Add `build_after` field to `grove.yml` for defining build dependency order (b77e7bb)
- Add comprehensive set of over 100 theme icons with Nerd Font and ASCII fallbacks (bccd627)
- Add support for user-configurable note types with TUI metadata (d9a4582)
- Add global writer for dynamic TUI output redirection, fixing output mangling (0dce554)
- Add workspace-aware path resolution for log files (63767f9)
- Add reusable scrollbar utility package for TUI components (d506f4a)
- Add `StreamWriter` utility for streaming command output to TUIs without splitting lines (abeacbe)
- Add support for global override configuration file (`~/.config/grove/grove.override.yml`) (8b2bb9d)
- Add `core config-layers` command to diagnose configuration merging (0c3e40f)
- Add session registry for tracking live agent sessions from `grove-hooks` (13d2214)

### Bug Fixes

- Fix path normalization to correctly handle filesystem case on macOS (dfed5b8)
- Prevent recreation of "zombie" worktree directories by long-running loggers (ed647c8, de8429f)
- Fix `FindEcosystemConfig` to correctly recognize ecosystem worktrees (7489d3d)
- Prevent logviewer deadlock when using `StreamWriter` for TUI output (b89a451)
- Require `nb:` prefix for notebook aliases to resolve ambiguity with project paths (bf00107)
- Fix TUI log filtering to respect CLI override flags (3cf7532)
- Fix logviewer scrollbar calculation to account for text wrapping (e52a77b)
- Make default log filtering show all logs unless explicitly configured (466153c)
- Fix `tmux` commands to use exact session name matching to prevent ambiguity (8651e9b)
- Fix `core ws list` to correctly handle ecosystem worktree contents in focus mode (4b7b0db)

### Miscellaneous

- Standardize CI release workflows and disable CI runs on branches to reduce cost (bba4be4)
- Migrate `docgen` prompts to the notebook workspace (a61df2d)

### File Changes

```
 .cx/core.rules                              |    3 +
 .cx/dev-no-tests.rules                      |   19 +
 .cx/dev-with-tests.rules                    |   16 +
 .cx/docs-only.rules                         |    8 +
 .cx/pkg-only.rules                          |    1 +
 .cx/workspace-only.rules                    |    1 +
 .github/workflows/ci.yml                    |   52 ++
 .github/workflows/release.yml               |   38 +-
 .gitignore                                  |   56 +-
 CLAUDE.md                                   |   30 +
 Makefile                                    |  102 +-
 README.md                                   |    4 +-
 cmd/config.go                               |   60 ++
 cmd/core/main.go                            |   29 +
 cmd/editor.go                               |   78 ++
 cmd/logs.go                                 |  732 +++++++++++++++
 cmd/logs_tui.go                             | 1334 +++++++++++++++++++++++++++
 cmd/nvim_demo.go                            |  342 +++++++
 cmd/open_in_window.go                       |   51 +
 cmd/tmux.go                                 |   18 +
 cmd/tmux_editor.go                          |  147 +++
 cmd/version.go                              |   36 +
 cmd/ws.go                                   |  122 +++
 command/builder.go                          |   29 +-
 command/executor.go                         |   31 +
 config/config.go                            |  340 ++++---
 config/config_test.go                       |   77 ++
 config/merge.go                             |  108 ++-
 config/schema.go                            |   72 +-
 config/schema/definitions/base.schema.json  |  211 +++++
 config/types.go                             |  271 +++++-
 config/validator.go                         |  207 ----
 docs/01-overview.md                         |  170 +++-
 docs/docgen.config.yml                      |   35 +-
 docs/prompts/01-overview.md                 |   30 -
 fs/utils.go                                 |   63 +-
 git/extended_status.go                      |   91 ++
 git/status.go                               |   54 +-
 git/utils.go                                |   24 +-
 git/worktree.go                             |   27 +-
 go.mod                                      |   25 +-
 go.sum                                      |   53 +-
 grove.yml                                   |   12 +-
 logging.schema.json                         |  109 +++
 logging/config.go                           |   90 +-
 logging/context_writer.go                   |   24 +
 logging/formatter.go                        |    8 +-
 logging/global_writer.go                    |   43 +
 logging/logger.go                           |  268 +++++-
 logging/logger_test.go                      |  635 ++++++++++++-
 logging/pretty.go                           |  188 ++--
 logging/unified.go                          |  318 +++++++
 logging/unified_test.go                     |  414 +++++++++
 logging/zombie_writer.go                    |  110 +++
 notebook.schema.json                        |   71 ++
 pkg/alias/notebook.go                       |   17 +
 pkg/alias/resolver.go                       |  425 +++++++++
 pkg/alias/resolver_test.go                  |  143 +++
 pkg/docs/docs.json                          |   14 +-
 pkg/logging/logutil/finder.go               |   87 ++
 pkg/models/session.go                       |    5 +
 pkg/process/process.go                      |   31 +
 pkg/profiling/cobra.go                      |   79 ++
 pkg/profiling/timer.go                      |  142 +++
 pkg/repo/manager.go                         |  440 +++++++--
 pkg/sessions/metadata.go                    |   22 +
 pkg/sessions/registry.go                    |  132 +++
 pkg/tmux/client.go                          |   47 +
 pkg/tmux/launch.go                          |   42 +
 pkg/tmux/session.go                         |  284 +++++-
 pkg/tmux/tui.go                             |  258 ++++++
 pkg/tmux/types.go                           |   21 +
 pkg/workspace/discover.go                   |  582 ++++++++----
 pkg/workspace/discover_test.go              |    8 +-
 pkg/workspace/enrich.go                     |  222 -----
 pkg/workspace/filter/filter.go              |  115 ++-
 pkg/workspace/history.go                    |  127 +++
 pkg/workspace/identifier.go                 |   73 +-
 pkg/workspace/identifier_test.go            |   72 +-
 pkg/workspace/lookup.go                     |  448 +++++++--
 pkg/workspace/lookup_test.go                |  323 ++++++-
 pkg/workspace/notebook_locator.go           |  777 ++++++++++++++++
 pkg/workspace/notebook_locator_test.go      |  116 +++
 pkg/workspace/notebook_resolver.go          |  316 +++++++
 pkg/workspace/prepare.go                    |   83 +-
 pkg/workspace/prepare_test.go               |   51 +-
 pkg/workspace/provider.go                   |  245 +++++
 pkg/workspace/provider_test.go              |  183 ++++
 pkg/workspace/submodules.go                 |  142 +--
 pkg/workspace/submodules_test.go            |   95 +-
 pkg/workspace/transform.go                  |  484 ++++++++--
 pkg/workspace/transform_test.go             |  291 ++++++
 pkg/workspace/types.go                      |  322 ++++++-
 pkg/workspace/utils.go                      |   19 +
 pkg/workspace/zombie.go                     |   68 ++
 schema/definitions/base.schema.json         |  113 +++
 schema/grove.embedded.schema.json           |  113 +++
 schema/manifest.go                          |   18 +
 schema/validator.go                         |   71 ++
 starship/command.go                         |  158 ++++
 starship/provider.go                        |   30 +
 state/state.go                              |  158 ++++
 state/state_test.go                         |  167 ++++
 tests/e2e/main.go                           |   82 ++
 tests/e2e/scenarios_basic.go                |   42 +
 tests/e2e/scenarios_config.go               |  503 ++++++++++
 tests/e2e/scenarios_log_creation.go         |  526 +++++++++++
 tests/e2e/scenarios_logging.go              | 1031 +++++++++++++++++++++
 tests/e2e/scenarios_logs_filtering.go       |  240 +++++
 tests/e2e/scenarios_logs_tui.go             | 1000 ++++++++++++++++++++
 tests/e2e/scenarios_logs_tui_filtering.go   |  186 ++++
 tests/e2e/scenarios_logs_tui_sorting.go     |  508 ++++++++++
 tests/e2e/scenarios_notebooks_debug.go      |  272 ++++++
 tests/e2e/scenarios_workspace.go            |  226 +++++
 tests/e2e/scenarios_workspace_advanced.go   |  481 ++++++++++
 tests/e2e/scenarios_workspace_edge_cases.go |  513 ++++++++++
 tests/e2e/scenarios_zombie_worktrees.go     |  202 ++++
 tests/e2e/test_utils.go                     |   25 +
 tools/logging-schema-generator/main.go      |   37 +
 tools/notebook-schema-generator/main.go     |   37 +
 tools/schema-composer/main.go               |  163 ++++
 tools/schema-generator/main.go              |   30 +
 tui/components/components.go                |    2 +-
 tui/components/help/help.go                 |  264 ++++--
 tui/components/jsontree/keymap.go           |  113 +++
 tui/components/jsontree/model.go            |  969 +++++++++++++++++++
 tui/components/logviewer/logviewer.go       |  311 +++++++
 tui/components/logviewer/writer.go          |   62 ++
 tui/components/navigator/io.go              |    9 +-
 tui/components/navigator/keymap.go          |    6 +-
 tui/components/navigator/model.go           |  105 ++-
 tui/components/nvim/model.go                |  303 ++++++
 tui/components/nvim/nvim.go                 |  538 +++++++++++
 tui/components/table/table.go               |   87 +-
 tui/keymap/keymap.go                        |    1 -
 tui/theme/icons.go                          |  777 ++++++++++++++++
 tui/theme/theme.go                          |  553 ++++++++---
 tui/utils/scrollbar/scrollbar.go            |  100 ++
 tui/wsnav/io.go                             |   14 +
 tui/wsnav/model.go                          |   68 ++
 tui/wsnav/update.go                         |   42 +
 tui/wsnav/util.go                           |   39 +
 tui/wsnav/view.go                           |  241 +++++
 tui/wsnav/wsnav.go                          |   60 ++
 util/pathutil/normalize.go                  |  119 +++
 145 files changed, 24669 insertions(+), 1884 deletions(-)
```

## v0.4.2 (2026-01-14)

This release introduces a complete overhaul of the logging system, a new interactive logs TUI, a reusable embedded Neovim component, and major improvements to the workspace discovery and path resolution systems. The new unified logging API provides a chainable builder pattern for creating both structured and pretty-printed log output from a single call, with extensive configuration options for component-based filtering (dbc1670, c455796). The new `core logs` command leverages this system to provide a powerful tool for viewing logs across an entire ecosystem, complete with a full-featured TUI for interactive browsing, searching, and filtering (ce02c65, 769ec18).

Workspace management has been significantly enhanced with the introduction of a centralized `NotebookLocator` for consistent, configuration-driven path resolution for notes, plans, and chats across both local and centralized notebook structures (f6ded98, d9a4582). Path handling is now more robust with new utilities that correctly normalize for case-sensitivity on different operating systems (6ea4031, dfed5b8), and a new a new utility prevents bugs related to deleted worktrees (210604e, ed647c8). The alias resolution system has also been centralized into `grove-core` and expanded to natively support notebook resources (9c81658, 8574644).

For developers, this release adds a `build_after` field to `grove.yml` for defining build dependencies (b77e7bb), a reusable/experimental embedded Neovim TUI component (3a07cca), and a comprehensive set of over 100 theme icons with Nerd Font and ASCII fallbacks to create richer and more informative TUIs across the ecosystem (bccd627, 081b068, 667bf5a).

### Features

- Add unified logging API with a chainable builder pattern for structured and pretty output (dbc1670)
- Implement `core logs` command with text, JSON, and interactive TUI modes (ce02c65)
- Add extensive component-based log filtering with CLI overrides and filter statistics (c455796)
- Add reusable embedded Neovim TUI component with a demo command (3a07cca)
- Add centralized `NotebookLocator` for consistent notebook, plan, and chat path resolution (f6ded98)
- Add `IsZombieWorktree` utility to prevent bugs from deleted worktrees (210604e)
- Add `CanonicalPath` function for case-correct path normalization on macOS (6ea4031)
- Move and enhance alias resolver from `grove-context` to `grove-core` for shared use (9c81658)
- Add native support for notebook resource aliases (e.g., `@a:nb:notebook:path`) (8574644)
- Add `build_after` field to `grove.yml` for defining build dependency order (b77e7bb)
- Add comprehensive set of over 100 theme icons with Nerd Font and ASCII fallbacks (bccd627)
- Add support for user-configurable note types with TUI metadata (d9a4582)
- Add global writer for dynamic TUI output redirection, fixing output mangling (0dce554)
- Add workspace-aware path resolution for log files (63767f9)
- Add reusable scrollbar utility package for TUI components (d506f4a)
- Add `StreamWriter` utility for streaming command output to TUIs without splitting lines (abeacbe)
- Add support for global override configuration file (`~/.config/grove/grove.override.yml`) (8b2bb9d)
- Add `core config-layers` command to diagnose configuration merging (0c3e40f)
- Add session registry for tracking live agent sessions from `grove-hooks` (13d2214)

### Bug Fixes

- Fix path normalization to correctly handle filesystem case on macOS (dfed5b8)
- Prevent recreation of "zombie" worktree directories by long-running loggers (ed647c8, de8429f)
- Fix `FindEcosystemConfig` to correctly recognize ecosystem worktrees (7489d3d)
- Prevent logviewer deadlock when using `StreamWriter` for TUI output (b89a451)
- Require `nb:` prefix for notebook aliases to resolve ambiguity with project paths (bf00107)
- Fix TUI log filtering to respect CLI override flags (3cf7532)
- Fix logviewer scrollbar calculation to account for text wrapping (e52a77b)
- Make default log filtering show all logs unless explicitly configured (466153c)
- Fix `tmux` commands to use exact session name matching to prevent ambiguity (8651e9b)
- Fix `core ws list` to correctly handle ecosystem worktree contents in focus mode (4b7b0db)

### Miscellaneous

- Standardize CI release workflows and disable CI runs on branches to reduce cost (bba4be4)
- Migrate `docgen` prompts to the notebook workspace (a61df2d)

### File Changes

```
 .cx/core.rules                              |    3 +
 .cx/dev-no-tests.rules                      |   19 +
 .cx/dev-with-tests.rules                    |   16 +
 .cx/docs-only.rules                         |    8 +
 .cx/pkg-only.rules                          |    1 +
 .cx/workspace-only.rules                    |    1 +
 .github/workflows/ci.yml                    |   52 ++
 .github/workflows/release.yml               |   38 +-
 .gitignore                                  |   56 +-
 CLAUDE.md                                   |   30 +
 Makefile                                    |  102 +-
 README.md                                   |    4 +-
 cmd/config.go                               |   60 ++
 cmd/core/main.go                            |   29 +
 cmd/editor.go                               |   78 ++
 cmd/logs.go                                 |  732 +++++++++++++++
 cmd/logs_tui.go                             | 1334 +++++++++++++++++++++++++++
 cmd/nvim_demo.go                            |  342 +++++++
 cmd/open_in_window.go                       |   51 +
 cmd/tmux.go                                 |   18 +
 cmd/tmux_editor.go                          |  147 +++
 cmd/version.go                              |   36 +
 cmd/ws.go                                   |  122 +++
 command/builder.go                          |   29 +-
 command/executor.go                         |   31 +
 config/config.go                            |  340 ++++---
 config/config_test.go                       |   77 ++
 config/merge.go                             |  108 ++-
 config/schema.go                            |   72 +-
 config/schema/definitions/base.schema.json  |  211 +++++
 config/types.go                             |  271 +++++-
 config/validator.go                         |  207 ----
 docs/01-overview.md                         |  170 +++-
 docs/docgen.config.yml                      |   35 +-
 docs/prompts/01-overview.md                 |   30 -
 fs/utils.go                                 |   63 +-
 git/extended_status.go                      |   91 ++
 git/status.go                               |   54 +-
 git/utils.go                                |   24 +-
 git/worktree.go                             |   27 +-
 go.mod                                      |   25 +-
 go.sum                                      |   53 +-
 grove.yml                                   |   12 +-
 logging.schema.json                         |  109 +++
 logging/config.go                           |   90 +-
 logging/context_writer.go                   |   24 +
 logging/formatter.go                        |    8 +-
 logging/global_writer.go                    |   43 +
 logging/logger.go                           |  268 +++++-
 logging/logger_test.go                      |  635 ++++++++++++-
 logging/pretty.go                           |  188 ++--
 logging/unified.go                          |  318 +++++++
 logging/unified_test.go                     |  414 +++++++++
 logging/zombie_writer.go                    |  110 +++
 notebook.schema.json                        |   71 ++
 pkg/alias/notebook.go                       |   17 +
 pkg/alias/resolver.go                       |  425 +++++++++
 pkg/alias/resolver_test.go                  |  143 +++
 pkg/docs/docs.json                          |   14 +-
 pkg/logging/logutil/finder.go               |   87 ++
 pkg/models/session.go                       |    5 +
 pkg/process/process.go                      |   31 +
 pkg/profiling/cobra.go                      |   79 ++
 pkg/profiling/timer.go                      |  142 +++
 pkg/repo/manager.go                         |  440 +++++++--
 pkg/sessions/metadata.go                    |   22 +
 pkg/sessions/registry.go                    |  132 +++
 pkg/tmux/client.go                          |   47 +
 pkg/tmux/launch.go                          |   42 +
 pkg/tmux/session.go                         |  284 +++++-
 pkg/tmux/tui.go                             |  258 ++++++
 pkg/tmux/types.go                           |   21 +
 pkg/workspace/discover.go                   |  582 ++++++++----
 pkg/workspace/discover_test.go              |    8 +-
 pkg/workspace/enrich.go                     |  222 -----
 pkg/workspace/filter/filter.go              |  115 ++-
 pkg/workspace/history.go                    |  127 +++
 pkg/workspace/identifier.go                 |   73 +-
 pkg/workspace/identifier_test.go            |   72 +-
 pkg/workspace/lookup.go                     |  448 +++++++--
 pkg/workspace/lookup_test.go                |  323 ++++++-
 pkg/workspace/notebook_locator.go           |  777 ++++++++++++++++
 pkg/workspace/notebook_locator_test.go      |  116 +++
 pkg/workspace/notebook_resolver.go          |  316 +++++++
 pkg/workspace/prepare.go                    |   83 +-
 pkg/workspace/prepare_test.go               |   51 +-
 pkg/workspace/provider.go                   |  245 +++++
 pkg/workspace/provider_test.go              |  183 ++++
 pkg/workspace/submodules.go                 |  142 +--
 pkg/workspace/submodules_test.go            |   95 +-
 pkg/workspace/transform.go                  |  484 ++++++++--
 pkg/workspace/transform_test.go             |  291 ++++++
 pkg/workspace/types.go                      |  322 ++++++-
 pkg/workspace/utils.go                      |   19 +
 pkg/workspace/zombie.go                     |   68 ++
 schema/definitions/base.schema.json         |  113 +++
 schema/grove.embedded.schema.json           |  113 +++
 schema/manifest.go                          |   18 +
 schema/validator.go                         |   71 ++
 starship/command.go                         |  158 ++++
 starship/provider.go                        |   30 +
 state/state.go                              |  158 ++++
 state/state_test.go                         |  167 ++++
 tests/e2e/main.go                           |   82 ++
 tests/e2e/scenarios_basic.go                |   42 +
 tests/e2e/scenarios_config.go               |  503 ++++++++++
 tests/e2e/scenarios_log_creation.go         |  526 +++++++++++
 tests/e2e/scenarios_logging.go              | 1031 +++++++++++++++++++++
 tests/e2e/scenarios_logs_filtering.go       |  240 +++++
 tests/e2e/scenarios_logs_tui.go             | 1000 ++++++++++++++++++++
 tests/e2e/scenarios_logs_tui_filtering.go   |  186 ++++
 tests/e2e/scenarios_logs_tui_sorting.go     |  508 ++++++++++
 tests/e2e/scenarios_notebooks_debug.go      |  272 ++++++
 tests/e2e/scenarios_workspace.go            |  226 +++++
 tests/e2e/scenarios_workspace_advanced.go   |  481 ++++++++++
 tests/e2e/scenarios_workspace_edge_cases.go |  513 ++++++++++
 tests/e2e/scenarios_zombie_worktrees.go     |  202 ++++
 tests/e2e/test_utils.go                     |   25 +
 tools/logging-schema-generator/main.go      |   37 +
 tools/notebook-schema-generator/main.go     |   37 +
 tools/schema-composer/main.go               |  163 ++++
 tools/schema-generator/main.go              |   30 +
 tui/components/components.go                |    2 +-
 tui/components/help/help.go                 |  264 ++++--
 tui/components/jsontree/keymap.go           |  113 +++
 tui/components/jsontree/model.go            |  969 +++++++++++++++++++
 tui/components/logviewer/logviewer.go       |  311 +++++++
 tui/components/logviewer/writer.go          |   62 ++
 tui/components/navigator/io.go              |    9 +-
 tui/components/navigator/keymap.go          |    6 +-
 tui/components/navigator/model.go           |  105 ++-
 tui/components/nvim/model.go                |  303 ++++++
 tui/components/nvim/nvim.go                 |  538 +++++++++++
 tui/components/table/table.go               |   87 +-
 tui/keymap/keymap.go                        |    1 -
 tui/theme/icons.go                          |  777 ++++++++++++++++
 tui/theme/theme.go                          |  553 ++++++++---
 tui/utils/scrollbar/scrollbar.go            |  100 ++
 tui/wsnav/io.go                             |   14 +
 tui/wsnav/model.go                          |   68 ++
 tui/wsnav/update.go                         |   42 +
 tui/wsnav/util.go                           |   39 +
 tui/wsnav/view.go                           |  241 +++++
 tui/wsnav/wsnav.go                          |   60 ++
 util/pathutil/normalize.go                  |  119 +++
 145 files changed, 24669 insertions(+), 1884 deletions(-)
```

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

