# Documentation Task: Contribution Guide

## Task
Create a contribution guide for the `grove-core` project itself.

## Content
1.  **Code of Conduct**: Add a placeholder for a future Code of Conduct.
2.  **Getting Started**: How to clone the repo and install dependencies (`go mod tidy`).
3.  **Running Checks**: Explain the `Makefile` targets: `make fmt`, `make vet`, `make lint`, `make test`, and the all-in-one `make check`. Mention the `golangci-lint` requirement.
4.  **Commit Messages**: Explain that the project uses the Conventional Commits specification, referencing the `conventional/` package.
5.  **Pull Requests**: Outline the process for submitting a pull request.
6.  **Releases**: Describe the automated release process via the `.github/workflows/release.yml` GitHub Action.