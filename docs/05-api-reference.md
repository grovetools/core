# API Reference

The definitive API reference for `grove-core` is the official Go package documentation, which is generated directly from the source code and its comments. This ensures that the documentation is always synchronized with the code.

## Official Go Package Documentation

The complete API reference is hosted on `pkg.go.dev`. This site provides detailed information on all public types, functions, and methods available in the library.

**[View the `grove-core` API Reference on pkg.go.dev](https://pkg.go.dev/github.com/mattsolo1/grove-core)**

### How to Navigate the Documentation

When you visit the link above, you will find:
-   **Package Overview**: A summary of the `grove-core` library.
-   **Index**: A complete, alphabetized list of all public symbols.
-   **Subpackages**: A "Directories" or "Packages" section in the navigation sidebar lists all the subpackages. You can click on any of these to view the documentation for that specific package.

Key packages you may want to explore include:
-   `cli`: For building command-line interfaces.
-   `config`: For loading and managing `grove.yml` configuration.
-   `logging`: For structured logging.
-   `errors`: For creating and handling structured errors.
-   `git`: For interacting with Git repositories.
-   `pkg/tmux`: A client for managing `tmux` sessions.

## IDE and Editor Support

Standard Go development environments, such as Visual Studio Code with the Go extension or JetBrains GoLand, integrate with the Go documentation system. This provides inline access to documentation, function signatures, and auto-completion directly within your editor, based on the same source code comments that generate the `pkg.go.dev` site.