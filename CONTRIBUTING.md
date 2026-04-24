# Contributing to grove-core

## First-time setup

After cloning or creating a new worktree:

```bash
make setup
```

This is idempotent — safe to re-run. It installs `gofumpt`, configures `git blame` to skip formatting commits, and installs the pre-commit hook.

## Formatting

This repo uses **gofumpt** (stricter superset of gofmt) with `extra-rules: true`.

```bash
make fmt        # format the tree in-place
make fmt-check  # verify formatting without modifying (used by CI / `make check`)
```

The pre-commit hook (installed by `make setup`) rejects `git commit` if any staged `.go` file isn't gofumpt-clean. To fix: `make fmt` and re-stage.

Agent sessions also trigger `gofumpt -w .` on stop via `grove.yml`, so uncommitted changes from an agent session arrive pre-formatted.

## Pre-push checklist

```bash
make check    # fmt-check + vet + lint + test
```

All four must pass before pushing.

## Blame history

Big formatting commits are listed in `.git-blame-ignore-revs` and skipped by `git blame` (and GitHub's blame UI). Local setup for this is done automatically by `make setup`.

