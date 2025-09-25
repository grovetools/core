# Grove Logging Package

A centralized, structured logging package for the Grove ecosystem based on `logrus`.

## Features

- **Unified Configuration**: Configure logging once in `grove.yml` for all Grove tools
- **Structured Logging**: Use fields for better log analysis and filtering
- **Multiple Output Sinks**: Log to stderr and/or files
- **Component Tagging**: Each log entry is automatically tagged with its source component
- **Flexible Formatting**: Support for text, simple, and JSON output formats
- **Environment Variable Overrides**: Override configuration via environment variables
- **Version Information**: Automatically logs binary version info on first logger initialization
- **Enhanced Caller Info**: Includes file, line, and function name when enabled

## Usage

### Basic Usage

```go
import "github.com/mattsolo1/grove-core/logging"

// Create a logger for your component
log := logging.NewLogger("my-component")

// Log messages at different levels
log.Debug("Detailed debug information")
log.Info("Informational message")
log.Warn("Warning message")
log.Error("Error message")

// Add structured fields
log.WithFields(logrus.Fields{
    "user_id": 123,
    "action": "login",
}).Info("User logged in")
```

### Configuration via grove.yml

Add a `logging` section to your `grove.yml`:

```yaml
logging:
  level: info              # debug, info, warn, error
  report_caller: false     # Include file:line:function in logs
  file:
    enabled: true
    path: ~/.grove/logs/grove.log
  format:
    preset: default        # default, simple, json
    disable_timestamp: false
    disable_component: false
```

### Environment Variable Overrides

- `GROVE_LOG_LEVEL`: Set the minimum log level (debug, info, warn, error)
- `GROVE_LOG_CALLER`: Set to "true" to include file, line, and function information

### Version Information Logging

The first time any logger is created in an application, version information is automatically logged. This includes build version, commit hash, branch, build date, Go version, and platform details:

```
2024-03-21 15:04:05 [INFO] [grove-init] Grove logging initialized version=v1.2.3 commit=abc123 branch=main buildDate=2024-03-21T14:00:00Z goVersion=go1.21.0 platform=darwin/arm64
```

### Output Format Examples

**Default format:**
```
2024-03-21 15:04:05 [INFO] [grove-flow] Starting job execution job_id=123
```

**With caller information enabled:**
```
2024-03-21 15:04:05 [INFO] [grove-flow] [flow.go:42 flow.Execute] Starting job execution job_id=123
```

**Simple format:**
```
[INFO] Starting job execution job_id=123
```

**JSON format:**
```json
{"component":"grove-flow","job_id":123,"level":"info","msg":"Starting job execution","time":"2024-03-21T15:04:05Z"}
```

## Best Practices

1. **Component Naming**: Use consistent, descriptive component names (e.g., "grove-flow", "gemini-client")

2. **Structured Fields**: Prefer structured fields over string formatting:
   ```go
   // Good
   log.WithField("file", path).Info("Processing file")
   
   // Avoid
   log.Infof("Processing file %s", path)
   ```

3. **Error Handling**: Use `WithError` for consistent error logging:
   ```go
   if err := doSomething(); err != nil {
       log.WithError(err).Error("Failed to do something")
   }
   ```

4. **Log Levels**:
   - **Debug**: Detailed information for debugging
   - **Info**: General informational messages
   - **Warn**: Warning messages that don't prevent operation
   - **Error**: Error messages for failures

5. **Context Fields**: Add relevant context as fields:
   ```go
   log.WithFields(logrus.Fields{
       "job_id": job.ID,
       "plan": plan.Name,
       "model": model,
   }).Info("Starting LLM request")
   ```

## Integration with Existing Code

When migrating from `fmt.Printf` to structured logging:

```go
// Before
fmt.Printf("Warning: failed to load config: %v\n", err)

// After
log.WithError(err).Warn("Failed to load config")
```

```go
// Before
fmt.Printf("Processing %d files in %s\n", count, directory)

// After
log.WithFields(logrus.Fields{
    "count": count,
    "directory": directory,
}).Info("Processing files")
```

## Output Streams

Following Unix conventions:
- **stdout**: Reserved for program output (e.g., LLM responses, command results)
- **stderr**: All logs, status messages, and diagnostics go here
- **File sink**: Optional additional output for persistent logging

This ensures clean piping and output redirection in shell scripts.