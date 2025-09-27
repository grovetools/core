# Contributing to Grove Core

Contributions are welcome and appreciated. This guide outlines the standards and procedures for contributing to the `grove-core` project.

## Code of Conduct

This project will adopt a Code of Conduct in the future. In the meantime, all contributors are expected to interact respectfully and professionally.

## Getting Started

To begin contributing, set up the project locally:

1.  **Clone the repository:**
    ```sh
    git clone https://github.com/mattsolo1/grove-core.git
    cd grove-core
    ```

2.  **Install dependencies:**
    The project uses Go modules to manage dependencies. They will be automatically downloaded when you build or test the code. You can also install them manually:
    ```sh
    go mod tidy
    ```

## Running Checks and Tests

The project includes a `Makefile` with targets for running common development tasks, including formatting, linting, and testing. Before submitting any changes, run the full suite of checks to ensure code quality and consistency.

*   `make fmt`: Formats the Go source code using `go fmt`.
*   `make vet`: Runs `go vet` to check for suspicious constructs.
*   `make test`: Runs the test suite using `go test`.
*   `make lint`: Runs `golangci-lint` to check for style and common errors. You must have `golangci-lint` installed:
    ```sh
    go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
    ```
*   `make check`: A convenience target that runs all of the above checks (`fmt`, `vet`, `lint`, and `test`).

To run all checks before committing, execute:
```sh
make check
```

## Commit Messages

This project follows the **Conventional Commits** specification. This convention provides a clear and structured commit history, which is used to automate changelog generation. The `conventional/` package in this repository contains the logic for parsing these commit messages.

Each commit message should be structured as follows:

```
<type>[optional scope]: <description>

[optional body]

[optional footer]
```

**Common types:**
*   `feat`: A new feature.
*   `fix`: A bug fix.
*   `docs`: Documentation-only changes.
*   `style`: Code style changes (formatting, etc.).
*   `refactor`: A code change that neither fixes a bug nor adds a feature.
*   `perf`: A code change that improves performance.
*   `test`: Adding missing tests or correcting existing tests.
*   `chore`: Changes to the build process or auxiliary tools.

**Example:**
```
feat(config): add support for hierarchical overrides

Implement a three-tiered loading system for grove.yml that merges global,
project, and local override configurations. This provides greater
flexibility for developers to manage settings.
```

## Pull Requests

1.  **Fork the repository** on GitHub.
2.  **Create a new branch** for your feature or bug fix: `git checkout -b my-new-feature`.
3.  **Make your changes** and commit them using the Conventional Commits format.
4.  **Push your branch** to your fork: `git push origin my-new-feature`.
5.  **Open a pull request** from your fork to the `main` branch of the original repository.
6.  Provide a clear title and description for your pull request, explaining the purpose and scope of your changes.

## Release Process

Releases are automated using a GitHub Actions workflow defined in `.github/workflows/release.yml`. The process is triggered when a new tag matching the `v*` pattern (e.g., `v0.4.0`) is pushed to the repository.

The workflow performs the following steps:
1.  Checks out the code at the specified tag.
2.  Sets up the Go environment and installs dependencies.
3.  Automatically extracts the release notes for the new version from `CHANGELOG.md`. It finds the section corresponding to the tag and copies its content.
4.  Creates a new GitHub Release using the tag name and the extracted release notes.

Because this process is automated, it is important that `CHANGELOG.md` is kept up-to-date and that commit messages are properly formatted to support changelog generation.