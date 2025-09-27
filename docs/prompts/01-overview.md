# Documentation Task: Overview

You are an expert technical writer documenting `grove-core`, a shared Go library for the Grove ecosystem.

## Task
Based on your analysis of the codebase, write a clear, engaging overview that:
- Explains that `grove-core` is the foundational library for building standardized, robust CLI tools.
- Highlights its key features: A CLI framework (`cli`), hierarchical configuration management via `grove.yml` (`config`), structured logging (`logging`), custom error handling (`errors`), and utilities for Git (`git`) and tmux (`pkg/tmux`).
- Describes the problem it solves: enforcing consistency, reducing boilerplate, and providing shared functionality across a suite of developer tools.
- Identifies the target audience: Developers building tools within the Grove ecosystem.

## Context
The main `README.md` provides a good starting point. The library's purpose is to be imported by other Go projects to build command-line applications.