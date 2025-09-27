# Documentation Task: Best Practices

## Task
Document recommended best practices for developing tools with the `grove-core` library.

## Topics to Cover
- **Logging**: Emphasize using structured fields (`WithField`, `WithFields`) over string formatting (`Infof`). Explain when to use `WithError`.
- **Configuration**: Advise on keeping `grove.yml` declarative. Explain the use of environment variable expansion (`${VAR}`) for secrets.
- **Error Handling**: Stress returning `GroveError` types so the `cli.ErrorHandler` can provide rich context.
- **Extensibility**: When creating a new tool, design a custom configuration extension for `grove.yml`.
- **CLI Design**: Use `cli.NewStandardCommand` to maintain consistency across all tools in the ecosystem.