# fsevents Stub

This directory contains a stub implementation of the `github.com/fsnotify/fsevents` package.

## Why is this needed?

The `fsevents` package is a macOS-specific library that provides native file system event monitoring using Apple's FSEvents API. It requires CGO and links against macOS system frameworks (`CoreServices`).

When building Grove, we encounter a dependency chain:
- Grove → Docker Compose v2 → fsevents

The problem is that `fsevents` v0.2.0 has build issues on modern macOS/Go versions, failing with undefined symbols when CGO is enabled.

## How does this stub work?

Instead of fixing the upstream `fsevents` package, we use Go's module replacement feature to substitute it with this minimal stub implementation that:

1. Provides all the types and functions that Docker Compose's file watcher expects
2. Returns safe no-op implementations for all operations
3. Allows the build to complete successfully

The stub is activated via the `replace` directive in go.mod:
```
replace github.com/fsnotify/fsevents => ./internal/fsevents-stub
```

## Impact

Since Grove uses Docker Compose primarily for container orchestration (not file watching), losing FSEvents functionality has no practical impact on Grove's operation. The file watching features in Docker Compose are used for development hot-reloading, which isn't part of Grove's use case.

## Alternative approaches considered

1. **Using CGO_ENABLED=0**: This doesn't work because fsevents has no pure Go fallback
2. **Downgrading Docker Compose**: Older versions have other compatibility issues
3. **Using different fsevents versions**: v0.1.1 has different API issues, v0.2.0 is the latest

This stub solution is the cleanest approach that maintains compatibility while avoiding complex upstream fixes.