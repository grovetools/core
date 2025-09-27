# Documentation Task: Getting Started Guide

## Task
Create a "Getting Started" guide that walks a developer through building a simple "hello world" CLI tool using `grove-core`. The guide must be a step-by-step tutorial.

## Content
1.  **Setup**: Briefly explain `go get github.com/mattsolo1/grove-core`.
2.  **Basic Command**: Show how to create a `main.go` file and use `cli.NewStandardCommand` to set up a root command.
3.  **Adding Logic**: Add a simple `Run` function that prints "Hello, World!".
4.  **Using the Logger**: Demonstrate how to get a logger with `logging.NewLogger` and use it to print an informational message. Explain that the logger is already configured to write to stderr.
5.  **Handling Flags**: Show how to access the standard `--verbose` flag and change the log level accordingly.
6.  **Using Configuration**: Create a minimal `grove.yml` with a custom setting. Show how to load it using `config.LoadDefault()` and access the value.

## Output Format
Use clear headings for each step and provide concise, runnable Go code snippets.